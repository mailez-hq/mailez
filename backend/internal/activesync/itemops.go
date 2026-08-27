package activesync

// ItemOperations, Search, ResolveRecipients and MeetingResponse handlers.
import (
	"encoding/base64"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core/models"
	"mailez/backend/internal/invite"
	"mailez/backend/internal/mail"
)

// cmdItemOperations serves Fetch (message bodies, attachments) and
// EmptyFolderContents.
func (s *Service) cmdItemOperations(c *fiber.Ctx, req *easRequest, user *models.User, credential string) error {
	root, err := readBody(c)
	if err != nil {
		return err
	}
	resp := &Element{NS: nsItemOperations, Name: "ItemOperations"}
	response := resp.Add(nsItemOperations, "Response", "")
	if root != nil {
		if fetch := root.Child(nsItemOperations, "Fetch"); fetch != nil {
			for _, store := range fetch.ChildrenNamed(nsItemOperations, "Store") {
				out := response.Add(nsItemOperations, "Fetch", "")
				pref := parseBodyPreference(store)
				collectionID := store.ChildText(nsAirSync, "CollectionId")
				serverID := store.ChildText(nsAirSync, "ServerId")
				fileRef := store.ChildText(nsAirSyncBase, "FileReference")
				if fileRef == "" {
					fileRef = store.ChildText(nsAirSync, "FileReference")
				}
				switch {
				case fileRef != "":
					s.fetchAttachment(c, user, credential, collectionID, serverID, fileRef, out)
				case serverID != "":
					s.fetchMessage(c, user, credential, collectionID, serverID, pref, out)
				default:
					out.Add(nsItemOperations, "Status", "130")
				}
			}
		}
		if efc := root.Child(nsItemOperations, "EmptyFolderContents"); efc != nil {
			id := efc.ChildText(nsAirSync, "CollectionId")
			out := response.Add(nsItemOperations, "EmptyFolderContents", "")
			folder, ok := s.folderNameFor(user.Email, credential, id)
			if !ok {
				out.Add(nsItemOperations, "Status", "12")
			} else if err := s.Mail.ClearFolder(user.Email, credential, folder); err != nil {
				log.Printf("activesync: empty folder %s: %v", folder, err)
				out.Add(nsItemOperations, "Status", "130")
			} else {
				out.Add(nsItemOperations, "Status", "1")
			}
		}
	}
	if len(response.Children) == 0 {
		resp.Children = resp.Children[:0]
	}
	resp.Add(nsItemOperations, "Status", "1")
	return writeWBXML(c, resp, req.protocolVer)
}

// fetchMessage returns a full message (MIME or preferred body) to the client.
func (s *Service) fetchMessage(c *fiber.Ctx, user *models.User, credential, collectionID, serverID string, pref bodyPreference, out *Element) {
	folder, ok := s.folderNameFor(user.Email, credential, collectionID)
	uid := parseUID(serverID)
	if !ok || uid == 0 {
		out.Add(nsItemOperations, "Status", "130")
		return
	}
	props := out.Add(nsItemOperations, "Properties", "")
	body := props.Add(nsAirSyncBase, "Body", "")
	if !pref.includeBody || pref.mime {
		raw, err := s.Mail.GetRaw(user.Email, credential, folder, uid)
		if err != nil {
			out.Add(nsItemOperations, "Status", "130")
			return
		}
		body.Add(nsAirSyncBase, "Type", "4")
		body.Add(nsAirSyncBase, "Data", base64.StdEncoding.EncodeToString([]byte(raw)))
		body.Add(nsAirSyncBase, "EstimatedDataSize", strconv.Itoa(len(raw)))
	} else {
		msg, err := s.Mail.GetMessage(user.Email, credential, folder, uid)
		if err != nil {
			out.Add(nsItemOperations, "Status", "130")
			return
		}
		built := buildEmailBody(msg, pref)
		if built == nil {
			out.Add(nsItemOperations, "Status", "130")
			return
		}
		body.Children = append(body.Children, built.Children...)
	}
	out.Add(nsItemOperations, "Status", "1")
}

