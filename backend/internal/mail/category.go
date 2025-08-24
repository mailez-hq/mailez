package mail

import (
	"bytes"
	"io"
	"net/mail"
	"strings"

	"github.com/emersion/go-imap"
)

// classifyFetched derives a category for a freshly fetched list row, reading
// the sender and subject from the envelope and the List-Unsubscribe header
// from the separately fetched header section.
func classifyFetched(msg *imap.Message, headerSection *imap.BodySectionName) string {
	from := ""
	subject := ""
	if msg.Envelope != nil {
		subject = msg.Envelope.Subject
		if len(msg.Envelope.From) > 0 {
			a := msg.Envelope.From[0]
			from = a.MailboxName + "@" + a.HostName
		}
	}
	headers := map[string]string{}
	if r := msg.GetBody(headerSection); r != nil {
		if raw, err := io.ReadAll(r); err == nil {
			if hdr, err := mail.ReadMessage(bytes.NewReader(raw)); err == nil {
				headers["List-Unsubscribe"] = hdr.Header.Get("List-Unsubscribe")
			}
		}
	}
	return classifyMessage(from, subject, headers)
}

// Category values assigned by classify() and returned in Message.Category. The
// frontend renders a badge per message using i18n keys category<Value>.
const (
	CategoryWork       = "work"
	CategorySocial     = "social"
	CategoryNewsletter = "newsletter"
	CategoryShopping   = "shopping"
	CategoryFinance    = "finance"
	CategoryOther      = "other"
)

// Rule-based categorization, matching the deterministic tier of
// auto-sorting: List-Unsubscribe marks newsletters, then known sender domains
// (with a couple of subject keywords) cover shopping/finance/work/social, and
// personal mailbox domains fall through to social. Everything else is other.
func classifyMessage(fromEmail, subject string, headers map[string]string) string {
	domain := fromEmail
	if i := strings.LastIndex(fromEmail, "@"); i >= 0 {
		domain = strings.ToLower(fromEmail[i+1:])
	}
	lowSubject := strings.ToLower(subject)

	// Newsletters advertise a working List-Unsubscribe (RFC 2369); treat any
	// such message as a newsletter regardless of sender.
	if hasListUnsubscribe(headers) {
		return CategoryNewsletter
	}

	switch {
	case financeDomains[domain] || containsAny(lowSubject, []string{"statement", "invoice", "receipt", "payment", "bank", "tax", "salary", "payslip"}):
		return CategoryFinance
	case shoppingDomains[domain] || containsAny(lowSubject, []string{"order confirmation", "shipping", "delivery", "tracking", "your order", "package"}):
		return CategoryShopping
	case workDomains[domain] || containsAny(lowSubject, []string{"calendar invitation", "meeting", "pull request", "code review", "deployment", "incident", "build failed"}):
		return CategoryWork
	case socialDomains[domain] || consumerMailDomains[domain]:
		return CategorySocial
	default:
		return CategoryOther
	}
}

func hasListUnsubscribe(headers map[string]string) bool {
	v, ok := headers["List-Unsubscribe"]
	return ok && strings.Contains(v, ":")
}

func containsAny(s string, words []string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// shoppingDomains: major e-commerce marketplaces.
var shoppingDomains = map[string]bool{
	"amazon.com": true, "amazon.co.uk": true, "amazon.co.jp": true, "amazon.de": true,
	"amazon.cn": true, "jd.com": true, "taobao.com": true, "tmall.com": true,
	"ebay.com": true, "etsy.com": true, "aliexpress.com": true, "shopify.com": true,
	"walmart.com": true, "target.com": true, "bestbuy.com": true, "homedepot.com": true,
	"ikea.com": true, "zara.com": true, "uniqlo.com": true, "nike.com": true,
	"steam.com": true, "epicgames.com": true, "bilibili.com": true, "pinduoduo.com": true,
}

// financeDomains: banks, payment providers and billers.
var financeDomains = map[string]bool{
	"paypal.com": true, "stripe.com": true, "square.com": true, "wise.com": true,
	"revolut.com": true, "chase.com": true, "bankofamerica.com": true, "wellsfargo.com": true,
	"citi.com": true, "capitalone.com": true, "americanexpress.com": true,
	"hsbc.com": true, "icbc.com.cn": true, "ccb.com": true, "citicbank.com": true,
	"alipay.com": true, "wechatpay.com": true, "pingan.com": true, "cmbchina.com": true,
	"securities.com": true, "fidelity.com": true, "vanguard.com": true, "schwab.com": true,
	"robinhood.com": true, "coinbase.com": true, "binance.com": true,
}

// workDomains: SaaS and developer tools whose mail is work-related.
var workDomains = map[string]bool{
	"github.com": true, "gitlab.com": true, "bitbucket.org": true, "slack.com": true,
	"atlassian.com": true, "jira.com": true, "confluence.com": true, "trello.com": true,
	"asana.com": true, "notion.so": true, "linear.app": true, "zoom.us": true,
	"microsoft.com": true, "office.com": true, "google.com": true, "cloud.google.com": true,
	"amazonaws.com": true, "azure.com": true, "sentry.io": true, "datadoghq.com": true,
	"okta.com": true, "1password.com": true, "workday.com": true, "adp.com": true,
	"salesforce.com": true, "hubspot.com": true, "zendesk.com": true, "figma.com": true,
	"vercel.com": true, "netlify.com": true, "cloudflare.com": true, "digitalocean.com": true,
}

// socialDomains: social networks and forums.
var socialDomains = map[string]bool{
	"facebook.com": true, "fb.com": true, "instagram.com": true, "x.com": true,
	"twitter.com": true, "linkedin.com": true, "reddit.com": true, "pinterest.com": true,
	"tiktok.com": true, "youtube.com": true, "twitch.tv": true, "discord.com": true,
	"weibo.com": true, "zhihu.com": true, "douban.com": true, "xiaohongshu.com": true,
	"bilibili.com": true, "quora.com": true, "medium.com": true, "substack.com": true,
}

// consumerMailDomains: free personal mailboxes → social by default.
var consumerMailDomains = map[string]bool{
	"gmail.com": true, "googlemail.com": true, "outlook.com": true, "hotmail.com": true,
	"live.com": true, "yahoo.com": true, "icloud.com": true, "me.com": true,
	"qq.com": true, "163.com": true, "126.com": true, "foxmail.com": true,
	"proton.me": true, "protonmail.com": true, "zoho.com": true, "aol.com": true,
}
