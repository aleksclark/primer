package stateseal_test

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/stateseal"
)

func fixedSealer(t *testing.T) *stateseal.Sealer {
	t.Helper()
	s, err := stateseal.New(stateseal.Config{ActiveKeyVersion: 7, Keys: map[int][]byte{7: bytes.Repeat([]byte{0x11}, 32), 6: bytes.Repeat([]byte{0x22}, 32)}})
	require.NoError(t, err)
	return s
}

func bindings() stateseal.Bindings {
	return stateseal.Bindings{
		TransactionID: uuid.MustParse("00112233-4455-6677-8899-aabbccddeeff"),
		OAuthClientID: uuid.MustParse("fedcba98-7654-3210-fedc-ba9876543210"),
		RedirectURI:   "https://a.example/callback", ResourceURI: "https://resource.example/x", Audience: "audience",
	}
}

func TestSealRoundTripsExactStateBoundaries(t *testing.T) {
	for _, state := range [][]byte{{0xa5}, bytes.Repeat([]byte{0x5a}, 1024)} {
		s := fixedSealer(t)
		envelope, version, err := s.Seal(state, bindings())
		require.NoError(t, err)
		require.Equal(t, 7, version)
		got, err := s.Open(envelope, version, bindings())
		require.NoError(t, err)
		require.Equal(t, state, got)
	}
}

func TestSealRejectsStateOutsideExactBounds(t *testing.T) {
	s := fixedSealer(t)
	for _, state := range [][]byte{nil, bytes.Repeat([]byte{1}, 1025)} {
		_, _, err := s.Seal(state, bindings())
		require.ErrorIs(t, err, stateseal.ErrInvalid)
	}
}

func TestSealFramingSeparatesOtherwiseCollidingTextFields(t *testing.T) {
	s := fixedSealer(t)
	left := bindings()
	left.RedirectURI, left.ResourceURI = "https://a/b", "c"
	right := bindings()
	right.RedirectURI, right.ResourceURI = "https://a", "/bc"
	leftAAD, err := stateseal.AAD(left)
	require.NoError(t, err)
	rightAAD, err := stateseal.AAD(right)
	require.NoError(t, err)
	require.NotEqual(t, leftAAD, rightAAD)
	envelope, version, err := s.Seal([]byte("x"), left)
	require.NoError(t, err)
	_, err = s.Open(envelope, version, right)
	require.ErrorIs(t, err, stateseal.ErrInvalid)
}

func TestSealGoldenAADAndEnvelope(t *testing.T) {
	nonce := bytes.Repeat([]byte{0x42}, 12)
	s, err := stateseal.New(stateseal.Config{ActiveKeyVersion: 7, Keys: map[int][]byte{7: bytes.Repeat([]byte{0x11}, 32)}, NonceSource: bytes.NewReader(nonce)})
	require.NoError(t, err)
	aad, err := stateseal.AAD(bindings())
	require.NoError(t, err)
	envelope, _, err := s.Seal([]byte("golden-state"), bindings())
	require.NoError(t, err)
	const wantAAD = "7072696d65722e6f617574682e73746174650100112233445566778899aabbccddeefffedcba9876543210fedcba98765432100000001a68747470733a2f2f612e6578616d706c652f63616c6c6261636b0000001a68747470733a2f2f7265736f757263652e6578616d706c652f780000000861756469656e6365"
	const wantEnvelope = "0142424242424242424242424276e273f2f9f15859092c7cb1f0bba353a29ca1cf93d3f87097659dc5"
	require.Equal(t, wantAAD, hex.EncodeToString(aad))
	require.Equal(t, wantEnvelope, hex.EncodeToString(envelope))
}

