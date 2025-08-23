package mail

// Gateway is the IMAP/SMTP service surface that domain handlers depend on.
// *Client is the production implementation; tests substitute a fake so
// handlers can be exercised without a live mail stack.
type Gateway interface {
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
	SetFlag(email, token, folder string, uid uint32, flag string, value bool) error
	MoveMany(email, token, folder string, uids []uint32, destination string) error
	Delete(email, token, folder string, uid uint32) error

	// Sieve
	SieveListScripts(email, token string) ([]SieveScript, error)
	SieveGetScript(email, token, name string) (string, error)
	SievePutScript(email, token, name, content string, activate bool) error
	SieveDeleteScript(email, token, name string) error
	SieveSetActive(email, token, name string) error
}

// compile-time check that the production client satisfies the gateway.
var _ Gateway = (*Client)(nil)
