package http

import (
	"bytes"
	"fmt"
	"net/http"
	"os/exec"
	"time"

	"github.com/labstack/echo/v5"
)

type downloadBackupRequest struct {
	DBName     string `json:"db_name"`
	DBHost     string `json:"db_host"`
	DBPort     string `json:"db_port"`
	DBUser     string `json:"db_user"`
	DBPassword string `json:"db_password"`
}

func (h *Handler) DownloadPostgresBackup(c *echo.Context) error {
	var req downloadBackupRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	if req.DBName == "" || req.DBUser == "" || req.DBPassword == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "db_name, db_user and db_password are required")
	}

	host := req.DBHost
	if host == "" {
		host = "localhost"
	}
	port := req.DBPort
	if port == "" {
		port = "5432"
	}

	cmd := exec.CommandContext(
		c.Request().Context(),
		"pg_dump",
		"-h", host,
		"-p", port,
		"-U", req.DBUser,
		"-d", req.DBName,
		"--no-password",
		"-F", "c",
	)
	cmd.Env = append(cmd.Environ(), "PGPASSWORD="+req.DBPassword)

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return echo.NewHTTPError(
			http.StatusInternalServerError,
			fmt.Sprintf("pg_dump failed: %s", stderr.String()),
		)
	}

	filename := fmt.Sprintf("%s-%s.dump", req.DBName, time.Now().UTC().Format("2006-01-02T15-04-05"))

	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Response().Header().Set("Content-Type", "application/octet-stream")
	c.Response().Header().Set("Content-Length", fmt.Sprintf("%d", out.Len()))

	return c.Blob(http.StatusOK, "application/octet-stream", out.Bytes())
}
