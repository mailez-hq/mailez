// Calendar sharing quota of this build: each account may share its
// calendar with one person, read-only.
package calendar

// shareQuota is the sharing quota policy.
type shareQuota struct {
	// maxOwned limits how many grants an account may share out; 0 means
	// unlimited.
	maxOwned int
	// forceReadOnly pins grants to read-only.
	forceReadOnly bool
}

func (h *Handler) shareQuota() shareQuota {
	return shareQuota{maxOwned: 1, forceReadOnly: true}
}
