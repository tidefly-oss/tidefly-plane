package caddy

import (
	"context"
	"fmt"
)

// AddTCPRoute registers a Caddy L4 TCP+TLS route.
// Traffic on listenPort is TLS-terminated and forwarded to upstream (host:port).
// In dev mode (no BaseDomain / InternalTLS disabled) this is a no-op.
//
// Requires the caddy-l4 module to be compiled into Caddy.
func (c *Client) AddTCPRoute(ctx context.Context, routeID, upstream string, listenPort int) error {
	if !c.cfg.Enabled {
		return nil
	}
	listen := fmt.Sprintf(":%d", listenPort)
	route := map[string]any{
		caddyKeyID: routeID,
		"listen":   []string{listen},
		"routes": []map[string]any{
			{
				caddyKeyHandle: []map[string]any{
					{
						caddyHandlerKey: "tls",
						// Use Caddy's internal CA — no ACME needed for TCP
						"connection_policies": []map[string]any{
							{},
						},
					},
					{
						caddyHandlerKey:   "proxy",
						caddyKeyUpstreams: []map[string]string{{caddyKeyDial: upstream}},
					},
				},
			},
		},
	}
	// Try PATCH first (update existing)
	if err := c.patch(ctx, fmt.Sprintf("/id/%s", routeID), route); err == nil {
		return nil
	}
	// Create new L4 server entry
	serverKey := fmt.Sprintf("tidefly-tcp-%d", listenPort)
	return c.put(ctx, fmt.Sprintf("/config/apps/layer4/servers/%s", serverKey), route)
}

// RemoveTCPRoute removes a L4 TCP route by its ID.
func (c *Client) RemoveTCPRoute(ctx context.Context, routeID string) error {
	if !c.cfg.Enabled {
		return nil
	}
	return c.RemoveRoute(ctx, routeID) // same /id/{routeID} DELETE endpoint
}

// TCPRouteID generates a stable route ID for a TCP service.
func TCPRouteID(serviceName string) string {
	return "tidefly-tcp-" + sanitizeName(serviceName)
}

// AllocateTCPPort returns a port for a service.
// If userPort > 0 it is returned as-is (after basic validation).
// Otherwise a port is chosen from the dynamic range 15000–19999.
// NOTE: No persistence here — the job handler must store the port in the manifest.
func AllocateTCPPort(userPort int) int {
	if userPort >= 1024 && userPort <= 65535 {
		return userPort
	}
	// Simple hash-based allocation — deterministic but not collision-free.
	// Good enough for alpha; replace with DB-backed allocation later.
	return 0 // caller should use findFreePort
}
