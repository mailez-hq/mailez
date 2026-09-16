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
