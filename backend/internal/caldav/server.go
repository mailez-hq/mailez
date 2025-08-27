// Package caldav implements the built-in CalDAV (RFC 4791) server. It maps
// DAV paths to CalendarEvent rows so phones and desktop clients can
// synchronize events bidirectionally. RRULE recurrence and timezone data are
// stored verbatim and returned to clients, which expand them locally.
package caldav

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"mailez/backend/internal/core/models"
	"mailez/backend/internal/webdav"
)

const calendarPath = "/dav/calendars"
const calendarName = "default"

// Server is the CalDAV backend bound to the database.
type Server struct {
	DB *gorm.DB
}

// New returns a CalDAV backend for db.
func New(db *gorm.DB) *Server { return &Server{DB: db} }

type kind int

const (
	kindRoot       kind = iota
	kindHome            // /dav/calendars/{user}
	kindCollection      // /dav/calendars/{user}/default
	kindResource        // /dav/calendars/{user}/default/{uid}.ics
)

func parsePath(path string) (k kind, user, uid string, ok bool) {
	decoded, err := url.PathUnescape(path)
	if err != nil {
		return kindRoot, "", "", false
	}
	decoded = strings.TrimSuffix(decoded, "/")
	rest, found := strings.CutPrefix(decoded, calendarPath)
	if !found {
		return kindRoot, "", "", false
	}
	rest = strings.TrimPrefix(rest, "/")
	if rest == "" {
		return kindRoot, "", "", true
	}
	parts := strings.Split(rest, "/")
	user = parts[0]
	switch len(parts) {
	case 1:
		return kindHome, user, "", true
	case 2:
		if parts[1] != calendarName {
			return kindRoot, "", "", false
		}
		return kindCollection, user, "", true
	case 3:
		if parts[1] != calendarName {
			return kindRoot, "", "", false
		}
		uid = strings.TrimSuffix(parts[2], ".ics")
		if uid == "" || uid == parts[2] {
			return kindRoot, "", "", false
		}
		return kindResource, user, uid, true
	}
	return kindRoot, "", "", false
}

func calendarHome(user string) string { return calendarPath + "/" + url.PathEscape(user) }
func collectionURL(user string) string {
	return calendarHome(user) + "/" + calendarName
}
func eventURL(user, uid string) string {
	return collectionURL(user) + "/" + url.PathEscape(uid) + ".ics"
}
func principalURL(user string) string {
	return "/dav/principals/" + url.PathEscape(user) + "/"
}

// etag hashes the raw ICS body.
func etagOf(icsText string) string {
	sum := md5.Sum([]byte(icsText))
	return hex.EncodeToString(sum[:])
}

// ctag hashes the sorted (uid, etag) set of the calendar.
func (s *Server) ctag(ctx context.Context, user string) (string, error) {
	var list []models.CalendarEvent
	if err := s.DB.WithContext(ctx).Select("uid", "ics").Where("user_email = ?", user).Find(&list).Error; err != nil {
		return "", err
	}
	sort.Slice(list, func(i, j int) bool { return list[i].UID < list[j].UID })
	var b strings.Builder
	for i := range list {
		b.WriteString(list[i].UID)
		b.WriteString(":")
		b.WriteString(etagOf(list[i].ICS))
		b.WriteString("\n")
	}
	sum := md5.Sum([]byte(b.String()))
	return hex.EncodeToString(sum[:]), nil
}

