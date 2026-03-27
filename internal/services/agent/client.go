package agent

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	agentpb "github.com/tidefly-oss/tidefly-plane/internal/api/v1/proto/agent"
)

// Client wraps the Registry and provides a clean API for other services
// to send commands to workers without dealing with proto directly.
type Client struct {
	registry *Registry
}

func NewClient(registry *Registry) *Client {
	return &Client{registry: registry}
}

func (c *Client) conn(workerID string) (*WorkerConn, error) {
	conn, ok := c.registry.Get(workerID)
	if !ok {
		return nil, fmt.Errorf("worker %s is not connected", workerID)
	}
	return conn, nil
}

func cmdID() string { return uuid.New().String() }

// ── Container commands ────────────────────────────────────────────────────────

func (c *Client) ListContainers(ctx context.Context, workerID string) ([]*agentpb.Container, error) {
	conn, err := c.conn(workerID)
	if err != nil {
		return nil, err
	}

	id := cmdID()
	result, err := conn.Send(&agentpb.PlaneMessage{
		CommandId: id,
		WorkerId:  workerID,
		Payload:   &agentpb.PlaneMessage_ListContainers{ListContainers: &agentpb.CmdListContainers{}},
	})
	if err != nil {
		return nil, err
	}

	res, ok := result.(*agentpb.ContainerListResult)
	if !ok {
		return nil, fmt.Errorf("unexpected result type")
	}
	return res.Containers, nil
}

func (c *Client) StartContainer(ctx context.Context, workerID, containerID string) error {
	conn, err := c.conn(workerID)
	if err != nil {
		return err
	}
	_, err = conn.Send(&agentpb.PlaneMessage{
		CommandId: cmdID(),
		WorkerId:  workerID,
		Payload: &agentpb.PlaneMessage_StartContainer{
			StartContainer: &agentpb.CmdStartContainer{ContainerId: containerID},
		},
	})
	return err
}

func (c *Client) StopContainer(ctx context.Context, workerID, containerID string, timeoutSec int32) error {
	conn, err := c.conn(workerID)
	if err != nil {
		return err
	}
	_, err = conn.Send(&agentpb.PlaneMessage{
		CommandId: cmdID(),
		WorkerId:  workerID,
		Payload: &agentpb.PlaneMessage_StopContainer{
			StopContainer: &agentpb.CmdStopContainer{
				ContainerId: containerID,
				TimeoutSec:  timeoutSec,
			},
		},
	})
	return err
}

func (c *Client) RestartContainer(ctx context.Context, workerID, containerID string) error {
	conn, err := c.conn(workerID)
	if err != nil {
		return err
	}
	_, err = conn.Send(&agentpb.PlaneMessage{
		CommandId: cmdID(),
		WorkerId:  workerID,
		Payload: &agentpb.PlaneMessage_RestartContainer{
			RestartContainer: &agentpb.CmdRestartContainer{ContainerId: containerID},
		},
	})
	return err
}

func (c *Client) RemoveContainer(ctx context.Context, workerID, containerID string, force bool) error {
	conn, err := c.conn(workerID)
	if err != nil {
		return err
	}
	_, err = conn.Send(&agentpb.PlaneMessage{
		CommandId: cmdID(),
		WorkerId:  workerID,
		Payload: &agentpb.PlaneMessage_RemoveContainer{
			RemoveContainer: &agentpb.CmdRemoveContainer{
				ContainerId: containerID,
				Force:       force,
			},
		},
	})
	return err
}

// ── Deploy ────────────────────────────────────────────────────────────────────

type DeployRequest struct {
	ProjectID   string
	ServiceName string
	Image       string
	Env         []string
	Ports       []*agentpb.PortSpec
	Labels      map[string]string
	Limits      *agentpb.ResourceLimits
	Volumes     []*agentpb.VolumeMount
	Network     string
}

