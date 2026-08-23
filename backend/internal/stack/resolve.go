package stack

import (
	"strings"

	"mailez/backend/internal/core/models"
)

// resolveDomain splits an address into localpart and real domain, following
// alternative (domain alias) resolution. For a bare domain it returns the
// resolved domain with an empty localpart.
func (h *Handler) resolveDomain(value string) (localpart, domain string, ok bool) {
	at := strings.LastIndex(value, "@")
	if at < 0 {
		return "", h.realDomain(value), true
	}
	localpart = value[:at]
	domain = h.realDomain(value[at+1:])
	return localpart, domain, true
}

// realDomain maps an alternative name to its canonical domain.
func (h *Handler) realDomain(domain string) string {
	var alt models.Alternative
	if err := h.DB.Where("name IN ?", domainCandidates(domain)).First(&alt).Error; err == nil {
		return alt.DomainName
	}
	return domain
}

// resolveDestination computes delivery targets for localpart@domain, mirroring
// the the mail stack's Email.resolve_destination: users (with forwarding), then aliases
// (exact, wildcard, recipient-delimiter aware).
func (h *Handler) resolveDestination(localpart, domain string, ignoreForwardKeep bool) []string {
	stripped := h.stripDelimiter(localpart)

	if user := h.findUser(localpart + "@" + domain); user != nil {
		return h.userDestinations(user, localpart+"@"+domain, ignoreForwardKeep)
	}
	if stripped != "" && stripped != localpart {
		if user := h.findUser(stripped + "@" + domain); user != nil {
			return h.userDestinations(user, stripped+"@"+domain, ignoreForwardKeep)
		}
	}

	if pure := h.resolveAlias(localpart, domain); pure != nil {
		if !pure.Wildcard {
			return pure.Destinations()
		}
	}
	if stripped != "" && stripped != localpart {
		if s := h.resolveAlias(stripped, domain); s != nil {
			if s.Wildcard {
				return s.Destinations()
			}
			// Re-attach the delimiter detail for explicit aliases, mirroring
			// postfix' propagate_unmatched_extensions.
			detail := localpart[len(stripped):]
			return appendDetail(s.Destinations(), detail)
		}
	}
	if pure := h.resolveAlias(localpart, domain); pure != nil {
		return pure.Destinations()
	}
	return nil
}

func (h *Handler) stripDelimiter(localpart string) string {
	if h.Cfg.RecipientDelimiter == "" {
		return ""
	}
	if i := strings.IndexAny(localpart, h.Cfg.RecipientDelimiter); i >= 0 {
		return localpart[:i]
	}
	return ""
}

func (h *Handler) userDestinations(u *models.User, email string, ignoreForwardKeep bool) []string {
	if !u.ForwardEnabled {
		return []string{email}
	}
	dest := splitCSV(u.ForwardDestination)
	if u.ForwardKeep || ignoreForwardKeep {
		dest = append(dest, email)
	}
	return dest
}

// resolveAlias finds a matching active alias: not disabled, owner enabled,
// exact (case-insensitive) before wildcard (SQL LIKE semantics on the stored
// localpart pattern).
func (h *Handler) resolveAlias(localpart, domain string) *models.Alias {
	var aliases []models.Alias
	if err := h.DB.Where("domain_name = ? AND disabled = ?", domain, false).Find(&aliases).Error; err != nil {
		return nil
	}
	var active []models.Alias
	for i := range aliases {
		a := &aliases[i]
		if a.OwnerEmail != "" {
			var owner models.User
			if err := h.DB.First(&owner, "email = ?", a.OwnerEmail).Error; err != nil || !owner.Enabled {
				continue
			}
		}
		active = append(active, *a)
	}
	for i := range active {
		a := &active[i]
		if !a.Wildcard && strings.EqualFold(a.Localpart, localpart) {
			return a
		}
	}
	for i := range active {
		a := &active[i]
		if a.Wildcard && matchWildcard(a.Localpart, localpart) {
			return a
		}
	}
	return nil
}

// matchWildcard treats the stored pattern as an SQL LIKE pattern (leading *).
func matchWildcard(pattern, value string) bool {
	if strings.HasPrefix(pattern, "*") {
		return strings.HasSuffix(value, strings.TrimPrefix(pattern, "*"))
	}
	return strings.HasSuffix(pattern, "*") && strings.HasPrefix(value, strings.TrimSuffix(pattern, "*"))
}

func (h *Handler) findUser(email string) *models.User {
	var u models.User
	if err := h.DB.First(&u, "email = ?", email).Error; err != nil {
		return nil
	}
	return &u
}

// appendDetail inserts a recipient-delimiter detail into each destination.
func appendDetail(destinations []string, detail string) []string {
	out := make([]string, 0, len(destinations))
	for _, d := range destinations {
		if i := strings.LastIndex(d, "@"); i >= 0 {
			out = append(out, d[:i]+detail+d[i:])
		} else {
			out = append(out, d)
		}
	}
	return out
}

func splitCSV(s string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

