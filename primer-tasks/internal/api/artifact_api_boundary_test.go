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

func TestArtifactAPIRejectsUnconfiguredAndMalformedBoundaryRequests(t *testing.T) {
	s := &Server{}
	reserveRec := httptest.NewRecorder()
	s.reserveArtifact(reserveRec, httptest.NewRequest(http.MethodPost, "/student/artifacts/reserve", bytes.NewReader([]byte(`{}`))), uuid.Nil)
	if reserveRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured reserve status=%d body=%s", reserveRec.Code, reserveRec.Body.String())
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
