//go:build !mailez_ee

// Community-edition calendar sharing quota: each account may share its
// calendar with one person, read-only. Unlimited sharing (and read-write
// grants) is an enterprise capability.
package calendar

// shareQuota is the per-edition sharing quota policy.
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
