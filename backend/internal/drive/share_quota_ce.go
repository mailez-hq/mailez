//go:build !mailez_ee

// Community-edition drive share quota: up to 3 active share links per
// account. Unlimited links is an enterprise capability.
package drive

// driveShareQuota is the per-edition share-link quota policy.
type driveShareQuota struct {
	// maxShareTokens limits the number of active share links per account;
	// 0 means unlimited.
	maxShareTokens int
}

func (s *Service) shareQuota() driveShareQuota {
	return driveShareQuota{maxShareTokens: 3}
}
