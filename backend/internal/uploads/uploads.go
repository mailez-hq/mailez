// Package uploads implements the large-attachment relay (Coremail-style 超大
// 附件): files that exceed the inline attachment cap are stored server-side
// and mailed as a token-protected download link instead.
package uploads

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// Retention is how long an uploaded relay file is kept (rolling expiry).
const Retention = 30 * 24 * time.Hour

// Service serves the upload endpoints and the expiry cleanup worker.
type Service struct {
	DB  *gorm.DB
	Cfg core.Config
}

// New assembles the uploads service.
func New(db *gorm.DB, cfg core.Config) *Service {
	return &Service{DB: db, Cfg: cfg}
}

// Register mounts the upload routes under the authenticated group.
func (s *Service) Register(r fiber.Router) {
	r.Post("/uploads", s.upload)
	r.Get("/uploads/:id", s.meta)
	r.Get("/uploads/:id/download", s.download)
	r.Delete("/uploads/:id", s.remove)
}

// dir returns the storage root, creating it on demand.
func (s *Service) dir() (string, error) {
	root := s.Cfg.UploadDir
	if root == "" {
		root = "uploads"
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return "", err
	}
	return root, nil
}

// upload stores one file and returns a token-protected download link.
// @Summary Upload large attachment
// @Tags uploads
// @Accept multipart/form-data
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /uploads [post]
func (s *Service) upload(c *fiber.Ctx) error {
	user := core.CurrentUser(c)
	fh, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "file is required"})
	}
	root, err := s.dir()
	if err != nil {
		return core.Fail(c, 500, err, "storage error")
	}
	name := sanitizeName(fh.Filename)
	key := randomKey(12)
	rel := filepath.Join(user.Email, key+"-"+name)
	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
		return core.Fail(c, 500, err, "storage error")
	}
	src, err := fh.Open()
	if err != nil {
		return core.Fail(c, 500, err, "storage error")
	}
	defer src.Close()
	sum, size, err := writeFile(abs, src)
	if err != nil {
		return core.Fail(c, 500, err, "storage error")
	}
	expires := time.Now().Add(Retention)
	row := models.UploadedFile{
		UserEmail:   user.Email,
		Filename:    name,
		ContentType: fh.Header.Get("Content-Type"),
		Size:        size,
		SHA256:      sum,
		StoredPath:  rel,
		ExpiresAt:   &expires,
	}
	if err := s.DB.Create(&row).Error; err != nil {
		_ = os.Remove(abs)
		return core.Fail(c, 500, err, "db error")
	}
	return c.JSON(fiber.Map{
		"id":       row.ID,
		"filename": name,
		"size":     size,
		"expires":  expires.Format(time.RFC3339),
		"url":      s.downloadURL(c.BaseURL(), user.Email, row.ID),
	})
}

// meta returns the caller's upload metadata (owner only).
// @Summary Upload metadata
// @Tags uploads
// @Produce json
// @Router /uploads/{id} [get]
func (s *Service) meta(c *fiber.Ctx) error {
	user := core.CurrentUser(c)
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	var row models.UploadedFile
	if err := s.DB.First(&row, "id = ? AND user_email = ?", id, user.Email).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(fiber.Map{
		"id": row.ID, "filename": row.Filename, "content_type": row.ContentType,
		"size": row.Size, "sha256": row.SHA256,
	})
}

// download streams a relay file to the holder of a valid token.
// @Summary Download large attachment
// @Tags uploads
// @Produce application/octet-stream
// @Router /uploads/{id}/download [get]
func (s *Service) download(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	var row models.UploadedFile
	if err := s.DB.First(&row, "id = ?", id).Error; err != nil {
		return c.SendStatus(fiber.StatusNotFound)
	}
	if row.ExpiresAt != nil && row.ExpiresAt.Before(time.Now()) {
		return c.SendStatus(fiber.StatusGone)
	}
	if !s.tokenValid(row.UserEmail, row.ID, c.Query("token")) {
		return c.SendStatus(fiber.StatusForbidden)
	}
	root, err := s.dir()
	if err != nil {
		return core.Fail(c, 500, err, "storage error")
	}
	abs := filepath.Join(root, row.StoredPath)
	f, err := os.Open(abs)
	if err != nil {
		return c.SendStatus(fiber.StatusNotFound)
	}
	defer f.Close()
	c.Set("Content-Type", "application/octet-stream")
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, sanitizeName(row.Filename)))
	if _, err := io.Copy(c.Response().BodyWriter(), f); err != nil {
		return core.Fail(c, 500, err, "stream error")
	}
	return nil
}

// remove deletes an upload the caller owns.
// @Summary Delete upload
// @Tags uploads
// @Success 204
// @Router /uploads/{id} [delete]
func (s *Service) remove(c *fiber.Ctx) error {
	user := core.CurrentUser(c)
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	var row models.UploadedFile
	if err := s.DB.First(&row, "id = ? AND user_email = ?", id, user.Email).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	root, _ := s.dir()
	_ = os.Remove(filepath.Join(root, row.StoredPath))
	if err := s.DB.Delete(&row).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Service) downloadURL(base, email string, id uint) string {
	return base + "/api/v1/uploads/" + strconv.FormatUint(uint64(id), 10) + "/download?token=" + s.token(email, id)
}

func (s *Service) token(email string, id uint) string {
	mac := hmac.New(sha256.New, []byte(s.Cfg.SecretKey))
	mac.Write([]byte(fmt.Sprintf("mailez-upload:%s:%d", email, id)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Service) tokenValid(email string, id uint, token string) bool {
	if token == "" {
		return false
	}
	return hmac.Equal([]byte(token), []byte(s.token(email, id)))
}

func sanitizeName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == "/" {
		return "attachment.bin"
	}
	return name
}

func randomKey(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b)
}

func writeFile(dst string, src multipart.File) (string, int64, error) {
	out, err := os.Create(dst)
	if err != nil {
		return "", 0, err
	}
	defer out.Close()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), src)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
