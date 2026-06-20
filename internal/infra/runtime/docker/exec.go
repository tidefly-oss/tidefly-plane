package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
)

func (d *Runtime) ExecAttach(ctx context.Context, containerID string, ws runtime.ExecConn) error {
	shell := d.detectShell(ctx, containerID)
	cmd := []string{shell}
	if shell == "redis-cli" {
		cmd = []string{"redis-cli", "-i"}
	}

	execResp, err := d.client.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		Cmd:          cmd,
	})
	if err != nil {
		return fmt.Errorf("exec create: %w", err)
	}

	resp, err := d.client.ContainerExecAttach(ctx, execResp.ID, container.ExecStartOptions{Tty: true})
	if err != nil {
		return fmt.Errorf("exec attach: %w", err)
	}
	defer resp.Close()

	_ = d.client.ContainerExecResize(ctx, execResp.ID, container.ResizeOptions{Width: 80, Height: 24})

	shellInfo, _ := json.Marshal(runtime.ExecMessage{Type: "shell", Data: shell})
	_ = ws.WriteMessage(runtime.WSText, shellInfo)

	errCh := make(chan error, 2)

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := resp.Reader.Read(buf)
			if n > 0 {
				msg := runtime.ExecMessage{Type: "output", Data: string(buf[:n])}
				data, _ := json.Marshal(msg)
				if sendErr := ws.WriteMessage(runtime.WSText, data); sendErr != nil {
					errCh <- sendErr
					return
				}
			}
			if err != nil {
				if err == io.EOF {
					exitMsg, _ := json.Marshal(runtime.ExecMessage{Type: "exit"})
					_ = ws.WriteMessage(runtime.WSText, exitMsg)
				}
				errCh <- err
				return
			}
		}
	}()

	go func() {
		for {
			msgType, raw, err := ws.ReadMessage()
			if err != nil {
				errCh <- err
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
				if _, err := resp.Conn.Write([]byte(msg.Data)); err != nil {
					errCh <- err
					return
				}
			case "resize":
				if msg.Cols > 0 && msg.Rows > 0 {
					_ = d.client.ContainerExecResize(ctx, execResp.ID, container.ResizeOptions{
						Width:  msg.Cols,
						Height: msg.Rows,
					})
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

func (d *Runtime) detectShell(ctx context.Context, containerID string) string {
	if d.canExec(ctx, containerID, "/bin/bash") {
		return "/bin/bash"
	}
	return "/bin/sh"
}

func (d *Runtime) canExec(ctx context.Context, containerID string, binary string) bool {
	execResp, err := d.client.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{binary, "--version"},
	})
	if err != nil {
		return false
	}
	resp, err := d.client.ContainerExecAttach(ctx, execResp.ID, container.ExecStartOptions{})
	if err != nil {
		return false
	}
	resp.Close()
	inspect, err := d.client.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return false
	}
	return inspect.ExitCode != 126 && inspect.ExitCode != 127
}
