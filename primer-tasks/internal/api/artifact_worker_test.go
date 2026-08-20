package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/jackc/pgx/v5"
	"primer-tasks/internal/artifactstore"
	"primer-tasks/internal/domain/parent"
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

func TestArtifactWorkerPurePolicyBranches(t *testing.T) {
	if got := maxArtifactSteps(0); got != 4 {
		t.Fatalf("zero criteria steps=%d", got)
	}
	if got := maxArtifactSteps(3); got != 5 {
		t.Fatalf("criteria steps=%d", got)
	}
	for _, provider := range []string{"openrouter", "scripted"} {
		if !artifactProviderSupportsFiles(provider) {
			t.Fatalf("provider %q should support files", provider)
		}
	}
	if artifactProviderSupportsFiles("bedrock") {
		t.Fatal("bedrock must remain file-unqualified")
	}
	model := &scriptedArtifactModel{}
	if _, err := model.Generate(context.Background(), fantasy.Call{}); err == nil {
		t.Fatal("scripted Generate unexpectedly enabled")
	}
	if _, err := model.GenerateObject(context.Background(), fantasy.ObjectCall{}); err == nil {
		t.Fatal("scripted GenerateObject unexpectedly enabled")
	}
	if _, err := model.StreamObject(context.Background(), fantasy.ObjectCall{}); err == nil {
		t.Fatal("scripted StreamObject unexpectedly enabled")
	}
}

func TestArtifactModelScriptedConfigurationAndLiveQualificationFailClosed(t *testing.T) {
	t.Setenv("TASKS_ARTIFACT_SCRIPTED_FIXTURE", "1")
	t.Setenv("TASKS_ARTIFACT_SCRIPTED_DIGEST", "digest")
	model, provider, err := (&Server{}).artifactModel(context.Background(), parent.ProviderConfig{Mode: parent.ProviderScripted}, "digest", []byte("derivative"), verification.ArtifactRubric{Criteria: []verification.ArtifactCriterion{{ID: "criterion"}}})
	if err != nil || provider != "scripted" || model == nil {
		t.Fatalf("scripted model=%v provider=%q err=%v", model, provider, err)
	}
	if _, _, err = (&Server{}).artifactModel(context.Background(), parent.ProviderConfig{Mode: parent.ProviderScripted}, "wrong", nil, verification.ArtifactRubric{}); err == nil {
		t.Fatal("scripted digest mismatch was accepted")
	}
	t.Setenv("TASKS_ARTIFACT_SCRIPTED_FIXTURE", "")
	if _, _, err = (&Server{}).artifactModel(context.Background(), parent.ProviderConfig{}, "digest", nil, verification.ArtifactRubric{}); err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("live qualification err=%v", err)
	}
	t.Setenv("TASKS_ARTIFACT_LIVE_QUALIFICATION", "1")
	t.Setenv("TASKS_ARTIFACT_SCRIPTED_FIXTURE", "")
	liveModel, liveProvider, liveErr := (&Server{}).artifactModel(context.Background(), parent.ProviderConfig{Mode: parent.ProviderScripted}, "digest", nil, verification.ArtifactRubric{})
	if liveErr != nil || liveModel == nil || liveProvider != "scripted" {
		t.Fatalf("scripted live qualification model=%v provider=%q err=%v", liveModel, liveProvider, liveErr)
	}
}

type artifactLeaseRow struct {
	active bool
	err    error
}

func (r artifactLeaseRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*(dest[0].(*bool)) = r.active
	return nil
}

type artifactLeaseDB struct{ row pgx.Row }

func (d artifactLeaseDB) QueryRow(context.Context, string, ...any) pgx.Row { return d.row }

type rejectingArtifactOpenStore struct{ artifactstore.Store }

func (rejectingArtifactOpenStore) Open(context.Context, string) (io.ReadCloser, artifactstore.Object, error) {
	return nil, artifactstore.Object{}, errors.New("test authorized derivative open failure")
}

type failingArtifactReader struct{}

func (failingArtifactReader) Read([]byte) (int, error) {
	return 0, errors.New("test authorized derivative read failure")
}
func (failingArtifactReader) Close() error { return nil }

type rejectingArtifactReadStore struct{ artifactstore.Store }

func (rejectingArtifactReadStore) Open(context.Context, string) (io.ReadCloser, artifactstore.Object, error) {
	return failingArtifactReader{}, artifactstore.Object{}, nil
}

func TestArtifactWorkerLifecycleTicksAndStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{}
	s.StartArtifactWorker(ctx)
	time.Sleep(300 * time.Millisecond)
	cancel()
	time.Sleep(20 * time.Millisecond)
}

func TestArtifactWorkerRequiresDurableConfigurationAndLeaseOwner(t *testing.T) {
	if err := requireArtifactLeaseWithOwner(context.Background(), artifactLeaseDB{row: artifactLeaseRow{active: true}}, "tenant", "job", "owner"); err != nil {
		t.Fatalf("active lease rejected: %v", err)
	}
	if err := requireArtifactLeaseWithOwner(context.Background(), artifactLeaseDB{row: artifactLeaseRow{}}, "tenant", "job", "owner"); !errors.Is(err, errArtifactLeaseLost) {
		t.Fatalf("inactive lease error=%v", err)
	}
	queryErr := errors.New("lease query failed")
	if err := requireArtifactLeaseWithOwner(context.Background(), artifactLeaseDB{row: artifactLeaseRow{err: queryErr}}, "tenant", "job", "owner"); !errors.Is(err, queryErr) {
		t.Fatalf("lease query error=%v", err)
	}
	if err := (&Server{}).runArtifactStep(context.Background()); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("unconfigured worker error=%v", err)
	}
	if err := (&Server{}).requireArtifactLease(context.Background(), "tenant", "job"); err == nil || !strings.Contains(err.Error(), "owner missing") {
		t.Fatalf("missing lease owner error=%v", err)
	}
	if got := artifactPrompt([]byte(`{"criteria":[]}`), "digest"); !strings.Contains(got, "authorized derivative") || !strings.Contains(got, "ARTIFACT_DIGEST=digest") {
		t.Fatalf("worker prompt lost authorization boundary: %q", got)
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
