package agent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log/slog"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"

	agentpb "github.com/tidefly-oss/tidefly-plane/internal/api/v1/proto/agent"
	"github.com/tidefly-oss/tidefly-plane/internal/ca"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

const (
	defaultGRPCPort = ":7443"
)

// Server is the gRPC server running on the Plane.
// Workers connect to it via mTLS and establish a bidirectional stream.
type Server struct {
	agentpb.UnimplementedAgentServiceServer

	db        *gorm.DB
	caService *ca.Service
	registry  *Registry
	grpcSrv   *grpc.Server
	port      string
}

func NewServer(db *gorm.DB, caService *ca.Service, port string) *Server {
	if port == "" {
		port = defaultGRPCPort
	}
	return &Server{
		db:        db,
		caService: caService,
		registry:  NewRegistry(),
		port:      port,
	}
}

// Start builds the mTLS gRPC server and starts listening.
// Blocks until ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	tlsCfg, err := s.buildTLSConfig()
	if err != nil {
		return fmt.Errorf("agent server: build TLS config: %w", err)
	}

	creds := credentials.NewTLS(tlsCfg)
	s.grpcSrv = grpc.NewServer(
		grpc.Creds(creds),
		grpc.KeepaliveParams(keepaliveParams()),
		grpc.KeepaliveEnforcementPolicy(keepalivePolicy()),
	)

	agentpb.RegisterAgentServiceServer(s.grpcSrv, s)

	lis, err := net.Listen("tcp", s.port)
	if err != nil {
		return fmt.Errorf("agent server: listen %s: %w", s.port, err)
	}

	slog.Info("agent server: listening", "port", s.port)

	errCh := make(chan error, 1)
	go func() { errCh <- s.grpcSrv.Serve(lis) }()

	select {
	case <-ctx.Done():
		slog.Info("agent server: shutting down")
		s.grpcSrv.GracefulStop()
		return nil
	case err := <-errCh:
		return fmt.Errorf("agent server: %w", err)
	}
}

// Registry returns the worker registry (used by other services to send commands).
func (s *Server) Registry() *Registry {
	return s.registry
}

// ── AgentService implementation ───────────────────────────────────────────────

func (s *Server) Ping(_ context.Context, req *agentpb.PingRequest) (*agentpb.PingResponse, error) {
	// Verify worker exists and is not revoked
	var worker models.WorkerNode
	if err := s.db.Where("id = ? AND status != ?", req.WorkerId, models.WorkerStatusRevoked).
		First(&worker).Error; err != nil {
		return nil, status.Error(codes.Unauthenticated, "worker not found or revoked")
	}

	return &agentpb.PingResponse{
		PlaneVersion: "0.1.0",
		Timestamp:    time.Now().UnixMilli(),
	}, nil
}

func (s *Server) Connect(stream agentpb.AgentService_ConnectServer) error {
	// Extract worker ID from mTLS cert CommonName
	workerID, err := workerIDFromContext(stream.Context())
	if err != nil {
		return status.Error(codes.Unauthenticated, "could not identify worker from certificate")
	}

	// Load worker from DB
	var worker models.WorkerNode
	if err := s.db.Where("id = ? AND status != ?", workerID, models.WorkerStatusRevoked).
		First(&worker).Error; err != nil {
		return status.Error(codes.Unauthenticated, "worker not found or revoked")
	}

	// Extract peer IP
	peerIP := ""
	if p, ok := peer.FromContext(stream.Context()); ok {
		if addr, ok := p.Addr.(*net.TCPAddr); ok {
			peerIP = addr.IP.String()
		}
	}

	// Register in memory registry
	conn := s.registry.Register(workerID, stream)
	defer s.registry.Unregister(workerID)

	slog.Info("agent server: worker connected", "worker_id", workerID, "ip", peerIP)

	// Wait for AgentHello as first message
	if err := s.handleHello(stream, &worker, peerIP); err != nil {
		return err
	}

	// Main receive loop
	for {
		select {
		case <-stream.Context().Done():
			slog.Info("agent server: worker disconnected", "worker_id", workerID)
			s.markDisconnected(&worker)
			return nil
		case <-conn.done:
			return nil
		default:
			msg, err := stream.Recv()
			if err == io.EOF {
				slog.Info("agent server: worker closed stream", "worker_id", workerID)
				s.markDisconnected(&worker)
				return nil
			}
			if err != nil {
				slog.Error("agent server: recv error", "worker_id", workerID, "error", err)
				s.markDisconnected(&worker)
				return status.Error(codes.Internal, "recv error")
			}

			s.handleMessage(stream.Context(), &worker, msg, conn)
		}
	}
}

