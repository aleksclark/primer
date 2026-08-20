package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"charm.land/fantasy"
	"primer-tasks/internal/verification"
)

func TestScriptedArtifactFixtureAssertsDigestAndAuthorizedDerivativeBytes(t *testing.T) {
	derivative := []byte("authorized derivative bytes")
	sum := sha256.Sum256([]byte("original artifact bytes"))
	model := &scriptedArtifactModel{expectedDigest: hex.EncodeToString(sum[:]), expectedDerivative: derivative, criteria: []verification.ArtifactCriterion{{ID: "shows-work", Required: true}}, accepted: true}
	call := fantasy.Call{Prompt: fantasy.Prompt{fantasy.NewUserMessage("ARTIFACT_DIGEST="+hex.EncodeToString(sum[:]), fantasy.FilePart{Filename: "authorized-derivative", Data: derivative, MediaType: "image/png"})}}
	if _, err := model.Stream(context.Background(), call); err != nil {
		t.Fatal(err)
	}
	negativeDigest := &scriptedArtifactModel{expectedDigest: "wrong", expectedDerivative: derivative, criteria: model.criteria, accepted: true}
	if _, err := negativeDigest.Stream(context.Background(), call); err == nil {
		t.Fatal("fixture accepted a digest mismatch")
	}
	negativeBytes := &scriptedArtifactModel{expectedDigest: hex.EncodeToString(sum[:]), expectedDerivative: []byte("different"), criteria: model.criteria, accepted: true}
	if _, err := negativeBytes.Stream(context.Background(), call); err == nil {
		t.Fatal("fixture accepted derivative bytes that were not authorized")
	}
}

func TestScriptedArtifactFixtureNegativeCriterionIsNotAcceptance(t *testing.T) {
	model := &scriptedArtifactModel{expectedDigest: "digest", expectedDerivative: []byte("bytes"), criteria: []verification.ArtifactCriterion{{ID: "shows-work", Required: true}}, accepted: false}
	stream, err := model.Stream(context.Background(), fantasy.Call{Prompt: fantasy.Prompt{fantasy.NewUserMessage("ARTIFACT_DIGEST=digest", fantasy.FilePart{Data: []byte("bytes")})}})
	if err != nil || stream == nil {
		t.Fatalf("negative fixture stream=%v err=%v", stream, err)
	}
	if model.Provider() != "scripted" || model.Model() != "scripted-fixture" {
		t.Fatal("fixture provenance changed")
	}
}

func TestScriptedArtifactProviderFailureCanBeRetried(t *testing.T) {
	model := &scriptedArtifactModel{fault: "once", expectedDigest: "digest", expectedDerivative: []byte("bytes"), criteria: []verification.ArtifactCriterion{{ID: "shows-work", Required: true}}, accepted: true}
	call := fantasy.Call{Prompt: fantasy.Prompt{fantasy.NewUserMessage("ARTIFACT_DIGEST=digest", fantasy.FilePart{Data: []byte("bytes")})}}
	if _, err := model.Stream(context.Background(), call); err == nil || err.Error() != "scripted artifact provider failure" {
		t.Fatalf("provider failure = %v", err)
	}
	if _, err := model.Stream(context.Background(), call); err != nil {
		t.Fatalf("retry did not reach fixture: %v", err)
	}
}
