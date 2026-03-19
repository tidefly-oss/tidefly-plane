package helpers

import (
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func GenerateTempPassword() (plain string, hash string, err error) {
	id := uuid.New().String()
	plain = id[:8] + id[9:13]
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", "", err
	}
	return plain, string(b), nil
}
