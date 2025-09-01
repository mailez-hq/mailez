//go:build mailez_ee

// Enterprise-edition calendar sharing quota: unlimited grants, read-write
// allowed.
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
	return shareQuota{}
}
