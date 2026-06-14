// Package http provides the HTTP handler for the dashboard overview aggregation endpoint.
package http

import (
	"context"
	"sync"

	"github.com/tidefly-oss/tidefly-plane/internal/api/middleware"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

// ── Input / Output ────────────────────────────────────────────────────────────

type OverviewInput struct{}

type OverviewOutput struct {
	Body OverviewBody
}

type OverviewBody struct {
	User          *models.User           `json:"user"`
	Projects      []models.Project       `json:"projects"`
	Notifications []models.Notification  `json:"notifications"`
	Containers    []runtime.Container    `json:"containers"`
	Images        []runtime.Image        `json:"images"`
	Networks      []runtime.Network      `json:"networks"`
	Volumes       []runtime.Volume       `json:"volumes"`
	Settings      *models.SystemSettings `json:"settings,omitempty"` // only for admins
}

// ── Handler ───────────────────────────────────────────────────────────────────

// Overview aggregates all data needed for the initial dashboard page load.
// Runs all DB/runtime queries concurrently and returns in a single response.
// Replaces: /auth/me, /projects, /notifications,
//
//	/containers?all=true, /images, /networks, /volumes, /admin/settings
func (h *Handler) Overview(ctx context.Context, _ *OverviewInput) (*OverviewOutput, error) {
	claims := middleware.UserFromHumaCtx(ctx)
	if claims == nil {
		return nil, huma401("unauthorized")
	}

	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		firstErr error

		user          *models.User
		projects      []models.Project
		notifications []models.Notification
		containers    []runtime.Container
		images        []runtime.Image
		networks      []runtime.Network
		volumes       []runtime.Volume
		settings      *models.SystemSettings
	)

	setErr := func(err error) {
		mu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		mu.Unlock()
	}

	// ── User ─────────────────────────────────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		var u models.User
		if err := h.db.WithContext(ctx).Where("id = ?", claims.UserID).First(&u).Error; err != nil {
			setErr(err)
			return
		}
		mu.Lock()
		user = &u
		mu.Unlock()
	}()

	// ── Projects ─────────────────────────────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		var p []models.Project
		q := h.db.WithContext(ctx)
		if claims.Role != string(models.RoleAdmin) {
			q = q.Joins("JOIN project_members pm ON pm.project_id = projects.id AND pm.user_id = ?", claims.UserID)
		}
		if err := q.Find(&p).Error; err != nil {
			setErr(err)
			return
		}
		mu.Lock()
		projects = p
		mu.Unlock()
	}()

	// ── Notifications ─────────────────────────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		ns, err := h.notifSvc.List(ctx)
		if err != nil {
			setErr(err)
			return
		}
		mu.Lock()
		notifications = ns
		mu.Unlock()
	}()

	// ── Containers ───────────────────────────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		cs, err := h.runtime.ListContainers(ctx, true)
		if err != nil {
			setErr(err)
			return
		}
		mu.Lock()
		containers = cs
		mu.Unlock()
	}()

	// ── Images ───────────────────────────────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		imgs, err := h.runtime.ListImages(ctx)
		if err != nil {
			setErr(err)
			return
		}
		mu.Lock()
		images = imgs
		mu.Unlock()
	}()

	// ── Networks ─────────────────────────────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		nets, err := h.runtime.ListNetworks(ctx)
		if err != nil {
			setErr(err)
			return
		}
		mu.Lock()
		networks = nets
		mu.Unlock()
	}()

	// ── Volumes ──────────────────────────────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		vols, err := h.runtime.ListVolumes(ctx)
		if err != nil {
			setErr(err)
			return
		}
		mu.Lock()
		volumes = vols
		mu.Unlock()
	}()

	// ── Settings (admin only) ─────────────────────────────────────────────────
	if claims.Role == string(models.RoleAdmin) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var s models.SystemSettings
			if err := h.db.WithContext(ctx).First(&s).Error; err != nil {
				return // non-fatal
			}
			mu.Lock()
			settings = &s
			mu.Unlock()
		}()
	}

	wg.Wait()

	if firstErr != nil {
		h.log.Error("dashboard", "overview aggregation failed", firstErr)
		return nil, huma500("failed to load dashboard data")
	}

	return &OverviewOutput{
		Body: OverviewBody{
			User:          user,
			Projects:      projects,
			Notifications: notifications,
			Containers:    containers,
			Images:        images,
			Networks:      networks,
			Volumes:       volumes,
			Settings:      settings,
		},
	}, nil
}
