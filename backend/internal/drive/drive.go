// Package drive implements the built-in cloud drive (云盘): a per-user file
// and folder tree with blob storage on local disk or MinIO, plus share links.
package drive

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// Service serves the drive API.
type Service struct {
	DB    *gorm.DB
	Cfg   core.Config
	Store Store
}

// New assembles the drive service with its blob store.
func New(db *gorm.DB, cfg core.Config) (*Service, error) {
	st, err := NewStore(cfg)
	if err != nil {
		return nil, err
	}
	return &Service{DB: db, Cfg: cfg, Store: st}, nil
}

// Register mounts the drive routes under the authenticated group.
func (s *Service) Register(r fiber.Router) {
	r.Get("/drive/tree", s.list)
	r.Get("/drive/trash", s.listTrash)
	r.Post("/drive/folders", s.createFolder)
	r.Post("/drive/upload", s.upload)
	r.Get("/drive/download/:id", s.download)
	r.Get("/drive/share/:id", s.share)
	r.Post("/drive/rename", s.rename)
	r.Post("/drive/move", s.move)
	r.Post("/drive/trash", s.trash)
	r.Post("/drive/restore", s.restore)
	r.Post("/drive/trash/empty", s.emptyTrash)
}

func view(f *models.DriveFile) fiber.Map {
	return fiber.Map{
		"id": f.ID, "parent_id": f.ParentID, "name": f.Name, "is_dir": f.IsDir,
		"size": f.Size, "content_type": f.ContentType, "sha256": f.SHA256,
		"share_token": f.ShareToken != "", "trashed": f.Trashed,
		"created_at": f.CreatedAt, "updated_at": f.UpdatedAt,
	}
}

// list returns the children of a folder (or the root when parent_id is 0).
// @Summary List drive folder
// @Tags drive
// @Produce json
// @Router /drive/tree [get]
func (s *Service) list(c *fiber.Ctx) error {
	user := core.CurrentUser(c)
	parent, _ := strconv.ParseUint(c.Query("parent_id"), 10, 64)
	if parent != 0 {
		var parentRow models.DriveFile
		if err := s.DB.First(&parentRow, "id = ? AND user_email = ? AND is_dir = ? AND trashed = ?", parent, user.Email, true, false).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "folder not found"})
		}
	}
	var rows []models.DriveFile
	if err := s.DB.Where("user_email = ? AND parent_id = ? AND trashed = ?", user.Email, parent, false).
		Order("is_dir DESC").Order("name").Find(&rows).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	out := make([]fiber.Map, 0, len(rows))
	for i := range rows {
		out = append(out, view(&rows[i]))
	}
	return c.JSON(out)
}

// listTrash returns the caller's trashed entries.
// @Summary List trash
// @Tags drive
// @Produce json
// @Router /drive/trash [get]
func (s *Service) listTrash(c *fiber.Ctx) error {
	user := core.CurrentUser(c)
	var rows []models.DriveFile
	if err := s.DB.Where("user_email = ? AND trashed = ?", user.Email, true).
		Order("updated_at DESC").Find(&rows).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	out := make([]fiber.Map, 0, len(rows))
	for i := range rows {
		out = append(out, view(&rows[i]))
	}
	return c.JSON(out)
}

// createFolder makes a new folder under parent_id.
// @Summary Create drive folder
// @Tags drive
// @Accept json
// @Produce json
// @Router /drive/folders [post]
func (s *Service) createFolder(c *fiber.Ctx) error {
	user := core.CurrentUser(c)
	var in struct {
		ParentID uint   `json:"parent_id"`
		Name     string `json:"name"`
	}
	if err := c.BodyParser(&in); err != nil || strings.TrimSpace(in.Name) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name is required"})
	}
	if in.ParentID != 0 {
		var parent models.DriveFile
		if err := s.DB.First(&parent, "id = ? AND user_email = ? AND is_dir = ? AND trashed = ?", in.ParentID, user.Email, true, false).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "folder not found"})
		}
	}
	name := sanitizeEntryName(in.Name)
	row := models.DriveFile{UserEmail: user.Email, ParentID: in.ParentID, Name: name, IsDir: true}
	if err := s.DB.Create(&row).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	return c.JSON(view(&row))
}