func TestOpenRejectsUnknownEnvelopeAndKeyVersions(t *testing.T) {
	s := fixedSealer(t)
	envelope, version, err := s.Seal([]byte("state"), bindings())
	require.NoError(t, err)
	for _, bad := range []byte{0, 2, 255} {
		mutated := append([]byte(nil), envelope...)
		mutated[0] = bad
		_, err := s.Open(mutated, version, bindings())
		require.ErrorIs(t, err, stateseal.ErrInvalid)
	}
	_, err = s.Open(envelope, 99, bindings())
	require.ErrorIs(t, err, stateseal.ErrInvalid)
	newer, err := stateseal.New(stateseal.Config{ActiveKeyVersion: 7, Keys: map[int][]byte{7: bytes.Repeat([]byte{0x11}, 32)}})
	require.NoError(t, err)
	_, err = newer.Open(envelope, 6, bindings())
	require.ErrorIs(t, err, stateseal.ErrInvalid)
}

func TestOpenRotationAndTamperFailClosed(t *testing.T) {
	old, err := stateseal.New(stateseal.Config{ActiveKeyVersion: 6, Keys: map[int][]byte{6: bytes.Repeat([]byte{0x22}, 32)}})
	require.NoError(t, err)
	envelope, version, err := old.Seal([]byte("state"), bindings())
	require.NoError(t, err)
	rotated := fixedSealer(t)
	got, err := rotated.Open(envelope, version, bindings())
	require.NoError(t, err)
	require.Equal(t, []byte("state"), got)
	for _, position := range []int{1, 13, len(envelope) - 1} {
		mutated := append([]byte(nil), envelope...)
		mutated[position] ^= 1
		_, err := rotated.Open(mutated, version, bindings())
		require.ErrorIs(t, err, stateseal.ErrInvalid)
	}
	for _, mutate := range []func(*stateseal.Bindings){func(b *stateseal.Bindings) { b.TransactionID = uuid.New() }, func(b *stateseal.Bindings) { b.OAuthClientID = uuid.New() }, func(b *stateseal.Bindings) { b.RedirectURI += "x" }, func(b *stateseal.Bindings) { b.ResourceURI += "x" }, func(b *stateseal.Bindings) { b.Audience += "x" }} {
		b := bindings()
		mutate(&b)
		_, err := rotated.Open(envelope, version, b)
		require.ErrorIs(t, err, stateseal.ErrInvalid)
	}
}

func TestZeroBytesWipesBufferAndErrorsNeverContainState(t *testing.T) {
	plain := []byte("super-secret-state")
	stateseal.Zero(plain)
	require.Equal(t, make([]byte, len(plain)), plain)
	s := fixedSealer(t)
	_, _, err := s.Seal([]byte("super-secret-state"), stateseal.Bindings{})
	require.Error(t, err)
	require.NotContains(t, strings.ToLower(err.Error()), "super-secret-state")
}

func TestAADRejectsEmptyControlInvalidAndOverlongText(t *testing.T) {
	for _, mutate := range []func(*stateseal.Bindings){
		func(b *stateseal.Bindings) { b.RedirectURI = "" },
		func(b *stateseal.Bindings) { b.ResourceURI = "https://ok\x00/x" },
		func(b *stateseal.Bindings) { b.Audience = string([]byte{0xff, 0xfe}) },
		func(b *stateseal.Bindings) { b.RedirectURI = strings.Repeat("a", 2049) },
		func(b *stateseal.Bindings) { b.ResourceURI = strings.Repeat("b", 2049) },
		func(b *stateseal.Bindings) { b.Audience = strings.Repeat("c", 129) },
	} {
		b := bindings()
		mutate(&b)
		_, err := stateseal.AAD(b)
		require.ErrorIs(t, err, stateseal.ErrInvalid)
		_, _, err = fixedSealer(t).Seal([]byte("x"), b)
		require.ErrorIs(t, err, stateseal.ErrInvalid)
	}
}