// PropFind implements webdav.Backend.
func (s *Server) PropFind(ctx context.Context, path string, props []xml.Name, depth webdav.Depth) ([]webdav.Response, error) {
	k, user, uid, ok := parsePath(path)
	if !ok {
		return nil, webdav.HTTPError(404, "not found")
	}
	switch k {
	case kindRoot:
		user := webdav.UserFrom(ctx)
		return []webdav.Response{{
			Href: "/dav/",
			Props: []webdav.Prop{
				{XMLName: xml.Name{Space: webdav.NSDAV, Local: "resourcetype"}, Value: webdav.Empty(xml.Name{Space: webdav.NSDAV, Local: "collection"})},
				{XMLName: xml.Name{Space: webdav.NSDAV, Local: "current-user-principal"}, Value: webdav.Href(principalURL(user))},
			},
		}}, nil
	case kindHome:
		resp := webdav.Response{
			Href: calendarHome(user) + "/",
			Props: []webdav.Prop{
				{XMLName: xml.Name{Space: webdav.NSDAV, Local: "resourcetype"}, Value: webdav.Empty(xml.Name{Space: webdav.NSDAV, Local: "collection"})},
				{XMLName: xml.Name{Space: webdav.NSDAV, Local: "current-user-principal"}, Value: webdav.Href(principalURL(user))},
				{XMLName: xml.Name{Space: webdav.NSCalDAV, Local: "calendar-home-set"}, Value: webdav.Href(calendarHome(user) + "/")},
			},
		}
		out := []webdav.Response{resp}
		if depth >= webdav.DepthOne {
			col, err := s.collectionResponse(ctx, user)
			if err != nil {
				return nil, err
			}
			out = append(out, col)
		}
		return out, nil
	case kindCollection:
		if depth >= webdav.DepthOne {
			return s.collectionWithChildren(ctx, user)
		}
		col, err := s.collectionResponse(ctx, user)
		if err != nil {
			return nil, err
		}
		return []webdav.Response{col}, nil
	case kindResource:
		var ev models.CalendarEvent
		if err := s.DB.WithContext(ctx).Where("user_email = ? AND uid = ?", user, uid).First(&ev).Error; err != nil {
			return nil, webdav.HTTPError(404, "event not found")
		}
		return []webdav.Response{{
			Href: eventURL(user, uid),
			Props: []webdav.Prop{
				{XMLName: xml.Name{Space: webdav.NSDAV, Local: "getetag"}, Value: webdav.Text(ev.ETag)},
				{XMLName: xml.Name{Space: webdav.NSDAV, Local: "getcontenttype"}, Value: webdav.Text("text/calendar; charset=utf-8")},
				{XMLName: xml.Name{Space: webdav.NSDAV, Local: "getcontentlength"}, Value: webdav.Text(fmt.Sprint(len(ev.ICS)))},
			},
		}}, nil
	}
	return nil, webdav.HTTPError(404, "not found")
}

func (s *Server) collectionResponse(ctx context.Context, user string) (webdav.Response, error) {
	tag, err := s.ctag(ctx, user)
	if err != nil {
		return webdav.Response{}, err
	}
	supportedReports := "<D:supported-report><CA:calendar-query/></D:supported-report>" +
		"<D:supported-report><CA:calendar-multiget/></D:supported-report>"
	return webdav.Response{
		Href: collectionURL(user) + "/",
		Props: []webdav.Prop{
			{XMLName: xml.Name{Space: webdav.NSDAV, Local: "resourcetype"}, Value: webdav.Empty(xml.Name{Space: webdav.NSDAV, Local: "collection"}) + webdav.Empty(xml.Name{Space: webdav.NSCalDAV, Local: "calendar"})},
			{XMLName: xml.Name{Space: webdav.NSDAV, Local: "displayname"}, Value: webdav.Text("Default Calendar")},
			{XMLName: xml.Name{Space: webdav.NSDAV, Local: "getctag"}, Value: webdav.Text(tag)},
			{XMLName: xml.Name{Space: webdav.NSDAV, Local: "getetag"}, Value: webdav.Text(tag)},
			{XMLName: xml.Name{Space: webdav.NSCalDAV, Local: "calendar-description"}, Value: webdav.Text("Mailez Calendar")},
			{XMLName: xml.Name{Space: webdav.NSCalDAV, Local: "supported-calendar-component-set"}, Value: "<CA:comp name=\"VEVENT\"/>"},
			{XMLName: xml.Name{Space: webdav.NSDAV, Local: "supported-report-set"}, Value: supportedReports},
		},
	}, nil
}

