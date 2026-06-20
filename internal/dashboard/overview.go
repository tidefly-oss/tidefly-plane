package dashboard

import (
	"context"
	"sync"

	"github.com/tidefly-oss/tidefly-plane/internal/access"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/middleware"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

type overviewOutput struct {
	Body overviewBody
}

type overviewBody struct {
	User          *models.User           `json:"user"`
	Projects      []models.Project       `json:"projects"`
	Notifications []models.Notification  `json:"notifications"`
	Containers    []runtime.Container    `json:"containers"`
	Images        []runtime.Image        `json:"images"`
	Networks      []runtime.Network      `json:"networks"`
	Volumes       []runtime.Volume       `json:"volumes"`
	Settings      *models.SystemSettings `json:"settings,omitempty"`
}

func (h *Handler) overview(ctx context.Context, _ *struct{}) (*overviewOutput, error) {
	claims := middleware.UserFromHumaCtx(ctx)
	if claims == nil {
		return nil, huma401("unauthorized")
	}

	isAdmin := claims.Role == string(models.RoleAdmin)

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

	// Preload allowed networks for non-admins (used by container/network/volume filter)
	var allowed map[string]struct{}
	if !isAdmin {
		var err error
		allowed, err = access.NewStore(h.db).AllowedNetworks(claims.UserID)
		if err != nil {
			return nil, huma500("failed to load access data")
		}
	}

	// ── User ──────────────────────────────────────────────────────────────────
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

	// ── Projects ──────────────────────────────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		var p []models.Project
		q := h.db.WithContext(ctx)
		if !isAdmin {
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

	// ── Containers ────────────────────────────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		cs, err := h.runtime.ListContainers(ctx, true)
		if err != nil {
			setErr(err)
			return
		}
		filtered := make([]runtime.Container, 0, len(cs))
		for _, c := range cs {
			if access.IsInternal(c.Labels) {
				continue
			}
			if isAdmin || access.NetworkAllowed(c.Networks, allowed) {
				filtered = append(filtered, c)
			}
		}
		mu.Lock()
		containers = filtered
		mu.Unlock()
	}()

	// ── Images ────────────────────────────────────────────────────────────────
	// Internal image filtering is handled inside ListImages (via container labels).
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

	// ── Networks ──────────────────────────────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		nets, err := h.runtime.ListNetworks(ctx)
		if err != nil {
			setErr(err)
			return
		}
		filtered := make([]runtime.Network, 0, len(nets))
		for _, n := range nets {
			if !access.IsManaged(n.Labels) || n.Name == access.NetworkProxy {
				continue
			}
			if isAdmin {
				filtered = append(filtered, n)
				continue
			}
			if _, ok := allowed[n.Name]; ok {
				filtered = append(filtered, n)
			}
		}
		mu.Lock()
		networks = filtered
		mu.Unlock()
	}()

	// ── Volumes ───────────────────────────────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		vols, err := h.runtime.ListVolumes(ctx)
		if err != nil {
			setErr(err)
			return
		}
		if isAdmin {
			filtered := make([]runtime.Volume, 0, len(vols))
			for _, v := range vols {
				if access.IsManaged(v.Labels) {
					filtered = append(filtered, v)
				}
			}
			mu.Lock()
			volumes = filtered
			mu.Unlock()
			return
		}
		cs, err := h.runtime.ListContainers(ctx, true)
		if err != nil {
			mu.Lock()
			volumes = []runtime.Volume{}
			mu.Unlock()
			return
		}
		allowedVols := make(map[string]struct{})
		for _, ct := range cs {
			if access.IsInternal(ct.Labels) || !access.NetworkAllowed(ct.Networks, allowed) {
				continue
			}
			details, err := h.runtime.GetContainer(ctx, ct.ID)
			if err != nil {
				continue
			}
			for _, m := range details.Mounts {
				if m.Source != "" {
					allowedVols[m.Source] = struct{}{}
				}
			}
		}
		filtered := make([]runtime.Volume, 0, len(vols))
		for _, v := range vols {
			if access.IsManaged(v.Labels) {
				if _, ok := allowedVols[v.Name]; ok {
					filtered = append(filtered, v)
				}
			}
		}
		mu.Lock()
		volumes = filtered
		mu.Unlock()
	}()

	// ── Settings (admin only) ─────────────────────────────────────────────────
	if isAdmin {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var s models.SystemSettings
			if err := h.db.WithContext(ctx).First(&s).Error; err != nil {
				return
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

	return &overviewOutput{
		Body: overviewBody{
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
