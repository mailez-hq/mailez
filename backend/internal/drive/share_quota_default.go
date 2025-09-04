// Default drive share quota: up to 3 active share links per account.
// Unlimited links is provided by an optional module.
package drive

// driveShareQuota is the share-link quota policy.
type driveShareQuota struct {
	// maxShareTokens limits the number of active share links per account;
	// 0 means unlimited.
	maxShareTokens int
}

func (s *Service) shareQuota() driveShareQuota {
	return driveShareQuota{maxShareTokens: 3}
}