func (s *Server) collectionWithChildren(ctx context.Context, user string) ([]webdav.Response, error) {
	col, err := s.collectionResponse(ctx, user)
	if err != nil {
		return nil, err
	}
	var list []models.CalendarEvent
	if err := s.DB.WithContext(ctx).Where("user_email = ?", user).Order("uid").Find(&list).Error; err != nil {
		return nil, err
	}
	out := []webdav.Response{col}
	for i := range list {
		out = append(out, s.eventResponse(user, &list[i]))
	}
	return out, nil
}

func (s *Server) eventResponse(user string, ev *models.CalendarEvent) webdav.Response {
	return webdav.Response{
		Href: eventURL(user, ev.UID),
		Props: []webdav.Prop{
			{XMLName: xml.Name{Space: webdav.NSDAV, Local: "getetag"}, Value: webdav.Text(ev.ETag)},
			{XMLName: xml.Name{Space: webdav.NSDAV, Local: "getcontenttype"}, Value: webdav.Text("text/calendar; charset=utf-8")},
			{XMLName: xml.Name{Space: webdav.NSDAV, Local: "getcontentlength"}, Value: webdav.Text(fmt.Sprint(len(ev.ICS)))},
			{XMLName: xml.Name{Space: webdav.NSCalDAV, Local: "calendar-data"}, Value: webdav.Text(ev.ICS)},
		},
	}
}

// Get implements webdav.Backend.
func (s *Server) Get(ctx context.Context, path string) ([]byte, string, string, error) {
	k, user, uid, ok := parsePath(path)
	if !ok || k != kindResource {
		return nil, "", "", webdav.HTTPError(404, "event not found")
	}
	var ev models.CalendarEvent
	if err := s.DB.WithContext(ctx).Where("user_email = ? AND uid = ?", user, uid).First(&ev).Error; err != nil {
		return nil, "", "", webdav.HTTPError(404, "event not found")
	}
	return []byte(ev.ICS), "text/calendar; charset=utf-8", ev.ETag, nil
}

// Put implements webdav.Backend.
func (s *Server) Put(ctx context.Context, path string, body []byte, ifMatch, ifNoneMatch string) (string, bool, error) {
	k, user, uid, ok := parsePath(path)
	if !ok || k != kindResource {
		return "", false, webdav.HTTPError(404, "invalid calendar resource path")
	}
	d, err := ParseEvent(string(body))
	if err != nil {
		return "", false, webdav.HTTPError(400, "%s", err.Error())
	}
	if d.UID == "" {
		d.UID = uid
	}
	if d.UID == "" {
		return "", false, webdav.HTTPError(400, "event is missing UID")
	}
	etag := etagOf(string(body))
	var ev models.CalendarEvent
	err = s.DB.WithContext(ctx).Where("user_email = ? AND uid = ?", user, d.UID).First(&ev).Error
	exists := err == nil
	if err != nil && err != gorm.ErrRecordNotFound {
		return "", false, err
	}
	if exists {
		if ifMatch != "" && !webdav.ETagMatches(ifMatch, ev.ETag) {
			return "", false, webdav.HTTPError(412, "precondition failed")
		}
		if ifNoneMatch != "" && webdav.ETagMatches(ifNoneMatch, ev.ETag) {
			return "", false, webdav.HTTPError(412, "event already exists")
		}
	} else if ifMatch != "" {
		return "", false, webdav.HTTPError(412, "precondition failed")
	}

	ev.UserEmail = user
	ev.UID = d.UID
	ev.ETag = etag
	ev.Summary = d.Summary
	ev.Location = d.Location
	ev.Description = d.Description
	ev.AllDay = d.AllDay
	ev.Start = d.Start
	ev.End = d.End
	ev.RRule = d.RRule
	ev.ICS = string(body)
	if err := s.DB.WithContext(ctx).Save(&ev).Error; err != nil {
		return "", false, err
	}
	return etag, !exists, nil
}