// fetchAttachment returns one attachment's bytes addressed by FileReference
// "<uid>:<index>".
func (s *Service) fetchAttachment(c *fiber.Ctx, user *models.User, credential, collectionID, serverID, fileRef string, out *Element) {
	uid := parseUID(serverID)
	idx := -1
	if i := strings.IndexByte(fileRef, ':'); i >= 0 {
		uid = parseUID(fileRef[:i])
		idx, _ = strconv.Atoi(fileRef[i+1:])
	}
	folder, ok := s.folderNameFor(user.Email, credential, collectionID)
	if !ok || uid == 0 || idx < 0 {
		out.Add(nsItemOperations, "Status", "130")
		return
	}
	msg, err := s.Mail.GetMessage(user.Email, credential, folder, uid)
	if err != nil || idx >= len(msg.Attachments) {
		out.Add(nsItemOperations, "Status", "130")
		return
	}
	att := msg.Attachments[idx]
	data, err := base64.StdEncoding.DecodeString(att.Data)
	if err != nil {
		out.Add(nsItemOperations, "Status", "130")
		return
	}
	props := out.Add(nsItemOperations, "Properties", "")
	body := props.Add(nsAirSyncBase, "Body", "")
	if att.ContentType != "" {
		body.Add(nsAirSyncBase, "Type", att.ContentType)
	} else {
		body.Add(nsAirSyncBase, "Type", "application/octet-stream")
	}
	body.Add(nsAirSyncBase, "Data", base64.StdEncoding.EncodeToString(data))
	body.Add(nsAirSyncBase, "EstimatedDataSize", strconv.Itoa(len(data)))
	out.Add(nsItemOperations, "Status", "1")
}

// cmdSearch runs mailbox and GAL searches.
func (s *Service) cmdSearch(c *fiber.Ctx, req *easRequest, user *models.User, credential string) error {
	root, err := readBody(c)
	if err != nil {
		return err
	}
	resp := &Element{NS: nsSearch, Name: "Search"}
	response := resp.Add(nsSearch, "Response", "")
	if root != nil {
		// Per [MS-ASCMD] the Store element is a direct child of Search.
		for _, storeEl := range root.ChildrenNamed(nsSearch, "Store") {
			name := storeEl.ChildText(nsSearch, "Name")
			query := storeEl.ChildText(nsSearch, "Query")
			storeOut := response.Add(nsSearch, "Store", "")
			switch strings.ToLower(name) {
			case "gal", "directory":
				s.searchGAL(c, user, query, storeOut)
			default:
				s.searchMailbox(c, user, credential, query, storeOut)
			}
		}
	}
	if len(response.Children) == 0 {
		storeOut := response.Add(nsSearch, "Store", "")
		storeOut.Add(nsSearch, "Status", "1")
		storeOut.Add(nsSearch, "Total", "0")
	}
	resp.Add(nsSearch, "Status", "1")
	return writeWBXML(c, resp, req.protocolVer)
}

func (s *Service) searchMailbox(c *fiber.Ctx, user *models.User, credential, query string, store *Element) {
	results, err := s.Mail.SearchAllMessages(user.Email, credential, query)
	if err != nil {
		store.Add(nsSearch, "Status", "130")
		return
	}
	if len(results) > 10 {
		results = results[:10]
	}
	for i := range results {
		result := store.Add(nsSearch, "Result", "")
		props := result.Add(nsSearch, "Properties", "")
		msg := &results[i]
		if len(msg.From) > 0 {
			props.Add(nsEmail, "From", addressString(msg.From[0]))
		}
		props.Add(nsEmail, "To", emailList(msg.To))
		props.Add(nsEmail, "Subject", msg.Subject)
		props.Add(nsEmail, "DateReceived", easTime(msg.Date))
		if hasFlag(msg.Flags, `\Seen`) {
			props.Add(nsEmail, "Read", "1")
		} else {
			props.Add(nsEmail, "Read", "0")
		}
		props.Add(nsEmail, "MessageClass", "IPM.Note")
		props.Add(nsEmail, "ThreadTopic", msg.Subject)
	}
	store.Add(nsSearch, "Total", strconv.Itoa(len(results)))
	store.Add(nsSearch, "Status", "1")
	if len(results) > 0 {
		store.Add(nsSearch, "Range", "0-"+strconv.Itoa(len(results)-1))
	} else {
		store.Add(nsSearch, "Range", "0-0")
	}
}

