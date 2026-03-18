package podman

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/gorilla/websocket"
)

// ExecMessage — WebSocket message format (identical to Docker runtime).
type ExecMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols uint   `json:"cols,omitempty"`
	Rows uint   `json:"rows,omitempty"`
}

// ExecAttach opens an interactive TTY session via Podman's Exec API
// and bridges it with a gorilla WebSocket connection.
//
// Flow:
//  1. POST /libpod/containers/{id}/exec  → create exec session, get ID
//  2. POST /libpod/exec/{id}/start       → HTTP Upgrade to raw TCP (hijack)
//  3. Bidirectional pipe: WebSocket ↔ raw TCP conn
func (p *Runtime) ExecAttach(ctx context.Context, containerID string, ws *websocket.Conn) error {
	shell := p.detectShell(ctx, containerID)

	// 1. Create exec session
	execBody := map[string]any{
		"AttachStdin":  true,
		"AttachStdout": true,
		"AttachStderr": true,
		"Tty":          true,
		"Cmd":          []string{shell},
	}

	code, respBody, err := p.c.post(ctx, "/libpod/containers/"+escPath(containerID)+"/exec", nil, execBody)
	if err != nil {
		return fmt.Errorf("podman exec create: %w", err)
	}
	if code != 201 {
		return fmt.Errorf("podman exec create: status %d", code)
	}

	var createResult struct {
		ID string `json:"Id"`
	}
	if err := json.Unmarshal(respBody, &createResult); err != nil {
		return fmt.Errorf("podman exec create: decode ID: %w", err)
	}
	execID := createResult.ID

	// Send shell info to frontend
	shellInfo, _ := json.Marshal(ExecMessage{Type: "shell", Data: shell})
	_ = ws.WriteMessage(websocket.TextMessage, shellInfo)

	// 2. Start exec via HTTP Upgrade (hijack) directly on the Unix socket
	conn, err := p.c.hijack(ctx, "/libpod/exec/"+execID+"/start", `{"Detach":false,"Tty":true,"h":24,"w":80}`)
	if err != nil {
		return fmt.Errorf("podman exec start hijack: %w", err)
	}
	defer conn.Close()

	errCh := make(chan error, 2)

	// raw TCP → WebSocket (output)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, readErr := conn.Read(buf)
			if n > 0 {
				msg := ExecMessage{Type: "output", Data: string(buf[:n])}
				data, _ := json.Marshal(msg)
				if sendErr := ws.WriteMessage(websocket.TextMessage, data); sendErr != nil {
					errCh <- sendErr
					return
				}
			}
			if readErr != nil {
				if readErr == io.EOF {
					exitMsg, _ := json.Marshal(ExecMessage{Type: "exit"})
					_ = ws.WriteMessage(websocket.TextMessage, exitMsg)
				}
				errCh <- readErr
				return
			}
		}
	}()

	// WebSocket → raw TCP (input + resize)
	go func() {
		for {
			msgType, raw, readErr := ws.ReadMessage()
			if readErr != nil {
				if websocket.IsCloseError(
					readErr,
					websocket.CloseNormalClosure,
					websocket.CloseGoingAway,
					websocket.CloseNoStatusReceived,
				) {
					errCh <- nil
				} else {
					errCh <- readErr
				}
				return
			}

			if msgType != websocket.TextMessage && msgType != websocket.BinaryMessage {
				continue
			}

			var msg ExecMessage
			if err := json.Unmarshal(raw, &msg); err != nil {
				continue
			}

			switch msg.Type {
			case "input":
				if _, writeErr := conn.Write([]byte(msg.Data)); writeErr != nil {
					errCh <- writeErr
					return
				}
			case "resize":
				if msg.Cols > 0 && msg.Rows > 0 {
					// Resize via Podman exec resize endpoint
					resizeBody := fmt.Sprintf(`{"h":%d,"w":%d}`, msg.Rows, msg.Cols)
					_, _, _ = p.c.post(ctx, "/libpod/exec/"+execID+"/resize", nil, json.RawMessage(resizeBody))
				}
			case "close":
				errCh <- nil
				return
			}
		}
	}()

	select {
	case <-ctx.Done():
	case <-errCh:
	}
	return nil
}

// detectShell tries shells in order and returns the first available one.
func (p *Runtime) detectShell(ctx context.Context, containerID string) string {
	for _, shell := range []string{"/bin/bash", "/bin/sh", "/bin/ash", "redis-cli"} {
		if p.canExec(ctx, containerID, shell) {
			return shell
		}
	}
	return "/bin/sh"
}

// canExec checks if a binary is executable inside the container.
func (p *Runtime) canExec(ctx context.Context, containerID string, binary string) bool {
	execBody := map[string]any{
		"AttachStdout": true,
		"AttachStderr": true,
		"Tty":          false,
		"Cmd":          []string{binary, "--version"},
	}

	code, respBody, err := p.c.post(ctx, "/libpod/containers/"+escPath(containerID)+"/exec", nil, execBody)
	if err != nil || code != 201 {
		return false
	}

	var result struct {
		ID string `json:"Id"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return false
	}

	startBody := map[string]any{"Detach": false, "Tty": false}
	_, _, _ = p.c.post(ctx, "/libpod/exec/"+result.ID+"/start", nil, startBody)

	var session struct {
		ExitCode int `json:"ExitCode"`
	}
	inspCode, err := p.c.getJSON(ctx, "/libpod/exec/"+result.ID+"/json", nil, &session)
	if err != nil || inspCode != 200 {
		return false
	}

	return session.ExitCode != 126 && session.ExitCode != 127
}
