package activesync

// SendMail / SmartReply / SmartForward, MoveItems and folder mutation
// command handlers.
import (
	"encoding/base64"
	"encoding/json"
	"log"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core/models"
)

// cmdSendMail delivers a client-supplied MIME message. The same handler
// serves SendMail, SmartReply and SmartForward (all three carry the full
// message in ComposeMail:Mime; the Source element just identifies the
// original item for the client).
func (s *Service) cmdSendMail(c *fiber.Ctx, req *easRequest, user *models.User, credential, variant string) error {
	root, err := readBody(c)
	if err != nil {
		return err
	}
	if root == nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	var mime64, clientID string
	saveInSent := true
	switch variant {
	case "sendmail":
		el := root.Child(nsComposeMail, "SendMail")
		if el == nil {
			el = root
		}
		mime64 = el.ChildText(nsComposeMail, "Mime")
		clientID = el.ChildText(nsComposeMail, "ClientId")
		saveInSent = el.ChildText(nsComposeMail, "SaveInSentItems") != "0"
	case "smartreply":
		el := root.Child(nsComposeMail, "SmartReply")
		if el == nil {
			el = root
		}
		mime64 = el.ChildText(nsComposeMail, "Mime")
		clientID = el.ChildText(nsComposeMail, "ClientId")
		saveInSent = el.ChildText(nsComposeMail, "SaveInSentItems") != "0"
	case "smartforward":
		el := root.Child(nsComposeMail, "SmartForward")
		if el == nil {
			el = root
		}
		mime64 = el.ChildText(nsComposeMail, "Mime")
		clientID = el.ChildText(nsComposeMail, "ClientId")
		saveInSent = el.ChildText(nsComposeMail, "SaveInSentItems") != "0"
	}
	if mime64 == "" {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(mime64))
	if err != nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	from, recipients := mimeEnvelope(string(raw), user.Email)
	if err := s.Mail.SubmitRawAs(user.Email, credential, from, recipients, string(raw)); err != nil {
		log.Printf("activesync: %s failed for %s: %v", variant, user.Email, err)
		resp := &Element{NS: nsComposeMail, Name: titleCommand(variant)}
		resp.Add(nsComposeMail, "Status", "130")
		return writeWBXML(c, resp, req.protocolVer)
	}
	if saveInSent {
		// Exchange semantics: the server keeps a copy in Sent Items when the
		// client asks; without this the message only exists in the client.
		if err := s.Mail.AppendRaw(user.Email, credential, "Sent", string(raw), nil); err != nil {
			log.Printf("activesync: save in sent for %s: %v", user.Email, err)
		}
	}
	resp := &Element{NS: nsComposeMail, Name: titleCommand(variant)}
	if clientID != "" {
		resp.Add(nsComposeMail, "ClientId", clientID)
	}
	resp.Add(nsComposeMail, "Status", "1")
	return writeWBXML(c, resp, req.protocolVer)
}

func titleCommand(variant string) string {
	switch variant {
	case "smartreply":
		return "SmartReply"
	case "smartforward":
		return "SmartForward"
	default:
		return "SendMail"
	}
}

// mimeEnvelope extracts the envelope sender and recipients from a raw MIME
// message, falling back to the account address when parsing fails.
func mimeEnvelope(raw, fallback string) (string, []string) {
	from := ""
	var recipients []string
	headerEnd := strings.Index(raw, "\n\n")
	if headerEnd < 0 {
		return fallback, nil
	}
	headerText := raw[:headerEnd]
	for _, line := range strings.Split(headerText, "\n") {
		line = strings.TrimSuffix(line, "\r")
		// Folded headers start with whitespace; only parse top-level ones.
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || !strings.Contains(line, ":") {
			continue
		}
		k, v, _ := strings.Cut(line, ":")
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
		switch k {
		case "from":
			if from == "" {
				from = extractEmail(v)
			}
		case "to", "cc", "bcc":
			recipients = append(recipients, extractEmails(v)...)
		}
	}
	if from == "" {
		from = fallback
	}
	return from, recipients
}

func extractEmails(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if e := extractEmail(part); e != "" {
			out = append(out, e)
		}
	}
	return out
}

// extractEmail pulls the bare address out of "Name <a@b.c>" forms.
func extractEmail(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '<'); i >= 0 {
		if j := strings.IndexByte(s[i:], '>'); j > 0 {
			return strings.TrimSpace(s[i+1 : i+j])
		}
	}
	if strings.Contains(s, "@") {
		return s
	}
	return ""
}

