package activesync

// EAS protocol helpers: folder identity mapping, wire time formats and the
// message → ApplicationData builder shared by Sync and Search responses.
import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"mailez/backend/internal/mail"
)

// EAS collection (folder) identity. Well-known mailboxes get readable ids so
// logs and client debugging stay friendly; everything else gets a stable
// hash-based id. The mapping is deterministic across devices and sync keys.
func folderID(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "inbox":
		return "inbox"
	case "sent", "sent items", "sentitems":
		return "sent"
	case "drafts":
		return "drafts"
	case "trash", "deleted", "deleted items", "deleteditems":
		return "trash"
	case "junk", "spam":
		return "junk"
	case "outbox":
		return "outbox"
	case "archive":
		return "archive"
	case "calendar":
		return "calendar"
	case "contacts":
		return "contacts"
	case "notes":
		return "notes"
	}
	sum := sha256.Sum256([]byte(strings.ToLower(name)))
	return "f" + hex.EncodeToString(sum[:8])
}

// folderType maps a folder name to its FolderHierarchy Type.
func folderType(name string) byte {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "inbox":
		return 2 // Default Inbox
	case "drafts":
		return 3 // Default Drafts
	case "trash", "deleted", "deleted items", "deleteditems":
		return 4 // Default Deleted
	case "sent", "sent items", "sentitems":
		return 5 // Default Sent
	case "outbox":
		return 6 // Default Outbox
	case "junk", "spam":
		return 7 // Default Junk
	case "calendar":
		return 9 // Default Calendar
	case "contacts":
		return 10 // Default Contacts
	case "notes":
		return 11 // Default Notes
	default:
		return 1 // User-created
	}
}

// folderForID resolves a known special id back to the mailbox name; hashed
// ids must be resolved against the live folder list instead.
func folderForID(id string) (string, bool) {
	switch id {
	case "inbox":
		return "Inbox", true
	case "sent":
		return "Sent", true
	case "drafts":
		return "Drafts", true
	case "trash":
		return "Trash", true
	case "junk":
		return "Junk", true
	case "outbox":
		return "Outbox", true
	case "archive":
		return "Archive", true
	case "calendar":
		return "Calendar", true
	case "contacts":
		return "Contacts", true
	case "notes":
		return "Notes", true
	}
	return "", false
}

// easTime renders a timestamp in the EAS "yyyymmddThhmmss.mmmZ" format.
func easTime(t time.Time) string {
	return t.UTC().Format("20060102T150405.000Z")
}