// ── message handling ──────────────────────────────────────────────────────────

func (s *Server) handleHello(stream agentpb.AgentService_ConnectServer, worker *models.WorkerNode, ip string) error {
	msg, err := stream.Recv()
	if err != nil {
		return status.Error(codes.Internal, "failed to receive hello")
	}

	hello, ok := msg.Payload.(*agentpb.AgentMessage_Hello)
	if !ok {
		return status.Error(codes.InvalidArgument, "expected AgentHello as first message")
	}

	// Update worker record
	worker.MarkConnected(ip, hello.Hello.AgentVersion)
	worker.OS = hello.Hello.Os
	worker.Arch = hello.Hello.Arch
	worker.RuntimeType = hello.Hello.RuntimeType
	s.db.Save(worker)

	// Send PlaneAck
	return stream.Send(&agentpb.PlaneMessage{
		CommandId: "hello-ack",
		WorkerId:  worker.ID,
		Payload: &agentpb.PlaneMessage_Ack{
			Ack: &agentpb.PlaneAck{Accepted: true},
		},
	})
}

func (s *Server) handleMessage(_ context.Context, worker *models.WorkerNode, msg *agentpb.AgentMessage, conn *WorkerConn) {
	switch p := msg.Payload.(type) {

	case *agentpb.AgentMessage_Heartbeat:
		now := time.Now()
		worker.LastSeenAt = &now
		s.db.Model(worker).Update("last_seen_at", now)
		slog.Debug("agent server: heartbeat",
			"worker_id", worker.ID,
			"containers", p.Heartbeat.ContainerCount,
			"cpu", p.Heartbeat.CpuPercent,
		)

	case *agentpb.AgentMessage_CommandAck:
		conn.resolveAck(p.CommandAck.CommandId, p.CommandAck.Accepted, p.CommandAck.Reason)

	case *agentpb.AgentMessage_ContainerList:
		conn.resolveResult(p.ContainerList.CommandId, p.ContainerList)

	case *agentpb.AgentMessage_ContainerLogs:
		conn.pushLog(p.ContainerLogs)

	case *agentpb.AgentMessage_Metrics:
		conn.resolveResult(p.Metrics.CommandId, p.Metrics)

	case *agentpb.AgentMessage_DeployResult:
		conn.resolveResult(p.DeployResult.CommandId, p.DeployResult)

	case *agentpb.AgentMessage_Error:
		slog.Error("agent server: worker error",
			"worker_id", worker.ID,
			"code", p.Error.Code,
			"message", p.Error.Message,
		)
		conn.resolveAck(p.Error.CommandId, false, p.Error.Message)
	}
}

func (s *Server) markDisconnected(worker *models.WorkerNode) {
	worker.MarkDisconnected()
	s.db.Model(worker).Update("status", models.WorkerStatusDisconnected)
}

// ── TLS ───────────────────────────────────────────────────────────────────────

func (s *Server) buildTLSConfig() (*tls.Config, error) {
	caCertPEM, err := s.caService.GetCACertPEM()
	if err != nil {
		return nil, fmt.Errorf("get CA cert: %w", err)
	}

	_, planeCertPEM, planeKeyPEM, err := s.caService.IssuePlaneCert(nil)
	if err != nil {
		return nil, fmt.Errorf("issue plane cert: %w", err)
	}

	cert, err := tls.X509KeyPair([]byte(planeCertPEM), []byte(planeKeyPEM))
	if err != nil {
		return nil, fmt.Errorf("load plane cert: %w", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM([]byte(caCertPEM)) {
		return nil, fmt.Errorf("parse CA cert")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    certPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func workerIDFromContext(ctx context.Context) (string, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return "", fmt.Errorf("no peer info")
	}
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return "", fmt.Errorf("no TLS info")
	}
	if len(tlsInfo.State.PeerCertificates) == 0 {
		return "", fmt.Errorf("no client certificate")
	}
	// Worker cert CN is "tidefly-worker-<uuid>"
	cn := tlsInfo.State.PeerCertificates[0].Subject.CommonName
	const prefix = "tidefly-worker-"
	if len(cn) <= len(prefix) {
		return "", fmt.Errorf("invalid cert CN: %s", cn)
	}
	return cn[len(prefix):], nil
}
