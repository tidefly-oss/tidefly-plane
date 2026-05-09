package agent

import (
	"fmt"
	"sync"
	"time"

	agentpb "github.com/tidefly-oss/tidefly-plane/internal/api/v1/proto/agent"
	"google.golang.org/grpc/keepalive"
)

const commandTimeout = 30 * time.Second

// WorkerConn represents an active worker connection.
// It holds the stream and pending command futures.
type WorkerConn struct {
	workerID string
	stream   agentpb.AgentService_ConnectServer
	done     chan struct{}

	mu      sync.Mutex
	pending map[string]*pendingCmd                       // commandID → pending
	logs    map[string]chan *agentpb.ContainerLogsResult // commandID → log channel
}

type pendingCmd struct {
	result chan any
	err    chan error
}

func newWorkerConn(workerID string, stream agentpb.AgentService_ConnectServer) *WorkerConn {
	return &WorkerConn{
		workerID: workerID,
		stream:   stream,
		done:     make(chan struct{}),
		pending:  make(map[string]*pendingCmd),
		logs:     make(map[string]chan *agentpb.ContainerLogsResult),
	}
}

// Send sends a command to the worker and waits for a result.
// Returns the result or an error if the worker doesn't respond within timeout.
func (c *WorkerConn) Send(msg *agentpb.PlaneMessage) (any, error) {
	p := &pendingCmd{
		result: make(chan any, 1),
		err:    make(chan error, 1),
	}

	c.mu.Lock()
	c.pending[msg.CommandId] = p
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, msg.CommandId)
		c.mu.Unlock()
	}()

	if err := c.stream.Send(msg); err != nil {
		return nil, fmt.Errorf("send command: %w", err)
	}

	select {
	case result := <-p.result:
		return result, nil
	case err := <-p.err:
		return nil, err
	case <-time.After(commandTimeout):
		return nil, fmt.Errorf("command %s timed out after %s", msg.CommandId, commandTimeout)
	case <-c.done:
		return nil, fmt.Errorf("worker disconnected")
	}
}

// SendNoWait sends a command and returns immediately (fire and forget).
func (c *WorkerConn) SendNoWait(msg *agentpb.PlaneMessage) error {
	return c.stream.Send(msg)
}

// StreamLogs opens a log channel for a command and returns it.
// Caller must close the returned channel when done.
func (c *WorkerConn) StreamLogs(commandID string) chan *agentpb.ContainerLogsResult {
	ch := make(chan *agentpb.ContainerLogsResult, 100)
	c.mu.Lock()
	c.logs[commandID] = ch
	c.mu.Unlock()
	return ch
}

// CloseLogStream removes and closes a log channel.
func (c *WorkerConn) CloseLogStream(commandID string) {
	c.mu.Lock()
	if ch, ok := c.logs[commandID]; ok {
		close(ch)
		delete(c.logs, commandID)
	}
	c.mu.Unlock()
}

// resolveAck resolves a pending command with an ack (no result payload).
func (c *WorkerConn) resolveAck(commandID string, accepted bool, reason string) {
	c.mu.Lock()
	p, ok := c.pending[commandID]
	c.mu.Unlock()
	if !ok {
		return
	}
	if accepted {
		p.result <- struct{}{}
	} else {
		p.err <- fmt.Errorf("worker rejected command: %s", reason)
	}
}

// resolveResult resolves a pending command with a typed result.
func (c *WorkerConn) resolveResult(commandID string, result any) {
	c.mu.Lock()
	p, ok := c.pending[commandID]
	c.mu.Unlock()
	if !ok {
		return
	}
	p.result <- result
}

// pushLog forwards a log line to the appropriate log channel.
func (c *WorkerConn) pushLog(log *agentpb.ContainerLogsResult) {
	c.mu.Lock()
	ch, ok := c.logs[log.CommandId]
	c.mu.Unlock()
	if !ok {
		return
	}
	select {
	case ch <- log:
	default:
		// channel full — drop line (backpressure)
	}
}

// ── Registry ──────────────────────────────────────────────────────────────────

// Registry holds all active WorkerConn instances.
// Thread-safe — used by other services to send commands to workers.
type Registry struct {
	mu      sync.RWMutex
	workers map[string]*WorkerConn
}

func NewRegistry() *Registry {
	return &Registry{
		workers: make(map[string]*WorkerConn),
	}
}

// Register adds a worker connection to the registry.
func (r *Registry) Register(workerID string, stream agentpb.AgentService_ConnectServer) *WorkerConn {
	conn := newWorkerConn(workerID, stream)
	r.mu.Lock()
	r.workers[workerID] = conn
	r.mu.Unlock()
	return conn
}

// Unregister removes a worker and closes its done channel.
func (r *Registry) Unregister(workerID string) {
	r.mu.Lock()
	if conn, ok := r.workers[workerID]; ok {
		close(conn.done)
		delete(r.workers, workerID)
	}
	r.mu.Unlock()
}

// Get returns the WorkerConn for a worker, or nil if not connected.
func (r *Registry) Get(workerID string) (*WorkerConn, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	conn, ok := r.workers[workerID]
	return conn, ok
}

// IsConnected returns true if the worker has an active stream.
func (r *Registry) IsConnected(workerID string) bool {
	_, ok := r.Get(workerID)
	return ok
}

// ConnectedWorkers returns all currently connected worker IDs.
func (r *Registry) ConnectedWorkers() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.workers))
	for id := range r.workers {
		ids = append(ids, id)
	}
	return ids
}

// ── keepalive config ──────────────────────────────────────────────────────────

func keepaliveParams() keepalive.ServerParameters {
	return keepalive.ServerParameters{
		MaxConnectionIdle:     5 * time.Minute,
		MaxConnectionAge:      30 * time.Minute,
		MaxConnectionAgeGrace: 10 * time.Second,
		Time:                  30 * time.Second,
		Timeout:               10 * time.Second,
	}
}

func keepalivePolicy() keepalive.EnforcementPolicy {
	return keepalive.EnforcementPolicy{
		MinTime:             10 * time.Second,
		PermitWithoutStream: true,
	}
}
