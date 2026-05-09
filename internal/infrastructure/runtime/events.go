package runtime

import "time"

// ContainerEventType — Docker Event Actions die uns interessieren
type ContainerEventType string

const (
	EventStart   ContainerEventType = "start"
	EventStop    ContainerEventType = "stop"
	EventDie     ContainerEventType = "die"
	EventKill    ContainerEventType = "kill"
	EventRestart ContainerEventType = "restart"
	EventPause   ContainerEventType = "pause"
	EventUnpause ContainerEventType = "unpause"
	EventDestroy ContainerEventType = "destroy"
	EventCreate  ContainerEventType = "create"
	EventOOM     ContainerEventType = "oom"
)

// ContainerEvent — normalisiertes Docker Event
type ContainerEvent struct {
	Type        ContainerEventType `json:"type"`
	ContainerID string             `json:"container_id"`
	Name        string             `json:"name"`
	Image       string             `json:"image"`
	Status      ContainerStatus    `json:"status"` // abgeleiteter Status nach dem Event
	Time        time.Time          `json:"time"`
}

// EventToStatus leitet den Container-Status aus dem Event-Typ ab
func EventToStatus(e ContainerEventType) ContainerStatus {
	switch e {
	case EventStart, EventUnpause, EventRestart:
		return StatusRunning
	case EventPause:
		return StatusPaused
	case EventStop, EventKill, EventDie, EventOOM:
		return StatusExited
	case EventCreate:
		return StatusCreated
	default:
		return StatusStopped
	}
}
