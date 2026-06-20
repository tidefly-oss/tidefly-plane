package runtime

// ExecConn abstracts a WebSocket connection for exec sessions.
// Implemented by melodyConn in the HTTP handler layer.
type ExecConn interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
}

// ExecMessage is the JSON envelope exchanged over the exec WebSocket.
type ExecMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols uint   `json:"cols,omitempty"`
	Rows uint   `json:"rows,omitempty"`
}

const (
	WSText   = 1
	WSBinary = 2
)
