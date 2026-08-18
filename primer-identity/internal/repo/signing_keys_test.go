package repo_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/keys"
	"github.com/aleksclark/primer/identity/internal/repo"
	"github.com/aleksclark/primer/identity/internal/testutil"
)

func signingTx(t *testing.T) pgx.Tx {
	t.Helper()
	return testutil.Tx(t)
}

func sealedRecord(t *testing.T, status string) repo.SigningKeyRecord {
	t.Helper()
	mat, err := keys.Generate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = mat.Destroy() })
	public, err := mat.PublicJWK()
	require.NoError(t, err)
	sealed, err := keys.Seal(mat, testSealKey())
	require.NoError(t, err)
	now := time.Now().UTC().Truncate(time.Microsecond)
	rec := repo.SigningKeyRecord{
		Kid: public.Kid, Alg: domain.SigningAlgES256, KeyVersion: 1, PublicJWK: public,
		SealedPrivateKey: sealed, Status: status, NotBefore: now,
	}
	if status == domain.SigningKeyStatusActive {
		activated := now
		rec.ActivatedAt = &activated
	}
	return rec
}

func testSealKey() [32]byte {
	var key [32]byte
	copy(key[:], []byte("PLANT_SEAL_SECRET_VALUE_AAAA!!!!"))
	return key
}

func TestInsertSigningKeyRoundTrip(t *testing.T) {
	tx := signingTx(t)
	ctx := context.Background()
	rec := sealedRecord(t, domain.SigningKeyStatusActive)

	got, err := repo.InsertSigningKey(ctx, tx, rec)
	require.NoError(t, err)
	assert.Equal(t, rec.Kid, got.Kid)
	assert.Equal(t, rec.PublicJWK, got.PublicJWK)
	assert.Equal(t, rec.SealedPrivateKey, got.SealedPrivateKey)
	assert.Equal(t, 1, got.KeyVersion)
	assert.Equal(t, domain.SigningKeyStatusActive, got.Status)
	assert.False(t, got.CreatedAt.IsZero())
	require.NotNil(t, got.ActivatedAt)

	loaded, err := repo.GetSigningKeyByKid(ctx, tx, rec.Kid)
	require.NoError(t, err)
	assert.Equal(t, got.ID, loaded.ID)
	assert.Equal(t, rec.SealedPrivateKey, loaded.SealedPrivateKey)

	shared, err := repo.GetSigningKeyByKidForShare(ctx, tx, rec.Kid)
	require.NoError(t, err)
	assert.Equal(t, got.ID, shared.ID)
	assert.Equal(t, rec.PublicJWK, shared.PublicJWK)
	assert.Equal(t, rec.SealedPrivateKey, shared.SealedPrivateKey)

	listed, err := repo.ListSigningKeysByStatusForShare(ctx, tx, domain.SigningKeyStatusActive)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, got.ID, listed[0].ID)
	assert.Equal(t, rec.SealedPrivateKey, listed[0].SealedPrivateKey)

	blob, err := json.Marshal(loaded)
	require.NoError(t, err)
	assert.NotContains(t, string(blob), `"d"`)
	assert.NotContains(t, strings.ToLower(string(blob)), "sealed_private")
	assert.NotContains(t, string(blob), string(rec.SealedPrivateKey))
}

func TestInsertSigningKeyEnforcesAtMostOneActiveAndNext(t *testing.T) {
	ctx := context.Background()
	activeTx := signingTx(t)
	_, err := repo.InsertSigningKey(ctx, activeTx, sealedRecord(t, domain.SigningKeyStatusActive))
	require.NoError(t, err)
	_, err = repo.InsertSigningKey(ctx, activeTx, sealedRecord(t, domain.SigningKeyStatusActive))
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrConflict)

	nextTx := signingTx(t)
	_, err = repo.InsertSigningKey(ctx, nextTx, sealedRecord(t, domain.SigningKeyStatusNext))
	require.NoError(t, err)
	_, err = repo.InsertSigningKey(ctx, nextTx, sealedRecord(t, domain.SigningKeyStatusNext))
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrConflict)
}

func TestCountSigningKeysByStatus(t *testing.T) {
	tx := signingTx(t)
	ctx := context.Background()
	n, err := repo.CountSigningKeysByStatus(ctx, tx, domain.SigningKeyStatusActive)
	require.NoError(t, err)
	assert.Zero(t, n)
	_, err = repo.InsertSigningKey(ctx, tx, sealedRecord(t, domain.SigningKeyStatusActive))
	require.NoError(t, err)
	n, err = repo.CountSigningKeysByStatus(ctx, tx, domain.SigningKeyStatusActive)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

func TestInsertSigningKeyRejectsDestroyedAndRetiredAdminWrites(t *testing.T) {
	tx := signingTx(t)
	ctx := context.Background()
	retired := sealedRecord(t, domain.SigningKeyStatusRetired)
	_, err := repo.InsertSigningKey(ctx, tx, retired)
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrInvalid)

	destroyed := sealedRecord(t, domain.SigningKeyStatusDestroyed)
	_, err = repo.InsertSigningKey(ctx, tx, destroyed)
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrInvalid)
}

func TestLoadedSigningKeyRejectsRowJWKIdentityMismatch(t *testing.T) {
	tx := signingTx(t)
	ctx := context.Background()
	rec, err := repo.InsertSigningKey(ctx, tx, sealedRecord(t, domain.SigningKeyStatusActive))
	require.NoError(t, err)

	other := uuid.NewString()
	_, err = tx.Exec(ctx, `UPDATE signing_keys SET public_jwk = jsonb_set(public_jwk, '{kid}', to_jsonb($2::text)) WHERE kid = $1`, rec.Kid, other)
	require.NoError(t, err)
	_, err = repo.GetSigningKeyByKid(ctx, tx, rec.Kid)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrCorruptSigner), fmt.Sprintf("expected corrupt signer, got %v", err))
}

func TestLoadedSigningKeyRejectsMalformedPublicCoordinates(t *testing.T) {
	tx := signingTx(t)
	ctx := context.Background()
	rec, err := repo.InsertSigningKey(ctx, tx, sealedRecord(t, domain.SigningKeyStatusActive))
	require.NoError(t, err)

	_, err = tx.Exec(ctx, `UPDATE signing_keys SET public_jwk = jsonb_set(public_jwk, '{x}', '"x"'::jsonb) WHERE kid = $1`, rec.Kid)
	require.NoError(t, err)
	_, err = repo.GetSigningKeyByKid(ctx, tx, rec.Kid)
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrCorruptSigner)
}

func TestSigningKeyRecordFormattingOmitsSealedPrivateBytes(t *testing.T) {
	rec := sealedRecord(t, domain.SigningKeyStatusActive)
	for _, value := range []any{rec, &rec} {
		baseline := fmt.Sprintf("%v", value)
		for _, format := range []string{"%v", "%+v", "%#v", "%s", "%d", "%x"} {
			formatted := fmt.Sprintf(format, value)
			assert.Equal(t, baseline, formatted, "format %s", format)
			assert.NotContains(t, formatted, string(rec.SealedPrivateKey))
			assert.NotContains(t, formatted, fmt.Sprintf("%x", rec.SealedPrivateKey))
			assert.NotContains(t, strings.ToLower(formatted), "private key")
		}

		blob, err := json.Marshal(value)
		require.NoError(t, err)
		assert.NotContains(t, string(blob), string(rec.SealedPrivateKey))
		assert.NotContains(t, strings.ToLower(string(blob)), "sealed_private")
	}
}
