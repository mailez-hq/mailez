package api

import (
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailess/backend/internal/models"
)

func (h *Handler) registerConfig(r fiber.Router, mw fiber.Handler) {
	r.Get("/config/export", mw, h.exportConfig)
	r.Post("/config/import", mw, h.importConfig)
}

// userBackup carries the password hash too (User.Password is json:"-"), so an
// exported backup can restore credentials on a fresh instance.
type userBackup struct {
	models.User
	Password string `json:"password"`
}

type configBackup struct {
	Domains      []models.Domain      `json:"domains"`
	Alternatives []models.Alternative `json:"alternatives"`
	Relays       []models.Relay       `json:"relays"`
	Users        []userBackup         `json:"users"`
	Aliases      []models.Alias       `json:"aliases"`
	Fetches      []models.Fetch       `json:"fetches"`
	Tokens       []models.Token       `json:"tokens"`
}

// exportConfig dumps all management data as JSON for backup or migration.
func (h *Handler) exportConfig(c *fiber.Ctx) error {
	backup := configBackup{}
	if err := h.DB.Find(&backup.Domains).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if err := h.DB.Find(&backup.Alternatives).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if err := h.DB.Find(&backup.Relays).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if err := h.DB.Find(&backup.Fetches).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if err := h.DB.Find(&backup.Tokens).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	var users []models.User
	if err := h.DB.Find(&users).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	for _, u := range users {
		backup.Users = append(backup.Users, userBackup{User: u, Password: u.Password})
	}
	if err := h.DB.Find(&backup.Aliases).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(backup)
}

// importConfig restores a backup in a transaction. Each entity is upserted by
// its natural key so the endpoint is idempotent.
func (h *Handler) importConfig(c *fiber.Ctx) error {
	var in configBackup
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid backup"})
	}
	stats := fiber.Map{"domains": 0, "alternatives": 0, "relays": 0, "users": 0, "aliases": 0, "fetches": 0, "tokens": 0}
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		for i := range in.Domains {
			if err := tx.Save(&in.Domains[i]).Error; err != nil {
				return err
			}
			stats["domains"] = stats["domains"].(int) + 1
		}
		for i := range in.Alternatives {
			if err := tx.Save(&in.Alternatives[i]).Error; err != nil {
				return err
			}
			stats["alternatives"] = stats["alternatives"].(int) + 1
		}
		for i := range in.Relays {
			if err := tx.Save(&in.Relays[i]).Error; err != nil {
				return err
			}
			stats["relays"] = stats["relays"].(int) + 1
		}
		for i := range in.Users {
			ub := &in.Users[i]
			ub.User.Password = ub.Password // hash restored verbatim
			if err := tx.Save(&ub.User).Error; err != nil {
				return err
			}
			stats["users"] = stats["users"].(int) + 1
		}
		for i := range in.Aliases {
			if err := tx.Save(&in.Aliases[i]).Error; err != nil {
				return err
			}
			stats["aliases"] = stats["aliases"].(int) + 1
		}
		for i := range in.Fetches {
			if err := tx.Save(&in.Fetches[i]).Error; err != nil {
				return err
			}
			stats["fetches"] = stats["fetches"].(int) + 1
		}
		for i := range in.Tokens {
			if err := tx.Save(&in.Tokens[i]).Error; err != nil {
				return err
			}
			stats["tokens"] = stats["tokens"].(int) + 1
		}
		return nil
	})
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(stats)
}
