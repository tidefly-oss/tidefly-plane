package caddy

import (
	"context"
	"fmt"
	"net"

	caddysvc "github.com/tidefly-oss/tidefly-plane/internal/infrastructure/caddy"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/ingress"
)

type CaddyAdapter struct {
	caddy *caddysvc.Client
}

func New(caddy *caddysvc.Client) *CaddyAdapter {
	return &CaddyAdapter{caddy: caddy}
}

// ── HTTP Routes ───────────────────────────────────────────────────────────────

func (a *CaddyAdapter) AddRoute(ctx context.Context, route ingress.Route) error {
	routeID := caddysvc.RouteID(route.ServiceName)
	upstream := route.Upstream
	if upstream == "" {
		upstream = fmt.Sprintf("%s:%d", route.ServiceName, defaultPort(route))
	}
	return a.caddy.AddHTTPRoute(ctx, routeID, route.Domain, upstream)
}

func (a *CaddyAdapter) RemoveRoute(ctx context.Context, serviceName string) error {
	return a.caddy.RemoveRoute(ctx, caddysvc.RouteID(serviceName))
}

func (a *CaddyAdapter) UpdateRoute(ctx context.Context, route ingress.Route) error {
	return a.caddy.UpdateRoute(ctx, caddysvc.RouteID(route.ServiceName), route.Domain, route.Upstream)
}

// ── TCP/TLS Routes ────────────────────────────────────────────────────────────

func (a *CaddyAdapter) AddTCPRoute(ctx context.Context, route ingress.TCPRoute) error {
	port := route.ListenPort
	if port == 0 {
		var err error
		port, err = findFreePort(ctx, 15000, 19999)
		if err != nil {
			return fmt.Errorf("caddy adapter: find free port: %w", err)
		}
	}
	routeID := route.RouteID
	if routeID == "" {
		routeID = caddysvc.TCPRouteID(route.ServiceName)
	}
	return a.caddy.AddTCPRoute(ctx, routeID, route.Upstream, port)
}

func (a *CaddyAdapter) RemoveTCPRoute(ctx context.Context, routeID string) error {
	return a.caddy.RemoveTCPRoute(ctx, routeID)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func defaultPort(route ingress.Route) int {
	if route.TLS {
		return 443
	}
	return 80
}

// findFreePort finds an available TCP port in [min, max].
func findFreePort(ctx context.Context, min, max int) (int, error) {
	for port := min; port <= max; port++ {
		var lc net.ListenConfig
		ln, err := lc.Listen(ctx, "tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			_ = ln.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free port in range %d-%d", min, max)
}

// RouteFromManifest builds an HTTP ingress Route from manifest fields.
func RouteFromManifest(serviceName, domain string, port int, tls, www bool) ingress.Route {
	upstream := fmt.Sprintf("%s:%d", serviceName, port)
	if port == 0 {
		upstream = serviceName
	}
	return ingress.Route{
		ServiceName: serviceName,
		Domain:      domain,
		Upstream:    upstream,
		TLS:         tls,
		WWW:         www,
	}
}
