// Package carddav implements the built-in CardDAV (RFC 6352) server. It maps
// DAV paths to the mailez Contact table so phone/desktop address books can
// synchronize bidirectionally over the standard protocol.
package carddav

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"gorm.io/gorm"

	"mailez/backend/internal/contacts"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/webdav"
)

const (
	homePath       = "/dav/addressbooks"
	collectionName = "default"
)

// Server is the CardDAV backend bound to the database.
type Server struct {
	DB *gorm.DB
}

// New returns a CardDAV backend for db.
func New(db *gorm.DB) *Server { return &Server{DB: db} }

// kind discriminates the path shapes the backend serves.
type kind int

const (
	kindRoot       kind = iota
	kindHome            // /dav/addressbooks/{user}
	kindCollection      // /dav/addressbooks/{user}/default
	kindResource        // /dav/addressbooks/{user}/default/{uid}.vcf
)

// parsePath resolves a DAV path into its CardDAV kind and user/uid parts.
func parsePath(path string) (k kind, user, uid string, ok bool) {
	decoded, err := url.PathUnescape(path)
	if err != nil {
		return kindRoot, "", "", false
	}
	decoded = strings.TrimSuffix(decoded, "/")
	if decoded == "/dav" {
		return kindRoot, "", "", true
	}
	rest, found := strings.CutPrefix(decoded, homePath)
	if !found {
		return kindRoot, "", "", false
	}
	rest = strings.TrimPrefix(rest, "/")
	if rest == "" {
		return kindRoot, "", "", true
	}
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		return kindHome, "", "", false
	}
	user = parts[0]
	switch len(parts) {
	case 1:
		return kindHome, user, "", true
	case 2:
		if parts[1] != collectionName {
			return kindRoot, "", "", false
		}
		return kindCollection, user, "", true
	case 3:
		if parts[1] != collectionName {
			return kindRoot, "", "", false
		}
		uid = strings.TrimSuffix(parts[2], ".vcf")
		if uid == "" || uid == parts[2] {
			return kindRoot, "", "", false
		}
		return kindResource, user, uid, true
	}
	return kindRoot, "", "", false
}

// addressbookPath builds the collection path for a user.
func addressbookPath(user string) string {
	return fmt.Sprintf("%s/%s/%s", homePath, url.PathEscape(user), collectionName)
}

// homePathOf builds the addressbook home path for a user.
func homePathOf(user string) string {
	return fmt.Sprintf("%s/%s", homePath, url.PathEscape(user))
}

// contactPath builds the resource path for a contact.
func contactPath(user, uid string) string {
	return addressbookPath(user) + "/" + url.PathEscape(uid) + ".vcf"
}

// principalPath builds the principal URL for a user.
func principalPath(user string) string {
	return "/dav/principals/" + url.PathEscape(user) + "/"
}

// etag computes the content ETag for a contact (md5 of its vCard body).
func (s *Server) etag(user string, c *models.Contact) string {
	body := contacts.EncodeVCard([]models.Contact{*c})
	sum := md5.Sum([]byte(body))
	return hex.EncodeToString(sum[:])
}

// ctag computes the collection change tag: hashes the sorted set of
// (uid, etag) pairs so any add/remove/edit changes it.
func (s *Server) ctag(ctx context.Context, user string) (string, error) {
	var list []models.Contact
	if err := s.DB.WithContext(ctx).Where("user_email = ?", user).Find(&list).Error; err != nil {
		return "", err
	}
	sort.Slice(list, func(i, j int) bool { return list[i].DavUID < list[j].DavUID })
	var b strings.Builder
	for i := range list {
		b.WriteString(list[i].DavUID)
		b.WriteString(":")
		b.WriteString(s.etag(user, &list[i]))
		b.WriteString("\n")
	}
	sum := md5.Sum([]byte(b.String()))
	return hex.EncodeToString(sum[:]), nil
}

