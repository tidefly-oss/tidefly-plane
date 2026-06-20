package notification

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/httpx"
)

func (h *Handler) RegisterRoutes(api huma.API, mw huma.Middlewares) {
	huma.Register(api, httpx.Op("notif-list", "GET", httpx.V1+"/notifications", "List notifications", "Notifications", mw...), h.list)
	huma.Register(api, httpx.Op("notif-list-all", "GET", httpx.V1+"/notifications/all", "All notifications", "Notifications", mw...), h.listAll)
	huma.Register(api, httpx.Op("notif-count", "GET", httpx.V1+"/notifications/count", "Unread count", "Notifications", mw...), h.count)
	huma.Register(api, httpx.Op("notif-ack", "POST", httpx.V1+"/notifications/{id}/acknowledge", "Acknowledge", "Notifications", mw...), h.acknowledge)
	huma.Register(api, httpx.Op("notif-delete", "DELETE", httpx.V1+"/notifications/{id}", "Delete notification", "Notifications", mw...), h.delete)
	huma.Register(api, httpx.Op("notif-delete-acked", "DELETE", httpx.V1+"/notifications/acknowledged", "Delete acknowledged", "Notifications", mw...), h.deleteAcknowledged)
}
