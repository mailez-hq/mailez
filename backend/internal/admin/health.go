// Domain health center: live DNS and service checks in the style of
// Mail-in-a-Box's status page. Every check renders a stable id plus a status
// (ok/warn/fail/unknown) and a technical detail string; the admin console
// maps ids to localized labels and fix hints.
package admin

import (
	"context"
	"crypto/tls"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core/models"
	"mailez/backend/internal/dnscheck"
	"mailez/backend/internal/domain"
)

// Check statuses.
const (
	statusOK      = "ok"
	statusWarn    = "warn"
	statusFail    = "fail"
	statusUnknown = "unknown"
)

// CheckItem is one health-check result. Detail carries observed values
// (record contents, addresses, percentages) — never user-facing copy.
type CheckItem struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// DomainReport groups the per-domain DNS checks.
type DomainReport struct {
	Domain string      `json:"domain"`
	Items  []CheckItem `json:"items"`
}

// HealthReport is the /admin/health payload.
type HealthReport struct {
	CheckedAt time.Time      `json:"checked_at"`
	Hostname  string         `json:"hostname"`
	Domains   []DomainReport `json:"domains"`
	System    []CheckItem    `json:"system"`
}

func ok(id, detail string) CheckItem    { return CheckItem{id, statusOK, detail} }
func warn(id, detail string) CheckItem  { return CheckItem{id, statusWarn, detail} }
func fail(id, detail string) CheckItem  { return CheckItem{id, statusFail, detail} }
func unknown(id, detail string) CheckItem {
	return CheckItem{id, statusUnknown, detail}
}

// registerHealth mounts the health-check endpoint.
func (h *Handler) registerHealth(r fiber.Router, mw fiber.Handler) {
	r.Get("/admin/health", mw, h.healthReport)
}

// healthReport runs the domain and system checks.
// @Summary Domain & system health
// @Tags admin
// @Produce json
// @Success 200 {object} HealthReport
// @Router /admin/health [get]
func (h *Handler) healthReport(c *fiber.Ctx) error {
	// DNS probes carry their own timeouts; a fresh context keeps them
	// independent of the request lifecycle so parallel domains finish
	// together instead of serially.
	ctx := context.Background()
	var domains []models.Domain
	if err := h.DB.WithContext(ctx).Order("name").Find(&domains).Error; err != nil {
		return coreFail(c, err, "load domains failed")
	}
	reports := make([]DomainReport, len(domains))
	var wg sync.WaitGroup
	for i := range domains {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			reports[i] = h.checkDomain(ctx, domains[i])
		}(i)
	}
	wg.Wait()
	if len(reports) == 0 {
		reports = nil
	}
	return c.JSON(HealthReport{
		CheckedAt: time.Now().UTC(),
		Hostname:  h.Cfg.Hostname,
		Domains:   reports,
		System:    h.systemChecks(),
	})
}

// checkDomain runs all DNS checks for one mail domain.
func (h *Handler) checkDomain(ctx context.Context, d models.Domain) DomainReport {
	host := h.Cfg.Hostname
	items := []CheckItem{
		checkMX(ctx, d.Name, host),
		checkSPF(ctx, d.Name, host),
		checkDMARC(ctx, d.Name),
		checkDKIM(ctx, d, h.Cfg.DkimSelector),
		checkBlacklist(ctx, host),
		checkHosted(ctx, "autoconfig."+d.Name, "autoconfig"),
		checkHosted(ctx, "mta-sts."+d.Name, "mta_sts"),
	}
	return DomainReport{Domain: d.Name, Items: items}
}

// checkMX verifies the domain's MX points at this server.
func checkMX(ctx context.Context, domain, hostname string) CheckItem {
	mxs, err := dnscheck.MX(ctx, domain)
	if err != nil {
		if dnscheck.IsNotFound(err) {
			return fail("mx", "no MX record")
		}
		return unknown("mx", err.Error())
	}
	for _, mx := range mxs {
		if strings.EqualFold(mx, hostname) {
			return ok("mx", strings.Join(mxs, ", "))
		}
	}
	// MX exists but points elsewhere: legitimate behind a gateway/relay,
	// otherwise inbound mail never arrives here.
	return warn("mx", strings.Join(mxs, ", "))
}

// checkSPF verifies a v=spf1 record exists and references this server.
func checkSPF(ctx context.Context, domain, hostname string) CheckItem {
	txts, err := dnscheck.TXT(ctx, domain)
	if err != nil {
		if dnscheck.IsNotFound(err) {
			return fail("spf", "no TXT record")
		}
		return unknown("spf", err.Error())
	}
	for _, txt := range txts {
		n := dnscheck.NormalizeTXT(txt)
		if !strings.HasPrefix(n, "v=spf1") {
			continue
		}
		references := strings.Contains(n, strings.ToLower(hostname)) ||
			strings.Contains(n, "mx")
		if !references {
			return warn("spf", txt)
		}
		if !strings.Contains(n, "-all") {
			// Soft policy: spoofing is possible but not blocked.
			return warn("spf", txt)
		}
		return ok("spf", txt)
	}
	return fail("spf", "no v=spf1 record")
}

