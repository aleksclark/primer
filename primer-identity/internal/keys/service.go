package keys

import (
	"context"
	"crypto"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aleksclark/primer/identity/internal/config"
	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/repo"
)

var (
	// ErrCustodyDisabled means key custody was explicitly disabled.
	ErrCustodyDisabled = errors.New("key custody is disabled")
	// ErrSignerRevoked is returned after a managed signer is retired or closed.
	ErrSignerRevoked = errors.New("signer revoked")
)

// Service owns persistent ES256 key custody. It never logs private material
// and never auto-generates keys in production.
type Service struct {
	pool          *pgxpool.Pool
	cfg           config.KeyConfig
	env           string
	validationErr error

	fence   sync.RWMutex
	holders map[string]*ManagedSigner
	closed  atomic.Bool

	// testCreateNextCommit is an internal fault seam for ambiguous commit
	// outcome tests. Production leaves it nil and calls tx.Commit directly.
	testCreateNextCommit func(context.Context, pgx.Tx) error
	// testCreateInitialCommit is an internal fault seam for unknown initial
	// insert commit outcomes. Production leaves it nil and calls tx.Commit.
	testCreateInitialCommit func(context.Context, pgx.Tx) error
	// testGenerateRecord stalls or replaces candidate generation in internal
	// tests. Production leaves it nil and calls generateRecord.
	testGenerateRecord func(string) (repo.SigningKeyRecord, error)
	// testBeforeSignerValidationFailure is an internal synchronization seam for
	// proving validation locks are released before revocation can block.
	testBeforeSignerValidationFailure func()
}

// ManagedSigner is a service-owned crypto.Signer. Callers never receive
// unmanaged private material; Close revokes every holder.
type ManagedSigner struct {
	svc    *Service
	kid    string
	public domain.PublicJWK
	state  *managedSignerState
}

type managedSignerState struct {
	mat         *Material
	fingerprint [sha256.Size]byte
	revoked     atomic.Bool
}

var _ crypto.Signer = (*ManagedSigner)(nil)

// NewService constructs a custody service. pool must be non-nil. Configuration
// is validated here so programmatic construction cannot bypass loader checks.
func NewService(pool *pgxpool.Pool, cfg config.KeyConfig, env string) *Service {
	normalized := cfg
	var validationErr error
	switch env {
	case "development", "test", "production":
		if err := normalized.Validate(env); err != nil {
			validationErr = fmt.Errorf("%w: invalid key configuration", domain.ErrInvalid)
		}
	default:
		validationErr = fmt.Errorf("%w: unsupported service environment", domain.ErrInvalid)
	}
	return &Service{
		pool:          pool,
		cfg:           normalized,
		env:           env,
		validationErr: validationErr,
		holders:       make(map[string]*ManagedSigner),
	}
}

func (s *Service) String() string {
	if s == nil {
		return "key-service <nil>"
	}
	return fmt.Sprintf("key-service env=%s %s", s.env, s.cfg.String())
}

func (s *Service) GoString() string { return s.String() }

func (s *Service) requireReady() error {
	if s == nil {
		return fmt.Errorf("%w: key service is not configured", domain.ErrInvalid)
	}
	if s.validationErr != nil {
		return s.validationErr
	}
	if s.pool == nil {
		return fmt.Errorf("%w: key service is not configured", domain.ErrInvalid)
	}
	if s.closed.Load() {
		return fmt.Errorf("%w: key service is closed", domain.ErrSignerUnavailable)
	}
	if !s.cfg.Enabled {
		return fmt.Errorf("%w: enable key custody before use", ErrCustodyDisabled)
	}
	if s.cfg.SealKey() == ([32]byte{}) {
		return fmt.Errorf("%w: seal key is required", domain.ErrInvalid)
	}
	return nil
}

const maxSerializableAttempts = 3

// Ready reports whether a usable authenticated active signer can be loaded.
func (s *Service) Ready(ctx context.Context) error {
	_, _, err := s.ActiveSigner(ctx)
	return err
}

