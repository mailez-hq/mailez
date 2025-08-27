package activesync

import (
	"net/http"
	"testing"
	"time"

	"mailez/backend/internal/core/models"
)

func TestSyncCalendar(t *testing.T) {
	app, _, db := newTestService(t)
	auth := basicAuth(testEmail, testPassword)
	start := time.Now().Add(24 * time.Hour)
	ev := models.CalendarEvent{
		UserEmail: testEmail, UID: "cal-1", Summary: "季度评审", Location: "会议室A",
		Start: &start, ICS: "BEGIN:VCALENDAR\r\nEND:VCALENDAR",
	}
	if err := db.Create(&ev).Error; err != nil {
		t.Fatal(err)
	}
	// Initial sync key.
	init := &Element{NS: nsAirSync, Name: "Sync"}
	cols := init.Add(nsAirSync, "Collections", "")
	col := cols.Add(nsAirSync, "Collection", "")
	col.Add(nsAirSync, "Class", "Calendar")
	col.Add(nsAirSync, "SyncKey", "0")
	col.Add(nsAirSync, "CollectionId", "calendar")
	col.Add(nsAirSync, "GetChanges", "1")
	_, raw := doEAS(t, app, http.MethodPost, authURL("Sync"), auth, init)
	root, err := DecodeWBXML(raw, nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	key1 := root.Child(nsAirSync, "Collections").Child(nsAirSync, "Collection").ChildText(nsAirSync, "SyncKey")
	if key1 == "" {
		t.Fatalf("no calendar sync key: %s", root.String())
	}
	// Full sync: the seeded event arrives as an Add.
	full := &Element{NS: nsAirSync, Name: "Sync"}
	cols2 := full.Add(nsAirSync, "Collections", "")
	col2 := cols2.Add(nsAirSync, "Collection", "")
	col2.Add(nsAirSync, "Class", "Calendar")
	col2.Add(nsAirSync, "SyncKey", key1)
	col2.Add(nsAirSync, "CollectionId", "calendar")
	col2.Add(nsAirSync, "GetChanges", "1")
	_, raw2 := doEAS(t, app, http.MethodPost, authURL("Sync"), auth, full)
	root2, err := DecodeWBXML(raw2, nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	out := root2.Child(nsAirSync, "Collections").Child(nsAirSync, "Collection")
	adds := out.Child(nsAirSync, "Commands").ChildrenNamed(nsAirSync, "Add")
	if len(adds) != 1 {
		t.Fatalf("calendar adds = %d: %s", len(adds), out.String())
	}
	appData := adds[0].Child(nsAirSync, "ApplicationData")
	if got := appData.ChildText(nsCalendar, "Subject"); got != "季度评审" {
		t.Fatalf("subject = %q", got)
	}
	if got := appData.ChildText(nsCalendar, "Location"); got != "会议室A" {
		t.Fatalf("location = %q", got)
	}
	key2 := out.ChildText(nsAirSync, "SyncKey")

	// Client creates a new event via Sync Add; it must land in the DB and be
	// acknowledged in the same response.
	create := &Element{NS: nsAirSync, Name: "Sync"}
	cols3 := create.Add(nsAirSync, "Collections", "")
	col3 := cols3.Add(nsAirSync, "Collection", "")
	col3.Add(nsAirSync, "Class", "Calendar")
	col3.Add(nsAirSync, "SyncKey", key2)
	col3.Add(nsAirSync, "CollectionId", "calendar")
	col3.Add(nsAirSync, "GetChanges", "1")
	cmds := col3.Add(nsAirSync, "Commands", "")
	add := cmds.Add(nsAirSync, "Add", "")
	add.Add(nsAirSync, "ClientId", "client-1")
	appIn := add.Add(nsAirSync, "ApplicationData", "")
	appIn.Add(nsCalendar, "Subject", "移动端新建会议")
	appIn.Add(nsCalendar, "StartTime", easTime(time.Now().Add(2*time.Hour)))
	appIn.Add(nsCalendar, "EndTime", easTime(time.Now().Add(3*time.Hour)))
	_, raw3 := doEAS(t, app, http.MethodPost, authURL("Sync"), auth, create)
	root3, err := DecodeWBXML(raw3, nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	out3 := root3.Child(nsAirSync, "Collections").Child(nsAirSync, "Collection")
	if got := out3.ChildText(nsAirSync, "Status"); got != "1" {
		t.Fatalf("create sync status = %q: %s", got, out3.String())
	}
	var count int64
	db.Model(&models.CalendarEvent{}).Where("user_email = ? AND summary = ?", testEmail, "移动端新建会议").Count(&count)
	if count != 1 {
		t.Fatalf("client-created event count = %d", count)
	}
}

func TestSyncContacts(t *testing.T) {
	app, _, db := newTestService(t)
	auth := basicAuth(testEmail, testPassword)
	if err := db.Create(&models.Contact{
		UserEmail: testEmail, Name: "张三", Email: "zhangsan@example.com", DavUID: "c-1",
	}).Error; err != nil {
		t.Fatal(err)
	}
	init := &Element{NS: nsAirSync, Name: "Sync"}
	cols := init.Add(nsAirSync, "Collections", "")
	col := cols.Add(nsAirSync, "Collection", "")
	col.Add(nsAirSync, "Class", "Contacts")
	col.Add(nsAirSync, "SyncKey", "0")
	col.Add(nsAirSync, "CollectionId", "contacts")
	col.Add(nsAirSync, "GetChanges", "1")
	_, raw := doEAS(t, app, http.MethodPost, authURL("Sync"), auth, init)
	root, err := DecodeWBXML(raw, nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	key1 := root.Child(nsAirSync, "Collections").Child(nsAirSync, "Collection").ChildText(nsAirSync, "SyncKey")
	full := &Element{NS: nsAirSync, Name: "Sync"}
	cols2 := full.Add(nsAirSync, "Collections", "")
	col2 := cols2.Add(nsAirSync, "Collection", "")
	col2.Add(nsAirSync, "Class", "Contacts")
	col2.Add(nsAirSync, "SyncKey", key1)
	col2.Add(nsAirSync, "CollectionId", "contacts")
	col2.Add(nsAirSync, "GetChanges", "1")
	_, raw2 := doEAS(t, app, http.MethodPost, authURL("Sync"), auth, full)
	root2, err := DecodeWBXML(raw2, nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	out := root2.Child(nsAirSync, "Collections").Child(nsAirSync, "Collection")
	adds := out.Child(nsAirSync, "Commands").ChildrenNamed(nsAirSync, "Add")
	if len(adds) != 1 {
		t.Fatalf("contacts adds = %d: %s", len(adds), out.String())
	}
	appData := adds[0].Child(nsAirSync, "ApplicationData")
	if got := appData.ChildText(nsContacts, "Email1Address"); got != "zhangsan@example.com" {
		t.Fatalf("email = %q", got)
	}
	if got := appData.ChildText(nsContacts, "FirstName"); got != "张三" {
		t.Fatalf("first name = %q", got)
	}
	key2 := out.ChildText(nsAirSync, "SyncKey")

	// Client creates a contact.
	create := &Element{NS: nsAirSync, Name: "Sync"}
	cols3 := create.Add(nsAirSync, "Collections", "")
	col3 := cols3.Add(nsAirSync, "Collection", "")
	col3.Add(nsAirSync, "Class", "Contacts")
	col3.Add(nsAirSync, "SyncKey", key2)
	col3.Add(nsAirSync, "CollectionId", "contacts")
	col3.Add(nsAirSync, "GetChanges", "1")
	cmds := col3.Add(nsAirSync, "Commands", "")
	add := cmds.Add(nsAirSync, "Add", "")
	add.Add(nsAirSync, "ClientId", "c-2")
	appIn := add.Add(nsAirSync, "ApplicationData", "")
	appIn.Add(nsContacts, "FirstName", "李")
	appIn.Add(nsContacts, "LastName", "四")
	appIn.Add(nsContacts, "Email1Address", "lisi@example.com")
	_, raw3 := doEAS(t, app, http.MethodPost, authURL("Sync"), auth, create)
	root3, err := DecodeWBXML(raw3, nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	out3 := root3.Child(nsAirSync, "Collections").Child(nsAirSync, "Collection")
	if got := out3.ChildText(nsAirSync, "Status"); got != "1" {
		t.Fatalf("create status = %q: %s", got, out3.String())
	}
	var count int64
	db.Model(&models.Contact{}).Where("user_email = ? AND email = ?", testEmail, "lisi@example.com").Count(&count)
	if count != 1 {
		t.Fatalf("client-created contact count = %d", count)
	}
}
