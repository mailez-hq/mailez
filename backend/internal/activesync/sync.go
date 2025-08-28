package activesync

// Sync, GetItemEstimate and Ping command handlers.
import (
	"log"
	"sort"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core/models"
	"mailez/backend/internal/mail"
)

// syncCollection is one parsed <Collection> of a Sync request.
type syncCollection struct {
	id          string
	class       string
	syncKey     string
	getChanges  bool
	windowSize  int
	filterType  int
	deletesMove bool
	pref        bodyPreference
	commands    []syncCommand
}

type syncCommand struct {
	kind        string // Add / Change / Delete
	clientID    string
	serverID    string
	application *Element
}

func parseSyncCollections(root *Element) []syncCollection {
	if root == nil {
		return nil
	}
	cols := root.Child(nsAirSync, "Collections")
	if cols == nil {
		return nil
	}
	var out []syncCollection
	for _, colEl := range cols.ChildrenNamed(nsAirSync, "Collection") {
		sc := syncCollection{
			id:          colEl.ChildText(nsAirSync, "CollectionId"),
			class:       colEl.ChildText(nsAirSync, "Class"),
			syncKey:     colEl.ChildText(nsAirSync, "SyncKey"),
			getChanges:  colEl.ChildText(nsAirSync, "GetChanges") == "1",
			windowSize:  atoiDefault(colEl.ChildText(nsAirSync, "WindowSize"), 100),
			filterType:  atoiDefault(colEl.ChildText(nsAirSync, "FilterType"), 0),
			deletesMove: colEl.ChildText(nsAirSync, "DeletesAsMoves") == "1",
			pref:        parseBodyPreference(colEl),
		}
		if sc.windowSize <= 0 || sc.windowSize > 512 {
			sc.windowSize = 100
		}
		if cmds := colEl.Child(nsAirSync, "Commands"); cmds != nil {
			for _, kind := range []string{"Add", "Change", "Delete"} {
				for _, el := range cmds.ChildrenNamed(nsAirSync, kind) {
					sc.commands = append(sc.commands, syncCommand{
						kind:        kind,
						clientID:    el.ChildText(nsAirSync, "ClientId"),
						serverID:    el.ChildText(nsAirSync, "ServerId"),
						application: el.Child(nsAirSync, "ApplicationData"),
					})
				}
			}
		}
		out = append(out, sc)
	}
	return out
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// cmdSync processes one Sync request with one or more collections.
func (s *Service) cmdSync(c *fiber.Ctx, req *easRequest, user *models.User, credential string) error {
	root, err := readBody(c)
	if err != nil {
		return err
	}
	collections := parseSyncCollections(root)
	if len(collections) == 0 {
		resp := &Element{NS: nsAirSync, Name: "Sync"}
		resp.Add(nsAirSync, "Status", "1")
		return writeWBXML(c, resp, req.protocolVer)
	}
	resp := &Element{NS: nsAirSync, Name: "Sync"}
	all := resp.Add(nsAirSync, "Collections", "")
	for _, col := range collections {
		out := s.syncOne(c, req, user, credential, col)
		all.Children = append(all.Children, out)
	}
	return writeWBXML(c, resp, req.protocolVer)
}

// syncOne handles one collection and returns its response <Collection>.
func (s *Service) syncOne(c *fiber.Ctx, req *easRequest, user *models.User, credential string, col syncCollection) *Element {
	// PIM collections (calendar/contacts) sync over the built-in stores.
	if col.id == "calendar" {
		return s.syncCalendar(c, req, user, col)
	}
	if col.id == "contacts" {
		return s.syncContacts(c, req, user, col)
	}
	out := &Element{NS: nsAirSync, Name: "Collection"}
	if col.class != "" {
		out.Add(nsAirSync, "Class", col.class)
	}
	out.Add(nsAirSync, "CollectionId", col.id)

	folder, ok := s.folderNameFor(user.Email, credential, col.id)
	if !ok {
		out.Add(nsAirSync, "Status", "12")
		return out
	}
	// Client-side commands (flag changes, deletes) are applied first so the
	// snapshot reflects them.
	if err := s.applyClientCommands(c, user, credential, folder, col); err != nil {
		log.Printf("activesync: client commands on %s: %v", col.id, err)
		out.Add(nsAirSync, "Status", "130")
		return out
	}
	if col.syncKey == "" || col.syncKey == "0" {
		// Initial sync: return the empty window so the client re-syncs with
		// the returned key and receives the full folder.
		key, err := s.saveSyncState(c.Context(), user.Email, req.deviceID, col.id, folder, emptySnapshot(0))
		if err != nil {
			log.Printf("activesync: init state: %v", err)
			out.Add(nsAirSync, "Status", "130")
			return out
		}
		out.Add(nsAirSync, "SyncKey", key)
		out.Add(nsAirSync, "Status", "1")
		return out
	}

	snap, storedKey, _, err := s.loadSyncState(c.Context(), user.Email, req.deviceID, col.id)
	if err != nil {
		log.Printf("activesync: load state: %v", err)
		out.Add(nsAirSync, "Status", "130")
		return out
	}
	if storedKey != "" && storedKey != col.syncKey {
		out.Add(nsAirSync, "SyncKey", col.syncKey)
		out.Add(nsAirSync, "Status", "103")
		return out
	}

	messages, err := s.Mail.ListAllMessages(user.Email, credential, folder)
	if err != nil {
		log.Printf("activesync: list %s: %v", folder, err)
		out.Add(nsAirSync, "Status", "130")
		return out
	}
	uidValidity := snap.UIDValidity
	stat, serr := s.Mail.FolderStat(user.Email, credential, folder)
	if serr == nil && stat.UidNext > 0 && snap.UIDValidity == 0 {
		uidValidity = stat.UidNext
	}

	current := map[uint32]mail.Message{}
	allCurrentUIDs := map[uint32]bool{}
	for _, m := range messages {
		current[m.UID] = m
		allCurrentUIDs[m.UID] = true
	}
	cutoff := filterCutoff(col.filterType)
	var (
		adds    []mail.Message
		changes []mail.Message
		deletes []string
	)
	for uid, m := range current {
		if cutoff != nil && m.Date.Before(*cutoff) {
			continue
		}
		key := strconv.FormatUint(uint64(uid), 10)
		item, seen := snap.Items[key]
		if !seen {
			adds = append(adds, m)
			continue
		}
		now := snapshotItem{
			Read:     hasFlag(m.Flags, `\Seen`),
			Flagged:  hasFlag(m.Flags, `\Flagged`),
			Answered: hasFlag(m.Flags, `\Answered`),
		}
		if now != item {
			changes = append(changes, m)
		}
	}
	for uidKey := range snap.Items {
		uid := parseUID(uidKey)
		if !allCurrentUIDs[uid] {
			deletes = append(deletes, uidKey)
		}
	}
	sort.Slice(adds, func(i, j int) bool { return adds[i].UID < adds[j].UID })
	sort.Slice(changes, func(i, j int) bool { return changes[i].UID < changes[j].UID })
	sort.Strings(deletes)

	window := col.windowSize
	more := false
	total := len(adds) + len(changes) + len(deletes)
	log.Printf("activesync: sync %s window=%d adds=%d changes=%d deletes=%d total=%d", col.id, window, len(adds), len(changes), len(deletes), total)
	if total > window {
		more = true
	}
	if col.getChanges {
		remaining := window
		var commandEls []*Element
		for _, m := range adds {
			if remaining <= 0 {
				more = true
				break
			}
			el := &Element{NS: nsAirSync, Name: "Add"}
			el.Add(nsAirSync, "ServerId", strconv.FormatUint(uint64(m.UID), 10))
			app := buildEmailAppData(&m, col.pref)
			el.Children = append(el.Children, app)
			commandEls = append(commandEls, el)
			remaining--
		}
		usedAdds := window - remaining
		remaining = window - usedAdds
		for _, m := range changes {
			if remaining <= 0 {
				more = true
				break
			}
			el := &Element{NS: nsAirSync, Name: "Change"}
			el.Add(nsAirSync, "ServerId", strconv.FormatUint(uint64(m.UID), 10))
			app := &Element{NS: nsAirSync, Name: "ApplicationData"}
			if hasFlag(m.Flags, `\Seen`) {
				app.Add(nsEmail, "Read", "1")
			} else {
				app.Add(nsEmail, "Read", "0")
			}
			if hasFlag(m.Flags, `\Flagged`) {
				f := app.Add(nsEmail, "Flag", "")
				f.Add(nsEmail, "Status", "2")
			} else {
				app.Add(nsEmail, "Flag", "0")
			}
			el.Children = append(el.Children, app)
			commandEls = append(commandEls, el)
			remaining--
		}
		used := window - remaining
		remaining = window - used
		for _, uidKey := range deletes {
			if remaining <= 0 {
				more = true
				break
			}
			el := &Element{NS: nsAirSync, Name: "Delete"}
			el.Add(nsAirSync, "ServerId", uidKey)
			commandEls = append(commandEls, el)
			remaining--
		}
		if len(commandEls) > 0 {
			commands := out.Add(nsAirSync, "Commands", "")
			commands.Children = append(commands.Children, commandEls...)
		}
		log.Printf("activesync: sync %s emitted=%d more=%v", col.id, len(commandEls), more)
		if more {
			log.Printf("activesync: sync %s PAGING window=%d (partial snapshot)", col.id, window)
			out.Add(nsAirSync, "MoreAvailable", "")
			// Persist the window already handed out so the client's follow-up
			// Sync with the new key advances to the next batch instead of
			// repeating the same window forever (Exchange semantics: every
			// Sync response returns a fresh key and the server tracks what
			// was already sent).
			partial := syncSnapshot{UIDValidity: uidValidity, Items: map[string]snapshotItem{}}
			for k, v := range snap.Items {
				partial.Items[k] = v
			}
			emitted := 0
			for _, m := range adds {
				if emitted >= window {
					break
				}
				partial.Items[strconv.FormatUint(uint64(m.UID), 10)] = snapshotItem{
					Read:     hasFlag(m.Flags, `\Seen`),
					Flagged:  hasFlag(m.Flags, `\Flagged`),
					Answered: hasFlag(m.Flags, `\Answered`),
				}
				emitted++
			}
			key, kerr := s.saveSyncState(c.Context(), user.Email, req.deviceID, col.id, folder, partial)
			if kerr != nil {
				log.Printf("activesync: save partial state: %v", kerr)
			}
			out.Add(nsAirSync, "SyncKey", key)
		} else {
			next := syncSnapshot{UIDValidity: uidValidity, Items: map[string]snapshotItem{}}
			for uid, m := range current {
				if cutoff != nil && m.Date.Before(*cutoff) {
					continue
				}
				next.Items[strconv.FormatUint(uint64(uid), 10)] = snapshotItem{
					Read:     hasFlag(m.Flags, `\Seen`),
					Flagged:  hasFlag(m.Flags, `\Flagged`),
					Answered: hasFlag(m.Flags, `\Answered`),
				}
			}
			key, kerr := s.saveSyncState(c.Context(), user.Email, req.deviceID, col.id, folder, next)
			if kerr != nil {
				log.Printf("activesync: save state: %v", kerr)
			}
			// Refresh the Ping baseline so the just-synced state is quiet.
			if stat2, serr := s.Mail.FolderStat(user.Email, credential, folder); serr == nil {
				_ = s.savePingState(c.Context(), user.Email, req.deviceID, col.id,
					pingState{UidNext: stat2.UidNext, Messages: stat2.Messages, Unseen: stat2.Unseen})
			}
			out.Add(nsAirSync, "SyncKey", key)
		}
	} else {
		out.Add(nsAirSync, "SyncKey", col.syncKey)
	}
	out.Add(nsAirSync, "Status", "1")
	return out
}

func parseUID(s string) uint32 {
	n, _ := strconv.ParseUint(s, 10, 32)
	return uint32(n)
}

// filterCutoff maps the EAS FilterType to a date cutoff.
func filterCutoff(filterType int) *time.Time {
	var days int
	switch filterType {
	case 1:
		days = 1
	case 2:
		days = 3
	case 3:
		days = 7
	case 4:
		days = 14
	case 5:
		days = 31
	case 6:
		days = 92
	case 7:
		days = 183
	case 8:
		days = 365
	default:
		return nil
	}
	t := time.Now().AddDate(0, 0, -days)
	return &t
}

// applyClientCommands handles Add/Change/Delete commands the client sent.
func (s *Service) applyClientCommands(c *fiber.Ctx, user *models.User, credential, folder string, col syncCollection) error {
	for _, cmd := range col.commands {
		switch cmd.kind {
		case "Change":
			uid := parseUID(cmd.serverID)
			if uid == 0 {
				continue
			}
			app := cmd.application
			if app == nil {
				continue
			}
			if r := app.ChildText(nsEmail, "Read"); r != "" {
				if err := s.Mail.SetFlag(user.Email, credential, folder, uid, `\Seen`, r == "1"); err != nil {
					return err
				}
			}
			if app.Child(nsEmail, "Flag") != nil {
				flagStatus := app.Child(nsEmail, "Flag").ChildText(nsEmail, "Status")
				if err := s.Mail.SetFlag(user.Email, credential, folder, uid, `\Flagged`, flagStatus != "0"); err != nil {
					return err
				}
			}
		case "Delete":
			uid := parseUID(cmd.serverID)
			if uid == 0 {
				continue
			}
			if err := s.Mail.Delete(user.Email, credential, folder, uid); err != nil {
				return err
			}
		case "Add":
			// Clients rarely Add email items; drafts created off-device are
			// synced from the server anyway. Report success without creating
			// anything so the client's own copy stays consistent.
		}
	}
	return nil
}

func folderForIDChecked(id string) string {
	if name, ok := folderForID(id); ok {
		return name
	}
	return id
}

// cmdGetItemEstimate returns how many items a collection would sync.
func (s *Service) cmdGetItemEstimate(c *fiber.Ctx, req *easRequest, user *models.User, credential string) error {
	root, err := readBody(c)
	if err != nil {
		return err
	}
	resp := &Element{NS: nsItemEstimate, Name: "GetItemEstimate"}
	if root != nil {
		if cols := root.Child(nsAirSync, "Collections"); cols != nil {
			response := resp.Add(nsItemEstimate, "Response", "")
			response.Add(nsItemEstimate, "Status", "1")
			for _, colEl := range cols.ChildrenNamed(nsAirSync, "Collection") {
				id := colEl.ChildText(nsAirSync, "CollectionId")
				col := &Element{NS: nsAirSync, Name: "Collection"}
				col.Add(nsAirSync, "Class", "Email")
				col.Add(nsAirSync, "CollectionId", id)
				count := 0
				if folder, ok := s.folderNameFor(user.Email, credential, id); ok {
					if stat, err := s.Mail.FolderStat(user.Email, credential, folder); err == nil {
						count = int(stat.Messages)
					}
				}
				col.Add(nsItemEstimate, "Estimate", strconv.Itoa(count))
				response.Children = append(response.Children, col)
			}
		}
	}
	return writeWBXML(c, resp, req.protocolVer)
}

// cmdPing answers the long-poll change notification. We compare the current
// SELECT counters with the last values the device observed; a difference
// means the client should re-sync those folders.
func (s *Service) cmdPing(c *fiber.Ctx, req *easRequest, user *models.User, credential string) error {
	root, err := readBody(c)
	if err != nil {
		return err
	}
	resp := &Element{NS: nsPing, Name: "Ping"}
	if root == nil {
		resp.Add(nsPing, "Status", "1")
		return writeWBXML(c, resp, req.protocolVer)
	}
	heartbeat := atoiDefault(root.ChildText(nsPing, "HeartbeatInterval"), 60)
	if heartbeat <= 0 || heartbeat > 3540 {
		heartbeat = 60
	}
	folders := root.Child(nsPing, "Folders")
	if folders == nil {
		resp.Add(nsPing, "Status", "1")
		return writeWBXML(c, resp, req.protocolVer)
	}
	var changed []*Element
	for _, folderEl := range folders.ChildrenNamed(nsPing, "Folder") {
		id := folderEl.ChildText(nsPing, "Id")
		if id == "" {
			continue
		}
		prev, _ := s.loadPingState(c.Context(), user.Email, req.deviceID, id)
		changedEl := &Element{NS: nsPing, Name: "Folder"}
		changedEl.Add(nsPing, "Id", id)
		changedEl.Add(nsPing, "Class", "Email")
		folder, ok := s.folderNameFor(user.Email, credential, id)
		if !ok {
			changed = append(changed, changedEl)
			continue
		}
		stat, err := s.Mail.FolderStat(user.Email, credential, folder)
		if err != nil {
			continue
		}
		cur := pingState{UidNext: stat.UidNext, Messages: stat.Messages, Unseen: stat.Unseen}
		if prev.Messages != 0 && (prev.Messages != cur.Messages || prev.Unseen != cur.Unseen || prev.UidNext != cur.UidNext) {
			// Report the change once, then baseline the new counters so
			// subsequent pings stay quiet until the next mutation.
			_ = s.savePingState(c.Context(), user.Email, req.deviceID, id, cur)
			changed = append(changed, changedEl)
			continue
		}
		if prev.Messages == 0 {
			_ = s.savePingState(c.Context(), user.Email, req.deviceID, id, cur)
		}
	}
	if len(changed) > 0 {
		resp.Add(nsPing, "Status", "2")
		foldersOut := resp.Add(nsPing, "Folders", "")
		foldersOut.Children = append(foldersOut.Children, changed...)
		return writeWBXML(c, resp, req.protocolVer)
	}
	// Nothing changed: hold the request for the heartbeat window (long-poll).
	if heartbeat > 0 {
		select {
		case <-time.After(time.Duration(heartbeat) * time.Second):
		case <-c.Context().Done():
			return nil
		}
	}
	resp.Add(nsPing, "Status", "1")
	resp.Add(nsPing, "HeartbeatInterval", strconv.Itoa(heartbeat))
	return writeWBXML(c, resp, req.protocolVer)
}
