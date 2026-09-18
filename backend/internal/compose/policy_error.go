package compose

import (
	"regexp"
	"strings"
)

// policyRejection recognises a permanent, policy-level refusal from the mail
// engine (SMTP 55x) so the API can answer with a specific message instead of a
// generic 502. The engine reports these as, for example,
// `smtp close: 554 "5.0.0 message rejected by policy"`.
var policyRejectionRe = regexp.MustCompile(`(?i)\b55[0-4]\b.*?message rejected by`)

// quotaRejectionRe matches the engine's recipient-quota refusal
// ("552 5.2.2 mailbox full").
var quotaRejectionRe = regexp.MustCompile(`(?i)(mailbox full|over ?quota|quota exceeded|exceeds? (the )?quota)`)

// quotaRejection reports a full recipient mailbox.
func quotaRejection(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	if !quotaRejectionRe.MatchString(err.Error()) {
		return "", false
	}
	return "the recipient's mailbox is full; the message was not delivered", true
}

// lineTooLongRejection matches the engine's line-length refusal
// ("554 5.0.0 line too long").
var lineTooLongRejectionRe = regexp.MustCompile(`(?i)line too long`)

func lineTooLongRejection(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	if !lineTooLongRejectionRe.MatchString(err.Error()) {
		return "", false
	}
	return "the message contains a line that is too long for the mail server; add line breaks and try again", true
}

func policyRejection(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	text := err.Error()
	if !policyRejectionRe.MatchString(text) && !strings.Contains(strings.ToLower(text), "rejected by policy") {
		return "", false
	}
	return "the mail server refused this message by policy (attachment type or content was rejected)", true
}

// recipientRejectionRe matches a refusal of the recipient list itself: a bad
// address (501), a mailbox that does not exist (550), or a relay the server
// will not accept mail for (553). The engine reports these as
// `smtp rcpt not-an-email: 501 5.1.3 ...`.
var recipientRejectionRe = regexp.MustCompile(`(?i)\b(50[01]|55[013])\b.*?(rcpt|recipient|address|relay|mailbox)`)

// recipientRejection reports a message that can never be delivered to the
// addresses as written — a client error, not a service outage.
func recipientRejection(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	if !recipientRejectionRe.MatchString(err.Error()) {
		return "", false
	}
	return "the mail server refused at least one recipient address; check the addresses and try again", true
}
