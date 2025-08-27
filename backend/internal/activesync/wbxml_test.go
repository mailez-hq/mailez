package activesync

import (
	"bytes"
	"testing"
)

// msExampleBytes is the Microsoft documentation example from [MS-ASWBXML]
// (also used by the activesync-go reference tests): a Sync response with
// AirSync/AirSyncBase/Contacts code pages.
func msExampleBytes() []byte {
	return []byte{
		0x03, 0x01, 0x6A, 0x00, // header
		0x05 | 0x40,                                                           // <Sync>
		0x1C | 0x40,                                                           // <Collections>
		0x0F | 0x40,                                                           // <Collection>
		0x10 | 0x40, 0x03, 'c', 'o', 'n', 't', 'a', 'c', 't', 's', 0x00, 0x01, // <Class>contacts</Class>
		0x0B | 0x40, 0x03, '2', 0x00, 0x01, // <SyncKey>2</SyncKey>
		0x12 | 0x40, 0x03, '2', 0x00, 0x01, // <CollectionId>2</CollectionId>
		0x0E | 0x40, 0x03, '1', 0x00, 0x01, // <Status>1</Status>
		0x16 | 0x40,                                  // <Commands>
		0x07 | 0x40,                                  // <Add>
		0x0D | 0x40, 0x03, '2', ':', '1', 0x00, 0x01, // <ServerId>2:1</ServerId>
		0x1D | 0x40, // <ApplicationData>
		0x00, 0x11,  // SWITCH_PAGE AirSyncBase
		0x0A | 0x40,                        // <Body>
		0x06 | 0x40, 0x03, '1', 0x00, 0x01, // <Type>1</Type>
		0x0C | 0x40, 0x03, '0', 0x00, 0x01, // <EstimatedDataSize>0</EstimatedDataSize>
		0x0D | 0x40, 0x03, '1', 0x00, 0x01, // <Truncated>1</Truncated>
		0x01,       // </Body>
		0x00, 0x01, // SWITCH_PAGE Contacts
		0x1E | 0x40, 0x03, 'F', 'u', 'n', 'k', ',', ' ', 'D', 'o', 'n', 0x00, 0x01, // <FileAs>Funk, Don</FileAs>
		0x1F | 0x40, 0x03, 'D', 'o', 'n', 0x00, 0x01, // <FirstName>Don</FirstName>
		0x29 | 0x40, 0x03, 'F', 'u', 'n', 'k', 0x00, 0x01, // <LastName>Funk</LastName>
		0x00, 0x11, // SWITCH_PAGE AirSyncBase
		0x16 | 0x40, 0x03, '1', 0x00, 0x01, // <NativeBodyType>1</NativeBodyType>
		0x01, // </ApplicationData>
		0x01, // </Add>
		0x01, // </Commands>
		0x01, // </Collection>
		0x01, // </Collections>
		0x01, // </Sync>
	}
}