// CreateInitialActive inserts the first active key. Concurrent callers
// converge on exactly one active row.
func (s *Service) CreateInitialActive(ctx context.Context) (*domain.SigningKey, error) {
	if err := s.requireReady(); err != nil {
		return nil, err
	}
	candidate, err := s.generateCandidate(domain.SigningKeyStatusActive)
	if err != nil {
		return nil, err
	}
	defer discardCandidate(&candidate)
	var last error
	for attempt := 1; attempt <= maxSerializableAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		got, err := s.createInitialActiveOnce(ctx, candidate)
		if err == nil {
			return got, nil
		}
		last = err
		if !isRetryableSerializationError(err) || attempt == maxSerializableAttempts {
			return nil, err
		}
	}
	return nil, last
}

func (s *Service) createInitialActiveOnce(ctx context.Context, candidate repo.SigningKeyRecord) (*domain.SigningKey, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("create initial active: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if _, err := tx.Exec(ctx, `LOCK TABLE signing_keys IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return nil, fmt.Errorf("create initial active: lock: %w", err)
	}
	existing, err := repo.ListSigningKeysByStatus(ctx, tx, domain.SigningKeyStatusActive, true)
	if err != nil {
		return nil, err
	}
	if len(existing) > 1 {
		return nil, fmt.Errorf("create initial active: %w", domain.ErrCorruptSigner)
	}
	if len(existing) == 1 {
		if err := s.acceptExistingActive(existing[0]); err != nil {
			return nil, err
		}
		pub := existing[0].Public()
		if err := s.commitCreateInitial(ctx, tx); err != nil {
			return nil, fmt.Errorf("create initial active: commit: %w", err)
		}
		return &pub, nil
	}
	got, err := repo.InsertSigningKey(ctx, tx, candidate)
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			again, listErr := repo.ListSigningKeysByStatus(ctx, tx, domain.SigningKeyStatusActive, true)
			if listErr == nil && len(again) == 1 {
				if acceptErr := s.acceptExistingActive(again[0]); acceptErr != nil {
					return nil, acceptErr
				}
				pub := again[0].Public()
				if commitErr := s.commitCreateInitial(ctx, tx); commitErr == nil {
					return &pub, nil
				} else {
					return nil, fmt.Errorf("create initial active: commit: %w", commitErr)
				}
			}
		}
		return nil, err
	}
	if err := s.commitCreateInitial(ctx, tx); err != nil {
		return nil, fmt.Errorf("create initial active: commit: %w", err)
	}
	pub := got.Public()
	return &pub, nil
}

func (s *Service) commitCreateInitial(ctx context.Context, tx pgx.Tx) error {
	if s.testCreateInitialCommit != nil {
		return s.testCreateInitialCommit(ctx, tx)
	}
	return tx.Commit(ctx)
}

func (s *Service) acceptExistingActive(rec repo.SigningKeyRecord) error {
	if err := temporalUsable(rec, time.Now().UTC()); err != nil {
		return err
	}
	return s.authenticateRecord(rec)
}

func isRetryableSerializationError(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "40001" || pgErr.Code == "40P01"
}

// CreateNext inserts a rotation candidate, or returns the existing next key
// when exactly one authenticated, temporally usable next row is already present.
func (s *Service) CreateNext(ctx context.Context) (*domain.SigningKey, error) {
	if err := s.requireReady(); err != nil {
		return nil, err
	}
	candidate, err := s.generateCandidate(domain.SigningKeyStatusNext)
	if err != nil {
		return nil, err
	}
	defer discardCandidate(&candidate)
	var last error
	for attempt := 1; attempt <= maxSerializableAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		got, err := s.createNextOnce(ctx, candidate)
		if err == nil {
			return got, nil
		}
		last = err
		if !isRetryableSerializationError(err) || attempt == maxSerializableAttempts {
			return nil, err
		}
	}
	return nil, last
}

