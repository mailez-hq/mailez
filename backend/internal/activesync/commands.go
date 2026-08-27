package activesync

// Provision, FolderSync and Settings command handlers.
import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailez/backend/internal/core/models"
)

// cmdProvision runs the device policy handshake. We ship a permissive policy
// document so iOS/Outlook complete provisioning without blocking users.
func (s *Service) cmdProvision(c *fiber.Ctx, req *easRequest, user *models.User) error {
	root, err := readBody(c)
	if err != nil {
		return err
	}
	if root == nil {
		return writeEmptyOK(c, req.protocolVer)
	}
	policies := root.Child(nsProvision, "Policies")
	var (
		policyType string
		policyKey  string
	)
	if policies != nil {
		if p := policies.Child(nsProvision, "Policy"); p != nil {
			policyType = p.ChildText(nsProvision, "PolicyType")
			policyKey = p.ChildText(nsProvision, "PolicyKey")
		}
	}
	if policyType == "" {
		policyType = "MS-EAS-Provisioning-WBXML"
	}
	if policyKey != "" {
		// Second round: client accepts the policy; store the key and finish.
		var dev models.EasDevice
		if err := s.DB.WithContext(c.Context()).
			Where("user_email = ? AND device_id = ?", user.Email, req.deviceID).
			First(&dev).Error; err == nil && dev.PolicyKey != "" && dev.PolicyKey != policyKey {
			// Policy key mismatch: client is stale; answer with the current key.
			policyKey = dev.PolicyKey
		}
		if err := s.DB.WithContext(c.Context()).Model(&models.EasDevice{}).
			Where("user_email = ? AND device_id = ?", user.Email, req.deviceID).
			Update("policy_key", policyKey).Error; err != nil {
			return err
		}
		return writeEmptyOK(c, req.protocolVer)
	}
	key := newSyncKey()
	if err := s.DB.WithContext(c.Context()).Model(&models.EasDevice{}).
		Where("user_email = ? AND device_id = ?", user.Email, req.deviceID).
		Update("policy_key", key).Error; err != nil {
		return err
	}
	resp := &Element{NS: nsProvision, Name: "Provision"}
	pols := resp.Add(nsProvision, "Policies", "")
	pol := pols.Add(nsProvision, "Policy", "")
	pol.Add(nsProvision, "PolicyType", policyType)
	pol.Add(nsProvision, "Status", "1")
	pol.Add(nsProvision, "PolicyKey", key)
	data := pol.Add(nsProvision, "Data", "")
	doc := data.Add(nsProvision, "EASProvisionDoc", "")
	doc.Add(nsProvision, "DevicePasswordEnabled", "1")
	doc.Add(nsProvision, "MinDevicePasswordLength", "4")
	doc.Add(nsProvision, "MaxInactivityTimeDeviceLock", "900")
	doc.Add(nsProvision, "MaxDevicePasswordFailedAttempts", "10")
	doc.Add(nsProvision, "AllowSimpleDevicePassword", "1")
	doc.Add(nsProvision, "AllowStorageCard", "1")
	doc.Add(nsProvision, "RequireDeviceEncryption", "0")
	resp.Add(nsProvision, "Status", "1")
	return writeWBXML(c, resp, req.protocolVer)
}

// cmdFolderSync synchronizes the folder hierarchy for a device.
func (s *Service) cmdFolderSync(c *fiber.Ctx, req *easRequest, user *models.User, credential string) error {
	root, err := readBody(c)
	if err != nil {
		return err
	}
	var syncKey string
	if root != nil {
		syncKey = root.ChildText(nsFolderHierarchy, "SyncKey")
	}
	resp := &Element{NS: nsFolderHierarchy, Name: "FolderSync"}
	if syncKey == "" || syncKey == "0" {
		// Initialize the hierarchy snapshot and hand back a fresh key.
		key, err := s.resetFolderSnapshot(c, user, req.deviceID, nil)
		if err != nil {
			return err
		}
		resp.Add(nsFolderHierarchy, "Status", "1")
		resp.Add(nsFolderHierarchy, "SyncKey", key)
		return writeWBXML(c, resp, req.protocolVer)
	}
	folders, err := s.Mail.ListFolders(user.Email, credential)
	if err != nil {
		return err
	}
	sort.Strings(folders)
	var dev models.EasDevice
	if err := s.DB.WithContext(c.Context()).
		Where("user_email = ? AND device_id = ?", user.Email, req.deviceID).
		First(&dev).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			key, rerr := s.resetFolderSnapshot(c, user, req.deviceID, folders)
			if rerr != nil {
				return rerr
			}
			resp.Add(nsFolderHierarchy, "Status", "1")
			resp.Add(nsFolderHierarchy, "SyncKey", key)
			return writeWBXML(c, resp, req.protocolVer)
		}
		return err
	}
	if dev.FolderSyncKey != "" && dev.FolderSyncKey != syncKey {
		// Stale key: force a clean start.
		key, rerr := s.resetFolderSnapshot(c, user, req.deviceID, folders)
		if rerr != nil {
			return rerr
		}
		resp.Add(nsFolderHierarchy, "Status", "1")
		resp.Add(nsFolderHierarchy, "SyncKey", key)
		return writeWBXML(c, resp, req.protocolVer)
	}
	var old []string
	_ = json.Unmarshal([]byte(dev.FolderSnapshot), &old)
	oldIDs := map[string]string{} // id -> display name
	for _, name := range old {
		oldIDs[folderID(name)] = name
	}
	changes := resp.Add(nsFolderHierarchy, "Changes", "")
	for _, name := range folders {
		id := folderID(name)
		if _, seen := oldIDs[id]; seen {
			continue
		}
		add := changes.Add(nsFolderHierarchy, "Add", "")
		add.Add(nsFolderHierarchy, "ServerId", id)
		add.Add(nsFolderHierarchy, "ParentId", "0")
		add.Add(nsFolderHierarchy, "DisplayName", name)
		add.Add(nsFolderHierarchy, "Type", fmt.Sprintf("%d", folderType(name)))
	}
	for id := range oldIDs {
		found := false
		for _, name := range folders {
			if folderID(name) == id {
				found = true
				break
			}
		}
		if !found {
			del := changes.Add(nsFolderHierarchy, "Delete", "")
			del.Add(nsFolderHierarchy, "ServerId", id)
		}
	}
	key := newSyncKey()
	snap, _ := json.Marshal(folders)
	if err := s.DB.WithContext(c.Context()).Model(&models.EasDevice{}).
		Where("id = ?", dev.ID).
		Updates(map[string]any{"folder_sync_key": key, "folder_snapshot": string(snap)}).Error; err != nil {
		return err
	}
	resp.Add(nsFolderHierarchy, "Status", "1")
	resp.Add(nsFolderHierarchy, "SyncKey", key)
	return writeWBXML(c, resp, req.protocolVer)
}

