package token

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/aleksclark/primer/identity/internal/domain"
)

// Minter issues compact ES256 access tokens with an exact returned lifetime.
type Minter struct {
	source SignerSource
	issuer string
	clock  Clock
	state  *minterState
}

type minterState struct {
	jtiGenerator       jtiGenerator
	recentMu           sync.Mutex
	recentJTIs         map[uuid.UUID]struct{}
	recentReservations map[uuid.UUID]uint64
	recentOrder        []uuid.UUID
	nextReservation    uint64
}

const recentJTICapacity = 32

type jtiGenerator func(context.Context) (uuid.UUID, error)

const minterRedacted = "token.Minter{redacted}"

func (Minter) String() string   { return minterRedacted }
func (Minter) GoString() string { return minterRedacted }
func (Minter) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, minterRedacted)
}
func (Minter) MarshalJSON() ([]byte, error) { return nil, denyInvalid() }

// NewMinter constructs a fail-closed minter. issuer must be absolute HTTPS.
func NewMinter(source SignerSource, issuer string, clock Clock) (*Minter, error) {
	if source == nil {
		return nil, denyUnavailable()
	}
	normalized, err := normalizeConfiguredIssuer(issuer)
	if err != nil {
		return nil, err
	}
	return &Minter{
		source: source,
		issuer: normalized,
		clock:  resolveClock(clock),
		state: &minterState{
			jtiGenerator:       newRandomJTI,
			recentJTIs:         make(map[uuid.UUID]struct{}, recentJTICapacity),
			recentReservations: make(map[uuid.UUID]uint64, recentJTICapacity),
			recentOrder:        make([]uuid.UUID, 0, recentJTICapacity),
		},
	}, nil
}

// IssueHuman signs a human access token, then invokes persist before returning it.
func (m *Minter) IssueHuman(ctx context.Context, in HumanInput, persist PersistFunc) (IssuedToken, error) {
	return m.finishIssue(ctx, in, persist, nil, nil)
}

// IssueHumanWithSigner signs with an already-prepared signer and metadata.
// Callers that already hold a transaction-bound signer must use this so mint
// does not check out another database connection.
func (m *Minter) IssueHumanWithSigner(ctx context.Context, in HumanInput, signer Signer, meta *domain.SigningKey, persist PersistFunc) (IssuedToken, error) {
	if signer == nil || meta == nil {
		return IssuedToken{}, denyUnavailable()
	}
	return m.finishIssue(ctx, in, persist, signer, meta)
}

func (m *Minter) finishIssue(ctx context.Context, in HumanInput, persist PersistFunc, signer Signer, meta *domain.SigningKey) (IssuedToken, error) {
	if persist == nil {
		return IssuedToken{}, denyUnavailable()
	}
	issued, err := m.issueHuman(ctx, in, signer, meta)
	if err != nil {
		return IssuedToken{}, err
	}
	if err := persist(ctx, issued); err != nil {
		return IssuedToken{}, persistFailure(err)
	}
	if ctx != nil && ctx.Err() != nil {
		return IssuedToken{}, denyUnavailable()
	}
	return issued, nil
}

