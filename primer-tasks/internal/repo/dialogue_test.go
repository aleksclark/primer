package repo

import (
	"context"
	"testing"
)

func TestDialoguePolicyRepositoryRequiresBoundDatabase(t *testing.T) {
	ctx := context.Background()
	for _, r := range []*DialogueRepository{nil, {}, NewDialogueRepository(nil)} {
		if err := r.PublishRevisionPolicies(ctx, "tenant", "revision"); err == nil {
			t.Fatal("missing database accepted")
		}
		if _, err := r.RevisionPolicy(ctx, "tenant", "revision", "requirement"); err == nil {
			t.Fatal("missing database read accepted")
		}
	}
}
