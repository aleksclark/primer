package contracts_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestSafeRelPathAccepts(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"notes.txt", "notes.txt"},
		{"docs/readme.md", "docs/readme.md"},
		{"./docs/a", "docs/a"},
		{"docs//b", "docs/b"},
	}
	for _, tc := range cases {
		got, err := contracts.SafeRelPath(tc.in)
		require.NoError(t, err, tc.in)
		assert.Equal(t, tc.want, got)
	}
}

func TestSafeRelPathRejects(t *testing.T) {
	t.Parallel()
	bad := []string{
		"",
		"/etc/passwd",
		"../secret",
		"foo/../../etc/passwd",
		`C:\windows`,
		`foo\bar`,
		"~/.ssh/id_rsa",
		"foo\x00bar",
		"..",
		".",
	}
	for _, p := range bad {
		_, err := contracts.SafeRelPath(p)
		require.Error(t, err, p)
		var u *contracts.ErrUnsafePath
		assert.ErrorAs(t, err, &u)
	}
}

func TestJoinUnder(t *testing.T) {
	t.Parallel()
	full, err := contracts.JoinUnder("/tmp/ws", "a/b.txt")
	require.NoError(t, err)
	assert.Equal(t, "/tmp/ws/a/b.txt", full)

	_, err = contracts.JoinUnder("/tmp/ws", "../x")
	require.Error(t, err)

	_, err = contracts.JoinUnder("relative", "a")
	require.Error(t, err)
}
