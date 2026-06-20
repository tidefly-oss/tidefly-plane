package log

import (
	"gorm.io/gorm"
)

type Handler struct {
	store *Store
}

func NewHandler(db *gorm.DB) *Handler {
	return &Handler{store: NewStore(db)}
}