func (s *Service) searchGAL(c *fiber.Ctx, user *models.User, query string, store *Element) {
	q := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"
	var org []models.OrgContact
	total := 0
	if err := s.DB.WithContext(c.Context()).
		Where("LOWER(email) LIKE ? OR LOWER(name) LIKE ?", q, q).
		Limit(20).Find(&org).Error; err == nil {
		for _, oc := range org {
			result := store.Add(nsSearch, "Result", "")
			props := result.Add(nsSearch, "Properties", "")
			props.Add(nsGAL, "DisplayName", oc.Name)
			props.Add(nsGAL, "EmailAddress", oc.Email)
			if oc.Title != "" {
				props.Add(nsGAL, "Title", oc.Title)
			}
			if oc.Phone != "" {
				props.Add(nsGAL, "Phone", oc.Phone)
			}
			total++
		}
	}
	var users []models.User
	if err := s.DB.WithContext(c.Context()).
		Where("LOWER(email) LIKE ?", q).
		Limit(20).Find(&users).Error; err == nil {
		for _, u := range users {
			result := store.Add(nsSearch, "Result", "")
			props := result.Add(nsSearch, "Properties", "")
			props.Add(nsGAL, "DisplayName", u.DisplayedName)
			props.Add(nsGAL, "EmailAddress", u.Email)
			total++
		}
	}
	store.Add(nsSearch, "Status", "1")
	store.Add(nsSearch, "Total", strconv.Itoa(total))
	if total > 0 {
		store.Add(nsSearch, "Range", "0-"+strconv.Itoa(total-1))
	} else {
		store.Add(nsSearch, "Range", "0-0")
	}
}

// cmdResolveRecipients resolves To addresses against users, aliases,
// personal contacts and the organization directory.
func (s *Service) cmdResolveRecipients(c *fiber.Ctx, req *easRequest, user *models.User) error {
	root, err := readBody(c)
	if err != nil {
		return err
	}
	resp := &Element{NS: nsResolveRecip, Name: "ResolveRecipients"}
	if root != nil {
		for _, to := range root.ChildrenNamed(nsResolveRecip, "To") {
			address := strings.TrimSpace(to.Text)
			response := resp.Add(nsResolveRecip, "Response", "")
			response.Add(nsResolveRecip, "To", address)
			displayName, found := s.resolveAddress(c, user, address)
			if !found {
				response.Add(nsResolveRecip, "Status", "5")
				continue
			}
			response.Add(nsResolveRecip, "Status", "1")
			recipient := response.Add(nsResolveRecip, "Recipient", "")
			recipient.Add(nsResolveRecip, "Type", "1")
			recipient.Add(nsResolveRecip, "DisplayName", displayName)
			recipient.Add(nsResolveRecip, "EmailAddress", address)
		}
	}
	resp.Add(nsResolveRecip, "Status", "1")
	return writeWBXML(c, resp, req.protocolVer)
}

func (s *Service) resolveAddress(c *fiber.Ctx, user *models.User, address string) (string, bool) {
	addr := strings.ToLower(strings.TrimSpace(address))
	if strings.Contains(addr, "<") {
		addr = extractEmail(addr)
	}
	var u models.User
	if err := s.DB.WithContext(c.Context()).Where("email = ?", addr).First(&u).Error; err == nil {
		if u.DisplayedName != "" {
			return u.DisplayedName, true
		}
		return u.Email, true
	}
	var alias models.Alias
	if err := s.DB.WithContext(c.Context()).Where("email = ?", addr).First(&alias).Error; err == nil {
		return alias.Email, true
	}
	var oc models.OrgContact
	if err := s.DB.WithContext(c.Context()).Where("email = ?", addr).First(&oc).Error; err == nil {
		return oc.Name, true
	}
	var contact models.Contact
	if err := s.DB.WithContext(c.Context()).
		Where("user_email = ? AND email = ?", user.Email, addr).First(&contact).Error; err == nil {
		return contact.Name, true
	}
	return "", false
}