// lookup finds a contact by DAV uid, ensuring one is assigned when absent.
func (s *Server) lookup(ctx context.Context, user, uid string) (*models.Contact, error) {
	var c models.Contact
	if err := s.DB.WithContext(ctx).Where("user_email = ? AND dav_uid = ?", user, uid).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
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
				{XMLName: xml.Name{Space: webdav.NSDAV, Local: "current-user-principal"}, Value: webdav.Href(principalPath(user))},
			},
		}}, nil
	case kindHome:
		resp := webdav.Response{
			Href: homePathOf(user) + "/",
			Props: []webdav.Prop{
				{XMLName: xml.Name{Space: webdav.NSDAV, Local: "resourcetype"}, Value: webdav.Empty(xml.Name{Space: webdav.NSDAV, Local: "collection"})},
				{XMLName: xml.Name{Space: webdav.NSDAV, Local: "current-user-principal"}, Value: webdav.Href(principalPath(user))},
				{XMLName: xml.Name{Space: webdav.NSCardDAV, Local: "addressbook-home-set"}, Value: webdav.Href(homePathOf(user) + "/")},
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
		c, err := s.lookup(ctx, user, uid)
		if err != nil {
			return nil, webdav.HTTPError(404, "contact not found")
		}
		body := contacts.EncodeVCard([]models.Contact{*c})
		return []webdav.Response{{
			Href: contactPath(user, uid),
			Props: []webdav.Prop{
				{XMLName: xml.Name{Space: webdav.NSDAV, Local: "getetag"}, Value: webdav.Text(s.etag(user, c))},
				{XMLName: xml.Name{Space: webdav.NSDAV, Local: "getcontenttype"}, Value: webdav.Text("text/vcard; charset=utf-8")},
				{XMLName: xml.Name{Space: webdav.NSDAV, Local: "getcontentlength"}, Value: webdav.Text(fmt.Sprint(len(body)))},
			},
		}}, nil
	}
	return nil, webdav.HTTPError(404, "not found")
}

// collectionResponse renders the addressbook collection properties.
func (s *Server) collectionResponse(ctx context.Context, user string) (webdav.Response, error) {
	tag, err := s.ctag(ctx, user)
	if err != nil {
		return webdav.Response{}, err
	}
	supported := webdav.Empty(xml.Name{Space: webdav.NSCardDAV, Local: "addressbook-query"}) +
		webdav.Empty(xml.Name{Space: webdav.NSCardDAV, Local: "addressbook-multiget"})
	supportedReports := "<D:supported-report>" + supported + "</D:supported-report>"
	return webdav.Response{
		Href: addressbookPath(user) + "/",
		Props: []webdav.Prop{
			{XMLName: xml.Name{Space: webdav.NSDAV, Local: "resourcetype"}, Value: webdav.Empty(xml.Name{Space: webdav.NSDAV, Local: "collection"}) + webdav.Empty(xml.Name{Space: webdav.NSCardDAV, Local: "addressbook"})},
			{XMLName: xml.Name{Space: webdav.NSDAV, Local: "displayname"}, Value: webdav.Text("Default Address Book")},
			{XMLName: xml.Name{Space: webdav.NSDAV, Local: "getctag"}, Value: webdav.Text(tag)},
			{XMLName: xml.Name{Space: webdav.NSDAV, Local: "getetag"}, Value: webdav.Text(tag)},
			{XMLName: xml.Name{Space: webdav.NSDAV, Local: "supported-report-set"}, Value: supportedReports},
		},
	}, nil
}

// collectionWithChildren lists the collection plus every contact resource.
func (s *Server) collectionWithChildren(ctx context.Context, user string) ([]webdav.Response, error) {
	col, err := s.collectionResponse(ctx, user)
	if err != nil {
		return nil, err
	}
	var list []models.Contact
	if err := s.DB.WithContext(ctx).Where("user_email = ?", user).Order("dav_uid").Find(&list).Error; err != nil {
		return nil, err
	}
	out := []webdav.Response{col}
	for i := range list {
		if list[i].DavUID == "" {
			continue
		}
		out = append(out, s.contactResponse(user, &list[i]))
	}
	return out, nil
}

func (s *Server) contactResponse(user string, c *models.Contact) webdav.Response {
	body := contacts.EncodeVCard([]models.Contact{*c})
	return webdav.Response{
		Href: contactPath(user, c.DavUID),
		Props: []webdav.Prop{
			{XMLName: xml.Name{Space: webdav.NSDAV, Local: "getetag"}, Value: webdav.Text(s.etag(user, c))},
			{XMLName: xml.Name{Space: webdav.NSDAV, Local: "getcontenttype"}, Value: webdav.Text("text/vcard; charset=utf-8")},
			{XMLName: xml.Name{Space: webdav.NSDAV, Local: "getcontentlength"}, Value: webdav.Text(fmt.Sprint(len(body)))},
			{XMLName: xml.Name{Space: webdav.NSCardDAV, Local: "address-data"}, Value: webdav.Text(body)},
		},
	}
}

