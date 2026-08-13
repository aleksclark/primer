package mastery

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestObservationHasStructuredCommandEvidenceMatrix(t *testing.T) {
	t.Parallel()

	// nil details
	assert.False(t, observationHasStructuredCommandEvidence(contracts.Observation{}))

	// untrusted sources fail closed even with structured flag
	for _, src := range []string{"pty-shell", "synthetic-pty", "screen", "PTY-SHELL"} {
		assert.False(t, observationHasStructuredCommandEvidence(contracts.Observation{
			Details: map[string]any{
				"source":                    src,
				"structuredCommandEvidence": true,
			},
		}), src)
	}

	// trusted sources with boolean
	for _, src := range []string{"structured", "structured_command", "command_instrumentation", "observe-bash", "process-wait", "STRUCTURED"} {
		assert.True(t, observationHasStructuredCommandEvidence(contracts.Observation{
			Details: map[string]any{
				"source":                    src,
				"structuredCommandEvidence": true,
			},
		}), src)
	}

	// boolean true without source fails
	assert.False(t, observationHasStructuredCommandEvidence(contracts.Observation{
		Details: map[string]any{"structuredCommandEvidence": true},
	}))

	// boolean true with unknown source fails
	assert.False(t, observationHasStructuredCommandEvidence(contracts.Observation{
		Details: map[string]any{
			"source":                    "mystery",
			"structuredCommandEvidence": true,
		},
	}))

	// capability alone without source accepted
	assert.True(t, observationHasStructuredCommandEvidence(contracts.Observation{
		Details: map[string]any{"capability": contracts.CapStructuredCommandEvidence},
	}))

	// capability + trusted source
	assert.True(t, observationHasStructuredCommandEvidence(contracts.Observation{
		Details: map[string]any{
			"capability": contracts.CapStructuredCommandEvidence,
			"source":     "observe-bash",
		},
	}))

	// capability + untrusted source fails
	assert.False(t, observationHasStructuredCommandEvidence(contracts.Observation{
		Details: map[string]any{
			"capability": contracts.CapStructuredCommandEvidence,
			"source":     "screen",
		},
	}))

	// capability with unknown source still accepted (legacy)
	assert.True(t, observationHasStructuredCommandEvidence(contracts.Observation{
		Details: map[string]any{
			"capability": contracts.CapStructuredCommandEvidence,
			"source":     "legacy-runner",
		},
	}))

	// source-only trusted without boolean/capability
	assert.True(t, observationHasStructuredCommandEvidence(contracts.Observation{
		Details: map[string]any{"source": "process-wait"},
	}))

	// source-only untrusted / unknown
	assert.False(t, observationHasStructuredCommandEvidence(contracts.Observation{
		Details: map[string]any{"source": "screen"},
	}))
	assert.False(t, observationHasStructuredCommandEvidence(contracts.Observation{
		Details: map[string]any{"source": "other"},
	}))

	// wrong capability value
	assert.False(t, observationHasStructuredCommandEvidence(contracts.Observation{
		Details: map[string]any{"capability": "nope"},
	}))
}
