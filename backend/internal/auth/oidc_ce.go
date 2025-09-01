//go:build !mailez_ee

package auth

import "github.com/gofiber/fiber/v2"

// OIDCEnabled always reports false in the community edition: the federated
// sign-in routes are never mounted, so /server/settings must not advertise
// them and the login pages must not render the SSO button.
func OIDCEnabled(string, string, string) bool { return false }

// RegisterOIDC is a no-op in the community edition.
func (m *Manager) RegisterOIDC(fiber.Router, OIDCConfig) {}