func TestAADAcceptsExactFieldAndTotalBounds(t *testing.T) {
	b := bindings()
	b.RedirectURI = strings.Repeat("r", 2048)
	b.ResourceURI = strings.Repeat("s", 2048)
	b.Audience = strings.Repeat("t", 128)
	aad, err := stateseal.AAD(b)
	require.NoError(t, err)
	require.Equal(t, 4287, len(aad))
	envelope, version, err := fixedSealer(t).Seal([]byte{0x01}, b)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(envelope), 30)
	require.LessOrEqual(t, len(envelope), 1053)
	got, err := fixedSealer(t).Open(envelope, version, b)
	require.NoError(t, err)
	require.Equal(t, []byte{0x01}, got)
	stateseal.Zero(got)
	require.Equal(t, []byte{0}, got)
}

func TestNewCopiesKeysAndRejectsNonAES256Material(t *testing.T) {
	raw := bytes.Repeat([]byte{0x33}, 32)
	s, err := stateseal.New(stateseal.Config{ActiveKeyVersion: 1, Keys: map[int][]byte{1: raw}})
	require.NoError(t, err)
	raw[0] ^= 0xff
	envelope, version, err := s.Seal([]byte("copied-key"), bindings())
	require.NoError(t, err)
	got, err := s.Open(envelope, version, bindings())
	require.NoError(t, err)
	require.Equal(t, []byte("copied-key"), got)
	for _, cfg := range []stateseal.Config{
		{ActiveKeyVersion: 0, Keys: map[int][]byte{1: bytes.Repeat([]byte{1}, 32)}},
		{ActiveKeyVersion: 1, Keys: map[int][]byte{}},
		{ActiveKeyVersion: 1, Keys: map[int][]byte{1: bytes.Repeat([]byte{1}, 31)}},
		{ActiveKeyVersion: 2, Keys: map[int][]byte{1: bytes.Repeat([]byte{1}, 32)}},
		{ActiveKeyVersion: 1, Keys: map[int][]byte{0: bytes.Repeat([]byte{1}, 32)}},
	} {
		_, err := stateseal.New(cfg)
		require.ErrorIs(t, err, stateseal.ErrInvalid)
	}
}

func TestSealUsesOnlyActiveKeyAndCSPRNGFailuresAreNonOracular(t *testing.T) {
	s, err := stateseal.New(stateseal.Config{
		ActiveKeyVersion: 7,
		Keys:             map[int][]byte{7: bytes.Repeat([]byte{0x11}, 32), 6: bytes.Repeat([]byte{0x22}, 32)},
		NonceSource:      bytes.NewReader(nil),
	})
	require.NoError(t, err)
	_, _, err = s.Seal([]byte("state"), bindings())
	require.ErrorIs(t, err, stateseal.ErrInvalid)
	require.Equal(t, stateseal.ErrInvalid.Error(), err.Error())

	active, err := stateseal.New(stateseal.Config{ActiveKeyVersion: 7, Keys: map[int][]byte{7: bytes.Repeat([]byte{0x11}, 32)}})
	require.NoError(t, err)
	envelope, version, err := active.Seal([]byte("only-active"), bindings())
	require.NoError(t, err)
	require.Equal(t, 7, version)
	retired, err := stateseal.New(stateseal.Config{ActiveKeyVersion: 8, Keys: map[int][]byte{8: bytes.Repeat([]byte{0x44}, 32)}})
	require.NoError(t, err)
	_, err = retired.Open(envelope, version, bindings())
	require.ErrorIs(t, err, stateseal.ErrInvalid)
}

func TestOpenRejectsEnvelopeLengthAndDoesNotReturnPlaintextOnFailure(t *testing.T) {
	s := fixedSealer(t)
	envelope, version, err := s.Seal([]byte("state"), bindings())
	require.NoError(t, err)
	for _, bad := range [][]byte{nil, envelope[:29], append(append([]byte(nil), envelope...), 0x00)} {
		plain, err := s.Open(bad, version, bindings())
		require.ErrorIs(t, err, stateseal.ErrInvalid)
		require.Nil(t, plain)
	}
}
