//go:build mailez_ee

// Enterprise-edition drive share quota: unlimited active links.
package drive

// driveShareQuota is the per-edition share-link quota policy.
type driveShareQuota struct {
	// maxShareTokens limits the number of active share links per account;
	// 0 means unlimited.
	maxShareTokens int
}

func (s *Service) shareQuota() driveShareQuota {
	return driveShareQuota{}
}