// checkDMARC verifies a DMARC policy record and reports its enforcement.
func checkDMARC(ctx context.Context, domain string) CheckItem {
	txts, err := dnscheck.TXT(ctx, "_dmarc."+domain)
	if err != nil {
		if dnscheck.IsNotFound(err) {
			return warn("dmarc", "no DMARC record")
		}
		return unknown("dmarc", err.Error())
	}
	for _, txt := range txts {
		n := dnscheck.NormalizeTXT(txt)
		if !strings.HasPrefix(n, "v=dmarc1") {
			continue
		}
		switch {
		case strings.Contains(n, "p=reject"):
			return ok("dmarc", txt)
		case strings.Contains(n, "p=quarantine"):
			return ok("dmarc", txt)
		default:
			// p=none (or absent): monitoring only, no enforcement.
			return warn("dmarc", txt)
		}
	}
	return warn("dmarc", "no v=DMARC1 record")
}

// checkDKIM compares the published selector TXT with the server's key.
func checkDKIM(ctx context.Context, d models.Domain, selector string) CheckItem {
	if d.DkimKey == "" {
		return warn("dkim", "key not generated")
	}
	expected := dnscheck.PValue(domain.DkimPublicKeyTXT(d.DkimKey))
	if expected == "" {
		return warn("dkim", "stored key unusable")
	}
	name := selector + "._domainkey." + d.Name
	txts, err := dnscheck.TXT(ctx, name)
	if err != nil {
		if dnscheck.IsNotFound(err) {
			return fail("dkim", "record not published: "+name)
		}
		return unknown("dkim", err.Error())
	}
	for _, txt := range txts {
		if got := dnscheck.PValue(txt); got != "" {
			if got == expected {
				return ok("dkim", name)
			}
			return fail("dkim", "published key mismatch: "+name)
		}
	}
	return fail("dkim", "record without p= tag: "+name)
}

// checkBlacklist queries Spamhaus ZEN for every IPv4 address of the mail
// host. Queries through public resolvers are refused by Spamhaus; that maps
// to unknown rather than a false pass.
func checkBlacklist(ctx context.Context, hostname string) CheckItem {
	addrs, err := dnscheck.Hosts(ctx, hostname)
	if err != nil || len(addrs) == 0 {
		return unknown("blacklist", "cannot resolve "+hostname)
	}
	var ipv4 []string
	for _, a := range addrs {
		if ip := net.ParseIP(a); ip != nil && ip.To4() != nil {
			ipv4 = append(ipv4, a)
		}
	}
	if len(ipv4) == 0 {
		return unknown("blacklist", "no IPv4 address")
	}
	clean := 0
	for _, a := range ipv4 {
		rev := reverseIP(a) + ".zen.spamhaus.org"
		ans, err := dnscheck.Hosts(ctx, rev)
		switch {
		case err == nil && len(ans) > 0:
			return fail("blacklist", a+" listed ("+strings.Join(ans, ", ")+")")
		case err == nil || dnscheck.IsNotFound(err):
			clean++
		default:
			return unknown("blacklist", "Spamhaus query refused (public resolver?)")
		}
	}
	if clean == len(ipv4) {
		return ok("blacklist", strings.Join(ipv4, ", "))
	}
	return unknown("blacklist", "inconclusive")
}

// checkHosted verifies that a service hostname (autoconfig./mta-sts.)
// resolves at all; pointing it at this server is documented in the wizard.
func checkHosted(ctx context.Context, name, id string) CheckItem {
	if target, err := dnscheck.CNAME(ctx, name); err == nil && target != "" {
		return ok(id, "CNAME "+target)
	}
	if addrs, err := dnscheck.Hosts(ctx, name); err == nil && len(addrs) > 0 {
		return ok(id, strings.Join(addrs, ", "))
	}
	return warn(id, "not published")
}

// systemChecks probes local services and resources.
func (h *Handler) systemChecks() []CheckItem {
	items := []CheckItem{
		tcpCheck("engine_imap", h.Cfg.MailImapAddr),
		tcpCheck("engine_mta", h.Cfg.MailMtaAddr),
		tcpCheck("redis", h.Cfg.RedisAddr),
	}
	var one int
	if err := h.DB.Raw("SELECT 1").Scan(&one).Error; err != nil {
		items = append(items, fail("database", err.Error()))
	} else {
		items = append(items, ok("database", h.Cfg.DBDriver))
	}
	items = append(items, diskCheck(h.Cfg.UploadDir), memCheck())
	if host := h.Cfg.Hostname; host != "" && host != "localhost" && host != "127.0.0.1" {
		items = append(items, certCheck(host, h.Cfg.PublicImapPort))
	}
	return items
}

func tcpCheck(id, addr string) CheckItem {
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return fail(id, addr+": "+err.Error())
	}
	_ = conn.Close()
	return ok(id, addr)
}

// certCheck reports the remaining validity of the public TLS certificate.
func certCheck(host string, port int) CheckItem {
	d := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := tls.DialWithDialer(d, "tcp", net.JoinHostPort(host, itoa(port)), &tls.Config{
		ServerName: host,
	})
	if err != nil {
		return unknown("cert", err.Error())
	}
	_ = conn.Close()
	cert := conn.ConnectionState().PeerCertificates[0]
	days := int(time.Until(cert.NotAfter).Hours() / 24)
	if days < 0 {
		return fail("cert", "expired "+itoa(-days)+"d ago: "+cert.Subject.String())
	}
	if days <= 14 {
		return warn("cert", "expires in "+itoa(days)+"d")
	}
	return ok("cert", "expires in "+itoa(days)+"d")
}

func reverseIP(s string) string {
	parts := strings.Split(s, ".")
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, ".")
}

func itoa(n int) string { return strconv.Itoa(n) }

// coreFail adapts the error path for this file's handlers.
func coreFail(c *fiber.Ctx, err error, msg string) error {
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": msg, "detail": err.Error()})
}