func (m *Minter) issueHuman(ctx context.Context, in HumanInput, prepared Signer, preparedMeta *domain.SigningKey) (IssuedToken, error) {
	if m == nil || m.source == nil {
		return IssuedToken{}, denyUnavailable()
	}
	if ctx == nil || ctx.Err() != nil {
		return IssuedToken{}, denyUnavailable()
	}
	operationCtx, cancel := context.WithTimeout(ctx, MintOperationTimeout)
	defer cancel()
	freshness := mintFreshness{
		ctx:          operationCtx,
		clock:        m.clock,
		startedWall:  time.Now(),
		startedClock: m.clock.Now().UTC(),
	}
	freshness.lastClock = freshness.startedClock
	if _, err := freshness.sample(0, false); err != nil {
		return IssuedToken{}, err
	}
	if err := requireHumanSubject(in.Subject); err != nil {
		return IssuedToken{}, err
	}
	if err := requireAudience(in.Audience); err != nil {
		return IssuedToken{}, err
	}
	if err := requireClientID(in.ClientID); err != nil {
		return IssuedToken{}, err
	}
	canonScope, err := canonicalScope(in.Scope)
	if err != nil {
		return IssuedToken{}, err
	}
	if in.TTL <= 0 || in.TTL > MaxLifetime || in.TTL%time.Second != 0 {
		return IssuedToken{}, denyInvalid()
	}
	if _, err := freshness.sample(0, false); err != nil {
		return IssuedToken{}, err
	}

	jti, releaseJTI, err := m.reserveJTI(operationCtx)
	if err != nil {
		return IssuedToken{}, err
	}
	committedJTI := false
	defer func() {
		if !committedJTI {
			releaseJTI()
		}
	}()
	if _, err := freshness.sample(0, false); err != nil {
		return IssuedToken{}, err
	}

	var (
		signer Signer
		meta   *domain.SigningKey
	)
	if prepared != nil {
		signer, meta = prepared, preparedMeta
	} else {
		var sourceErr error
		signer, meta, sourceErr = m.source.Current(operationCtx)
		if sourceErr != nil {
			err = sourceErr
		}
	}
	if _, freshnessErr := freshness.sample(0, false); freshnessErr != nil {
		return IssuedToken{}, freshnessErr
	}
	if err != nil || signer == nil || meta == nil {
		return IssuedToken{}, denyUnavailable()
	}
	jwk, err := signer.PublicJWK()
	if _, freshnessErr := freshness.sample(0, false); freshnessErr != nil {
		return IssuedToken{}, freshnessErr
	}
	if err != nil {
		return IssuedToken{}, denyUnavailable()
	}
	if err := domain.ValidateSigningKid(jwk.Kid); err != nil || jwk.Kid != meta.Kid || jwk.Alg != domain.SigningAlgES256 {
		return IssuedToken{}, denyUnavailable()
	}
	jwkPub, err := jwk.ECDSAPublic()
	if _, freshnessErr := freshness.sample(0, false); freshnessErr != nil {
		return IssuedToken{}, freshnessErr
	}
	if err != nil {
		return IssuedToken{}, denyUnavailable()
	}
	if err := publicMatchesJWK(signer.Public(), jwkPub); err != nil {
		return IssuedToken{}, denyUnavailable()
	}

	now, err := freshness.sample(0, false)
	if err != nil {
		return IssuedToken{}, err
	}
	iat := now.UTC().Truncate(time.Second)
	lifetime := in.TTL
	if !in.GrantNotAfter.IsZero() {
		remain := in.GrantNotAfter.UTC().Truncate(time.Second).Sub(iat)
		if remain <= 0 {
			return IssuedToken{}, denyInvalid()
		}
		if remain < lifetime {
			lifetime = remain
		}
	}
	if !in.ProviderExpiresAt.IsZero() {
		remain := in.ProviderExpiresAt.UTC().Truncate(time.Second).Sub(iat)
		if remain <= 0 {
			return IssuedToken{}, denyInvalid()
		}
		if remain < lifetime {
			lifetime = remain
		}
	}
	if lifetime <= 0 || lifetime > MaxLifetime || lifetime%time.Second != 0 {
		return IssuedToken{}, denyInvalid()
	}
	expUnix := iat.Unix() + int64(lifetime/time.Second)
	if iat.Unix() < numericDateMinUnix || expUnix > numericDateMaxUnix {
		return IssuedToken{}, denyInvalid()
	}
	if _, err := freshness.sample(expUnix, true); err != nil {
		return IssuedToken{}, err
	}

	payload, err := encodeClaims(m.issuer, in.Subject, in.Audience, in.ClientID, canonScope, jti, iat.Unix(), expUnix)
	if err != nil {
		return IssuedToken{}, err
	}
	header, err := encodeHeader(jwk.Kid)
	if err != nil {
		return IssuedToken{}, err
	}
	signingInput := header + "." + payload
	sum := sha256.Sum256([]byte(signingInput))
	der, signErr := signer.Sign(rand.Reader, sum[:], crypto.SHA256)
	if _, err := freshness.sample(expUnix, true); err != nil {
		return IssuedToken{}, err
	}
	if signErr != nil || len(der) == 0 {
		return IssuedToken{}, denyUnavailable()
	}
	rawSig, err := canonicalizeES256Signature(der)
	if err != nil {
		return IssuedToken{}, denyUnavailable()
	}
	if err := verifyES256(jwkPub, []byte(signingInput), rawSig); err != nil {
		return IssuedToken{}, denyUnavailable()
	}
	compact := signingInput + "." + encodeRawURL(rawSig)
	if len(compact) > MaxTokenBytes {
		return IssuedToken{}, denyInvalid()
	}
	if _, err := freshness.sample(expUnix, true); err != nil {
		return IssuedToken{}, err
	}
	committedJTI = true
	return IssuedToken{
		Compact:   compact,
		Lifetime:  lifetime,
		IssuedAt:  iat.UTC(),
		NotBefore: iat.UTC(),
		ExpiresAt: time.Unix(expUnix, 0).UTC(),
		JTI:       jti,
		Kid:       jwk.Kid,
	}, nil
}

type mintFreshness struct {
	ctx          context.Context
	clock        Clock
	startedWall  time.Time
	startedClock time.Time
	lastClock    time.Time
}

func (f *mintFreshness) sample(exp int64, enforceExpiry bool) (time.Time, error) {
	if f == nil || f.ctx == nil || f.clock == nil {
		return time.Time{}, denyUnavailable()
	}
	if f.ctx.Err() != nil || time.Since(f.startedWall) >= MintOperationTimeout {
		return time.Time{}, denyUnavailable()
	}
	now := f.clock.Now().UTC()
	if now.Before(f.lastClock) || now.Sub(f.startedClock) >= MintOperationTimeout {
		return time.Time{}, denyUnavailable()
	}
	f.lastClock = now
	if enforceExpiry && now.Unix() >= exp {
		return time.Time{}, denyUnavailable()
	}
	if f.ctx.Err() != nil || time.Since(f.startedWall) >= MintOperationTimeout {
		return time.Time{}, denyUnavailable()
	}
	return now, nil
}

