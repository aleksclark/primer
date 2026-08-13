package terminal_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/terminal"
)

func TestDigestBytes(t *testing.T) {
	t.Parallel()
	sum := sha256.Sum256([]byte("hello"))
	want := hex.EncodeToString(sum[:])
	assert.Equal(t, want, terminal.DigestBytes([]byte("hello")))
	empty := sha256.Sum256(nil)
	assert.Equal(t, hex.EncodeToString(empty[:]), terminal.DigestBytes(nil))
	assert.NotEqual(t, terminal.DigestBytes([]byte("a")), terminal.DigestBytes([]byte("b")))
}

func TestBuildStructuredEventParsesShellCInvocation(t *testing.T) {
	t.Parallel()
	before := contracts.WorkspaceManifest{Digest: "aa"}
	after := contracts.WorkspaceManifest{Digest: "bb"}

	// /bin/sh -c "ls -la" → parse submitted line into executable/args.
	ev := terminal.BuildStructuredEvent(
		3, "sess-1", ".", "home",
		"/bin/sh", []string{"-c", "ls -la"},
		"ls -la",
		0, "out\n", "err\n",
		"runner/1", "ver/1",
		before, after,
	)
	assert.Equal(t, int64(3), ev.Sequence)
	assert.Equal(t, "sess-1", ev.SessionID)
	assert.Equal(t, "ls", ev.Executable)
	assert.Equal(t, []string{"-la"}, ev.Argv)
	assert.True(t, ev.ArgvAvailable)
	assert.Equal(t, 0, ev.ExitCode)
	assert.True(t, ev.ExitAvailable)
	assert.Equal(t, "aa", ev.ManifestBefore)
	assert.Equal(t, "bb", ev.ManifestAfter)
	assert.Equal(t, contracts.SourceStructured, ev.Source)
	assert.True(t, ev.Structured)
	assert.Equal(t, "out\n", ev.Stdout.Text)
	assert.True(t, ev.Stdout.Trusted)

	// Empty executable with submitted line fills from parse.
	ev2 := terminal.BuildStructuredEvent(
		1, "s", "", "docs",
		"", nil,
		"cat welcome.txt",
		1, "", "nope",
		"", "",
		contracts.WorkspaceManifest{}, contracts.WorkspaceManifest{},
	)
	assert.Equal(t, "cat", ev2.Executable)
	assert.Equal(t, []string{"welcome.txt"}, ev2.Argv)
	assert.True(t, ev2.ArgvAvailable)
	assert.Equal(t, 1, ev2.ExitCode)
	assert.True(t, ev2.CwdAvailable)
	assert.Equal(t, "docs", ev2.CwdAfter)

	// Empty cwdAfter → CwdAvailable false.
	evEmptyCwd := terminal.BuildStructuredEvent(
		9, "s", "", "",
		"true", nil, "true", 0, "", "", "", "",
		contracts.WorkspaceManifest{}, contracts.WorkspaceManifest{},
	)
	assert.False(t, evEmptyCwd.CwdAvailable)

	// Non-sh executable keeps argv but still attaches pipeline from submitted line when parseable.
	ev3 := terminal.BuildStructuredEvent(
		2, "s", ".", ".",
		"grep", []string{"-n", "x"},
		"grep -n x | wc -l",
		0, "1\n", "",
		"r", "v",
		before, after,
	)
	assert.Equal(t, "grep", ev3.Executable)
	assert.Equal(t, []string{"-n", "x"}, ev3.Argv)
	require.NotNil(t, ev3.Pipeline)
}
