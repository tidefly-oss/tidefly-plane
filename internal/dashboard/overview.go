package dashboard

import (
	"context"
	"sync"

	"github.com/tidefly-oss/tidefly-plane/internal/access"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/middleware"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/version"
	"github.com/tidefly-oss/tidefly-plane/internal/system"
)

type systemInfoSnapshot struct {
	TideflyVersion string `json:"tidefly_version"`
}

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
	SystemInfo    *systemInfoSnapshot    `json:"system_info,omitempty"`
	Version       *system.VersionInfo    `json:"version,omitempty"`
}

// ── Fetch helpers ─────────────────────────────────────────────────────────────

func (h *Handler) fetchUser(ctx context.Context, userID string, mu *sync.Mutex, out **models.User, setErr func(error)) {
	var u models.User
	if err := h.db.WithContext(ctx).Where("id = ?", userID).First(&u).Error; err != nil {
		setErr(err)
		return
	}
	mu.Lock()
	*out = &u
	mu.Unlock()
}

func (h *Handler) fetchProjects(ctx context.Context, isAdmin bool, userID string, mu *sync.Mutex, out *[]models.Project, setErr func(error)) {
	var p []models.Project
	q := h.db.WithContext(ctx)
	if !isAdmin {
		q = q.Joins("JOIN project_members pm ON pm.project_id = projects.id AND pm.user_id = ?", userID)
	}
	if err := q.Find(&p).Error; err != nil {
		setErr(err)
		return
	}
	mu.Lock()
	*out = p
	mu.Unlock()
}

func (h *Handler) fetchNotifications(ctx context.Context, mu *sync.Mutex, out *[]models.Notification, setErr func(error)) {
	ns, err := h.notifSvc.List(ctx)
	if err != nil {
		setErr(err)
		return
	}
	mu.Lock()
	*out = ns
	mu.Unlock()
}

func (h *Handler) fetchContainers(ctx context.Context, isAdmin bool, allowed map[string]struct{}, mu *sync.Mutex, out *[]runtime.Container, setErr func(error)) {
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
	*out = filtered
	mu.Unlock()
}

func (h *Handler) fetchImages(ctx context.Context, isAdmin bool, allowed map[string]struct{}, mu *sync.Mutex, out *[]runtime.Image, setErr func(error)) {
	cs, err := h.runtime.ListContainers(ctx, true)
	if err != nil {
		setErr(err)
		return
	}
	allowedTags := make(map[string]struct{})
	for _, c := range cs {
		if access.IsInternal(c.Labels) {
			continue
		}
		if (isAdmin || access.NetworkAllowed(c.Networks, allowed)) && c.Image != "" {
			allowedTags[c.Image] = struct{}{}
		}
	}
	if len(allowedTags) == 0 {
		mu.Lock()
		*out = []runtime.Image{}
		mu.Unlock()
		return
	}
	imgs, err := h.runtime.ListImages(ctx)
	if err != nil {
		setErr(err)
		return
	}
	filtered := make([]runtime.Image, 0)
	for _, img := range imgs {
		for _, tag := range img.Tags {
			if _, ok := allowedTags[tag]; ok {
				filtered = append(filtered, img)
				break
			}
		}
	}
	mu.Lock()
	*out = filtered
	mu.Unlock()
}

func (h *Handler) fetchNetworks(ctx context.Context, isAdmin bool, allowed map[string]struct{}, mu *sync.Mutex, out *[]runtime.Network, setErr func(error)) {
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
	*out = filtered
	mu.Unlock()
}

func (h *Handler) fetchVolumes(ctx context.Context, isAdmin bool, allowed map[string]struct{}, mu *sync.Mutex, out *[]runtime.Volume, setErr func(error)) {
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
		*out = filtered
		mu.Unlock()
		return
	}
	cs, err := h.runtime.ListContainers(ctx, true)
	if err != nil {
		mu.Lock()
		*out = []runtime.Volume{}
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
	*out = filtered
	mu.Unlock()
}

func (h *Handler) fetchSettings(ctx context.Context, mu *sync.Mutex, out **models.SystemSettings) {
	var s models.SystemSettings
	if err := h.db.WithContext(ctx).First(&s).Error; err != nil {
		return
	}
	mu.Lock()
	*out = &s
	mu.Unlock()
}

// ── Overview ──────────────────────────────────────────────────────────────────

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
		versionInfo   *system.VersionInfo
	)

	setErr := func(err error) {
		mu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		mu.Unlock()
	}

	var allowed map[string]struct{}
	if !isAdmin {
		var err error
		allowed, err = access.NewStore(h.db).AllowedNetworks(claims.UserID)
		if err != nil {
			return nil, huma500("failed to load access data")
		}
	}

	wg.Add(1)
	go func() { defer wg.Done(); h.fetchUser(ctx, claims.UserID, &mu, &user, setErr) }()

	wg.Add(1)
	go func() { defer wg.Done(); h.fetchProjects(ctx, isAdmin, claims.UserID, &mu, &projects, setErr) }()

	wg.Add(1)
	go func() { defer wg.Done(); h.fetchNotifications(ctx, &mu, &notifications, setErr) }()

	wg.Add(1)
	go func() { defer wg.Done(); h.fetchContainers(ctx, isAdmin, allowed, &mu, &containers, setErr) }()

	wg.Add(1)
	go func() { defer wg.Done(); h.fetchImages(ctx, isAdmin, allowed, &mu, &images, setErr) }()

	wg.Add(1)
	go func() { defer wg.Done(); h.fetchNetworks(ctx, isAdmin, allowed, &mu, &networks, setErr) }()

	wg.Add(1)
	go func() { defer wg.Done(); h.fetchVolumes(ctx, isAdmin, allowed, &mu, &volumes, setErr) }()

	if isAdmin {
		wg.Add(1)
		go func() { defer wg.Done(); h.fetchSettings(ctx, &mu, &settings) }()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		v := system.GetCachedVersion()
		if v != nil {
			mu.Lock()
			versionInfo = v
			mu.Unlock()
		}
	}()

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
			SystemInfo:    &systemInfoSnapshot{TideflyVersion: version.Version},
			Version:       versionInfo,
		},
	}, nil
}