func (c *Client) Deploy(ctx context.Context, workerID string, req DeployRequest) (*agentpb.DeployResult, error) {
	conn, err := c.conn(workerID)
	if err != nil {
		return nil, err
	}

	id := cmdID()
	result, err := conn.Send(&agentpb.PlaneMessage{
		CommandId: id,
		WorkerId:  workerID,
		Payload: &agentpb.PlaneMessage_Deploy{
			Deploy: &agentpb.CmdDeploy{
				ProjectId:   req.ProjectID,
				ServiceName: req.ServiceName,
				Image:       req.Image,
				Env:         req.Env,
				Ports:       req.Ports,
				Labels:      req.Labels,
				Limits:      req.Limits,
				Volumes:     req.Volumes,
				Network:     req.Network,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	res, ok := result.(*agentpb.DeployResult)
	if !ok {
		return nil, fmt.Errorf("unexpected result type")
	}
	if !res.Success {
		return nil, fmt.Errorf("deploy failed: %s", res.Error)
	}
	return res, nil
}

// ── Metrics ───────────────────────────────────────────────────────────────────

func (c *Client) CollectMetrics(ctx context.Context, workerID string) (*agentpb.MetricsResult, error) {
	conn, err := c.conn(workerID)
	if err != nil {
		return nil, err
	}

	id := cmdID()
	result, err := conn.Send(&agentpb.PlaneMessage{
		CommandId: id,
		WorkerId:  workerID,
		Payload:   &agentpb.PlaneMessage_CollectMetrics{CollectMetrics: &agentpb.CmdCollectMetrics{}},
	})
	if err != nil {
		return nil, err
	}

	res, ok := result.(*agentpb.MetricsResult)
	if !ok {
		return nil, fmt.Errorf("unexpected result type")
	}
	return res, nil
}

// ── Logs ──────────────────────────────────────────────────────────────────────

func (c *Client) StreamLogs(ctx context.Context, workerID, containerID string, follow bool, tailLines int32) (<-chan *agentpb.ContainerLogsResult, error) {
	conn, err := c.conn(workerID)
	if err != nil {
		return nil, err
	}

	id := cmdID()
	ch := conn.StreamLogs(id)

	if err := conn.SendNoWait(&agentpb.PlaneMessage{
		CommandId: id,
		WorkerId:  workerID,
		Payload: &agentpb.PlaneMessage_StreamLogs{
			StreamLogs: &agentpb.CmdStreamLogs{
				ContainerId: containerID,
				Follow:      follow,
				TailLines:   tailLines,
			},
		},
	}); err != nil {
		conn.CloseLogStream(id)
		return nil, err
	}

	// Auto-close log stream when context is cancelled
	go func() {
		<-ctx.Done()
		conn.CloseLogStream(id)
	}()

	return ch, nil
}

// ── Routes ────────────────────────────────────────────────────────────────────

func (c *Client) RegisterRoute(ctx context.Context, workerID, upstream, domain string, tls bool) error {
	conn, err := c.conn(workerID)
	if err != nil {
		return err
	}
	_, err = conn.Send(&agentpb.PlaneMessage{
		CommandId: cmdID(),
		WorkerId:  workerID,
		Payload: &agentpb.PlaneMessage_RegisterRoute{
			RegisterRoute: &agentpb.CmdRegisterRoute{
				Upstream: upstream,
				Domain:   domain,
				Tls:      tls,
			},
		},
	})
	return err
}

func (c *Client) RemoveRoute(ctx context.Context, workerID, domain string) error {
	conn, err := c.conn(workerID)
	if err != nil {
		return err
	}
	_, err = conn.Send(&agentpb.PlaneMessage{
		CommandId: cmdID(),
		WorkerId:  workerID,
		Payload: &agentpb.PlaneMessage_RemoveRoute{
			RemoveRoute: &agentpb.CmdRemoveRoute{Domain: domain},
		},
	})
	return err
}

// ── Status ────────────────────────────────────────────────────────────────────

func (c *Client) IsConnected(workerID string) bool {
	return c.registry.IsConnected(workerID)
}

func (c *Client) ConnectedWorkers() []string {
	return c.registry.ConnectedWorkers()
}
