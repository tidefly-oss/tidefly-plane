package ingress

import "context"

// Route describes an HTTP(S) ingress route.
type Route struct {
	ServiceName string
	Domain      string
	Upstream    string // host:port
	TLS         bool
	WWW         bool
}

// TCPRoute describes a TCP/TLS L4 ingress route.
type TCPRoute struct {
	ServiceName string
	RouteID     string
	Upstream    string // container:port
	ListenPort  int    // port Caddy binds on the host
}

// Adapter abstracts the ingress controller (Caddy, K8s Ingress, etc.)
type Adapter interface {
	// AddRoute HTTP routes
	AddRoute(ctx context.Context, route Route) error
	RemoveRoute(ctx context.Context, serviceName string) error
	UpdateRoute(ctx context.Context, route Route) error

	// AddTCPRoute TCP/TLS L4 routes (for databases and other TCP services)
	AddTCPRoute(ctx context.Context, route TCPRoute) error
	RemoveTCPRoute(ctx context.Context, routeID string) error
}