// parseEASDate parses the wire date formats clients send back to us.
func parseEASDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{
		"20060102T150405.000Z",
		"20060102T150405Z",
		"2006-01-02T15:04:05.000Z",
		time.RFC3339,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// addressString formats a mail address the way EAS clients display it.
func addressString(a mail.Address) string {
	if a.Name != "" && a.Name != a.Email {
		return a.Name + " <" + a.Email + ">"
	}
	return a.Email
}

func addressList(addrs []mail.Address) string {
	parts := make([]string, 0, len(addrs))
	for _, a := range addrs {
		parts = append(parts, addressString(a))
	}
	return strings.Join(parts, "; ")
}

func emailList(addrs []mail.Address) string {
	parts := make([]string, 0, len(addrs))
	for _, a := range addrs {
		if a.Email != "" {
			parts = append(parts, a.Email)
		}
	}
	return strings.Join(parts, "; ")
}

func hasFlag(flags []string, flag string) bool {
	for _, f := range flags {
		if strings.EqualFold(f, flag) {
			return true
		}
	}
	return false
}

// bodyPreference describes what the client asked for in Sync/ItemOperations.
type bodyPreference struct {
	html        bool
	truncation  int
	allOrNone   bool
	mime        bool
	includeBody bool
}

func parseBodyPreference(col *Element) bodyPreference {
	var pref bodyPreference
	opts := col.Child("AirSync", "Options")
	if opts == nil {
		return pref
	}
	bp := opts.Child("AirSyncBase", "BodyPreference")
	if bp == nil {
		bp = opts.Child("AirSyncBase", "BodyPartPreference")
	}
	if bp == nil {
		return pref
	}
	pref.includeBody = true
	switch bp.ChildText("AirSyncBase", "Type") {
	case "1": // HTML
		pref.html = true
	case "3": // MIME
		pref.mime = true
	default: // 0 plain text, 2 RTF → treated as plain
		pref.html = false
	}
	if ts := bp.ChildText("AirSyncBase", "TruncationSize"); ts != "" {
		if n, err := strconv.Atoi(ts); err == nil {
			pref.truncation = n
		}
	}
	pref.allOrNone = bp.ChildText("AirSyncBase", "AllOrNone") == "1"
	return pref
}

// buildEmailAppData renders a mail.Message as an EAS ApplicationData element
// (Email namespace + AirSyncBase body), honouring the client body preference.
func buildEmailAppData(msg *mail.Message, pref bodyPreference) *Element {
	app := &Element{NS: nsAirSync, Name: "ApplicationData"}
	app.Add(nsEmail, "To", emailList(msg.To))
	app.Add(nsEmail, "Cc", emailList(msg.Cc))
	if len(msg.From) > 0 {
		app.Add(nsEmail, "From", addressString(msg.From[0]))
	}
	app.Add(nsEmail, "Subject", msg.Subject)
	app.Add(nsEmail, "ThreadTopic", msg.Subject)
	app.Add(nsEmail, "DateReceived", easTime(msg.Date))
	app.Add(nsEmail, "DisplayTo", addressList(msg.To))
	app.Add(nsEmail, "Importance", "1")
	app.Add(nsEmail, "MessageClass", "IPM.Note")
	if hasFlag(msg.Flags, `\Seen`) {
		app.Add(nsEmail, "Read", "1")
	} else {
		app.Add(nsEmail, "Read", "0")
	}
	if hasFlag(msg.Flags, `\Flagged`) {
		flag := app.Add(nsEmail, "Flag", "")
		flag.Add(nsEmail, "Status", "2")
	}
	if msg.HasAttachment {
		atts := app.Add(nsAirSyncBase, "Attachments", "")
		for i, a := range msg.Attachments {
			att := atts.Add(nsAirSyncBase, "Attachment", "")
			att.Add(nsAirSyncBase, "DisplayName", a.Filename)
			att.Add(nsAirSyncBase, "FileReference", attachmentRef(msg.UID, i))
			att.Add(nsAirSyncBase, "Method", "1")
			att.Add(nsAirSyncBase, "EstimatedDataSize", strconv.Itoa(a.Size))
			if a.ContentType == "image/png" || strings.HasPrefix(a.ContentType, "image/") {
				att.Add(nsAirSyncBase, "IsInline", "1")
			}
		}
	}
	body := buildEmailBody(msg, pref)
	if body != nil {
		app.Children = append(app.Children, body)
	}
	return app
}

// attachmentRef is the FileReference used by ItemOperations to fetch one
// attachment: "<uid>:<index>".
func attachmentRef(uid uint32, idx int) string {
	return strconv.FormatUint(uint64(uid), 10) + ":" + strconv.Itoa(idx)
}

// buildEmailBody renders the AirSyncBase Body element for a message.
func buildEmailBody(msg *mail.Message, pref bodyPreference) *Element {
	var (
		data string
		typ  string
	)
	text := strings.TrimSpace(msg.TextBody)
	html := strings.TrimSpace(msg.HTMLBody)
	switch {
	case pref.mime:
		// MIME bodies are only available through GetRaw; Sync responses use
		// plain text/HTML and clients fetch the raw stream via ItemOperations.
		return nil
	case pref.html && html != "":
		data, typ = html, "2"
	case pref.html && text != "":
		data, typ = text, "1"
	case text != "":
		data, typ = text, "1"
	case html != "":
		data, typ = html, "2"
	default:
		return nil
	}
	// EAS bodies travel as UTF-8 text for plain/HTML; base64 only for MIME.
	body := &Element{NS: nsAirSyncBase, Name: "Body"}
	body.Add(nsAirSyncBase, "Type", typ)
	body.Add(nsAirSyncBase, "EstimatedDataSize", strconv.Itoa(len(data)))
	if pref.truncation > 0 && len(data) > pref.truncation {
		if pref.allOrNone {
			return nil
		}
		data = data[:pref.truncation]
		body.Add(nsAirSyncBase, "Truncated", "1")
	}
	if data != "" {
		body.Add(nsAirSyncBase, "Data", data)
	}
	if msg.HTMLBody != "" && !pref.html {
		// Native type tells the client which format the server stored.
		body.Add(nsAirSyncBase, "NativeBodyType", "2")
	} else {
		body.Add(nsAirSyncBase, "NativeBodyType", "1")
	}
	return body
}

// Namespaces used across the command handlers.
const (
	nsAirSync         = "AirSync"
	nsContacts        = "Contacts"
	nsEmail           = "Email"
	nsCalendar        = "Calendar"
	nsMove            = "Move"
	nsItemEstimate    = "ItemEstimate"
	nsFolderHierarchy = "FolderHierarchy"
	nsMeetingResponse = "MeetingResponse"
	nsTasks           = "Tasks"
	nsResolveRecip    = "ResolveRecipients"
	nsValidateCert    = "ValidateCert"
	nsContacts2       = "Contacts2"
	nsPing            = "Ping"
	nsProvision       = "Provision"
	nsSearch          = "Search"
	nsGAL             = "GAL"
	nsAirSyncBase     = "AirSyncBase"
	nsSettings        = "Settings"
	nsDocumentLibrary = "DocumentLibrary"
	nsItemOperations  = "ItemOperations"
	nsComposeMail     = "ComposeMail"
	nsEmail2          = "Email2"
	nsNotes           = "Notes"
	nsRightsMgmt      = "RightsManagement"
)