func (s *Service) createNextOnce(ctx context.Context, candidate repo.SigningKeyRecord) (*domain.SigningKey, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("create next: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, `LOCK TABLE signing_keys IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return nil, fmt.Errorf("create next: lock: %w", err)
	}
	active, err := repo.ListSigningKeysByStatus(ctx, tx, domain.SigningKeyStatusActive, true)
	if err != nil {
		return nil, err
	}
	if len(active) != 1 {
		return nil, fmt.Errorf("create next: %w", domain.ErrSignerUnavailable)
	}
	next, err := repo.ListSigningKeysByStatus(ctx, tx, domain.SigningKeyStatusNext, true)
	if err != nil {
		return nil, err
	}
	if len(next) > 1 {
		return nil, fmt.Errorf("create next: %w", domain.ErrCorruptSigner)
	}
	if len(next) == 1 {
		if err := s.acceptExistingNext(next[0]); err != nil {
			return nil, err
		}
		pub := next[0].Public()
		if err := s.commitCreateNext(ctx, tx); err != nil {
			return s.reconcileExistingNextAfterUnknownCommit(ctx, err)
		}
		return &pub, nil
	}
	got, err := repo.InsertSigningKey(ctx, tx, candidate)
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			again, listErr := repo.ListSigningKeysByStatus(ctx, tx, domain.SigningKeyStatusNext, true)
			if listErr == nil && len(again) > 1 {
				return nil, fmt.Errorf("create next: %w", domain.ErrCorruptSigner)
			}
			if listErr == nil && len(again) == 1 {
				if acceptErr := s.acceptExistingNext(again[0]); acceptErr != nil {
					return nil, acceptErr
				}
				pub := again[0].Public()
				commitErr := s.commitCreateNext(ctx, tx)
				if commitErr == nil {
					return &pub, nil
				}
				return s.reconcileExistingNextAfterUnknownCommit(ctx, commitErr)
			}
		}
		return nil, err
	}
	if err := s.commitCreateNext(ctx, tx); err != nil {
		return s.reconcileExistingNextAfterUnknownCommit(ctx, err)
	}
	pub := got.Public()
	return &pub, nil
}

func (s *Service) commitCreateNext(ctx context.Context, tx pgx.Tx) error {
	if s.testCreateNextCommit != nil {
		return s.testCreateNextCommit(ctx, tx)
	}
	return tx.Commit(ctx)
}

func (s *Service) acceptExistingNext(rec repo.SigningKeyRecord) error {
	if rec.Status != domain.SigningKeyStatusNext {
		return fmt.Errorf("create next: %w", domain.ErrCorruptSigner)
	}
	if err := temporalUsable(rec, time.Now().UTC()); err != nil {
		return err
	}
	return s.authenticateRecord(rec)
}

func (s *Service) reconcileExistingNextAfterUnknownCommit(ctx context.Context, commitErr error) (*domain.SigningKey, error) {
	next, err := repo.ListSigningKeysByStatus(ctx, s.pool, domain.SigningKeyStatusNext, false)
	if err != nil || len(next) != 1 {
		return nil, fmt.Errorf("create next: commit: %w", commitErr)
	}
	if acceptErr := s.acceptExistingNext(next[0]); acceptErr != nil {
		return nil, fmt.Errorf("create next: commit: %w", commitErr)
	}
	pub := next[0].Public()
	return &pub, nil
}