func (s *Service) resetFolderSnapshot(c *fiber.Ctx, user *models.User, deviceID string, folders []string) (string, error) {
	key := newSyncKey()
	snap, _ := json.Marshal(folders)
	err := s.DB.WithContext(c.Context()).Model(&models.EasDevice{}).
		Where("user_email = ? AND device_id = ?", user.Email, deviceID).
		Updates(map[string]any{"folder_sync_key": key, "folder_snapshot": string(snap)}).Error
	return key, err
}

// cmdSettings answers Settings requests: user information, device
// information and OOF status. Mailbox OOF is managed by webmail/Sieve, so
// the responses are minimal but valid.
func (s *Service) cmdSettings(c *fiber.Ctx, req *easRequest, user *models.User) error {
	root, err := readBody(c)
	if err != nil {
		return err
	}
	resp := &Element{NS: nsSettings, Name: "Settings"}
	if root != nil {
		if ui := root.Child(nsSettings, "UserInformation"); ui != nil {
			info := resp.Add(nsSettings, "UserInformation", "")
			info.Add(nsSettings, "Status", "1")
			addrs := info.Add(nsSettings, "EmailAddresses", "")
			addrs.Add(nsSettings, "SmtpAddress", user.Email)
			info.Add(nsSettings, "Accounts", "")
		}
		if di := root.Child(nsSettings, "DeviceInformation"); di != nil {
			out := resp.Add(nsSettings, "DeviceInformation", "")
			out.Add(nsSettings, "Status", "1")
			// Persist the reported model/OS into the device registry.
			updates := map[string]any{}
			if v := di.ChildText(nsSettings, "Model"); v != "" {
				updates["device_type"] = v
			}
			if v := di.ChildText(nsSettings, "OS"); v != "" {
				updates["user_agent"] = v
			}
			if len(updates) > 0 {
				_ = s.DB.WithContext(c.Context()).Model(&models.EasDevice{}).
					Where("user_email = ? AND device_id = ?", user.Email, req.deviceID).
					Updates(updates).Error
			}
		}
		if oof := root.Child(nsSettings, "Oof"); oof != nil {
			if oof.Child(nsSettings, "Get") != nil {
				get := resp.Add(nsSettings, "Oof", "")
				get.Add(nsSettings, "OofState", "0")
				get.Add(nsSettings, "Status", "1")
			}
		}
	}
	resp.Add(nsSettings, "Status", "1")
	return writeWBXML(c, resp, req.protocolVer)
}

// folderNameFor resolves an EAS collection id to an IMAP folder name, or ""
// when the folder does not exist.
func (s *Service) folderNameFor(email, credential, collectionID string) (string, bool) {
	if name, ok := folderForID(collectionID); ok {
		return name, true
	}
	folders, err := s.Mail.ListFolders(email, credential)
	if err != nil {
		return "", false
	}
	for _, name := range folders {
		if folderID(name) == collectionID {
			return name, true
		}
	}
	return "", false
}

// mailboxesByID returns the folder-id → name map for the account.
func (s *Service) mailboxesByID(email, credential string) (map[string]string, error) {
	folders, err := s.Mail.ListFolders(email, credential)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(folders))
	for _, name := range folders {
		out[folderID(name)] = name
	}
	return out, nil
}

func normalizeFolderName(name string) string {
	return strings.TrimSpace(name)
}
