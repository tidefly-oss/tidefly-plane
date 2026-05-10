package main

import (
	"net/http"
	"os"
)

func main() {
	resp, err := http.Get("http://localhost:8181/api/v1/system/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		os.Exit(1)
	}
	_ = resp.Body.Close()
	os.Exit(0)
}