func TestDecodeMSExample(t *testing.T) {
	root, err := DecodeWBXML(msExampleBytes(), nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if root.NS != "AirSync" || root.Name != "Sync" {
		t.Fatalf("root = %s:%s", root.NS, root.Name)
	}
	collections := root.Child("AirSync", "Collections")
	if collections == nil {
		t.Fatal("missing Collections")
	}
	col := collections.Child("AirSync", "Collection")
	if col == nil {
		t.Fatal("missing Collection")
	}
	if got := col.ChildText("AirSync", "SyncKey"); got != "2" {
		t.Fatalf("SyncKey = %q", got)
	}
	add := col.Child("AirSync", "Commands").Child("AirSync", "Add")
	if add == nil {
		t.Fatal("missing Add")
	}
	if got := add.ChildText("AirSync", "ServerId"); got != "2:1" {
		t.Fatalf("ServerId = %q", got)
	}
	app := add.Child("AirSync", "ApplicationData")
	if app == nil {
		t.Fatal("missing ApplicationData")
	}
	if got := app.ChildText("Contacts", "FirstName"); got != "Don" {
		t.Fatalf("FirstName = %q", got)
	}
	body := app.Child("AirSyncBase", "Body")
	if body == nil {
		t.Fatal("missing Body")
	}
	if got := body.ChildText("AirSyncBase", "Type"); got != "1" {
		t.Fatalf("Body/Type = %q", got)
	}
}

func TestEncodeMSExample(t *testing.T) {
	root := &Element{NS: "AirSync", Name: "Sync"}
	collections := root.Add("AirSync", "Collections", "")
	col := collections.Add("AirSync", "Collection", "")
	col.Add("AirSync", "Class", "contacts")
	col.Add("AirSync", "SyncKey", "2")
	col.Add("AirSync", "CollectionId", "2")
	col.Add("AirSync", "Status", "1")
	commands := col.Add("AirSync", "Commands", "")
	add := commands.Add("AirSync", "Add", "")
	add.Add("AirSync", "ServerId", "2:1")
	app := add.Add("AirSync", "ApplicationData", "")
	body := app.Add("AirSyncBase", "Body", "")
	body.Add("AirSyncBase", "Type", "1")
	body.Add("AirSyncBase", "EstimatedDataSize", "0")
	body.Add("AirSyncBase", "Truncated", "1")
	app.Add("Contacts", "FileAs", "Funk, Don")
	app.Add("Contacts", "FirstName", "Don")
	app.Add("Contacts", "LastName", "Funk")
	app.Add("AirSyncBase", "NativeBodyType", "1")

	got, err := EncodeWBXML(root)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !bytes.Equal(got, msExampleBytes()) {
		t.Fatalf("encoded bytes differ\n got: %x\nwant: %x", got, msExampleBytes())
	}
}

func TestRoundTripFolderSync(t *testing.T) {
	root := &Element{NS: "FolderHierarchy", Name: "FolderSync"}
	root.Add("FolderHierarchy", "Status", "1")
	root.Add("FolderHierarchy", "SyncKey", "s-abc123")
	changes := root.Add("FolderHierarchy", "Changes", "")
	add := changes.Add("FolderHierarchy", "Add", "")
	add.Add("FolderHierarchy", "ServerId", "inbox")
	add.Add("FolderHierarchy", "ParentId", "0")
	add.Add("FolderHierarchy", "DisplayName", "Inbox")
	add.Add("FolderHierarchy", "Type", "2")

	enc, err := EncodeWBXML(root)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	dec, err := DecodeWBXML(enc, nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if dec.NS != "FolderHierarchy" || dec.Name != "FolderSync" {
		t.Fatalf("root = %s:%s", dec.NS, dec.Name)
	}
	if got := dec.ChildText("FolderHierarchy", "SyncKey"); got != "s-abc123" {
		t.Fatalf("SyncKey = %q", got)
	}
	adds := dec.Child("FolderHierarchy", "Changes").ChildrenNamed("FolderHierarchy", "Add")
	if len(adds) != 1 {
		t.Fatalf("Add count = %d", len(adds))
	}
	if got := adds[0].ChildText("FolderHierarchy", "DisplayName"); got != "Inbox" {
		t.Fatalf("DisplayName = %q", got)
	}
}

func TestRoundTripUTF8AndEmptyTags(t *testing.T) {
	root := &Element{NS: "AirSync", Name: "Sync"}
	collections := root.Add("AirSync", "Collections", "")
	col := collections.Add("AirSync", "Collection", "")
	col.Add("AirSync", "Class", "Email")
	col.Add("AirSync", "SyncKey", "0")
	col.Add("AirSync", "CollectionId", "inbox")
	col.Add("AirSync", "GetChanges", "") // empty element
	col.Add("AirSync", "Status", "1")
	// Unicode content (Chinese label body) must survive the round trip.
	col.Add("AirSync", "ServerId", "邮件标签/中文内容 🚀")

	enc, err := EncodeWBXML(root)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	dec, err := DecodeWBXML(enc, nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	c := dec.Child("AirSync", "Collections").Child("AirSync", "Collection")
	if got := c.ChildText("AirSync", "ServerId"); got != "邮件标签/中文内容 🚀" {
		t.Fatalf("unicode round trip = %q", got)
	}
	if c.Child("AirSync", "GetChanges") == nil {
		t.Fatal("empty GetChanges element lost")
	}
}

// TestDecodeTagWithAttributesAndContent covers the 0xC0|code token shape
// (content + attributes bits set together), which the codec must treat as a
// tag rather than a global EXT/OPAQUE token.
func TestDecodeTagWithAttributesAndContent(t *testing.T) {
	data := []byte{
		0x03, 0x01, 0x6A, 0x00,
		0xC5,                        // AirSync:Sync with content + attributes
		0x05, 0x03, 'x', 0x00, 0x01, // attribute "x" = "" (attr token, STR_I, END)
		0x03, 'h', 'i', 0x00, // text "hi"
		0x01, // END
	}
	root, err := DecodeWBXML(data, nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if root.NS != "AirSync" || root.Name != "Sync" || root.Text != "hi" {
		t.Fatalf("root = %s:%s text=%q", root.NS, root.Name, root.Text)
	}
	if len(root.Attrs) == 0 {
		t.Fatalf("attributes not captured: %+v", root.Attrs)
	}
}