// upload streams one file into the blob store and records the row.
// @Summary Upload drive file
// @Tags drive
// @Accept multipart/form-data
// @Produce json
// @Router /drive/upload [post]
func (s *Service) upload(c *fiber.Ctx) error {
	user := core.CurrentUser(c)
	parent, _ := strconv.ParseUint(c.FormValue("parent_id"), 10, 64)
	if parent != 0 {
		var parentRow models.DriveFile
		if err := s.DB.First(&parentRow, "id = ? AND user_email = ? AND is_dir = ? AND trashed = ?", parent, user.Email, true, false).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "folder not found"})
		}
	}
	fh, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "file is required"})
	}
	src, err := fh.Open()
	if err != nil {
		return core.Fail(c, 500, err, "storage error")
	}
	defer src.Close()
	key := newBlobKey(user.Email, fh.Filename, randomBytes(8))
	h := sha256.New()
	if err := s.Store.Put(c.Context(), key, io.TeeReader(src, h), fh.Size, fh.Header.Get("Content-Type")); err != nil {
		return core.Fail(c, 500, err, "storage error")
	}
	row := models.DriveFile{
		UserEmail: user.Email, ParentID: uint(parent), Name: sanitizeEntryName(fh.Filename),
		Size: fh.Size, ContentType: fh.Header.Get("Content-Type"),
		SHA256: hex.EncodeToString(h.Sum(nil)), StoredPath: key,
	}
	if err := s.DB.Create(&row).Error; err != nil {
		_ = s.Store.Delete(c.Context(), key)
		return core.Fail(c, 500, err, "db error")
	}
	return c.JSON(view(&row))
}

// download streams a file to its owner or a share-link holder.
// @Summary Download drive file
// @Tags drive
// @Produce application/octet-stream
// @Router /drive/download/{id} [get]
func (s *Service) download(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	var row models.DriveFile
	if err := s.DB.First(&row, "id = ?", id).Error; err != nil {
		return c.SendStatus(fiber.StatusNotFound)
	}
	user := core.CurrentUser(c)
	token := c.Query("token")
	if !strings.EqualFold(row.UserEmail, user.Email) && (row.ShareToken == "" || token != row.ShareToken) {
		return c.SendStatus(fiber.StatusForbidden)
	}
	rc, err := s.Store.Get(c.Context(), row.StoredPath)
	if err != nil {
		return c.SendStatus(fiber.StatusNotFound)
	}
	defer rc.Close()
	c.Set("Content-Type", row.ContentType)
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, row.Name))
	if _, err := io.Copy(c.Response().BodyWriter(), rc); err != nil {
		return core.Fail(c, 500, err, "stream error")
	}
	return nil
}

// share creates (or reuses) a share link for a file.
// @Summary Share drive file
// @Tags drive
// @Produce json
// @Router /drive/share/{id} [get]
func (s *Service) share(c *fiber.Ctx) error {
	user := core.CurrentUser(c)
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	var row models.DriveFile
	if err := s.DB.First(&row, "id = ? AND user_email = ?", id, user.Email).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	if row.ShareToken == "" {
		row.ShareToken = hex.EncodeToString(randomBytes(12))
		if err := s.DB.Model(&row).Update("share_token", row.ShareToken).Error; err != nil {
			return core.Fail(c, 500, err, "db error")
		}
	}
	return c.JSON(fiber.Map{
		"url": c.BaseURL() + "/api/v1/drive/download/" + strconv.FormatUint(uint64(row.ID), 10) + "?token=" + row.ShareToken,
	})
}

// rename renames an entry.
// @Summary Rename drive entry
// @Tags drive
// @Accept json
// @Success 204
// @Router /drive/rename [post]
func (s *Service) rename(c *fiber.Ctx) error {
	user := core.CurrentUser(c)
	var in struct {
		ID   uint   `json:"id"`
		Name string `json:"name"`
	}
	if err := c.BodyParser(&in); err != nil || in.ID == 0 || strings.TrimSpace(in.Name) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "id and name are required"})
	}
	res := s.DB.Model(&models.DriveFile{}).
		Where("id = ? AND user_email = ?", in.ID, user.Email).
		Update("name", sanitizeEntryName(in.Name))
	if res.Error != nil {
		return core.Fail(c, 500, res.Error, "db error")
	}
	if res.RowsAffected == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// move relocates an entry into another folder.
