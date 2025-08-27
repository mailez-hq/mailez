package activesync

// EAS calendar and contacts collection sync over the built-in CalDAV/
// CardDAV stores (CalendarEvent / Contact rows).
import (
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/caldav"
	"mailez/backend/internal/core/models"
)

// syncCalendar handles a Sync request for the "calendar" collection.
func (s *Service) syncCalendar(c *fiber.Ctx, req *easRequest, user *models.User, col syncCollection) *Element {
	out := &Element{NS: nsAirSync, Name: "Collection"}
	out.Add(nsAirSync, "Class", "Calendar")
	out.Add(nsAirSync, "CollectionId", col.id)
	if col.syncKey == "" || col.syncKey == "0" {
		key, err := s.saveSyncState(c.Context(), user.Email, req.deviceID, col.id, "Calendar", emptySnapshot(0))
		if err != nil {
			out.Add(nsAirSync, "Status", "130")
			return out
		}
		out.Add(nsAirSync, "SyncKey", key)
		out.Add(nsAirSync, "Status", "1")
		return out
	}
	snap, storedKey, _, err := s.loadSyncState(c.Context(), user.Email, req.deviceID, col.id)
	if err != nil {
		out.Add(nsAirSync, "Status", "130")
		return out
	}
	if storedKey != "" && storedKey != col.syncKey {
		out.Add(nsAirSync, "SyncKey", col.syncKey)
		out.Add(nsAirSync, "Status", "103")
		return out
	}
	// Client commands first so the diff reflects the post-command state.
	if err := s.applyCalendarCommands(c, user, col); err != nil {
		out.Add(nsAirSync, "Status", "130")
		return out
	}
	var events []models.CalendarEvent
	if err := s.DB.Where("user_email = ?", user.Email).Find(&events).Error; err != nil {
		out.Add(nsAirSync, "Status", "130")
		return out
	}
	current := map[string]models.CalendarEvent{}
	for _, ev := range events {
		current[eventServerID(ev.ID)] = ev
	}
	var adds, changes []models.CalendarEvent
	var deletes []string
	for id, ev := range current {
		item, seen := snap.Items[id]
		updated := ev.UpdatedAt.Unix()
		if !seen {
			adds = append(adds, ev)
			continue
		}
		if updated > item.UpdatedAt {
			changes = append(changes, ev)
		}
	}
	for id := range snap.Items {
		if _, still := current[id]; !still {
			deletes = append(deletes, id)
		}
	}
	sort.Slice(adds, func(i, j int) bool { return adds[i].ID < adds[j].ID })
	sort.Slice(changes, func(i, j int) bool { return changes[i].ID < changes[j].ID })
	sort.Strings(deletes)

	window := col.windowSize
	more := false
	total := len(adds) + len(changes) + len(deletes)
	if total > window {
		more = true
	}
	if col.getChanges {
		var commandEls []*Element
		remaining := window
		for i, ev := range adds {
			if i >= remaining {
				more = true
				break
			}
			el := &Element{NS: nsAirSync, Name: "Add"}
			el.Add(nsAirSync, "ServerId", eventServerID(ev.ID))
			el.Children = append(el.Children, eventToEASAppData(&ev))
			commandEls = append(commandEls, el)
			remaining--
		}
		used := window - remaining
		remaining = window - used
		for i, ev := range changes {
			if i >= remaining {
				more = true
				break
			}
			el := &Element{NS: nsAirSync, Name: "Change"}
			el.Add(nsAirSync, "ServerId", eventServerID(ev.ID))
			el.Children = append(el.Children, eventToEASAppData(&ev))
			commandEls = append(commandEls, el)
			remaining--
		}
		used = window - remaining
		remaining = window - used
		for i, id := range deletes {
			if i >= remaining {
				more = true
				break
			}
			el := &Element{NS: nsAirSync, Name: "Delete"}
			el.Add(nsAirSync, "ServerId", id)
			commandEls = append(commandEls, el)
			remaining--
		}
		if len(commandEls) > 0 {
			commands := out.Add(nsAirSync, "Commands", "")
			commands.Children = append(commands.Children, commandEls...)
		}
		if more {
			out.Add(nsAirSync, "MoreAvailable", "")
			out.Add(nsAirSync, "SyncKey", col.syncKey)
		} else {
			next := syncSnapshot{Items: map[string]snapshotItem{}}
			for id, ev := range current {
				next.Items[id] = snapshotItem{UpdatedAt: ev.UpdatedAt.Unix()}
			}
			key, kerr := s.saveSyncState(c.Context(), user.Email, req.deviceID, col.id, "Calendar", next)
			if kerr != nil {
				log.Printf("activesync: save calendar state: %v", kerr)
			}
			out.Add(nsAirSync, "SyncKey", key)
		}
	} else {
		out.Add(nsAirSync, "SyncKey", col.syncKey)
	}
	out.Add(nsAirSync, "Status", "1")
	return out
}

