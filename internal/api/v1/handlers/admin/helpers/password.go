package helpers

import (
	"github.com/google/uuid"
	"github.com/tidefly-oss/tidefly-plane/internal/auth"
)

func GenerateTempPassword() (plain string, hash string, err error) {
	id := uuid.New().String()
	plain = id[:8] + id[9:13]

	hash, err = auth.HashPassword(plain)
	if err != nil {
		return "", "", err
	}

	return plain, hash, nil
}
