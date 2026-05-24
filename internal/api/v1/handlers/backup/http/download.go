package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"time"
)

type downloadBackupRequest struct {
	DBName     string `json:"db_name"`
	DBHost     string `json:"db_host"`
	DBPort     string `json:"db_port"`
	DBUser     string `json:"db_user"`
	DBPassword string `json:"db_password"`
}

func (h *Handler) DownloadPostgresBackup(w http.ResponseWriter, r *http.Request) {
	var req downloadBackupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"message":"invalid request"}`, http.StatusBadRequest)
		return
	}
	if req.DBName == "" || req.DBUser == "" || req.DBPassword == "" {
		http.Error(w, `{"message":"db_name, db_user and db_password are required"}`, http.StatusBadRequest)
		return
	}

	host := req.DBHost
	if host == "" {
		host = "localhost"
	}
	port := req.DBPort
	if port == "" {
		port = "5432"
	}

	cmd := exec.CommandContext(r.Context(),
		"pg_dump",
		"-h", host,
		"-p", port,
		"-U", req.DBUser,
		"-d", req.DBName,
		"--no-password",
		"-F", "c",
	)
	cmd.Env = append(cmd.Environ(), "PGPASSWORD="+req.DBPassword)

	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		http.Error(w, fmt.Sprintf(`{"message":"pg_dump failed: %s"}`, stderr.String()), http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("%s-%s.dump", req.DBName, time.Now().UTC().Format("2006-01-02T15-04-05"))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", out.Len()))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out.Bytes())
}