func persistFailure(err error) error {
	if isRetryableSerialization(err) {
		return domain.ErrRetryableSerialization
	}
	return denyUnavailable()
}

func isRetryableSerialization(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, domain.ErrRetryableSerialization) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "SQLSTATE 40001") || strings.Contains(msg, "SQLSTATE 40P01")
}

func newRandomJTI(ctx context.Context) (uuid.UUID, error) {
	if ctx == nil || ctx.Err() != nil {
		return uuid.Nil, denyUnavailable()
	}
	jti, err := uuid.NewRandomFromReader(rand.Reader)
	if err != nil {
		return uuid.Nil, err
	}
	if ctx.Err() != nil {
		return uuid.Nil, denyUnavailable()
	}
	return jti, nil
}

func (m *Minter) reserveJTI(ctx context.Context) (string, func(), error) {
	if m == nil || m.state == nil || m.state.jtiGenerator == nil || ctx == nil || ctx.Err() != nil {
		return "", func() {}, denyUnavailable()
	}
	jti, err := m.state.jtiGenerator(ctx)
	if err != nil || jti == uuid.Nil || ctx.Err() != nil {
		return "", func() {}, denyUnavailable()
	}
	m.state.recentMu.Lock()
	defer m.state.recentMu.Unlock()
	if m.state.recentJTIs == nil {
		m.state.recentJTIs = make(map[uuid.UUID]struct{}, recentJTICapacity)
	}
	if m.state.recentReservations == nil {
		m.state.recentReservations = make(map[uuid.UUID]uint64, recentJTICapacity)
	}
	if _, exists := m.state.recentJTIs[jti]; exists {
		return "", func() {}, denyUnavailable()
	}
	m.state.nextReservation++
	reservation := m.state.nextReservation
	m.state.recentJTIs[jti] = struct{}{}
	m.state.recentReservations[jti] = reservation
	m.state.recentOrder = append(m.state.recentOrder, jti)
	if len(m.state.recentOrder) > recentJTICapacity {
		oldest := m.state.recentOrder[0]
		m.state.recentOrder = m.state.recentOrder[1:]
		delete(m.state.recentJTIs, oldest)
		delete(m.state.recentReservations, oldest)
	}
	return jti.String(), func() { m.releaseJTI(jti, reservation) }, nil
}

func (m *Minter) releaseJTI(jti uuid.UUID, reservation uint64) {
	if m == nil || m.state == nil {
		return
	}
	m.state.recentMu.Lock()
	defer m.state.recentMu.Unlock()
	if current, ok := m.state.recentReservations[jti]; !ok || current != reservation {
		return
	}
	delete(m.state.recentReservations, jti)
	delete(m.state.recentJTIs, jti)
	for i, candidate := range m.state.recentOrder {
		if candidate == jti {
			m.state.recentOrder = append(m.state.recentOrder[:i], m.state.recentOrder[i+1:]...)
			break
		}
	}
}

func encodeHeader(kid string) (string, error) {
	var buf bytes.Buffer
	buf.WriteString(`{"alg":`)
	if err := writeJSONString(&buf, Algorithm); err != nil {
		return "", err
	}
	buf.WriteString(`,"typ":`)
	if err := writeJSONString(&buf, HeaderType); err != nil {
		return "", err
	}
	buf.WriteString(`,"kid":`)
	if err := writeJSONString(&buf, kid); err != nil {
		return "", err
	}
	buf.WriteByte('}')
	raw := buf.Bytes()
	if len(raw) > maxHeaderBytes {
		return "", denyInvalid()
	}
	return encodeRawURL(raw), nil
}

func encodeClaims(iss, sub, aud, clientID, scope, jti string, iat, exp int64) (string, error) {
	var buf bytes.Buffer
	buf.WriteString(`{"iss":`)
	if err := writeJSONString(&buf, iss); err != nil {
		return "", err
	}
	buf.WriteString(`,"sub":`)
	if err := writeJSONString(&buf, sub); err != nil {
		return "", err
	}
	buf.WriteString(`,"aud":`)
	if err := writeJSONString(&buf, aud); err != nil {
		return "", err
	}
	buf.WriteString(`,"exp":`)
	buf.WriteString(fmtInt(exp))
	buf.WriteString(`,"iat":`)
	buf.WriteString(fmtInt(iat))
	buf.WriteString(`,"nbf":`)
	buf.WriteString(fmtInt(iat))
	buf.WriteString(`,"jti":`)
	if err := writeJSONString(&buf, jti); err != nil {
		return "", err
	}
	buf.WriteString(`,"client_id":`)
	if err := writeJSONString(&buf, clientID); err != nil {
		return "", err
	}
	buf.WriteString(`,"scope":`)
	if err := writeJSONString(&buf, scope); err != nil {
		return "", err
	}
	buf.WriteByte('}')
	raw := buf.Bytes()
	if len(raw) > maxPayloadBytes {
		return "", denyInvalid()
	}
	return encodeRawURL(raw), nil
}
