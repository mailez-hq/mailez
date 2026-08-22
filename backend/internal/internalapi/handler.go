package internalapi

import (
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailess/backend/internal/auth"
	"mailess/backend/internal/config"
)

// Handler implements the internal API consumed by nginx (auth_request) and the
// mail stack (nginx mail proxy auth). Its response contract must stay
// byte-compatible with Mailu's internal API.
type Handler struct {
	DB   *gorm.DB
	Auth *auth.Manager
	Cfg  config.Config
}

func New(db *gorm.DB, authMgr *auth.Manager, cfg config.Config) *Handler {
	return &Handler{DB: db, Auth: authMgr, Cfg: cfg}
}

// Register mounts the internal endpoints under /internal.
func (h *Handler) Register(r fiber.Router) {
	r.Get("/auth/user", h.authUser)
	r.Get("/auth/admin", h.authAdmin)
	r.Get("/auth/basic", h.authBasic)
	r.Get("/auth/email", h.authEmail)
	h.registerPostfix(r)
	h.registerDovecot(r)
}
