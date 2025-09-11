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
