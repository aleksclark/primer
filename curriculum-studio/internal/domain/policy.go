package domain

// CollaborationPolicy is deliberately closed. False/true preserve existing
// behavior. Disabling sharing revokes existing grants and blocks new grants, never revoke.
type CollaborationPolicy struct {
	RequireApprovalForPublish bool `json:"requireApprovalForPublish"`
	SharingEnabled            bool `json:"sharingEnabled"`
}

func DefaultCollaborationPolicy() CollaborationPolicy {
	return CollaborationPolicy{SharingEnabled: true}
}