// cmdMeetingResponse answers a meeting invitation from the mobile client
// (accept / decline / tentative).
func (s *Service) cmdMeetingResponse(c *fiber.Ctx, req *easRequest, user *models.User, credential string) error {
	root, err := readBody(c)
	if err != nil {
		return err
	}
	resp := &Element{NS: nsMeetingResponse, Name: "MeetingResponse"}
	if root != nil {
		for _, request := range root.ChildrenNamed(nsMeetingResponse, "Request") {
			requestID := request.ChildText(nsMeetingResponse, "RequestId")
			collectionID := request.ChildText(nsMeetingResponse, "CollectionId")
			userResponse := request.ChildText(nsMeetingResponse, "UserResponse")
			result := resp.Add(nsMeetingResponse, "Result", "")
			result.Add(nsMeetingResponse, "RequestId", requestID)
			kind := map[string]invite.RespondKind{
				"1": invite.Accept, "2": invite.Decline, "3": invite.Tentative,
			}[userResponse]
			if kind == "" {
				result.Add(nsMeetingResponse, "Status", "104")
				continue
			}
			uid := parseUID(requestID)
			folder, ok := s.folderNameFor(user.Email, credential, collectionID)
			if !ok || uid == 0 {
				result.Add(nsMeetingResponse, "Status", "130")
				continue
			}
			msg, err := s.Mail.GetMessage(user.Email, credential, folder, uid)
			if err != nil || msg.Invitation == nil || msg.Invitation.Organizer == "" {
				result.Add(nsMeetingResponse, "Status", "130")
				continue
			}
			inv := msg.Invitation
			replyICS, err := invite.BuildReplyICS(inv, user.Email, kind)
			if err != nil {
				result.Add(nsMeetingResponse, "Status", "130")
				continue
			}
			subject := inv.Summary
			if subject == "" {
				subject = "会议邀请"
			}
			prefix := map[invite.RespondKind]string{invite.Accept: "接受：", invite.Decline: "拒绝：", invite.Tentative: "暂定："}[kind]
			text := "您已回复会议「" + subject + "」。\n"
			att := mail.Attachment{Filename: "invite.ics", ContentType: "text/calendar", Size: len(replyICS), Data: base64.StdEncoding.EncodeToString([]byte(replyICS))}
			if err := s.Mail.Send(user.Email, credential, user.Email, []string{inv.Organizer}, nil, nil,
				prefix+subject, text, "", []mail.Attachment{att}); err != nil {
				log.Printf("activesync: meeting reply to %s: %v", inv.Organizer, err)
				result.Add(nsMeetingResponse, "Status", "130")
				continue
			}
			if kind == invite.Decline {
				s.DB.WithContext(c.Context()).Where("user_email = ? AND uid = ?", user.Email, inv.UID).
					Delete(&models.CalendarEvent{})
			} else {
				s.upsertEvent(user.Email, inv)
			}
			result.Add(nsMeetingResponse, "Status", "1")
		}
	}
	resp.Add(nsMeetingResponse, "Status", "1")
	return writeWBXML(c, resp, req.protocolVer)
}

func (s *Service) upsertEvent(userEmail string, inv *mail.Invitation) {
	var ev models.CalendarEvent
	err := s.DB.Where("user_email = ? AND uid = ?", userEmail, inv.UID).First(&ev).Error
	ev.UserEmail = userEmail
	ev.UID = inv.UID
	ev.Summary = inv.Summary
	ev.Location = inv.Location
	ev.Description = inv.Description
	ev.AllDay = inv.AllDay
	if inv.Start != "" {
		if t, err := time.Parse(time.RFC3339, inv.Start); err == nil {
			ev.Start = &t
		}
	}
	if inv.End != "" {
		if t, err := time.Parse(time.RFC3339, inv.End); err == nil {
			ev.End = &t
		}
	}
	ev.ICS = inv.ICS
	if err == nil {
		_ = s.DB.Save(&ev).Error
		return
	}
	_ = s.DB.Create(&ev).Error
}
