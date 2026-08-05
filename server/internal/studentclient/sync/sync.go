// Package sync implements the headless pull/flush loop for the student client.
package sync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"sync"
	"time"

	studentapi "github.com/aleksclark/primer/server/internal/studentclient/api"
	"github.com/aleksclark/primer/server/internal/studentclient/cache"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

// Status describes connectivity and last sync outcome for the TUI/harness.
type Status string

const (
	StatusOnline   Status = "online"
	StatusOffline  Status = "offline"
	StatusSyncing  Status = "syncing"
	StatusAwaiting Status = "awaiting_sync"
	StatusRevoked  Status = "revoked"
	StatusIdle     Status = "idle"
)

// Result is the outcome of one SyncOnce call.
type Result struct {
	Status           Status
	WorkItems        int
	EventsFlushed    int
	CompletionsSent  int
	ArtifactsSent    int
	PendingEvents    int
	PendingCompletes int
	Err              error
}

// Loop pulls work and flushes durable outbox/completion/artifact rows.
type Loop struct {
	Client *studentapi.Client
	Store  *cache.Store
	Log    *slog.Logger

	MinBackoff time.Duration
	MaxBackoff time.Duration

	mu     sync.Mutex
	status Status
	last   Result
}

// New constructs a Loop with sensible defaults.
func New(client *studentapi.Client, store *cache.Store) *Loop {
	return &Loop{
		Client:     client,
		Store:      store,
		Log:        slog.Default(),
		MinBackoff: 500 * time.Millisecond,
		MaxBackoff: 30 * time.Second,
		status:     StatusIdle,
	}
}

// Status returns the last known status.
func (l *Loop) Status() Status {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.status
}

// LastResult returns the most recent SyncOnce outcome.
func (l *Loop) LastResult() Result {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.last
}

func (l *Loop) set(status Status, res Result) Result {
	res.Status = status
	l.mu.Lock()
	defer l.mu.Unlock()
	l.status = status
	l.last = res
	return res
}

// DefaultWorkPageLimit is the page size for GET /student/work pagination.
const DefaultWorkPageLimit = 100

// SyncOnce pulls work and flushes pending durable rows once.
// On 401 it sets StatusRevoked and returns *api.ErrUnauthorized.
func (l *Loop) SyncOnce(ctx context.Context) Result {
	res := l.set(StatusSyncing, Result{})

	if err := l.ensureToken(ctx); err != nil {
		res.Err = err
		return l.set(StatusOffline, res)
	}

	// Best-effort capability heartbeat so the server can gate assignment.
	if caps := l.deviceCapabilities(); caps != nil {
		_ = l.Client.ReportCapabilities(ctx, *caps)
	}

	n, err := l.pullWork(ctx)
	if err != nil {
		return l.handleErr(ctx, res, err)
	}
	res.WorkItems = n

	eventsFlushed, err := l.flushEvents(ctx)
	if err != nil {
		res.EventsFlushed = eventsFlushed
		return l.handleErr(ctx, res, err)
	}
	res.EventsFlushed = eventsFlushed

	arts, err := l.flushArtifacts(ctx)
	if err != nil {
		res.ArtifactsSent = arts
		return l.handleErr(ctx, res, err)
	}
	res.ArtifactsSent = arts

	cmps, err := l.flushCompletions(ctx)
	if err != nil {
		res.CompletionsSent = cmps
		return l.handleErr(ctx, res, err)
	}
	res.CompletionsSent = cmps

	if _, err := l.flushResponses(ctx); err != nil {
		return l.handleErr(ctx, res, err)
	}

	pending, err := l.Store.GetPendingSync(ctx)
	if err != nil {
		res.Err = err
		return l.set(StatusOnline, res)
	}
	res.PendingEvents = pending.EventCount
	res.PendingCompletes = pending.CompleteCount
	status := StatusOnline
	if pending.EventCount > 0 || pending.CompleteCount > 0 || pending.ArtifactCount > 0 {
		status = StatusAwaiting
	}
	// Pending responses also mean awaiting sync.
	if rs, err := l.Store.ListPendingResponses(ctx); err == nil && len(rs) > 0 {
		status = StatusAwaiting
	}
	return l.set(status, res)
}

// Run repeatedly calls SyncOnce until ctx is cancelled.
// Backs off on errors; resets delay after a clean pass.
func (l *Loop) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	backoff := l.MinBackoff
	if backoff <= 0 {
		backoff = 500 * time.Millisecond
	}
	maxBO := l.MaxBackoff
	if maxBO <= 0 {
		maxBO = 30 * time.Second
	}

	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		res := l.SyncOnce(ctx)
		if res.Status == StatusRevoked {
			return
		}
		delay := interval
		if res.Err != nil {
			jitter := time.Duration(rand.Int63n(int64(backoff/2) + 1))
			delay = backoff + jitter
			backoff *= 2
			if backoff > maxBO {
				backoff = maxBO
			}
		} else {
			backoff = l.MinBackoff
			if backoff <= 0 {
				backoff = 500 * time.Millisecond
			}
		}
		timer.Reset(delay)
	}
}