// syncContacts handles a Sync request for the "contacts" collection.
func (s *Service) syncContacts(c *fiber.Ctx, req *easRequest, user *models.User, col syncCollection) *Element {
	out := &Element{NS: nsAirSync, Name: "Collection"}
	out.Add(nsAirSync, "Class", "Contacts")
	out.Add(nsAirSync, "CollectionId", col.id)
	if col.syncKey == "" || col.syncKey == "0" {
		key, err := s.saveSyncState(c.Context(), user.Email, req.deviceID, col.id, "Contacts", emptySnapshot(0))
		if err != nil {
			out.Add(nsAirSync, "Status", "130")
			return out
		}
		out.Add(nsAirSync, "SyncKey", key)
		out.Add(nsAirSync, "Status", "1")
		return out
	}
	snap, storedKey, _, err := s.loadSyncState(c.Context(), user.Email, req.deviceID, col.id)
	if err != nil {
		out.Add(nsAirSync, "Status", "130")
		return out
	}
	if storedKey != "" && storedKey != col.syncKey {
		out.Add(nsAirSync, "SyncKey", col.syncKey)
		out.Add(nsAirSync, "Status", "103")
		return out
	}
	if err := s.applyContactsCommands(c, user, col); err != nil {
		out.Add(nsAirSync, "Status", "130")
		return out
	}
	var contacts []models.Contact
	if err := s.DB.Where("user_email = ?", user.Email).Find(&contacts).Error; err != nil {
		out.Add(nsAirSync, "Status", "130")
		return out
	}
	current := map[string]models.Contact{}
	for _, ct := range contacts {
		current[contactServerID(ct.ID)] = ct
	}
	var adds, changes []models.Contact
	var deletes []string
	for id, ct := range current {
		item, seen := snap.Items[id]
		updated := ct.UpdatedAt.Unix()
		if !seen {
			adds = append(adds, ct)
			continue
		}
		if updated > item.UpdatedAt {
			changes = append(changes, ct)
		}
	}
	for id := range snap.Items {
		if _, still := current[id]; !still {
			deletes = append(deletes, id)
		}
	}
	sort.Slice(adds, func(i, j int) bool { return adds[i].ID < adds[j].ID })
	sort.Slice(changes, func(i, j int) bool { return changes[i].ID < changes[j].ID })
	sort.Strings(deletes)

	window := col.windowSize
	more := false
	total := len(adds) + len(changes) + len(deletes)
	if total > window {
		more = true
	}
	if col.getChanges {
		var commandEls []*Element
		remaining := window
		for i, ct := range adds {
			if i >= remaining {
				more = true
				break
			}
			el := &Element{NS: nsAirSync, Name: "Add"}
			el.Add(nsAirSync, "ServerId", contactServerID(ct.ID))
			el.Children = append(el.Children, contactToEASAppData(&ct))
			commandEls = append(commandEls, el)
			remaining--
		}
		used := window - remaining
		remaining = window - used
		for i, ct := range changes {
			if i >= remaining {
				more = true
				break
			}
			el := &Element{NS: nsAirSync, Name: "Change"}
			el.Add(nsAirSync, "ServerId", contactServerID(ct.ID))
			el.Children = append(el.Children, contactToEASAppData(&ct))
			commandEls = append(commandEls, el)
			remaining--
		}
		used = window - remaining
		remaining = window - used
		for i, id := range deletes {
			if i >= remaining {
				more = true
				break
			}
			el := &Element{NS: nsAirSync, Name: "Delete"}
			el.Add(nsAirSync, "ServerId", id)
			commandEls = append(commandEls, el)
			remaining--
		}
		if len(commandEls) > 0 {
			commands := out.Add(nsAirSync, "Commands", "")
			commands.Children = append(commands.Children, commandEls...)
		}
		if more {
			out.Add(nsAirSync, "MoreAvailable", "")
			out.Add(nsAirSync, "SyncKey", col.syncKey)
		} else {
			next := syncSnapshot{Items: map[string]snapshotItem{}}
			for id, ct := range current {
				next.Items[id] = snapshotItem{UpdatedAt: ct.UpdatedAt.Unix()}
			}
			key, kerr := s.saveSyncState(c.Context(), user.Email, req.deviceID, col.id, "Contacts", next)
			if kerr != nil {
				log.Printf("activesync: save contacts state: %v", kerr)
			}
			out.Add(nsAirSync, "SyncKey", key)
		}
	} else {
		out.Add(nsAirSync, "SyncKey", col.syncKey)
	}
	out.Add(nsAirSync, "Status", "1")
	return out
}