// Delete implements webdav.Backend.
func (s *Server) Delete(ctx context.Context, path string) error {
	k, user, uid, ok := parsePath(path)
	if !ok || k != kindResource {
		return webdav.HTTPError(404, "event not found")
	}
	res := s.DB.WithContext(ctx).Where("user_email = ? AND uid = ?", user, uid).Delete(&models.CalendarEvent{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return webdav.HTTPError(404, "event not found")
	}
	return nil
}

// Report implements webdav.Backend (calendar-query / calendar-multiget).
func (s *Server) Report(ctx context.Context, path string, name xml.Name, body []byte) ([]webdav.Response, error) {
	k, user, _, ok := parsePath(path)
	if !ok || k != kindCollection {
		return nil, webdav.HTTPError(404, "not found")
	}
	var list []models.CalendarEvent
	if err := s.DB.WithContext(ctx).Where("user_email = ?", user).Order("uid").Find(&list).Error; err != nil {
		return nil, err
	}
	switch name.Local {
	case "calendar-multiget":
		want := multigetHrefs(body)
		byUID := map[string]*models.CalendarEvent{}
		for i := range list {
			byUID[list[i].UID] = &list[i]
		}
		var out []webdav.Response
		for _, h := range want {
			uid := resourceUID(h)
			if ev, ok2 := byUID[uid]; ok2 {
				out = append(out, s.eventResponse(user, ev))
			} else {
				out = append(out, webdav.Response{Href: h, Status: 404})
			}
		}
		return out, nil
	case "calendar-query":
		start, end := queryTimeRange(body)
		var out []webdav.Response
		for i := range list {
			if !overlaps(&list[i], start, end) {
				continue
			}
			out = append(out, s.eventResponse(user, &list[i]))
		}
		return out, nil
	}
	return nil, webdav.HTTPError(400, "unsupported report")
}

// overlaps decides whether an event belongs to a query window. Recurring
// events (RRULE) are returned whenever their start precedes the window end:
// clients expand instances locally.
func overlaps(ev *models.CalendarEvent, start, end *time.Time) bool {
	if start == nil && end == nil {
		return true
	}
	if ev.RRule != "" {
		if start != nil && ev.Start != nil && ev.Start.After(*start) && end == nil {
			return false
		}
		return true
	}
	es, ee := ev.Start, ev.End
	if es == nil {
		return true
	}
	if end != nil && es.After(*end) {
		return false
	}
	if start != nil {
		if ee != nil && ee.Before(*start) {
			return false
		}
	}
	return true
}

// queryTimeRange extracts the first <time-range> start/end from a
// calendar-query body (RFC 4791 §7.8).
func queryTimeRange(body []byte) (*time.Time, *time.Time) {
	dec := xml.NewDecoder(strings.NewReader(string(body)))
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, nil
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "time-range" {
			var start, end *time.Time
			for _, a := range se.Attr {
				switch a.Name.Local {
				case "start":
					if t, err := time.Parse("20060102T150405Z", a.Value); err == nil {
						start = &t
					}
				case "end":
					if t, err := time.Parse("20060102T150405Z", a.Value); err == nil {
						end = &t
					}
				}
			}
			return start, end
		}
	}
}

func multigetHrefs(body []byte) []string {
	dec := xml.NewDecoder(strings.NewReader(string(body)))
	var hrefs []string
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "href" {
			var s string
			if err := dec.DecodeElement(&s, &se); err == nil {
				hrefs = append(hrefs, s)
			}
		}
	}
	return hrefs
}

func resourceUID(href string) string {
	decoded, err := url.PathUnescape(href)
	if err != nil {
		decoded = href
	}
	base := decoded
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	return strings.TrimSuffix(base, ".ics")
}