// ActivateNext promotes the single authenticated next key to active when no
// active key exists. Concurrent activation of a second active is rejected.
func (s *Service) ActivateNext(ctx context.Context) (*domain.SigningKey, error) {
	if err := s.requireReady(); err != nil {
		return nil, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("activate next: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, `LOCK TABLE signing_keys IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return nil, fmt.Errorf("activate next: lock: %w", err)
	}
	active, err := repo.ListSigningKeysByStatus(ctx, tx, domain.SigningKeyStatusActive, true)
	if err != nil {
		return nil, err
	}
	if len(active) > 1 {
		return nil, fmt.Errorf("activate next: %w", domain.ErrCorruptSigner)
	}
	if len(active) == 1 {
		return nil, fmt.Errorf("activate next: %w", domain.ErrConflict)
	}
	next, err := repo.ListSigningKeysByStatus(ctx, tx, domain.SigningKeyStatusNext, true)
	if err != nil {
		return nil, err
	}
	if len(next) != 1 {
		return nil, fmt.Errorf("activate next: %w", domain.ErrSignerUnavailable)
	}
	if err := s.acceptExistingNext(next[0]); err != nil {
		return nil, err
	}
	got, err := repo.ActivateSigningKey(ctx, tx, next[0].Kid, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("activate next: commit: %w", err)
	}
	pub := got.Public()
	return &pub, nil
}

// Bootstrap explicitly creates the first active key in development/test.
// Production always fails closed.
func (s *Service) Bootstrap(ctx context.Context) (*domain.SigningKey, error) {
	if err := s.requireReady(); err != nil {
		return nil, err
	}
	if s.env == "production" || !s.cfg.AutoBootstrap {
		return nil, fmt.Errorf("key bootstrap is not allowed")
	}
	if s.env != "development" && s.env != "test" {
		return nil, fmt.Errorf("key bootstrap is not allowed")
	}
	return s.CreateInitialActive(ctx)
}

// ActiveSigner returns the single usable active private signer and its public
// metadata. Empty, multiple, unsealable, or unpublished signers fail closed.
func (s *Service) ActiveSigner(ctx context.Context) (*ManagedSigner, *domain.SigningKey, error) {
	if err := s.requireReady(); err != nil {
		return nil, nil, err
	}
	s.fence.Lock()
	defer s.fence.Unlock()
	if s.closed.Load() {
		return nil, nil, fmt.Errorf("%w: key service is closed", domain.ErrSignerUnavailable)
	}
	active, err := repo.ListSigningKeysByStatus(ctx, s.pool, domain.SigningKeyStatusActive, false)
	if err != nil {
		return nil, nil, err
	}
	if len(active) == 0 {
		return nil, nil, domain.ErrSignerUnavailable
	}
	if len(active) != 1 {
		return nil, nil, domain.ErrCorruptSigner
	}
	rec := active[0]
	if err := temporalUsable(rec, time.Now().UTC()); err != nil {
		return nil, nil, err
	}
	pubs, err := s.publicJWKSUnlocked(ctx)
	if err != nil {
		return nil, nil, err
	}
	if !SignerInPublicSet(rec.PublicJWK, pubs) {
		return nil, nil, fmt.Errorf("%w: active signer is not in the public set", domain.ErrCorruptSigner)
	}
	holder, err := s.registerActiveLocked(rec)
	if err != nil {
		return nil, nil, err
	}
	pub := rec.Public()
	return holder, &pub, nil
}

// TransactionSigner is a one-transaction signing authority. It never opens a
// database connection. Close permanently disables it; the value must not be
// reused after the owning transaction ends.
type TransactionSigner struct {
	mat    *Material
	public domain.PublicJWK
	closed atomic.Bool
}

var (
	_ crypto.Signer = (*TransactionSigner)(nil)
	_ io.Closer     = (*TransactionSigner)(nil)
)

// ActiveSignerForTx authenticates the single active key on the caller's
// existing transaction, holds FOR SHARE until that transaction completes, and
// returns an unsealed signer that performs no additional pool checkout.
func (s *Service) ActiveSignerForTx(ctx context.Context, tx pgx.Tx) (*TransactionSigner, *domain.SigningKey, error) {
	if err := s.requireReady(); err != nil {
		return nil, nil, err
	}
	if ctx == nil || ctx.Err() != nil {
		if ctx != nil && ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		return nil, nil, fmt.Errorf("%w: request canceled", domain.ErrSignerUnavailable)
	}
	if tx == nil {
		return nil, nil, fmt.Errorf("%w: transaction is required", domain.ErrInvalid)
	}
	if s.closed.Load() {
		return nil, nil, fmt.Errorf("%w: key service is closed", domain.ErrSignerUnavailable)
	}

	active, err := repo.ListSigningKeysByStatusForShare(ctx, tx, domain.SigningKeyStatusActive)
	if err != nil {
		return nil, nil, err
	}
	if len(active) == 0 {
		return nil, nil, domain.ErrSignerUnavailable
	}
	if len(active) != 1 {
		return nil, nil, domain.ErrCorruptSigner
	}
	rec := active[0]
	if err := temporalUsable(rec, time.Now().UTC()); err != nil {
		return nil, nil, err
	}

	next, err := repo.ListSigningKeysByStatusForShare(ctx, tx, domain.SigningKeyStatusNext)
	if err != nil {
		return nil, nil, err
	}
	if len(next) > 1 {
		return nil, nil, domain.ErrCorruptSigner
	}
	pubs := make([]domain.PublicJWK, 0, 1+len(next))
	if err := s.authenticateRecord(rec); err != nil {
		return nil, nil, err
	}
	pubs = append(pubs, rec.PublicJWK)
	for _, nextRec := range next {
		if err := temporalUsable(nextRec, time.Now().UTC()); err != nil {
			return nil, nil, err
		}
		if err := s.authenticateRecord(nextRec); err != nil {
			return nil, nil, err
		}
		pubs = append(pubs, nextRec.PublicJWK)
	}
	if !SignerInPublicSet(rec.PublicJWK, pubs) {
		return nil, nil, fmt.Errorf("%w: active signer is not in the public set", domain.ErrCorruptSigner)
	}

	mat, err := Unseal(rec.SealedPrivateKey, rec.PublicJWK, s.cfg.SealKey())
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", domain.ErrCorruptSigner, ErrUnsealFailed)
	}
	holder := &TransactionSigner{mat: mat, public: rec.PublicJWK}
	pub := rec.Public()
	return holder, &pub, nil
}

