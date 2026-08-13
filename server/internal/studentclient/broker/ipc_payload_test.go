package broker

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	studentsync "github.com/aleksclark/primer/server/internal/studentclient/sync"
)

func TestMarshalUnmarshalPayloadAndSyncResult(t *testing.T) {
	t.Parallel()
	raw, err := MarshalPayload(nil)
	require.NoError(t, err)
	assert.Nil(t, raw)

	raw, err = MarshalPayload(map[string]any{"a": 1})
	require.NoError(t, err)

	var dest map[string]any
	require.NoError(t, UnmarshalPayload(Envelope{Payload: raw}, &dest))
	assert.Equal(t, float64(1), dest["a"])

	require.NoError(t, UnmarshalPayload(Envelope{}, &dest))
	require.NoError(t, UnmarshalPayload(Envelope{Payload: []byte("null")}, &dest))

	out := SyncResultFrom(studentsync.Result{
		Status: studentsync.StatusOnline, WorkItems: 2, EventsFlushed: 1,
		Err: errors.New("sync boom"),
	})
	assert.Equal(t, string(studentsync.StatusOnline), out.Status)
	assert.Equal(t, 2, out.WorkItems)
	assert.NotEmpty(t, out.Error)
}