// Get implements webdav.Backend.
func (s *Server) Get(ctx context.Context, path string) ([]byte, string, string, error) {
	k, user, uid, ok := parsePath(path)
	if !ok || k != kindResource {
		return nil, "", "", webdav.HTTPError(404, "contact not found")
	}
	c, err := s.lookup(ctx, user, uid)
	if err != nil {
		return nil, "", "", webdav.HTTPError(404, "contact not found")
	}
	body := contacts.EncodeVCard([]models.Contact{*c})
	return []byte(body), "text/vcard; charset=utf-8", s.etag(user, c), nil
}

// Put implements webdav.Backend.
func (s *Server) Put(ctx context.Context, path string, body []byte, ifMatch, ifNoneMatch string) (string, bool, error) {
	k, user, uid, ok := parsePath(path)
	if !ok || k != kindResource {
		return "", false, webdav.HTTPError(404, "invalid address-book resource path")
	}
	parsed := contacts.ParseVCard(string(body))
	if len(parsed) == 0 {
		return "", false, webdav.HTTPError(400, "invalid vCard body")
	}
	p := parsed[0]
	if p.UID == "" {
		p.UID = uid
	}
	if p.UID == "" {
		return "", false, webdav.HTTPError(400, "vCard is missing UID")
	}
	var c models.Contact
	err := s.DB.WithContext(ctx).Where("user_email = ? AND dav_uid = ?", user, p.UID).First(&c).Error
	exists := err == nil
	if err != nil && err != gorm.ErrRecordNotFound {
		return "", false, err
	}
	if exists {
		current := s.etag(user, &c)
		if ifMatch != "" && !webdav.ETagMatches(ifMatch, current) {
			return "", false, webdav.HTTPError(412, "precondition failed")
		}
		if ifNoneMatch != "" && webdav.ETagMatches(ifNoneMatch, current) {
			return "", false, webdav.HTTPError(412, "resource already exists")
		}
	} else if ifMatch != "" {
		return "", false, webdav.HTTPError(412, "precondition failed")
	}

	c.UserEmail = user
	c.DavUID = p.UID
	c.Name = p.Name
	c.Email = p.Email
	c.Comment = p.Comment
	c.Groups = p.Groups
	if p.Avatar != "" {
		c.Avatar = p.Avatar
	}
	c.DavRev++
	if err := s.DB.WithContext(ctx).Save(&c).Error; err != nil {
		return "", false, err
	}
	return s.etag(user, &c), !exists, nil
}

// Delete implements webdav.Backend.
func (s *Server) Delete(ctx context.Context, path string) error {
	k, user, uid, ok := parsePath(path)
	if !ok || k != kindResource {
		return webdav.HTTPError(404, "contact not found")
	}
	res := s.DB.WithContext(ctx).Where("user_email = ? AND dav_uid = ?", user, uid).Delete(&models.Contact{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return webdav.HTTPError(404, "contact not found")
	}
	return nil
}

// Report implements webdav.Backend (addressbook-query and multiget).
func (s *Server) Report(ctx context.Context, path string, name xml.Name, body []byte) ([]webdav.Response, error) {
	k, user, _, ok := parsePath(path)
	if !ok || k != kindCollection {
		return nil, webdav.HTTPError(404, "not found")
	}
	var list []models.Contact
	if err := s.DB.WithContext(ctx).Where("user_email = ?", user).Order("dav_uid").Find(&list).Error; err != nil {
		return nil, err
	}
	switch name.Local {
	case "addressbook-query":
		var out []webdav.Response
		for i := range list {
			if list[i].DavUID == "" {
				continue
			}
			out = append(out, s.contactResponse(user, &list[i]))
		}
		return out, nil
	case "addressbook-multiget":
		want := multigetHrefs(body)
		byUID := map[string]*models.Contact{}
		for i := range list {
			byUID[list[i].DavUID] = &list[i]
		}
		var out []webdav.Response
		for _, h := range want {
			uid := resourceUID(h)
			if c, ok2 := byUID[uid]; ok2 {
				out = append(out, s.contactResponse(user, c))
			} else {
				out = append(out, webdav.Response{Href: h, Status: 404})
			}
		}
		return out, nil
	}
	return nil, webdav.HTTPError(400, "unsupported report")
}

// multigetHrefs extracts the <D:href> entries from a multiget report body.
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

// resourceUID extracts the uid from a contact resource href.
func resourceUID(href string) string {
	decoded, err := url.PathUnescape(href)
	if err != nil {
		decoded = href
	}
	base := decoded
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	base = strings.TrimSuffix(base, ".vcf")
	return base
}