func (t *TransactionSigner) acquire() error {
	if t == nil || t.mat == nil || t.closed.Load() {
		return ErrSignerRevoked
	}
	return nil
}

// Close permanently disables the transaction-scoped signer and destroys material.
func (t *TransactionSigner) Close() error {
	if t == nil {
		return nil
	}
	t.closed.Store(true)
	if t.mat != nil {
		_ = t.mat.Destroy()
		t.mat = nil
	}
	return nil
}

func (t *TransactionSigner) Sign(random io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	if err := t.acquire(); err != nil {
		return nil, err
	}
	signature, err := t.mat.Sign(random, digest, opts)
	if err != nil {
		_ = t.Close()
		return nil, ErrSignerRevoked
	}
	return signature, nil
}

func (t *TransactionSigner) Public() crypto.PublicKey {
	if err := t.acquire(); err != nil {
		return nil
	}
	return t.mat.Public()
}

func (t *TransactionSigner) PublicJWK() (domain.PublicJWK, error) {
	if err := t.acquire(); err != nil {
		return domain.PublicJWK{}, err
	}
	return t.mat.PublicJWK()
}

func (t *TransactionSigner) String() string {
	if t == nil {
		return "transaction-signer <nil>"
	}
	return fmt.Sprintf("transaction-signer kid=%s", t.public.Kid)
}

func (t *TransactionSigner) GoString() string { return t.String() }

func (t *TransactionSigner) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, t.String())
}

func (*TransactionSigner) MarshalJSON() ([]byte, error) {
	return nil, errors.New("transaction signer JSON serialization refused; use PublicJWK")
}

func (s *Service) registerActiveLocked(rec repo.SigningKeyRecord) (*ManagedSigner, error) {
	fingerprint, err := signingKeyFingerprint(rec)
	if err != nil {
		return nil, fmt.Errorf("%w: active signer fingerprint", domain.ErrCorruptSigner)
	}
	if existing, ok := s.holders[rec.Kid]; ok {
		if existing.public != rec.PublicJWK {
			return nil, fmt.Errorf("%w: registered public jwk mismatch", domain.ErrCorruptSigner)
		}
		if existing.state == nil || existing.state.revoked.Load() {
			delete(s.holders, rec.Kid)
		} else {
			if existing.state.fingerprint != fingerprint {
				return nil, fmt.Errorf("%w: registered signer fingerprint mismatch", domain.ErrCorruptSigner)
			}
			return existing, nil
		}
	}
	mat, err := Unseal(rec.SealedPrivateKey, rec.PublicJWK, s.cfg.SealKey())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrCorruptSigner, ErrUnsealFailed)
	}
	holder := &ManagedSigner{
		svc: s, kid: rec.Kid, public: rec.PublicJWK,
		state: &managedSignerState{mat: mat, fingerprint: fingerprint},
	}
	s.holders[rec.Kid] = holder
	return holder, nil
}