// cmdMoveItems moves messages between folders.
func (s *Service) cmdMoveItems(c *fiber.Ctx, req *easRequest, user *models.User, credential string) error {
	root, err := readBody(c)
	if err != nil {
		return err
	}
	resp := &Element{NS: nsMove, Name: "MoveItems"}
	if root != nil {
		for _, move := range root.ChildrenNamed(nsMove, "Move") {
			response := resp.Add(nsMove, "Response", "")
			srcMsgID := move.ChildText(nsMove, "SrcMsgId")
			srcFldID := move.ChildText(nsMove, "SrcFldId")
			dstFldID := move.ChildText(nsMove, "DstFldId")
			response.Add(nsMove, "SrcMsgId", srcMsgID)
			response.Add(nsMove, "SrcFldId", srcFldID)
			response.Add(nsMove, "DstFldId", dstFldID)
			srcFolder, ok1 := s.folderNameFor(user.Email, credential, srcFldID)
			dstFolder, ok2 := s.folderNameFor(user.Email, credential, dstFldID)
			uid := parseUID(srcMsgID)
			if !ok1 || !ok2 || uid == 0 {
				response.Add(nsMove, "Status", "130")
				continue
			}
			msgID := ""
			if m, gerr := s.Mail.GetMessage(user.Email, credential, srcFolder, uid); gerr == nil {
				msgID = m.ID
			}
			if merr := s.Mail.MoveMany(user.Email, credential, srcFolder, []uint32{uid}, dstFolder); merr != nil {
				log.Printf("activesync: move %d to %s: %v", uid, dstFolder, merr)
				response.Add(nsMove, "Status", "130")
				continue
			}
			dstUID := srcMsgID
			if msgID != "" {
				if u, uerr := s.Mail.UIDByMessageID(user.Email, credential, dstFolder, msgID); uerr == nil && u > 0 {
					dstUID = strconv.FormatUint(uint64(u), 10)
				}
			}
			response.Add(nsMove, "DstMsgId", dstUID)
			response.Add(nsMove, "Status", "1")
		}
	}
	return writeWBXML(c, resp, req.protocolVer)
}

// cmdFolderMutation handles FolderCreate / FolderUpdate / FolderDelete.
func (s *Service) cmdFolderMutation(c *fiber.Ctx, req *easRequest, user *models.User, credential, variant string) error {
	root, err := readBody(c)
	if err != nil {
		return err
	}
	if root == nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	title := "FolderCreate"
	switch variant {
	case "folderupdate":
		title = "FolderUpdate"
	case "folderdelete":
		title = "FolderDelete"
	}
	resp := &Element{NS: nsFolderHierarchy, Name: title}
	displayName := root.ChildText(nsFolderHierarchy, "DisplayName")
	serverID := root.ChildText(nsFolderHierarchy, "ServerId")
	var opErr error
	switch variant {
	case "foldercreate":
		if displayName == "" {
			resp.Add(nsFolderHierarchy, "Status", "104")
			return writeWBXML(c, resp, req.protocolVer)
		}
		opErr = s.Mail.CreateFolder(user.Email, credential, displayName)
	case "folderupdate":
		oldName, ok := s.folderNameFor(user.Email, credential, serverID)
		if !ok || displayName == "" {
			resp.Add(nsFolderHierarchy, "Status", "104")
			return writeWBXML(c, resp, req.protocolVer)
		}
		opErr = s.Mail.RenameFolder(user.Email, credential, oldName, displayName)
	case "folderdelete":
		oldName, ok := s.folderNameFor(user.Email, credential, serverID)
		if !ok {
			resp.Add(nsFolderHierarchy, "Status", "104")
			return writeWBXML(c, resp, req.protocolVer)
		}
		opErr = s.Mail.DeleteFolder(user.Email, credential, oldName)
	}
	if opErr != nil {
		log.Printf("activesync: %s %s: %v", variant, displayName, opErr)
		resp.Add(nsFolderHierarchy, "Status", "130")
		return writeWBXML(c, resp, req.protocolVer)
	}
	key, kerr := s.bumpFolderSync(c, user, req.deviceID)
	if kerr != nil {
		log.Printf("activesync: folder sync key: %v", kerr)
	}
	if variant == "foldercreate" {
		_ = s.refreshFolderSnapshot(c, user, req.deviceID, credential)
	}
	resp.Add(nsFolderHierarchy, "SyncKey", key)
	if variant == "foldercreate" {
		resp.Add(nsFolderHierarchy, "ServerId", folderID(displayName))
	}
	resp.Add(nsFolderHierarchy, "Status", "1")
	return writeWBXML(c, resp, req.protocolVer)
}

func (s *Service) bumpFolderSync(c *fiber.Ctx, user *models.User, deviceID string) (string, error) {
	key := newSyncKey()
	err := s.DB.WithContext(c.Context()).Model(&models.EasDevice{}).
		Where("user_email = ? AND device_id = ?", user.Email, deviceID).
		Update("folder_sync_key", key).Error
	return key, err
}

func (s *Service) refreshFolderSnapshot(c *fiber.Ctx, user *models.User, deviceID, credential string) error {
	folders, err := s.Mail.ListFolders(user.Email, credential)
	if err != nil {
		return err
	}
	snap, _ := json.Marshal(folders)
	return s.DB.WithContext(c.Context()).Model(&models.EasDevice{}).
		Where("user_email = ? AND device_id = ?", user.Email, deviceID).
		Update("folder_snapshot", string(snap)).Error
}
