// Domain DNS wizard: the exact records a deployment must publish for a mail
// domain, each paired with a live verification result so the admin console
// can render "configured / missing / mismatch" per record.
package domain

import (
	"context"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core/models"
	"mailez/backend/internal/dnscheck"
)

// Wizard record statuses: configured correctly, absent, present but with a
// different value, or the probe itself failed.
const (
	wizOK       = "ok"
	wizMissing  = "missing"
	wizMismatch = "mismatch"
	wizUnknown  = "unknown"
)

// DNSRecord is one expected DNS record plus its live status.
type DNSRecord struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Name   string `json:"name"`
	Value  string `json:"value"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// DNSWizardResponse is the /domains/:name/dns-records payload.
type DNSWizardResponse struct {
	Domain   string      `json:"domain"`
	Hostname string      `json:"hostname"`
	Records  []DNSRecord `json:"records"`
}

func (h *Handler) registerDnsWizard(r fiber.Router, mw fiber.Handler) {
	r.Get("/domains/:name/dns-records", mw, h.dnsRecords)
}

// dnsRecords returns the expected DNS records for a domain with live checks.
// @Summary Domain DNS wizard
// @Tags domains
// @Produce json
// @Param name path string true "domain name"
// @Success 200 {object} DNSWizardResponse
// @Router /domains/{name}/dns-records [get]
func (h *Handler) dnsRecords(c *fiber.Ctx) error {
	d, err := h.findDomain(c.Params("name"))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "domain not found"})
	}
	ctx := context.Background()
	host := h.Cfg.Hostname
	records := []DNSRecord{
		{
			ID: "mx", Type: "MX", Name: "@",
			Value:  "10 " + host,
			Status: checkWizardMX(ctx, d.Name, host),
		},
		{
			ID: "spf", Type: "TXT", Name: "@",
			Value:  "v=spf1 mx -all",
			Status: checkWizardSPF(ctx, d.Name, host),
		},
		h.dkimWizardRecord(ctx, d),
		{
			ID: "dmarc", Type: "TXT", Name: "_dmarc",
			Value:  "v=DMARC1; p=quarantine; rua=mailto:postmaster@" + d.Name,
			Status: checkWizardTXTPrefix(ctx, "_dmarc."+d.Name, "v=dmarc1"),
		},
		wizardCNAME(ctx, "autoconfig", "autoconfig", d.Name, host),
		wizardCNAME(ctx, "autodiscover", "autodiscover", d.Name, host),
		wizardCNAME(ctx, "mta-sts", "mtasts", d.Name, host),
		{
			ID: "mtasts_txt", Type: "TXT", Name: "_mta-sts",
			Value:  "v=STSv1; id=" + time.Now().UTC().Format("20060102") + "; max_age=86400",
			Status: checkWizardTXTPrefix(ctx, "_mta-sts."+d.Name, "v=stsv1"),
		},
		{
			ID: "tlsrpt", Type: "TXT", Name: "_smtp._tls",
			Value:  "v=TLSRPT; rua=mailto:postmaster@" + d.Name,
			Status: checkWizardTXTPrefix(ctx, "_smtp._tls."+d.Name, "v=tlsrpt"),
		},
	}
	return c.JSON(DNSWizardResponse{Domain: d.Name, Hostname: host, Records: records})
}

// dkimWizardRecord builds the selector TXT from the stored key; without a
// key the record cannot be produced yet (generate first in the domains page).
func (h *Handler) dkimWizardRecord(ctx context.Context, d models.Domain) DNSRecord {
	name := h.Cfg.DkimSelector + "._domainkey"
	if d.DkimKey == "" {
		return DNSRecord{
			ID: "dkim", Type: "TXT", Name: name,
			Value: "", Status: wizMissing, Detail: "dkim key not generated",
		}
	}
	value := DkimPublicKeyTXT(d.DkimKey)
	rec := DNSRecord{ID: "dkim", Type: "TXT", Name: name, Value: value}
	txts, err := dnscheck.TXT(ctx, name+"."+d.Name)
	switch {
	case err != nil && dnscheck.IsNotFound(err):
		rec.Status = wizMissing
	case err != nil:
		rec.Status = wizUnknown
		rec.Detail = err.Error()
	default:
		expected := dnscheck.PValue(value)
		for _, txt := range txts {
			if got := dnscheck.PValue(txt); got != "" {
				if got == expected {
					rec.Status = wizOK
					return rec
				}
				rec.Status = wizMismatch
				rec.Detail = "published key differs"
				return rec
			}
		}
		rec.Status = wizMissing
	}
	return rec
}

func checkWizardMX(ctx context.Context, domain, host string) string {
	mxs, err := dnscheck.MX(ctx, domain)
	if err != nil {
		return wizUnknown
	}
	for _, mx := range mxs {
		if strings.EqualFold(mx, host) {
			return wizOK
		}
	}
	if len(mxs) > 0 {
		return wizMismatch
	}
	return wizMissing
}

func checkWizardSPF(ctx context.Context, domain, host string) string {
	txts, err := dnscheck.TXT(ctx, domain)
	if err != nil {
		return wizUnknown
	}
	for _, txt := range txts {
		n := dnscheck.NormalizeTXT(txt)
		if strings.HasPrefix(n, "v=spf1") {
			if strings.Contains(n, strings.ToLower(host)) || strings.Contains(n, "mx") {
				return wizOK
			}
			return wizMismatch
		}
	}
	return wizMissing
}

func checkWizardTXTPrefix(ctx context.Context, name, prefix string) string {
	txts, err := dnscheck.TXT(ctx, name)
	if err != nil {
		if dnscheck.IsNotFound(err) {
			return wizMissing
		}
		return wizUnknown
	}
	for _, txt := range txts {
		if strings.HasPrefix(dnscheck.NormalizeTXT(txt), prefix) {
			return wizOK
		}
	}
	return wizMissing
}

func wizardCNAME(ctx context.Context, prefix, id, domain, host string) DNSRecord {
	rec := DNSRecord{ID: id, Type: "CNAME", Name: prefix, Value: host}
	target, err := dnscheck.CNAME(ctx, prefix+"."+domain)
	switch {
	case err != nil && dnscheck.IsNotFound(err):
		// A direct A/AAAA record works as well as a CNAME.
		if addrs, aerr := dnscheck.Hosts(ctx, prefix+"."+domain); aerr == nil && len(addrs) > 0 {
			rec.Status = wizOK
			rec.Detail = "A " + strings.Join(addrs, ", ")
		} else {
			rec.Status = wizMissing
		}
	case err != nil:
		rec.Status = wizUnknown
		rec.Detail = err.Error()
	case strings.EqualFold(target, host):
		rec.Status = wizOK
	default:
		rec.Status = wizMismatch
		rec.Detail = "CNAME " + target
	}
	return rec
}