// PublicJWKS returns authenticated active+next public JWKs only.
func (s *Service) PublicJWKS(ctx context.Context) ([]domain.PublicJWK, error) {
	if err := s.requireReady(); err != nil {
		return nil, err
	}
	return s.publicJWKSUnlocked(ctx)
}

// PublicSetETag returns HTTP-free ETag material for the authenticated public set.
func (s *Service) PublicSetETag(ctx context.Context) (string, error) {
	pubs, err := s.PublicJWKS(ctx)
	if err != nil {
		return "", err
	}
	return PublicSetETag(pubs)
}

func (s *Service) publicJWKSUnlocked(ctx context.Context) ([]domain.PublicJWK, error) {
	recs, err := s.loadPublishedRecords(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	seen := make(map[string]struct{}, len(recs))
	out := make([]domain.PublicJWK, 0, len(recs))
	for _, rec := range recs {
		if _, dup := seen[rec.Kid]; dup {
			return nil, fmt.Errorf("%w: duplicate kid", domain.ErrCorruptSigner)
		}
		seen[rec.Kid] = struct{}{}
		if err := temporalUsable(rec, now); err != nil {
			return nil, err
		}
		if err := s.authenticateRecord(rec); err != nil {
			return nil, err
		}
		out = append(out, rec.PublicJWK)
	}
	return out, nil
}

func (s *Service) loadPublishedRecords(ctx context.Context) ([]repo.SigningKeyRecord, error) {
	active, err := repo.ListSigningKeysByStatus(ctx, s.pool, domain.SigningKeyStatusActive, false)
	if err != nil {
		return nil, err
	}
	if len(active) == 0 {
		return nil, domain.ErrSignerUnavailable
	}
	if len(active) != 1 {
		return nil, domain.ErrCorruptSigner
	}
	next, err := repo.ListSigningKeysByStatus(ctx, s.pool, domain.SigningKeyStatusNext, false)
	if err != nil {
		return nil, err
	}
	if len(next) > 1 {
		return nil, domain.ErrCorruptSigner
	}
	out := make([]repo.SigningKeyRecord, 0, 1+len(next))
	out = append(out, active...)
	out = append(out, next...)
	return out, nil
}

func temporalUsable(rec repo.SigningKeyRecord, now time.Time) error {
	if rec.NotBefore.After(now) {
		return domain.ErrSignerUnavailable
	}
	if rec.NotAfter != nil && !rec.NotAfter.After(now) {
		return domain.ErrSignerUnavailable
	}
	return nil
}

func signingKeyFingerprint(rec repo.SigningKeyRecord) ([sha256.Size]byte, error) {
	publicJSON, err := rec.PublicJWK.MarshalJSON()
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	sealedDigest := sha256.Sum256(rec.SealedPrivateKey)
	h := sha256.New()
	writeField := func(value []byte) {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = h.Write(length[:])
		_, _ = h.Write(value)
	}
	writeString := func(value string) { writeField([]byte(value)) }
	writeTime := func(value time.Time) {
		if value.IsZero() {
			writeField([]byte{0})
			return
		}
		writeField([]byte{1})
		writeString(value.UTC().Format(time.RFC3339Nano))
	}
	writeOptionalTime := func(value *time.Time) {
		if value == nil {
			writeField([]byte{0})
			return
		}
		writeField([]byte{1})
		writeTime(*value)
	}

	writeString(rec.ID.String())
	writeString(rec.Kid)
	writeString(rec.Alg)
	writeString(fmt.Sprintf("%d", rec.KeyVersion))
	writeString(rec.Status)
	writeField(publicJSON)
	writeField(sealedDigest[:])
	writeTime(rec.CreatedAt)
	writeTime(rec.NotBefore)
	writeOptionalTime(rec.NotAfter)
	writeOptionalTime(rec.ActivatedAt)
	writeOptionalTime(rec.RetiredAt)
	writeOptionalTime(rec.DestroyedAt)
	var fingerprint [sha256.Size]byte
	copy(fingerprint[:], h.Sum(nil))
	return fingerprint, nil
}

func (s *Service) authenticateRecord(rec repo.SigningKeyRecord) error {
	if rec.Status == domain.SigningKeyStatusDestroyed {
		return fmt.Errorf("%w: destroyed signer", domain.ErrCorruptSigner)
	}
	mat, err := Unseal(rec.SealedPrivateKey, rec.PublicJWK, s.cfg.SealKey())
	if err != nil {
		return fmt.Errorf("%w: %v", domain.ErrCorruptSigner, ErrUnsealFailed)
	}
	defer func() { _ = mat.Destroy() }()
	got, err := mat.PublicJWK()
	if err != nil {
		return fmt.Errorf("%w: published public jwk", domain.ErrCorruptSigner)
	}
	if got != rec.PublicJWK {
		return fmt.Errorf("%w: published public jwk mismatch", domain.ErrCorruptSigner)
	}
	return nil
}

// Close revokes and destroys every outstanding managed signer. It does not
// push-invalidate holders owned by other processes.
func (s *Service) Close() error {
	if s == nil {
		return nil
	}
	s.fence.Lock()
	defer s.fence.Unlock()
	s.closed.Store(true)
	for kid := range s.holders {
		s.revokeHoldersLocked(kid)
	}
	return nil
}

func (s *Service) revokeHoldersLocked(kid string) {
	holder, ok := s.holders[kid]
	if !ok {
		return
	}
	if holder.state != nil {
		holder.state.revoked.Store(true)
		if holder.state.mat != nil {
			_ = holder.state.mat.Destroy()
		}
	}
	delete(s.holders, kid)
}

const managedSignerDBTimeout = 2 * time.Second

func (m *ManagedSigner) acquireRead() error {
	if m == nil || m.svc == nil || m.state == nil || m.state.mat == nil {
		return ErrSignerRevoked
	}
	m.svc.fence.RLock()
	if m.state.revoked.Load() || m.svc.closed.Load() {
		m.svc.fence.RUnlock()
		return ErrSignerRevoked
	}
	return nil
}

func (m *ManagedSigner) revokeWhileRead() {
	if m == nil || m.state == nil {
		return
	}
	m.state.revoked.Store(true)
	if m.state.mat != nil {
		_ = m.state.mat.Destroy()
	}
}

func (m *ManagedSigner) validationFailure(tx pgx.Tx) {
	if tx != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), managedSignerDBTimeout)
		_ = tx.Rollback(closeCtx)
		cancel()
	}
	if m.svc != nil && m.svc.testBeforeSignerValidationFailure != nil {
		m.svc.testBeforeSignerValidationFailure()
	}
	m.revokeWhileRead()
}