// @Summary Move drive entry
// @Tags drive
// @Accept json
// @Success 204
// @Router /drive/move [post]
func (s *Service) move(c *fiber.Ctx) error {
	user := core.CurrentUser(c)
	var in struct {
		ID       uint `json:"id"`
		ParentID uint `json:"parent_id"`
	}
	if err := c.BodyParser(&in); err != nil || in.ID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "id is required"})
	}
	var row models.DriveFile
	if err := s.DB.First(&row, "id = ? AND user_email = ?", in.ID, user.Email).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	if in.ParentID != 0 {
		var parent models.DriveFile
		if err := s.DB.First(&parent, "id = ? AND user_email = ? AND is_dir = ? AND trashed = ?", in.ParentID, user.Email, true, false).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "folder not found"})
		}
		if row.IsDir && isDescendant(s.DB, in.ParentID, in.ID, user.Email) {
			return c.Status(400).JSON(fiber.Map{"error": "cannot move a folder into itself"})
		}
	}
	if err := s.DB.Model(&row).Update("parent_id", in.ParentID).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// trash moves entries (and descendants) to trash.
// @Summary Trash drive entries
// @Tags drive
// @Accept json
// @Success 204
// @Router /drive/trash [post]
func (s *Service) trash(c *fiber.Ctx) error {
	user := core.CurrentUser(c)
	var in struct {
		IDs []uint `json:"ids"`
	}
	if err := c.BodyParser(&in); err != nil || len(in.IDs) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "ids are required"})
	}
	return s.setTrashed(c, user.Email, in.IDs, true)
}

// restore brings entries back from trash.
// @Summary Restore drive entries
// @Tags drive
// @Accept json
// @Success 204
// @Router /drive/restore [post]
func (s *Service) restore(c *fiber.Ctx) error {
	user := core.CurrentUser(c)
	var in struct {
		IDs []uint `json:"ids"`
	}
	if err := c.BodyParser(&in); err != nil || len(in.IDs) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "ids are required"})
	}
	return s.setTrashed(c, user.Email, in.IDs, false)
}

func (s *Service) setTrashed(c *fiber.Ctx, email string, ids []uint, trashed bool) error {
	if err := s.DB.Model(&models.DriveFile{}).
		Where("user_email = ? AND id IN ?", email, ids).
		Update("trashed", trashed).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// emptyTrash permanently deletes trashed entries and their blobs.
// @Summary Empty drive trash
// @Tags drive
// @Success 204
// @Router /drive/trash/empty [post]
func (s *Service) emptyTrash(c *fiber.Ctx) error {
	user := core.CurrentUser(c)
	var rows []models.DriveFile
	if err := s.DB.Where("user_email = ? AND trashed = ?", user.Email, true).Find(&rows).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	for _, row := range rows {
		if !row.IsDir {
			if err := s.Store.Delete(c.Context(), row.StoredPath); err != nil {
				log.Printf("drive: delete blob %s: %v", row.StoredPath, err)
			}
		}
		if err := s.DB.Delete(&row).Error; err != nil {
			return core.Fail(c, 500, err, "db error")
		}
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// isDescendant reports whether candidate is inside the subtree rooted at root.
func isDescendant(db *gorm.DB, candidate, root uint, email string) bool {
	seen := map[uint]bool{}
	cur := candidate
	for cur != 0 && !seen[cur] {
		if cur == root {
			return true
		}
		seen[cur] = true
		var row models.DriveFile
		if err := db.First(&row, "id = ? AND user_email = ?", cur, email).Error; err != nil {
			return false
		}
		cur = row.ParentID
	}
	return false
}

func sanitizeEntryName(name string) string {
	name = strings.TrimSpace(name)
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "" || name == "." || name == "/" {
		return "untitled"
	}
	return name
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return []byte(strconv.FormatInt(time.Now().UnixNano(), 36))
	}
	return b
}
