package delegation

import (
	"mailez/backend/internal/core"
)

// Handler serves the delegation routes.
type Handler struct {
	*core.App
}

func New(app *core.App) *Handler { return &Handler{app} }

var currentUser = core.CurrentUser