func transientValidationError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "40001", "40P01", "55P03", "57014":
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "pool") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "deadline") ||
		strings.Contains(msg, "canceled") ||
		strings.Contains(msg, "conn busy") ||
		strings.Contains(msg, "too many")
}

// Sign computes a candidate signature before opening the bounded durable
// validation transaction. The signature is returned only after the complete
// current row is revalidated under FOR SHARE and the read transaction closes
// successfully. Transient pool/timeout errors fail the call without permanently
// revoking a still-valid active holder.
func (m *ManagedSigner) Sign(random io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	if err := m.acquireRead(); err != nil {
		return nil, err
	}
	defer m.svc.fence.RUnlock()

	signature, err := m.state.mat.Sign(random, digest, opts)
	if err != nil {
		m.revokeWhileRead()
		return nil, ErrSignerRevoked
	}

	dbCtx, cancel := context.WithTimeout(context.Background(), managedSignerDBTimeout)
	defer cancel()
	tx, err := m.svc.pool.BeginTx(dbCtx, pgx.TxOptions{})
	if err != nil {
		if transientValidationError(err) {
			return nil, fmt.Errorf("%w: %v", domain.ErrSignerUnavailable, err)
		}
		m.revokeWhileRead()
		return nil, ErrSignerRevoked
	}

	rec, err := repo.GetSigningKeyByKidForShare(dbCtx, tx, m.kid)
	if err != nil || rec.Kid != m.kid || rec.Status != domain.SigningKeyStatusActive {
		m.validationFailure(tx)
		return nil, ErrSignerRevoked
	}
	if err := temporalUsable(*rec, time.Now().UTC()); err != nil {
		m.validationFailure(tx)
		return nil, ErrSignerRevoked
	}
	if _, err := rec.PublicJWK.MarshalJSON(); err != nil {
		m.validationFailure(tx)
		return nil, ErrSignerRevoked
	}
	fingerprint, err := signingKeyFingerprint(*rec)
	if err != nil || fingerprint != m.state.fingerprint {
		m.validationFailure(tx)
		return nil, ErrSignerRevoked
	}

	if err := tx.Commit(dbCtx); err != nil {
		m.validationFailure(tx)
		return nil, ErrSignerRevoked
	}
	return signature, nil
}

