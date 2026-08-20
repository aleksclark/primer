package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"primer-tasks/internal/artifactstore"
)

type rejectingPutStore struct{ artifactstore.Store }

func (rejectingPutStore) Put(context.Context, string, string, io.Reader, int64) (artifactstore.Object, error) {
	return artifactstore.Object{}, errors.New("test object store rejection")
}

type rejectingDeleteStore struct{ artifactstore.Store }

func (rejectingDeleteStore) Delete(context.Context, string) error {
	return errors.New("test delete rejection")
}

type rejectingComposeStore struct{ artifactstore.Store }

func (rejectingComposeStore) Compose(context.Context, string, string, []string, int64) (artifactstore.Object, error) {
	return artifactstore.Object{}, errors.New("test compose rejection")
}

func TestArtifactMediaPolicyAndStatusMappings(t *testing.T) {
	for _, kind := range []string{"image", "audio", "video"} {
		parsed, err := parseArtifactKind(kind)
		if err != nil || string(parsed) != kind {
			t.Fatalf("parse kind %q = %q, err=%v", kind, parsed, err)
		}
	}
	for _, raw := range []string{"", `{"maxBytes":-1,"maxPixels":-1}`, `{not-json}`} {
		limits := mediaLimits("image", []byte(raw))
		if limits.MaxBytes <= 0 || limits.MaxCount <= 0 {
			t.Fatalf("invalid policy %q produced unusable limits=%+v", raw, limits)
		}
	}
	ms := mediaLimits("video", []byte(`{"maxBytes":123,"maxDurationSeconds":7,"maxPixels":456,"maxCount":2}`))
	if ms.MaxBytes != 123 || ms.MaxDurationMS != 7000 || ms.MaxPixels != 456 || ms.MaxCount != 2 {
		t.Fatalf("video policy=%+v", ms)
	}
	ms = mediaLimits("audio", []byte(`{"maxDurationMs":2500,"maxDurationSeconds":7}`))
	if ms.MaxDurationMS != 2500 {
		t.Fatalf("explicit duration policy was overridden=%+v", ms)
	}
	for status, want := range map[string]string{"submitted": "evaluating", "evaluating": "evaluating", "accepted": "complete", "rejected": "rejected", "review": "review", "reserved": "queued"} {
		if got := artifactSubmissionStatus(status); got != want {
			t.Errorf("status %q=%q, want %q", status, got, want)
		}
	}
	if _, err := parseArtifactKind("document"); err == nil {
		t.Fatal("unsupported artifact kind accepted")
	}
}

func TestArtifactAPIRejectsUnconfiguredAndMalformedBoundaryRequests(t *testing.T) {
	s := &Server{}
	reserveRec := httptest.NewRecorder()
	s.reserveArtifact(reserveRec, httptest.NewRequest(http.MethodPost, "/student/artifacts/reserve", bytes.NewReader([]byte(`{}`))), uuid.Nil)
	if reserveRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured reserve status=%d body=%s", reserveRec.Code, reserveRec.Body.String())
	}
	if _, err := s.reserveArtifactData(context.Background(), uuid.Nil, "occurrence", ArtifactReservationInput{}); err == nil || err.(interface{ GetStatus() int }).GetStatus() != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured typed reserve error=%v", err)
	}

	finalizeRec := httptest.NewRecorder()
	s.finalizeArtifact(finalizeRec, httptest.NewRequest(http.MethodPost, "/student/artifacts/finalize", bytes.NewReader([]byte("not-json"))), uuid.Nil)
	if finalizeRec.Code != http.StatusBadRequest {
		t.Fatalf("malformed finalize status=%d body=%s", finalizeRec.Code, finalizeRec.Body.String())
	}

	parentOriginalRec := httptest.NewRecorder()
	s.parentOriginal(parentOriginalRec, httptest.NewRequest(http.MethodGet, "/parent/artifacts/not-a-uuid/original", nil), scope{Tenant: "not-a-tenant"})
	if parentOriginalRec.Code != http.StatusNotFound {
		t.Fatalf("malformed original status=%d body=%s", parentOriginalRec.Code, parentOriginalRec.Body.String())
	}
	parentDerivativeRec := httptest.NewRecorder()
	s.parentDerivative(parentDerivativeRec, httptest.NewRequest(http.MethodGet, "/occurrences/x/artifacts/not-a-uuid/derivative", nil), scope{Tenant: "not-a-tenant"})
	if parentDerivativeRec.Code != http.StatusNotFound {
		t.Fatalf("malformed derivative status=%d body=%s", parentDerivativeRec.Code, parentDerivativeRec.Body.String())
	}
}