func (l *Loop) ensureToken(ctx context.Context) error {
	if l.Client.Token != "" {
		return nil
	}
	tok, err := l.Store.DeviceToken(ctx)
	if err != nil {
		return err
	}
	if tok == "" {
		return fmt.Errorf("no device token configured")
	}
	l.Client.SetToken(tok)
	return nil
}

// pullWork paginates GET /student/work until exhaustion, applying each page
// before advancing the durable cursor. A crash mid-sync resumes from the
// previous committed cursor (or continues an in-progress page cursor).
func (l *Loop) pullWork(ctx context.Context) (int, error) {
	st, err := l.Store.GetWorkSyncState(ctx)
	if err != nil {
		return 0, err
	}

	// Resume in-progress multi-page sync, otherwise start from durable cursor.
	after := st.Cursor
	mode := studentapi.WorkModeIncremental
	if st.InProgress && st.PageCursor != "" {
		after = st.PageCursor
		if st.PageMode != "" {
			mode = st.PageMode
		}
	} else if after == "" {
		mode = studentapi.WorkModeSnapshot
	}

	if err := l.Store.BeginWorkPage(ctx, mode); err != nil {
		return 0, err
	}

	total := 0
	finalCursor := after
	for {
		work, err := l.Client.Work(ctx, after, DefaultWorkPageLimit)
		if err != nil {
			// Invalid/expired cursor: drop durable cursor and restart as snapshot.
			var httpErr *studentapi.ErrHTTP
			if errors.As(err, &httpErr) && httpErr.StatusCode == 400 &&
				(strings.Contains(httpErr.Body, "cursor") || strings.Contains(httpErr.Error(), "cursor")) {
				if rerr := l.Store.ResetWorkSyncCursor(ctx); rerr != nil {
					return total, rerr
				}
				after = ""
				mode = studentapi.WorkModeSnapshot
				if err := l.Store.BeginWorkPage(ctx, mode); err != nil {
					return total, err
				}
				work, err = l.Client.Work(ctx, "", DefaultWorkPageLimit)
				if err != nil {
					return total, err
				}
			} else {
				return total, err
			}
		}
		// Lock mode from the first page of this walk. Subsequent pages carry after=
		// cursors and must not flip a snapshot walk to incremental.
		if total == 0 {
			if work.Mode != "" {
				mode = work.Mode
			} else if after == "" {
				mode = studentapi.WorkModeSnapshot
			}
		}

		// Detect end of page stream: empty page, or fewer than limit without hasMore.
		pageCursor := work.Cursor
		hasMore := work.HasMore
		if !hasMore && len(work.Items) >= DefaultWorkPageLimit && pageCursor != "" && pageCursor != after {
			// Server omitted hasMore but returned a full page — keep paging.
			hasMore = true
		}
		if len(work.Items) == 0 {
			hasMore = false
		}

		if err := l.Store.ApplyWorkPage(ctx, work.Items, pageCursor, mode); err != nil {
			return total, err
		}
		total += len(work.Items)
		if pageCursor != "" {
			finalCursor = pageCursor
		}

		if !hasMore {
			break
		}
		if pageCursor == "" || pageCursor == after {
			// Avoid infinite loop on a stuck cursor.
			break
		}
		after = pageCursor
	}

	if err := l.Store.CommitWorkSync(ctx, finalCursor, mode); err != nil {
		return total, err
	}
	return total, nil
}

// deviceCapabilities builds a report from the local sandbox/runtime environment.
// Returns nil when nothing useful is known (tests without profiles).
func (l *Loop) deviceCapabilities() *studentapi.DeviceCapabilities {
	// Deferred to sandbox helper to avoid import cycles in tests that stub Client.
	return CollectDeviceCapabilities()
}

func (l *Loop) handleErr(ctx context.Context, res Result, err error) Result {
	res.Err = err
	var uerr *studentapi.ErrUnauthorized
	if errors.As(err, &uerr) {
		return l.set(StatusRevoked, res)
	}
	if pending, perr := l.Store.GetPendingSync(ctx); perr == nil {
		res.PendingEvents = pending.EventCount
		res.PendingCompletes = pending.CompleteCount
	}
	status := StatusOffline
	if res.PendingEvents > 0 || res.PendingCompletes > 0 {
		status = StatusAwaiting
	}
	return l.set(status, res)
}

type eventBatch struct {
	clientID string
	serverID string
	events   []cache.OutboxEvent
}

