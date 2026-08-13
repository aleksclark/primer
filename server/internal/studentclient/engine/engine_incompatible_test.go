package engine

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestIsIncompatibleRevisionError(t *testing.T) {
	t.Parallel()

	assert.False(t, isIncompatibleRevisionError(nil))
	assert.False(t, isIncompatibleRevisionError(errors.New("network down")))
	assert.False(t, isIncompatibleRevisionError(&studentapi.ErrHTTP{
		StatusCode: http.StatusBadRequest,
		Body:       "invalid assignment id",
	}))
	// Non-400 without the substring in Error() stays false.
	assert.False(t, isIncompatibleRevisionError(&studentapi.ErrHTTP{
		StatusCode: http.StatusInternalServerError,
		Body:       "internal error",
	}))

	assert.True(t, isIncompatibleRevisionError(contracts.ErrIncompatibleRevision{}))
	assert.True(t, isIncompatibleRevisionError(contracts.ErrIncompatibleRevision{Msg: "custom"}))
	assert.True(t, isIncompatibleRevisionError(fmt.Errorf("wrap: %w", contracts.ErrIncompatibleRevision{
		Msg: "runner lacks structured_command_evidence",
	})))

	assert.True(t, isIncompatibleRevisionError(&studentapi.ErrHTTP{
		StatusCode: http.StatusBadRequest,
		Body:       `{"error":"structured_command_evidence required"}`,
	}))
	// Any error whose message mentions the capability is treated as incompatible.
	assert.True(t, isIncompatibleRevisionError(errors.New("assignment rejected: structured_command_evidence")))
	assert.True(t, isIncompatibleRevisionError(&studentapi.ErrHTTP{
		StatusCode: http.StatusInternalServerError,
		Body:       "structured_command_evidence missing",
	}))
}
