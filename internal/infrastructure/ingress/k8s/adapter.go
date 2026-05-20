// Package k8singress will implement the ingress.Adapter interface using
// Kubernetes Ingress objects. It is a placeholder — not implemented yet.
//
// When K8s support is added:
//  1. Implement Adapter using client-go to create/patch/delete Ingress objects.
//  2. Bind it in Wire instead of the Caddy adapter when the runtime is K8s.
//  3. No changes to Plane business logic required — the interface is identical.
package k8singress

import (
	"context"
	"fmt"

	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/ingress"
)

// Adapter implements ingress.Adapter using Kubernetes Ingress objects.
// NOT YET IMPLEMENTED.
type Adapter struct{}

// New returns a K8sAdapter. Panics — not implemented yet.
func New() *Adapter {
	return &Adapter{}
}

func (a *Adapter) AddRoute(_ context.Context, r ingress.Route) error {
	return fmt.Errorf("k8s ingress: not implemented (service=%q domain=%q)", r.ServiceName, r.Domain)
}

func (a *Adapter) RemoveRoute(_ context.Context, serviceName string) error {
	return fmt.Errorf("k8s ingress: not implemented (service=%q)", serviceName)
}

func (a *Adapter) UpdateRoute(_ context.Context, r ingress.Route) error {
	return fmt.Errorf("k8s ingress: not implemented (service=%q domain=%q)", r.ServiceName, r.Domain)
}