func (l *Loop) flushEvents(ctx context.Context) (int, error) {
	pending, err := l.Store.ListPendingEvents(ctx, 500)
	if err != nil {
		return 0, err
	}
	var batches []eventBatch
	for _, ev := range pending {
		n := len(batches)
		if n == 0 || batches[n-1].clientID != ev.ClientSessionID {
			batches = append(batches, eventBatch{
				clientID: ev.ClientSessionID,
				serverID: ev.ServerSessionID,
				events:   []cache.OutboxEvent{ev},
			})
			continue
		}
		batches[n-1].events = append(batches[n-1].events, ev)
		if batches[n-1].serverID == "" {
			batches[n-1].serverID = ev.ServerSessionID
		}
	}

	flushed := 0
	for _, b := range batches {
		serverID := b.serverID
		if serverID == "" {
			sess, err := l.Store.GetSession(ctx, b.clientID)
			if err != nil {
				return flushed, err
			}
			serverID = sess.ServerSessionID
		}
		if serverID == "" {
			// Cannot flush until StartSession binds a server id.
			continue
		}
		apiEvents := make([]contracts.SessionEvent, 0, len(b.events))
		for _, e := range b.events {
			apiEvents = append(apiEvents, e.Event)
		}
		ack, err := l.Client.PostEvents(ctx, serverID, apiEvents)
		if err != nil {
			return flushed, err
		}
		if err := l.Store.AckEvents(ctx, b.clientID, ack.AcknowledgedSequence); err != nil {
			return flushed, err
		}
		flushed += len(apiEvents)
	}
	return flushed, nil
}

func (l *Loop) flushArtifacts(ctx context.Context) (int, error) {
	pending, err := l.Store.ListPendingArtifacts(ctx)
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, a := range pending {
		serverID := a.ServerSessionID
		if serverID == "" {
			sess, err := l.Store.GetSession(ctx, a.ClientSessionID)
			if err != nil {
				return sent, err
			}
			serverID = sess.ServerSessionID
		}
		if serverID == "" {
			continue
		}
		if _, err := l.Client.PostArtifact(ctx, serverID, a.Meta); err != nil {
			return sent, err
		}
		if err := l.Store.MarkArtifactAcked(ctx, a.ArtifactID); err != nil {
			return sent, err
		}
		sent++
	}
	return sent, nil
}

func (l *Loop) flushResponses(ctx context.Context) (int, error) {
	pending, err := l.Store.ListPendingResponses(ctx)
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, r := range pending {
		serverID := r.ServerSessionID
		if serverID == "" {
			sess, err := l.Store.GetSession(ctx, r.ClientSessionID)
			if err != nil {
				return sent, err
			}
			serverID = sess.ServerSessionID
		}
		if serverID == "" {
			continue
		}
		result, err := l.Client.SubmitResponse(ctx, serverID, r.Request)
		if err != nil {
			return sent, err
		}
		if err := l.Store.MarkResponseAcked(ctx, r.SubmissionID, *result); err != nil {
			return sent, err
		}
		sent++
	}
	return sent, nil
}

func (l *Loop) flushCompletions(ctx context.Context) (int, error) {
	pending, err := l.Store.ListPendingCompletions(ctx)
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, c := range pending {
		serverID := c.ServerSessionID
		if serverID == "" {
			sess, err := l.Store.GetSession(ctx, c.ClientSessionID)
			if err != nil {
				return sent, err
			}
			serverID = sess.ServerSessionID
		}
		if serverID == "" {
			continue
		}
		result, err := l.Client.Complete(ctx, serverID, c.Request)
		if err != nil {
			// Cancelled-after-work: server rejects mastery but evidence stays local.
			// Ack the intent with Accepted=false so we stop retrying forever and
			// never invent mastery from a soft-cancelled assignment.
			var httpErr *studentapi.ErrHTTP
			if errors.As(err, &httpErr) && httpErr.StatusCode == 400 &&
				strings.Contains(strings.ToLower(httpErr.Body+httpErr.Error()), "cancel") {
				rejected := contracts.CompletionResult{
					SchemaVersion: contracts.CompletionSchemaVersion,
					CompletionID:  c.Request.CompletionID,
					Accepted:      false,
					RequestDigest: c.Request.RequestDigest,
					Observations:  c.Request.Observations,
					Message:       "assignment cancelled; evidence retained for parent review",
				}
				if aerr := l.Store.MarkCompletionAcked(ctx, c.CompletionID, rejected); aerr != nil {
					return sent, aerr
				}
				if sess, err := l.Store.GetSession(ctx, c.ClientSessionID); err == nil {
					sess.State = "cancelled_after_work"
					_ = l.Store.SaveSession(ctx, *sess)
				}
				sent++
				continue
			}
			return sent, err
		}
		if err := l.Store.MarkCompletionAcked(ctx, c.CompletionID, *result); err != nil {
			return sent, err
		}
		// Mark local session completed when server accepts.
		if result.Accepted {
			if sess, err := l.Store.GetSession(ctx, c.ClientSessionID); err == nil {
				sess.State = "completed"
				_ = l.Store.SaveSession(ctx, *sess)
			}
		} else if strings.Contains(strings.ToLower(result.Message), "cancel") {
			if sess, err := l.Store.GetSession(ctx, c.ClientSessionID); err == nil {
				sess.State = "cancelled_after_work"
				_ = l.Store.SaveSession(ctx, *sess)
			}
		}
		sent++
	}
	return sent, nil
}
