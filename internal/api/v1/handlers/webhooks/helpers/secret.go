package helpers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

func GenerateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func BuildURL(ctx context.Context, id string) string {
	if host, ok := ctx.Value("request_host").(string); ok && host != "" {
		return "https://" + host + "/webhooks/" + id
	}
	return "/webhooks/" + id
}