// Public returns a copy of the ECDSA public key, or nil after revocation.
func (m *ManagedSigner) Public() crypto.PublicKey {
	if err := m.acquireRead(); err != nil {
		return nil
	}
	defer m.svc.fence.RUnlock()
	return m.state.mat.Public()
}

// PublicJWK returns the safe public JWK and rejects revoked holders.
func (m *ManagedSigner) PublicJWK() (domain.PublicJWK, error) {
	if err := m.acquireRead(); err != nil {
		return domain.PublicJWK{}, err
	}
	defer m.svc.fence.RUnlock()
	return m.state.mat.PublicJWK()
}

func (m *ManagedSigner) String() string {
	if m == nil {
		return "managed-signer <nil>"
	}
	return fmt.Sprintf("managed-signer kid=%s", m.kid)
}

func (m *ManagedSigner) GoString() string { return m.String() }

func (m ManagedSigner) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, (&m).String())
}

// MarshalJSON refuses serialization rather than risking accidental custody
// representation. Call PublicJWK when a public representation is required.
func (ManagedSigner) MarshalJSON() ([]byte, error) {
	return nil, errors.New("managed signer JSON serialization refused; use PublicJWK")
}

// SignerInPublicSet reports whether the complete active public JWK is present.
func SignerInPublicSet(want domain.PublicJWK, pubs []domain.PublicJWK) bool {
	for _, pub := range pubs {
		if pub == want {
			return true
		}
	}
	return false
}

func (s *Service) generateRecord(status string) (repo.SigningKeyRecord, error) {
	mat, err := Generate()
	if err != nil {
		return repo.SigningKeyRecord{}, err
	}
	defer func() { _ = mat.Destroy() }()
	public, err := mat.PublicJWK()
	if err != nil {
		return repo.SigningKeyRecord{}, err
	}
	sealed, err := Seal(mat, s.cfg.SealKey())
	if err != nil {
		return repo.SigningKeyRecord{}, err
	}
	now := time.Now().UTC()
	rec := repo.SigningKeyRecord{
		Kid:              public.Kid,
		Alg:              domain.SigningAlgES256,
		KeyVersion:       1,
		PublicJWK:        public,
		SealedPrivateKey: sealed,
		Status:           status,
		NotBefore:        now,
	}
	if status == domain.SigningKeyStatusActive {
		activated := now
		rec.ActivatedAt = &activated
	}
	return rec, nil
}

func (s *Service) generateCandidate(status string) (repo.SigningKeyRecord, error) {
	if s.testGenerateRecord != nil {
		return s.testGenerateRecord(status)
	}
	return s.generateRecord(status)
}

func discardCandidate(rec *repo.SigningKeyRecord) {
	if rec == nil {
		return
	}
	zeroBytes(rec.SealedPrivateKey)
	rec.SealedPrivateKey = nil
}
