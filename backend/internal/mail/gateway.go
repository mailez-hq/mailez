package mail

import "time"

// Gateway is the IMAP/SMTP service surface that domain handlers depend on.
// *Client is the production implementation; tests substitute a fake so
// handlers can be exercised without a live mail stack.
type Gateway interface {
	// With returns a gateway bound to dial; the internal account is the
	// default (no external credentials), an external dial targets the
	// aggregated account's server.
	With(d Dial) Gateway

	// Compose
	Send(email, token, from string, to, cc, bcc []string, subject, text, html string, attachments []Attachment) error
	SaveDraft(email, token string, to, cc []string, subject, text, html string, attachments []Attachment, replaceUID uint32) (uint32, error)

	// Mailbox
	ListFolders(email, token string) ([]string, error)
	UnseenCounts(email, token string) (map[string]int, error)
	ListMessages(email, token, folder string, page int) ([]Message, int, error)
	GetMessage(email, token, folder string, uid uint32) (*Message, error)
	UIDByMessageID(email, token, folder, id string) (uint32, error)
	GetRaw(email, token, folder string, uid uint32) (string, error)
	Thread(email, token, folder, tid string) ([]Message, error)
	SearchMessages(email, token, folder, query string) ([]Message, error)
	SearchMessagesSpec(email, token, folder string, sel SearchQuery) ([]Message, error)
	SearchAllMessages(email, token, query string) ([]Message, error)
	SearchAllMessagesSpec(email, token string, sel SearchQuery) ([]Message, error)
	SetFlag(email, token, folder string, uid uint32, flag string, value bool) error
	ReplaceKeyword(email, token, oldKw, newKw string) error
	MoveMany(email, token, folder string, uids []uint32, destination string) error
	Delete(email, token, folder string, uid uint32) error
	Snooze(email, token, folder string, uid uint32, until *time.Time) error
	SnoozedMessages(email, token string) ([]SnoozedMessage, error)

	// Folder management
	CreateFolder(email, token, name string) error
	RenameFolder(email, token, oldName, newName string) error
	DeleteFolder(email, token, name string) error
	ClearFolder(email, token, name string) error

	// Folder ACL (RFC 4314)
	FolderACL(email, token, folder string) ([]ACLEntry, error)
	MyRights(email, token, folder string) (string, error)
	ListRights(email, token, folder, identifier string) (granted, available string, err error)
	SetFolderACL(email, token, folder, identifier, rights string) error
	DeleteFolderACL(email, token, folder, identifier string) error

	// Sieve
	SieveListScripts(email, token string) ([]SieveScript, error)
	SieveGetScript(email, token, name string) (string, error)
	SievePutScript(email, token, name, content string, activate bool) error
	SieveDeleteScript(email, token, name string) error
	SieveSetActive(email, token, name string) error
}

// compile-time check that the production client satisfies the gateway.
var _ Gateway = (*Client)(nil)