// applyCalendarCommands applies client Add/Change/Delete on the calendar.
func (s *Service) applyCalendarCommands(c *fiber.Ctx, user *models.User, col syncCollection) error {
	for _, cmd := range col.commands {
		switch cmd.kind {
		case "Delete":
			ev, err := s.eventByServerID(c, user, cmd.serverID)
			if err != nil {
				continue
			}
			_ = s.DB.WithContext(c.Context()).Delete(&ev).Error
		case "Add", "Change":
			app := cmd.application
			if app == nil {
				continue
			}
			subject := app.ChildText(nsCalendar, "Subject")
			startText := app.ChildText(nsCalendar, "StartTime")
			if subject == "" || startText == "" {
				continue
			}
			start, ok := parseEASDate(startText)
			if !ok {
				continue
			}
			ev := models.CalendarEvent{UserEmail: user.Email}
			if cmd.kind == "Change" {
				if existing, err := s.eventByServerID(c, user, cmd.serverID); err == nil {
					ev = existing
				}
			}
			if ev.UID == "" {
				ev.UID = "eas-" + newSyncKey()
			}
			ev.Summary = subject
			ev.Location = app.ChildText(nsCalendar, "Location")
			ev.Description = app.ChildText(nsCalendar, "Description")
			ev.AllDay = app.ChildText(nsCalendar, "AllDayEvent") == "1"
			ev.Start = &start
			if endText := app.ChildText(nsCalendar, "EndTime"); endText != "" {
				if end, ok := parseEASDate(endText); ok {
					ev.End = &end
				}
			}
			if rem := app.ChildText(nsCalendar, "Reminder"); rem != "" {
				if n, err := strconv.Atoi(rem); err == nil {
					ev.ReminderMinutes = n
				}
			}
			d := &caldav.EventData{
				UID: ev.UID, Summary: ev.Summary, Location: ev.Location,
				Description: ev.Description, AllDay: ev.AllDay, Start: ev.Start, End: ev.End,
				RRule: ev.RRule,
			}
			icsText, err := caldav.BuildICS(d)
			if err != nil {
				continue
			}
			ev.ICS = icsText
			if ev.ID == 0 {
				if err := s.DB.WithContext(c.Context()).Create(&ev).Error; err != nil {
					return err
				}
			} else {
				if err := s.DB.WithContext(c.Context()).Save(&ev).Error; err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// applyContactsCommands applies client Add/Change/Delete on the address book.
func (s *Service) applyContactsCommands(c *fiber.Ctx, user *models.User, col syncCollection) error {
	for _, cmd := range col.commands {
		switch cmd.kind {
		case "Delete":
			var ct models.Contact
			if err := s.DB.WithContext(c.Context()).
				First(&ct, "id = ? AND user_email = ?", contactID(cmd.serverID), user.Email).Error; err == nil {
				_ = s.DB.WithContext(c.Context()).Delete(&ct).Error
			}
		case "Add", "Change":
			app := cmd.application
			if app == nil {
				continue
			}
			name := app.ChildText(nsContacts, "FileAs")
			if name == "" {
				first := app.ChildText(nsContacts, "FirstName")
				last := app.ChildText(nsContacts, "LastName")
				name = strings.TrimSpace(first + " " + last)
			}
			email := app.ChildText(nsContacts, "Email1Address")
			if name == "" && email == "" {
				continue
			}
			ct := models.Contact{UserEmail: user.Email}
			if cmd.kind == "Change" {
				if err := s.DB.WithContext(c.Context()).
					First(&ct, "id = ? AND user_email = ?", contactID(cmd.serverID), user.Email).Error; err != nil {
					continue
				}
			}
			ct.Name = name
			ct.Email = email
			if ct.DavUID == "" {
				ct.DavUID = "eas-" + newSyncKey()
			}
			if ct.ID == 0 {
				if err := s.DB.WithContext(c.Context()).Create(&ct).Error; err != nil {
					return err
				}
			} else {
				if err := s.DB.WithContext(c.Context()).Save(&ct).Error; err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// eventToEASAppData renders a CalendarEvent as EAS ApplicationData.
func eventToEASAppData(ev *models.CalendarEvent) *Element {
	app := &Element{NS: nsAirSync, Name: "ApplicationData"}
	app.Add(nsCalendar, "Subject", ev.Summary)
	if ev.Start != nil {
		app.Add(nsCalendar, "StartTime", easTime(*ev.Start))
	}
	if ev.End != nil {
		app.Add(nsCalendar, "EndTime", easTime(*ev.End))
	}
	app.Add(nsCalendar, "DtStamp", easTime(ev.UpdatedAt))
	app.Add(nsCalendar, "UID", ev.UID)
	app.Add(nsCalendar, "Location", ev.Location)
	if ev.AllDay {
		app.Add(nsCalendar, "AllDayEvent", "1")
	}
	if ev.ReminderMinutes > 0 {
		app.Add(nsCalendar, "Reminder", strconv.Itoa(ev.ReminderMinutes))
	}
	return app
}

// contactToEASAppData renders a Contact as EAS ApplicationData.
func contactToEASAppData(ct *models.Contact) *Element {
	app := &Element{NS: nsAirSync, Name: "ApplicationData"}
	app.Add(nsContacts, "FileAs", ct.Name)
	first, last := splitContactName(ct.Name)
	if first != "" {
		app.Add(nsContacts, "FirstName", first)
	}
	if last != "" {
		app.Add(nsContacts, "LastName", last)
	}
	app.Add(nsContacts, "Email1Address", ct.Email)
	return app
}

func splitContactName(name string) (first, last string) {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return "", ""
	}
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], strings.Join(parts[1:], " ")
}

func eventServerID(id uint) string {
	return "ev" + strconv.FormatUint(uint64(id), 10)
}

func eventID(serverID string) uint {
	n, _ := strconv.ParseUint(strings.TrimPrefix(serverID, "ev"), 10, 64)
	return uint(n)
}

func (s *Service) eventByServerID(c *fiber.Ctx, user *models.User, serverID string) (models.CalendarEvent, error) {
	var ev models.CalendarEvent
	err := s.DB.WithContext(c.Context()).
		First(&ev, "id = ? AND user_email = ?", eventID(serverID), user.Email).Error
	return ev, err
}

func contactServerID(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}

func contactID(serverID string) uint {
	n, _ := strconv.ParseUint(serverID, 10, 64)
	return uint(n)
}
