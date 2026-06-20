package podman

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
)

func (p *Runtime) ExecAttach(ctx context.Context, containerID string, ws runtime.ExecConn) error {
	shell := p.detectShell(ctx, containerID)

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

	shellInfo, _ := json.Marshal(runtime.ExecMessage{Type: "shell", Data: shell})
	_ = ws.WriteMessage(runtime.WSText, shellInfo)

	conn, err := p.c.hijack(ctx, "/libpod/exec/"+execID+"/start", `{"Detach":false,"Tty":true,"h":24,"w":80}`)
	if err != nil {
		return fmt.Errorf("podman exec start hijack: %w", err)
	}
	defer func() { _ = conn.Close() }()

	errCh := make(chan error, 2)

	go func() {
		buf := make([]byte, 4096)
		for {
			n, readErr := conn.Read(buf)
			if n > 0 {
				msg := runtime.ExecMessage{Type: "output", Data: string(buf[:n])}
				data, _ := json.Marshal(msg)
				if sendErr := ws.WriteMessage(runtime.WSText, data); sendErr != nil {
					errCh <- sendErr
					return
				}
			}
			if readErr != nil {
				if readErr == io.EOF {
					exitMsg, _ := json.Marshal(runtime.ExecMessage{Type: "exit"})
					_ = ws.WriteMessage(runtime.WSText, exitMsg)
				}
				errCh <- readErr
				return
			}
		}
	}()

	go func() {
		for {
			msgType, raw, readErr := ws.ReadMessage()
			if readErr != nil {
				errCh <- readErr
				return
			}
			if msgType != runtime.WSText && msgType != runtime.WSBinary {
				continue
			}
			var msg runtime.ExecMessage
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

func (p *Runtime) detectShell(ctx context.Context, containerID string) string {
	if p.canExec(ctx, containerID, "/bin/bash") {
		return "/bin/bash"
	}
	return "/bin/sh"
}
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
