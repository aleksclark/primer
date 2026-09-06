package verification

// Original P4 authority helpers, adapted so questions AND messages are distinct
// bound evidence. These rules are shared by the durable engine, not a second
// client/model completion path.
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	tasksdb "primer-tasks/internal/db"
	"primer-tasks/internal/domain"
)

var (
	ErrDialogueContext     = errors.New("invalid dialogue server context")
	ErrDialogueQuestion    = errors.New("invalid dialogue question")
	ErrDialogueEvaluation  = errors.New("invalid dialogue evaluation")
	ErrDialogueTerminal    = errors.New("dialogue attempt is terminal")
	ErrDuplicateQuestion   = errors.New("dialogue question is not distinct")
	ErrDuplicateEvaluation = errors.New("dialogue evaluation is already recorded")
	ErrDialogueLimit       = errors.New("dialogue policy limit reached")
	ErrDialogueConflict    = errors.New("dialogue state changed")
)

type DialogueContext struct {
	TenantID, StudentID, OccurrenceID, RequirementID, AttemptID string
	PolicyVersion, SnapshotDigest                               string
}

func (c DialogueContext) Validate() error {
	for _, value := range []string{c.TenantID, c.StudentID, c.OccurrenceID, c.RequirementID, c.AttemptID, c.SnapshotDigest} {
		if strings.TrimSpace(value) == "" {
			return ErrDialogueContext
		}
	}
	if c.PolicyVersion != domain.DialoguePolicyVersion {
		return ErrDialogueContext
	}
	return nil
}

type DialogueState struct {
	Context          DialogueContext
	Snapshot         domain.DialogueSnapshot
	Version          int64
	Terminal         bool
	TerminalStatus   string
	OccurrenceStatus string
	Questions        []DialogueQuestion
	Messages         []DialogueMessage
	Evaluations      []DialogueEvaluation
}

type DialogueQuestion struct {
	ID, AttemptID, QuestionKey, Prompt string
	Ordinal                            int
	Version                            int64
	CreatedAt                          time.Time
}

type DialogueMessage struct {
	ID, AttemptID, QuestionID, PolicyVersion, SnapshotDigest string
	Role, Content, ClientMessageID                           string
	Sequence, ExpectedVersion                                int64
	CreatedAt                                                time.Time
}

type DialogueEvaluation struct {
	ID, AttemptID, QuestionID, MessageID string
	PolicyVersion, SnapshotDigest        string
	Accepted                             bool
	Criteria                             []string
	Rationale, Provider, Model           string
	InputTokens, OutputTokens            int64
	Version                              int64
	CreatedAt                            time.Time
}

type DecisionReady struct {
	Accepted      bool
	AcceptedCount int
	RequiredCount int
	Reason        string
}

func (s DialogueState) Validate() error {
	if err := s.Context.Validate(); err != nil {
		return err
	}
	if err := s.Snapshot.Validate(); err != nil {
		return err
	}
	if s.Version < 1 || s.Snapshot.RequirementID != s.Context.RequirementID || s.Snapshot.Digest != s.Context.SnapshotDigest || s.Snapshot.PolicyVersion != s.Context.PolicyVersion {
		return ErrDialogueContext
	}
	return nil
}

// Short server-owned rationale vocabulary: arbitrary provider prose, even if
// it evades a keyword blacklist, is never persisted as an evaluation rationale.
const (
	DialogueAcceptedRationale = "Answer addresses the current question using the assigned source."
	DialogueRetryRationale    = "Add a specific detail or reason from the assigned source."
)

func SafeRationale(raw string) (string, error) {
	if raw != DialogueAcceptedRationale && raw != DialogueRetryRationale {
		return "", ErrDialogueEvaluation
	}
	return raw, nil
}

// DialogueEvidence checks persisted evidence without trusting a mutable count.
// Each answer may evaluate exactly one question; each distinct question counts
// once. Every row must match the snapshotted policy and the durable message.
func DialogueEvidence(s DialogueState) (DecisionReady, error) {
	ready := DecisionReady{RequiredCount: domain.DialogueRequiredQuestions}
	if err := s.Validate(); err != nil {
		return ready, err
	}
	questions := map[string]DialogueQuestion{}
	keys, prompts := map[string]bool{}, map[string]bool{}
	for i, q := range s.Questions {
		prompt := strings.ToLower(strings.TrimSpace(q.Prompt))
		if q.ID == "" || q.AttemptID != s.Context.AttemptID || q.Ordinal != i+1 || q.Ordinal > ready.RequiredCount || q.Version < 1 || q.Version > s.Version || strings.TrimSpace(q.QuestionKey) == "" || prompt == "" || len([]rune(q.Prompt)) > 2000 {
			return ready, ErrDialogueQuestion
		}
		planned := s.Snapshot.Questions[q.Ordinal-1]
		if q.QuestionKey != planned.Key || q.Prompt != planned.Prompt {
			return ready, ErrDialogueQuestion
		}
		if _, exists := questions[q.ID]; exists || keys[q.QuestionKey] || prompts[prompt] {
			return ready, ErrDuplicateQuestion
		}
		questions[q.ID], keys[q.QuestionKey], prompts[prompt] = q, true, true
	}
	messages := map[string]DialogueMessage{}
	clientIDs := map[string]bool{}
	for i, m := range s.Messages {
		q, exists := questions[m.QuestionID]
		if !exists || m.ID == "" || m.AttemptID != s.Context.AttemptID || m.PolicyVersion != s.Context.PolicyVersion || m.SnapshotDigest != s.Context.SnapshotDigest || m.Role != "student" || strings.TrimSpace(m.Content) == "" || len(m.Content) > 12000 || m.ClientMessageID == "" || len(m.ClientMessageID) > 128 || m.Sequence != int64(i+1) || m.ExpectedVersion < q.Version || m.ExpectedVersion > s.Version {
			return ready, ErrDialogueEvaluation
		}
		if _, duplicate := messages[m.ID]; duplicate || clientIDs[m.ClientMessageID] {
			return ready, ErrDialogueEvaluation
		}
		messages[m.ID], clientIDs[m.ClientMessageID] = m, true
	}
	accepted, evaluated := map[string]bool{}, map[string]bool{}
	acceptedVersions := map[string]int64{}
	for _, e := range s.Evaluations {
		m, found := messages[e.MessageID]
		if !found || e.ID == "" || e.AttemptID != s.Context.AttemptID || e.QuestionID != m.QuestionID || e.PolicyVersion != s.Context.PolicyVersion || e.SnapshotDigest != s.Context.SnapshotDigest || e.Provider == "" || e.Model == "" || len(e.Provider) > 128 || len(e.Model) > 128 || e.InputTokens < 0 || e.OutputTokens < 0 || e.Version <= m.ExpectedVersion+1 || e.Version > s.Version {
			return ready, ErrDialogueEvaluation
		}
		if evaluated[e.MessageID] || accepted[e.QuestionID] {
			return ready, ErrDuplicateEvaluation
		}
		if (e.Accepted && e.Rationale != DialogueAcceptedRationale) || (!e.Accepted && e.Rationale != DialogueRetryRationale) {
			return ready, ErrDialogueEvaluation
		}
		if e.Accepted && len(e.Criteria) == 0 {
			return ready, ErrDialogueEvaluation
		}
		seen := map[string]bool{}
		for _, criterion := range e.Criteria {
			known := false
			for _, allowed := range s.Snapshot.Config.Rubric {
				known = known || criterion == allowed
			}
			if !known || seen[criterion] {
				return ready, ErrDialogueEvaluation
			}
			seen[criterion] = true
		}
		evaluated[e.MessageID] = true
		if e.Accepted {
			accepted[e.QuestionID] = true
			acceptedVersions[e.QuestionID] = e.Version
		}
	}
	// Later questions must never be issued before all preceding questions are
	// accepted. Pending/rejected answers to the current question do not advance.
	for i, q := range s.Questions[:max(0, len(s.Questions)-1)] {
		if !accepted[q.ID] || acceptedVersions[q.ID] >= s.Questions[i+1].Version {
			return ready, ErrDialogueQuestion
		}
	}
	ready.AcceptedCount = len(accepted)
	ready.Accepted = ready.AcceptedCount == ready.RequiredCount
	if ready.Accepted {
		ready.Reason = "required distinct dialogue questions accepted"
	}
	return ready, nil
}

func RecordQuestion(s DialogueState, q DialogueQuestion) (DialogueState, error) {
	ready, err := DialogueEvidence(s)
	if err != nil {
		return s, err
	}
	if s.Terminal || ready.Accepted {
		return s, ErrDialogueTerminal
	}
	if len(s.Questions) != ready.AcceptedCount || q.Ordinal != len(s.Questions)+1 || q.AttemptID != s.Context.AttemptID || q.Version != s.Version+1 {
		return s, ErrDialogueQuestion
	}
	next := s
	next.Version++
	next.Questions = append(append([]DialogueQuestion(nil), s.Questions...), q)
	if _, err = DialogueEvidence(next); err != nil {
		return s, err
	}
	return next, nil
}

func RecordMessage(s DialogueState, m DialogueMessage) (DialogueState, error) {
	ready, err := DialogueEvidence(s)
	if err != nil {
		return s, err
	}
	if s.Terminal || ready.Accepted {
		return s, ErrDialogueTerminal
	}
	if m.ExpectedVersion != s.Version || len(s.Questions) == 0 || ready.AcceptedCount == len(s.Questions) || m.QuestionID != s.Questions[len(s.Questions)-1].ID {
		return s, ErrDialogueConflict
	}
	answers, evaluations := 0, 0
	for _, previous := range s.Messages {
		if previous.QuestionID == m.QuestionID {
			answers++
		}
	}
	for _, previous := range s.Evaluations {
		if previous.QuestionID == m.QuestionID {
			evaluations++
		}
	}
	if answers != evaluations {
		return s, ErrDialogueConflict
	}
	if answers >= 1+s.Snapshot.Config.AllowedFollowUps || len(s.Messages) >= s.Snapshot.Config.MaxTurns {
		return s, ErrDialogueLimit
	}
	next := s
	next.Version++
	next.Messages = append(append([]DialogueMessage(nil), s.Messages...), m)
	if _, err = DialogueEvidence(next); err != nil {
		return s, err
	}
	return next, nil
}

func RecordEvaluation(s DialogueState, e DialogueEvaluation) (DialogueState, DecisionReady, error) {
	ready, err := DialogueEvidence(s)
	if err != nil {
		return s, ready, err
	}
	if s.Terminal || ready.Accepted {
		return s, ready, ErrDialogueTerminal
	}
	if len(s.Questions) == 0 || len(s.Messages) == 0 || e.Version != s.Version+1 || e.QuestionID != s.Questions[len(s.Questions)-1].ID || e.MessageID != s.Messages[len(s.Messages)-1].ID {
		return s, ready, ErrDialogueEvaluation
	}
	next := s
	next.Version++
	e.Criteria = append([]string(nil), e.Criteria...)
	next.Evaluations = append(append([]DialogueEvaluation(nil), s.Evaluations...), e)
	ready, err = DialogueEvidence(next)
	if err != nil {
		return s, ready, err
	}
	return next, ready, nil
}

// StudentAuthority is obtained from a host-only cookie lookup or the original
// admitted job row. It contains identifiers, never a credential or provider
// claim. Every operation revalidates it under student/session row locks.
type StudentAuthority struct{ TenantID, StudentID, SessionID string }
type DialogueLease struct {
	JobID, Owner string
	Generation   int64
}
type DialogueEngine struct{ DB tasksdb.Database }

var ErrDialogueRevoked = errors.New("student authority unavailable")
var ErrDialogueLease = errors.New("dialogue lease unavailable")

// DialogueEvent is the actual Go wire/persistence boundary. The student socket
// and later offline client emission use this type, not a copied client DTO.
const DialogueProtocolVersion = 1

type DialogueEvent struct {
	Protocol         int       `json:"protocol" wire:"*!"`
	Kind             string    `json:"kind" wire:"*!"`
	Sequence         int64     `json:"sequence" wire:"*!" wireMin:"0"`
	Cursor           int64     `json:"cursor" wire:"*!" wireMin:"0"`
	Time             time.Time `json:"time" wire:"*!" wireFormat:"date-time"`
	OccurrenceID     string    `json:"occurrenceId,omitempty" wire:"scope!" wireFormat:"uuid"`
	AttemptID        string    `json:"attemptId,omitempty" wire:"scope!" wireFormat:"uuid"`
	RequirementID    string    `json:"requirementId,omitempty" wire:"scope!" wireFormat:"uuid"`
	PolicyVersion    string    `json:"policyVersion,omitempty" wire:"scope!" wireEnum:"*=dialogue.v1"`
	SnapshotDigest   string    `json:"snapshotDigest,omitempty" wire:"scope!" wireFormat:"sha256"`
	Version          int64     `json:"version,omitempty" wire:"scope!" wireMin:"1"`
	QuestionID       string    `json:"questionId,omitempty" wire:"state,question!,message_ack!,answer_evaluation!" wireFormat:"uuid"`
	MessageID        string    `json:"messageId,omitempty" wire:"message_ack!,answer_evaluation!" wireFormat:"uuid"`
	ClientMessageID  string    `json:"clientMessageId,omitempty" wire:"message_ack!" wireMinLength:"1" wireMaxLength:"128"`
	Text             string    `json:"text,omitempty" wire:"state,question!,message_ack!,answer_evaluation!" wireMinLength:"1" wireMaxBytes:"12000" wireNonBlank:"true"`
	Phase            string    `json:"phase,omitempty" wire:"state,progress!" wireEnum:"state=failed|thinking|evaluating;progress=thinking|evaluating"`
	Status           string    `json:"status,omitempty" wire:"state!,answer_evaluation!,complete!,override!,terminal_error!" wireEnum:"state=open|accepted|rejected|exhausted;answer_evaluation=accepted|rejected;complete=accepted;override=accepted|rejected;terminal_error=exhausted"`
	OccurrenceStatus string    `json:"occurrenceStatus,omitempty" wire:"state!,complete!,override!,terminal_error!" wireEnum:"state=pending|in_progress|awaiting_verification|completed|excused|canceled;complete=completed;override=pending|awaiting_verification|completed;terminal_error=pending"`
	AcceptedCount    int       `json:"acceptedCount" wire:"*!" wireMin:"0" wireMax:"3" wireEnum:"hello=0;error=0"`
	RequiredCount    int       `json:"requiredCount" wire:"*!" wireEnum:"scope=3;hello=0;error=0"`
	DecisionID       string    `json:"decisionId,omitempty" wire:"complete!,override!,terminal_error!" wireFormat:"uuid"`
	DecisionSource   string    `json:"decisionSource,omitempty" wire:"state,complete!,override!,terminal_error!" wireEnum:"state=verification_engine|parent_override;complete=verification_engine|parent_override;override=parent_override;terminal_error=verification_engine"`
	Code             string    `json:"code,omitempty" wire:"state,error!,terminal_error!" wireEnum:"state=provider_unavailable;error=unavailable|revoked|not_found|conflict|invalid_request|exhausted;terminal_error=exhausted|provider_exhausted|deadline_exhausted|lease_exhausted|job_budget_exhausted"`
	Retryable        bool      `json:"retryable,omitempty" wire:"state,error"`
}

func ResolveStudentAuthority(ctx context.Context, db tasksdb.Database, handleHash []byte) (a StudentAuthority, err error) {
	if len(handleHash) != 32 {
		return a, ErrDialogueRevoked
	}
	err = db.QueryRow(ctx, `SELECT s.tenant_id,s.student_id,s.id FROM student_sessions s JOIN students st ON st.tenant_id=s.tenant_id AND st.id=s.student_id WHERE s.handle_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND st.archived_at IS NULL`, handleHash).Scan(&a.TenantID, &a.StudentID, &a.SessionID)
	if err != nil {
		return a, ErrDialogueRevoked
	}
	return a, nil
}

// Lock in the same student -> session order as archiveStudent. Check predicates
// in NEW statements after every waiting lock, including the occurrence lock in
// LockDialogueAccess. Holding these locks through ONE bounded frame linearizes
// private delivery against revocation, rather than check-then-write.
func LockStudentAuthority(ctx context.Context, tx pgx.Tx, a StudentAuthority) error {
	var id string
	if err := tx.QueryRow(ctx, `SELECT id FROM students WHERE tenant_id=$1 AND id=$2 FOR SHARE`, a.TenantID, a.StudentID).Scan(&id); err != nil {
		return ErrDialogueRevoked
	}
	if err := tx.QueryRow(ctx, `SELECT id FROM student_sessions WHERE id=$1 FOR SHARE`, a.SessionID).Scan(&id); err != nil {
		return ErrDialogueRevoked
	}
	return CheckLockedStudentAuthority(ctx, tx, a)
}
func CheckLockedStudentAuthority(ctx context.Context, tx pgx.Tx, a StudentAuthority) error {
	var allowed bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM student_sessions s JOIN students st ON st.tenant_id=s.tenant_id AND st.id=s.student_id WHERE s.id=$1 AND s.tenant_id=$2 AND s.student_id=$3 AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND st.archived_at IS NULL)`, a.SessionID, a.TenantID, a.StudentID).Scan(&allowed); err != nil || !allowed {
		return ErrDialogueRevoked
	}
	return nil
}
func LockDialogueAccess(ctx context.Context, tx pgx.Tx, a StudentAuthority, occurrence, attempt string, write bool) error {
	if err := LockStudentAuthority(ctx, tx, a); err != nil {
		return err
	}
	// Dialogue ownership is immutable (including its occurrence once bound).
	// Readers hold the student/session revocation locks, not mutable progress
	// rows through network writes: a slow reader cannot stall a worker effect.
	lock := ""
	if write {
		lock = " FOR UPDATE"
	}
	var id string
	if err := tx.QueryRow(ctx, `SELECT id FROM task_occurrences WHERE tenant_id=$1 AND student_id=$2 AND id=$3`+lock, a.TenantID, a.StudentID, occurrence).Scan(&id); err != nil {
		return ErrDialogueContext
	}
	if attempt != "" {
		if err := tx.QueryRow(ctx, `SELECT attempt_id FROM dialogue_attempts WHERE tenant_id=$1 AND student_id=$2 AND occurrence_id=$3 AND attempt_id=$4`+lock, a.TenantID, a.StudentID, occurrence, attempt).Scan(&id); err != nil {
			return ErrDialogueContext
		}
	}
	return CheckLockedStudentAuthority(ctx, tx, a)
}
func lockDialogueLease(ctx context.Context, tx pgx.Tx, a StudentAuthority, attempt string, lease DialogueLease) (string, string, error) {
	var id, stage, message string
	if err := tx.QueryRow(ctx, `SELECT id FROM verification_jobs WHERE id=$1 FOR UPDATE`, lease.JobID).Scan(&id); err != nil {
		return "", "", ErrDialogueLease
	}
	// A locking SELECT can evaluate predicates before waiting. Use fresh
	// statements AFTER the last authority/lease lock, including clock expiry.
	if err := CheckLockedStudentAuthority(ctx, tx, a); err != nil {
		return "", "", err
	}
	if err := tx.QueryRow(ctx, `SELECT stage,COALESCE(message_id::text,'') FROM verification_jobs WHERE id=$1 AND tenant_id=$2 AND attempt_id=$3 AND session_id=$4 AND lease_owner=$5 AND lease_generation=$6 AND status='running' AND lease_until>clock_timestamp() AND deadline>clock_timestamp()`, lease.JobID, a.TenantID, attempt, a.SessionID, lease.Owner, lease.Generation).Scan(&stage, &message); err != nil {
		return "", "", ErrDialogueLease
	}
	return stage, message, nil
}

func LoadDialogueState(ctx context.Context, db tasksdb.Database, a StudentAuthority, occurrence, attempt string) (s DialogueState, err error) {
	var raw []byte
	err = db.QueryRow(ctx, `SELECT d.requirement_id,d.policy_version,d.snapshot_digest,d.config_snapshot,d.version,v.status,o.status FROM dialogue_attempts d JOIN verification_attempts v ON v.tenant_id=d.tenant_id AND v.id=d.attempt_id JOIN task_occurrences o ON o.tenant_id=d.tenant_id AND o.id=d.occurrence_id WHERE d.tenant_id=$1 AND d.student_id=$2 AND d.occurrence_id=$3 AND d.attempt_id=$4 AND v.number=(SELECT max(number) FROM verification_attempts WHERE tenant_id=v.tenant_id AND occurrence_id=v.occurrence_id AND requirement_id=v.requirement_id)`, a.TenantID, a.StudentID, occurrence, attempt).Scan(&s.Context.RequirementID, &s.Context.PolicyVersion, &s.Context.SnapshotDigest, &raw, &s.Version, &s.TerminalStatus, &s.OccurrenceStatus)
	if err != nil {
		return s, ErrDialogueContext
	}
	s.Context.TenantID, s.Context.StudentID, s.Context.OccurrenceID, s.Context.AttemptID = a.TenantID, a.StudentID, occurrence, attempt
	if json.Unmarshal(raw, &s.Snapshot) != nil {
		return s, ErrDialogueContext
	}
	s.Terminal = s.TerminalStatus != "open" || domain.IsTerminal(domain.OccurrenceStatus(s.OccurrenceStatus))
	rows, err := db.Query(ctx, `SELECT id,attempt_id,question_key,prompt,ordinal,version,created_at FROM dialogue_questions WHERE tenant_id=$1 AND attempt_id=$2 ORDER BY ordinal LIMIT 4`, a.TenantID, attempt)
	if err != nil {
		return s, err
	}
	for rows.Next() {
		var q DialogueQuestion
		if err = rows.Scan(&q.ID, &q.AttemptID, &q.QuestionKey, &q.Prompt, &q.Ordinal, &q.Version, &q.CreatedAt); err != nil {
			break
		}
		s.Questions = append(s.Questions, q)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return s, err
	}
	rows, err = db.Query(ctx, `SELECT id,attempt_id,question_id,policy_version,snapshot_digest,role,content,client_message_id,sequence,expected_version,created_at FROM verification_messages WHERE tenant_id=$1 AND attempt_id=$2 ORDER BY sequence LIMIT 101`, a.TenantID, attempt)
	if err != nil {
		return s, err
	}
	for rows.Next() {
		var m DialogueMessage
		if err = rows.Scan(&m.ID, &m.AttemptID, &m.QuestionID, &m.PolicyVersion, &m.SnapshotDigest, &m.Role, &m.Content, &m.ClientMessageID, &m.Sequence, &m.ExpectedVersion, &m.CreatedAt); err != nil {
			break
		}
		s.Messages = append(s.Messages, m)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return s, err
	}
	rows, err = db.Query(ctx, `SELECT id,attempt_id,question_id,message_id,policy_version,snapshot_digest,accepted,criteria,rationale,provider,model,input_tokens,output_tokens,version,created_at FROM verification_evaluations WHERE tenant_id=$1 AND attempt_id=$2 ORDER BY version LIMIT 101`, a.TenantID, attempt)
	if err != nil {
		return s, err
	}
	for rows.Next() {
		var v DialogueEvaluation
		if err = rows.Scan(&v.ID, &v.AttemptID, &v.QuestionID, &v.MessageID, &v.PolicyVersion, &v.SnapshotDigest, &v.Accepted, &v.Criteria, &v.Rationale, &v.Provider, &v.Model, &v.InputTokens, &v.OutputTokens, &v.Version, &v.CreatedAt); err != nil {
			break
		}
		s.Evaluations = append(s.Evaluations, v)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return s, err
	}
	_, err = DialogueEvidence(s)
	return s, err
}

func DialogueStateEvent(s DialogueState) (DialogueEvent, error) {
	ready, err := DialogueEvidence(s)
	if err != nil {
		return DialogueEvent{}, err
	}
	e := DialogueEvent{Protocol: DialogueProtocolVersion, Kind: "state", Time: time.Now().UTC(), OccurrenceID: s.Context.OccurrenceID, AttemptID: s.Context.AttemptID, RequirementID: s.Context.RequirementID, PolicyVersion: s.Context.PolicyVersion, SnapshotDigest: s.Context.SnapshotDigest, Version: s.Version, Status: s.TerminalStatus, OccurrenceStatus: s.OccurrenceStatus, AcceptedCount: ready.AcceptedCount, RequiredCount: ready.RequiredCount}
	if len(s.Questions) > 0 {
		q := s.Questions[len(s.Questions)-1]
		e.QuestionID, e.Text = q.ID, q.Prompt
	}
	return e, nil
}

func appendDialogueEvent(ctx context.Context, tx pgx.Tx, s DialogueState, key string, event DialogueEvent) error {
	// Every caller holds this attempt/occurrence lock. Stable event keys make
	// replay of a committed stage idempotent, not another terminal event.
	event.Protocol, event.Time, event.OccurrenceID, event.AttemptID = DialogueProtocolVersion, time.Now().UTC(), s.Context.OccurrenceID, s.Context.AttemptID
	event.RequirementID, event.PolicyVersion, event.SnapshotDigest, event.Version = s.Context.RequirementID, s.Context.PolicyVersion, s.Context.SnapshotDigest, s.Version
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(sequence),0)+1 FROM verification_events WHERE tenant_id=$1 AND attempt_id=$2`, s.Context.TenantID, s.Context.AttemptID).Scan(&event.Sequence); err != nil {
		return err
	}
	event.Cursor = event.Sequence
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO verification_events(tenant_id,attempt_id,sequence,event_key,kind,payload) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(tenant_id,attempt_id,event_key) DO NOTHING`, s.Context.TenantID, s.Context.AttemptID, event.Sequence, key, event.Kind, payload)
	return err
}

// Start creates the generic attempt, policy projection and initial question job
// together. A reconnect sees the same attempt; no guessed/fallback question is
// sent before the actual Fantasy question tool and persistence commit.
func (e DialogueEngine) Start(ctx context.Context, a StudentAuthority, occurrence, requirement string) (s DialogueState, err error) {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return s, err
	}
	defer tx.Rollback(ctx)
	if err = LockDialogueAccess(ctx, tx, a, occurrence, "", true); err != nil {
		return s, err
	}
	var status, revision string
	if err = tx.QueryRow(ctx, `SELECT status,revision_id FROM task_occurrences WHERE tenant_id=$1 AND id=$2`, a.TenantID, occurrence).Scan(&status, &revision); err != nil {
		return s, err
	}
	if domain.IsTerminal(domain.OccurrenceStatus(status)) {
		return s, ErrDialogueTerminal
	}
	if requirement == "" {
		if err = tx.QueryRow(ctx, `SELECT requirement_id FROM dialogue_revision_policies WHERE tenant_id=$1 AND revision_id=$2 ORDER BY requirement_id LIMIT 1`, a.TenantID, revision).Scan(&requirement); err != nil {
			return s, ErrDialogueContext
		}
	}
	var raw []byte
	var snapshot domain.DialogueSnapshot
	if err = tx.QueryRow(ctx, `SELECT snapshot FROM dialogue_revision_policies WHERE tenant_id=$1 AND revision_id=$2 AND requirement_id=$3`, a.TenantID, revision, requirement).Scan(&raw); err != nil {
		return s, ErrDialogueContext
	}
	if json.Unmarshal(raw, &snapshot) != nil || snapshot.Validate() != nil {
		return s, ErrDialogueContext
	}
	var attempt, attemptStatus string
	var number int
	err = tx.QueryRow(ctx, `SELECT id,status,number FROM verification_attempts WHERE tenant_id=$1 AND occurrence_id=$2 AND requirement_id=$3 ORDER BY number DESC LIMIT 1 FOR UPDATE`, a.TenantID, occurrence, requirement).Scan(&attempt, &attemptStatus, &number)
	if errors.Is(err, pgx.ErrNoRows) {
		attempt, number = uuid.NewString(), 1
		_, err = tx.Exec(ctx, `INSERT INTO verification_attempts(id,tenant_id,occurrence_id,requirement_id,number) VALUES($1,$2,$3,$4,1)`, attempt, a.TenantID, occurrence, requirement)
	} else if err == nil && attemptStatus != "open" {
		return s, ErrDialogueTerminal
	}
	if err != nil {
		return s, err
	}
	if number > snapshot.Config.MaxAttempts {
		return s, ErrDialogueLimit
	}
	inserted, err := tx.Exec(ctx, `INSERT INTO dialogue_attempts(tenant_id,attempt_id,occurrence_id,requirement_id,student_id,revision_id,policy_version,snapshot_digest,config_snapshot) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(tenant_id,attempt_id) DO NOTHING`, a.TenantID, attempt, occurrence, requirement, a.StudentID, revision, snapshot.PolicyVersion, snapshot.Digest, raw)
	if err != nil {
		return s, err
	}
	if inserted.RowsAffected() == 1 {
		_, err = tx.Exec(ctx, `INSERT INTO verification_jobs(id,tenant_id,attempt_id,session_id,job_key,max_attempts,deadline) VALUES($1,$2,$3,$4,'initial-question',3,clock_timestamp()+interval '10 minutes')`, uuid.NewString(), a.TenantID, attempt, a.SessionID)
		if err != nil {
			return s, err
		}
	}
	// Dialogue start orchestrates the other issued manual requirements too;
	// their independent open attempts still require an actual parent decision.
	if _, err = tx.Exec(ctx, `INSERT INTO verification_attempts(id,tenant_id,occurrence_id,requirement_id,number) SELECT gen_random_uuid(),$1,$2,r.id,1 FROM verification_requirements r WHERE r.tenant_id=$1 AND r.revision_id=$3 AND r.kind='parent_approval' AND NOT EXISTS(SELECT 1 FROM verification_attempts a WHERE a.tenant_id=$1 AND a.occurrence_id=$2 AND a.requirement_id=r.id)`, a.TenantID, occurrence, revision); err != nil {
		return s, err
	}
	if status == "pending" {
		if _, err = tx.Exec(ctx, `UPDATE task_occurrences SET status='in_progress' WHERE tenant_id=$1 AND id=$2 AND status='pending'`, a.TenantID, occurrence); err != nil {
			return s, err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE task_occurrences SET status='awaiting_verification' WHERE tenant_id=$1 AND id=$2 AND status='in_progress'`, a.TenantID, occurrence); err != nil {
		return s, err
	}
	s, err = LoadDialogueState(ctx, tx, a, occurrence, attempt)
	if err != nil {
		return s, err
	}
	return s, tx.Commit(ctx)
}

// Admit atomically saves exactly the student's bound answer, its job and ack.
// Duplicate keys must carry the identical original payload/binding, even when
// replayed after acceptance. A changed payload is never echoed as original.
func (e DialogueEngine) Admit(ctx context.Context, a StudentAuthority, occurrence, attempt string, m DialogueMessage) (ack DialogueEvent, err error) {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return ack, err
	}
	defer tx.Rollback(ctx)
	if err = LockDialogueAccess(ctx, tx, a, occurrence, attempt, true); err != nil {
		return ack, err
	}
	s, err := LoadDialogueState(ctx, tx, a, occurrence, attempt)
	if err != nil {
		return ack, err
	}
	for _, old := range s.Messages {
		if old.ClientMessageID == m.ClientMessageID {
			if old.Content != m.Content || old.QuestionID != m.QuestionID || old.ExpectedVersion != m.ExpectedVersion || old.PolicyVersion != m.PolicyVersion || old.SnapshotDigest != m.SnapshotDigest {
				return ack, ErrDialogueConflict
			}
			var raw []byte
			if err = tx.QueryRow(ctx, `SELECT payload FROM verification_events WHERE tenant_id=$1 AND attempt_id=$2 AND event_key=$3`, a.TenantID, attempt, "message:"+old.ID).Scan(&raw); err != nil {
				return ack, err
			}
			if json.Unmarshal(raw, &ack) != nil {
				return ack, ErrDialogueContext
			}
			return ack, tx.Commit(ctx)
		}
	}
	var busy bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM verification_jobs WHERE tenant_id=$1 AND attempt_id=$2 AND status IN ('queued','running','failed'))`, a.TenantID, attempt).Scan(&busy); err != nil {
		return ack, err
	}
	if busy {
		return ack, ErrDialogueConflict
	}
	m.ID, m.AttemptID, m.Role, m.Sequence = uuid.NewString(), attempt, "student", int64(len(s.Messages)+1)
	next, err := RecordMessage(s, m)
	if err != nil {
		return ack, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO verification_messages(id,tenant_id,attempt_id,question_id,policy_version,snapshot_digest,sequence,expected_version,role,content,client_message_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'student',$9,$10)`, m.ID, a.TenantID, attempt, m.QuestionID, m.PolicyVersion, m.SnapshotDigest, m.Sequence, m.ExpectedVersion, m.Content, m.ClientMessageID)
	if err != nil {
		return ack, err
	}
	if _, err = tx.Exec(ctx, `UPDATE dialogue_attempts SET version=$3,next_sequence=$4,updated_at=clock_timestamp() WHERE tenant_id=$1 AND attempt_id=$2`, a.TenantID, attempt, next.Version, m.Sequence+1); err != nil {
		return ack, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO verification_jobs(id,tenant_id,attempt_id,message_id,session_id,job_key,stage,max_attempts,deadline) VALUES($1,$2,$3,$4,$5,$6,'evaluation',3,clock_timestamp()+interval '10 minutes')`, uuid.NewString(), a.TenantID, attempt, m.ID, a.SessionID, "message:"+m.ID); err != nil {
		return ack, err
	}
	ack = DialogueEvent{Kind: "message_ack", QuestionID: m.QuestionID, MessageID: m.ID, ClientMessageID: m.ClientMessageID, Text: m.Content, RequiredCount: 3}
	if err = appendDialogueEvent(ctx, tx, next, "message:"+m.ID, ack); err != nil {
		return ack, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ack, err
	}
	// Delivery comes from the durable cursor query, never this process's memory.
	return ack, nil
}

func (e DialogueEngine) WorkerState(ctx context.Context, a StudentAuthority, occurrence, attempt string, lease DialogueLease) (s DialogueState, stage, message string, err error) {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return s, "", "", err
	}
	defer tx.Rollback(ctx)
	if err = LockDialogueAccess(ctx, tx, a, occurrence, attempt, true); err != nil {
		return s, "", "", err
	}
	stage, message, err = lockDialogueLease(ctx, tx, a, attempt, lease)
	if err != nil {
		return s, "", "", err
	}
	s, err = LoadDialogueState(ctx, tx, a, occurrence, attempt)
	if err == nil && s.Terminal {
		err = ErrDialogueTerminal
	}
	return s, stage, message, err
}

func (e DialogueEngine) CommitQuestion(ctx context.Context, a StudentAuthority, occurrence, attempt string, lease DialogueLease, questionKey string) (err error) {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = LockDialogueAccess(ctx, tx, a, occurrence, attempt, true); err != nil {
		return err
	}
	stage, _, err := lockDialogueLease(ctx, tx, a, attempt, lease)
	if err != nil {
		return err
	}
	if stage != "question" {
		return ErrDialogueConflict
	}
	s, err := LoadDialogueState(ctx, tx, a, occurrence, attempt)
	if err != nil {
		return err
	}
	if len(s.Questions) >= len(s.Snapshot.Questions) {
		return ErrDialogueTerminal
	}
	planned := s.Snapshot.Questions[len(s.Questions)]
	if questionKey != planned.Key {
		return ErrDialogueQuestion
	}
	q := DialogueQuestion{ID: uuid.NewString(), AttemptID: attempt, QuestionKey: planned.Key, Ordinal: len(s.Questions) + 1, Version: s.Version + 1, Prompt: planned.Prompt}
	next, err := RecordQuestion(s, q)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO dialogue_questions(id,tenant_id,attempt_id,question_key,ordinal,version,prompt) VALUES($1,$2,$3,$4,$5,$6,$7)`, q.ID, a.TenantID, attempt, q.QuestionKey, q.Ordinal, q.Version, q.Prompt); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE dialogue_attempts SET version=$3,updated_at=clock_timestamp() WHERE tenant_id=$1 AND attempt_id=$2`, a.TenantID, attempt, next.Version); err != nil {
		return err
	}
	ready, _ := DialogueEvidence(next)
	if err = appendDialogueEvent(ctx, tx, next, "question:"+q.ID, DialogueEvent{Kind: "question", QuestionID: q.ID, Text: q.Prompt, AcceptedCount: ready.AcceptedCount, RequiredCount: 3}); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE verification_jobs SET status='succeeded',stage='done',lease_owner=NULL,lease_until=NULL,updated_at=clock_timestamp() WHERE id=$1`, lease.JobID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// CommitEvaluation is the only automatic dialogue decision/completion path.
// A successful bounded Fantasy turn supplies a proposal + measured usage; all
// identities, current state and the active lease are read/validated again here.
// Evidence, stage progression, decision and terminal event share one commit.
func (e DialogueEngine) CommitEvaluation(ctx context.Context, a StudentAuthority, occurrence, attempt string, lease DialogueLease, v DialogueEvaluation) (err error) {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = LockDialogueAccess(ctx, tx, a, occurrence, attempt, true); err != nil {
		return err
	}
	stage, message, err := lockDialogueLease(ctx, tx, a, attempt, lease)
	if err != nil {
		return err
	}
	if stage != "evaluation" || message == "" {
		return ErrDialogueConflict
	}
	s, err := LoadDialogueState(ctx, tx, a, occurrence, attempt)
	if err != nil {
		return err
	}
	var bound DialogueMessage
	for _, m := range s.Messages {
		if m.ID == message {
			bound = m
		}
	}
	v.ID, v.AttemptID, v.MessageID, v.QuestionID, v.Version = uuid.NewString(), attempt, message, bound.QuestionID, s.Version+1
	v.PolicyVersion, v.SnapshotDigest = s.Context.PolicyVersion, s.Context.SnapshotDigest
	v.Rationale = DialogueRetryRationale
	if v.Accepted {
		v.Rationale = DialogueAcceptedRationale
	}
	next, ready, err := RecordEvaluation(s, v)
	if err != nil {
		return err
	}
	criteria := v.Criteria
	if criteria == nil {
		criteria = []string{}
	}
	encoded, err := json.Marshal(criteria)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO verification_evaluations(id,tenant_id,attempt_id,question_id,message_id,job_id,policy_version,snapshot_digest,version,accepted,criteria,rationale,provider,model,input_tokens,output_tokens) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, v.ID, a.TenantID, attempt, v.QuestionID, message, lease.JobID, v.PolicyVersion, v.SnapshotDigest, v.Version, v.Accepted, encoded, v.Rationale, v.Provider, v.Model, v.InputTokens, v.OutputTokens)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE dialogue_attempts SET version=$3,updated_at=clock_timestamp() WHERE tenant_id=$1 AND attempt_id=$2`, a.TenantID, attempt, next.Version); err != nil {
		return err
	}
	status := "rejected"
	if v.Accepted {
		status = "accepted"
	}
	if err = appendDialogueEvent(ctx, tx, next, "evaluation:"+v.ID, DialogueEvent{Kind: "answer_evaluation", QuestionID: v.QuestionID, MessageID: message, Status: status, Text: v.Rationale, AcceptedCount: ready.AcceptedCount, RequiredCount: 3}); err != nil {
		return err
	}
	finished := !v.Accepted || ready.Accepted
	answersToQuestion := 0
	for _, answer := range next.Messages {
		if answer.QuestionID == v.QuestionID {
			answersToQuestion++
		}
	}
	if !v.Accepted && (answersToQuestion >= 1+next.Snapshot.Config.AllowedFollowUps || len(next.Messages) >= next.Snapshot.Config.MaxTurns) {
		decision := uuid.NewString()
		if _, err = tx.Exec(ctx, `INSERT INTO verification_decisions(id,tenant_id,attempt_id,accepted,reason,decided_by) VALUES($1,$2,$3,false,'dialogue answer policy exhausted','verification_engine')`, decision, a.TenantID, attempt); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE verification_attempts SET status='exhausted' WHERE tenant_id=$1 AND id=$2 AND status='open'`, a.TenantID, attempt); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE task_occurrences SET status='pending' WHERE tenant_id=$1 AND id=$2 AND status='awaiting_verification'`, a.TenantID, occurrence)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrDialogueConflict
		}
		if err = appendDialogueEvent(ctx, tx, next, "exhausted", DialogueEvent{Kind: "error", Status: "exhausted", OccurrenceStatus: "pending", Code: "exhausted", DecisionID: decision, DecisionSource: "verification_engine", AcceptedCount: ready.AcceptedCount, RequiredCount: 3}); err != nil {
			return err
		}
	}
	if ready.Accepted {
		decision := uuid.NewString()
		if _, err = tx.Exec(ctx, `INSERT INTO verification_decisions(id,tenant_id,attempt_id,accepted,reason,decided_by) VALUES($1,$2,$3,true,$4,'verification_engine')`, decision, a.TenantID, attempt, ready.Reason); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE verification_attempts SET status='accepted' WHERE tenant_id=$1 AND id=$2 AND status='open'`, a.TenantID, attempt); err != nil {
			return err
		}
		all, err := RequirementPolicySatisfied(ctx, tx, a.TenantID, occurrence)
		if err != nil {
			return err
		}
		if all {
			tag, err := tx.Exec(ctx, `UPDATE task_occurrences SET status='completed' WHERE tenant_id=$1 AND id=$2 AND status='awaiting_verification'`, a.TenantID, occurrence)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 1 {
				return ErrDialogueConflict
			}
			if err = appendDialogueEvent(ctx, tx, next, "completion", DialogueEvent{Kind: "complete", Status: "accepted", OccurrenceStatus: "completed", DecisionID: decision, DecisionSource: "verification_engine", AcceptedCount: 3, RequiredCount: 3}); err != nil {
				return err
			}
		}
	}
	if finished {
		_, err = tx.Exec(ctx, `UPDATE verification_jobs SET status='succeeded',stage='done',lease_owner=NULL,lease_until=NULL,updated_at=clock_timestamp() WHERE id=$1`, lease.JobID)
	} else {
		_, err = tx.Exec(ctx, `UPDATE verification_jobs SET stage='question',updated_at=clock_timestamp() WHERE id=$1`, lease.JobID)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// DialogueJobReference is queue/worker provenance, not student authority. A
// recovery actor may only requeue or reject admitted work; it cannot accept it.
type DialogueJobReference struct {
	JobID, TenantID, Owner string
	Generation             int64
}

// ReconcileDialogueJobs handles expired leases, deadlines and exhausted failed
// jobs through the SAME occurrence/attempt/job fences as ordinary effects. A
// bounded candidate pass is safe to replay after process death or concurrently.
func (e DialogueEngine) ReconcileDialogueJobs(ctx context.Context) error {
	for pass := 0; pass < 8; pass++ {
		var ref DialogueJobReference
		err := e.DB.QueryRow(ctx, `SELECT j.id,j.tenant_id FROM verification_jobs j
 JOIN verification_attempts a ON a.tenant_id=j.tenant_id AND a.id=j.attempt_id
 JOIN task_occurrences o ON o.tenant_id=a.tenant_id AND o.id=a.occurrence_id
 WHERE j.status IN ('queued','running','failed') AND (
  ((a.status<>'open' OR o.status IN ('completed','canceled','excused') OR a.number<>(SELECT max(number) FROM verification_attempts WHERE tenant_id=a.tenant_id AND occurrence_id=a.occurrence_id AND requirement_id=a.requirement_id)) AND j.status IN ('queued','running'))
  OR (a.status='open' AND o.status NOT IN ('completed','canceled','excused') AND a.number=(SELECT max(number) FROM verification_attempts WHERE tenant_id=a.tenant_id AND occurrence_id=a.occurrence_id AND requirement_id=a.requirement_id) AND
   (j.deadline<=clock_timestamp() OR (j.status IN ('queued','failed') AND j.attempts>=j.max_attempts) OR (j.status='running' AND j.lease_until<=clock_timestamp()))))
 ORDER BY j.available_at,j.id LIMIT 1`).Scan(&ref.JobID, &ref.TenantID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if err = e.settleDialogueJob(ctx, ref, "", true); err != nil && !errors.Is(err, ErrDialogueLease) {
			return err
		}
	}
	return nil
}
func (e DialogueEngine) FailDialogueJob(ctx context.Context, ref DialogueJobReference, code string) error {
	switch code {
	case "provider_unavailable", "lease_lost", "revoked", "exhausted", "invalid_evaluation":
	default:
		return ErrDialogueContext
	}
	return e.settleDialogueJob(ctx, ref, code, false)
}
func (e DialogueEngine) settleDialogueJob(ctx context.Context, ref DialogueJobReference, code string, recovery bool) error {
	var a StudentAuthority
	var occurrence, attempt string
	if err := e.DB.QueryRow(ctx, `SELECT d.tenant_id,d.student_id,j.session_id,d.occurrence_id,d.attempt_id FROM verification_jobs j JOIN dialogue_attempts d ON d.tenant_id=j.tenant_id AND d.attempt_id=j.attempt_id WHERE j.tenant_id=$1 AND j.id=$2`, ref.TenantID, ref.JobID).Scan(&a.TenantID, &a.StudentID, &a.SessionID, &occurrence, &attempt); err != nil {
		return ErrDialogueContext
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Negative recovery must not impersonate an expired/revoked session. Lock
	// its immutable admission rows, but do not turn them into positive authority.
	var id string
	if err = tx.QueryRow(ctx, `SELECT id FROM students WHERE tenant_id=$1 AND id=$2 FOR SHARE`, a.TenantID, a.StudentID).Scan(&id); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, `SELECT id FROM student_sessions WHERE id=$1 AND tenant_id=$2 AND student_id=$3 FOR SHARE`, a.SessionID, a.TenantID, a.StudentID).Scan(&id); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, `SELECT id FROM task_occurrences WHERE tenant_id=$1 AND id=$2 AND student_id=$3 FOR UPDATE`, a.TenantID, occurrence, a.StudentID).Scan(&id); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, `SELECT attempt_id FROM dialogue_attempts WHERE tenant_id=$1 AND attempt_id=$2 AND occurrence_id=$3 FOR UPDATE`, a.TenantID, attempt, occurrence).Scan(&id); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, `SELECT id FROM verification_jobs WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, a.TenantID, ref.JobID).Scan(&id); err != nil {
		return err
	}
	var status, owner, lastError, attemptStatus, occurrenceStatus string
	var generation int64
	var attempts, maximum int
	var expired, leaseExpired, latest bool
	if err = tx.QueryRow(ctx, `SELECT j.status,COALESCE(j.lease_owner,''),j.lease_generation,j.attempts,j.max_attempts,COALESCE(j.last_error,''),j.deadline<=clock_timestamp(),COALESCE(j.lease_until<=clock_timestamp(),false),v.status,o.status,v.number=(SELECT max(number) FROM verification_attempts WHERE tenant_id=v.tenant_id AND occurrence_id=v.occurrence_id AND requirement_id=v.requirement_id)
 FROM verification_jobs j JOIN verification_attempts v ON v.tenant_id=j.tenant_id AND v.id=j.attempt_id JOIN task_occurrences o ON o.tenant_id=v.tenant_id AND o.id=v.occurrence_id
 WHERE j.id=$1 AND j.tenant_id=$2 AND j.attempt_id=$3 AND j.session_id=$4`, ref.JobID, a.TenantID, attempt, a.SessionID).Scan(&status, &owner, &generation, &attempts, &maximum, &lastError, &expired, &leaseExpired, &attemptStatus, &occurrenceStatus, &latest); err != nil {
		return err
	}
	if !recovery && (status != "running" || owner != ref.Owner || generation != ref.Generation) {
		return ErrDialogueLease
	}
	if status == "succeeded" {
		return nil
	}
	terminalContext := attemptStatus != "open" || domain.IsTerminal(domain.OccurrenceStatus(occurrenceStatus)) || !latest
	if terminalContext {
		if status == "running" || status == "queued" {
			if _, err = tx.Exec(ctx, `UPDATE verification_jobs SET status='failed',lease_owner=NULL,lease_until=NULL,last_error='exhausted',updated_at=clock_timestamp() WHERE id=$1`, ref.JobID); err != nil {
				return err
			}
		}
		return tx.Commit(ctx) // Never rewrite completed/canceled/overridden evidence.
	}
	terminal := expired || attempts >= maximum
	if recovery {
		if status == "running" && !leaseExpired && !expired {
			return nil
		}
		if status != "running" && !terminal {
			return nil
		}
		if status == "running" && leaseExpired && !terminal {
			_, err = tx.Exec(ctx, `UPDATE verification_jobs SET status='queued',lease_owner=NULL,lease_until=NULL,last_error='lease_lost',updated_at=clock_timestamp() WHERE id=$1`, ref.JobID)
			if err != nil {
				return err
			}
			return tx.Commit(ctx)
		}
	} else if leaseExpired && !terminal {
		return ErrDialogueLease
	}
	if code == "" {
		code = lastError
	}
	if code == "" || expired {
		code = "exhausted"
	}
	if _, err = tx.Exec(ctx, `UPDATE verification_jobs SET status='failed',lease_owner=NULL,lease_until=NULL,last_error=$2,updated_at=clock_timestamp() WHERE id=$1`, ref.JobID, code); err != nil {
		return err
	}
	if !terminal {
		return tx.Commit(ctx)
	}
	s, err := LoadDialogueState(ctx, tx, a, occurrence, attempt)
	if err != nil {
		return err
	}
	reason, eventCode := "dialogue job retry budget exhausted", "job_budget_exhausted"
	if code == "provider_unavailable" || code == "invalid_evaluation" {
		reason, eventCode = "dialogue provider retry budget exhausted", "provider_exhausted"
	}
	if expired {
		reason, eventCode = "dialogue job deadline expired", "deadline_exhausted"
	} else if leaseExpired || code == "lease_lost" {
		reason, eventCode = "dialogue lease recovery budget exhausted", "lease_exhausted"
	}
	decision := uuid.NewString()
	if _, err = tx.Exec(ctx, `INSERT INTO verification_decisions(id,tenant_id,attempt_id,accepted,reason,decided_by) VALUES($1,$2,$3,false,$4,'verification_engine')`, decision, a.TenantID, attempt, reason); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE verification_attempts SET status='exhausted' WHERE tenant_id=$1 AND id=$2 AND status='open'`, a.TenantID, attempt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrDialogueConflict
	}
	tag, err = tx.Exec(ctx, `UPDATE task_occurrences SET status='pending' WHERE tenant_id=$1 AND id=$2 AND status NOT IN ('completed','canceled','excused')`, a.TenantID, occurrence)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrDialogueConflict
	}
	s.Version++
	if _, err = tx.Exec(ctx, `UPDATE dialogue_attempts SET version=$3,updated_at=clock_timestamp() WHERE tenant_id=$1 AND attempt_id=$2`, a.TenantID, attempt, s.Version); err != nil {
		return err
	}
	ready, err := DialogueEvidence(s)
	if err != nil {
		return err
	}
	if err = appendDialogueEvent(ctx, tx, s, "job-exhaustion", DialogueEvent{Kind: "error", Status: "exhausted", OccurrenceStatus: "pending", Code: eventCode, DecisionID: decision, DecisionSource: "verification_engine", AcceptedCount: ready.AcceptedCount, RequiredCount: 3}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (e DialogueEngine) Retry(ctx context.Context, a StudentAuthority, occurrence, attempt string, expected int64) error {
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = LockDialogueAccess(ctx, tx, a, occurrence, attempt, true); err != nil {
		return err
	}
	s, err := LoadDialogueState(ctx, tx, a, occurrence, attempt)
	if err != nil {
		return err
	}
	if s.Terminal {
		return ErrDialogueTerminal
	}
	if expected != s.Version {
		return ErrDialogueConflict
	}
	tag, err := tx.Exec(ctx, `UPDATE verification_jobs SET status='queued',available_at=clock_timestamp(),last_error=NULL,updated_at=clock_timestamp() WHERE tenant_id=$1 AND attempt_id=$2 AND session_id=$3 AND status='failed' AND attempts<max_attempts AND deadline>clock_timestamp()`, a.TenantID, attempt, a.SessionID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrDialogueLimit
	}
	return tx.Commit(ctx)
}

var ErrDialogueReversal = errors.New("completed dialogue reversal is unsupported")

type DialogueOverrideReceipt struct {
	OccurrenceID  string `json:"occurrenceId"`
	AttemptID     string `json:"attemptId"`
	OverrideID    string `json:"overrideId"`
	Accepted      bool   `json:"accepted"`
	ResultVersion int64  `json:"resultVersion"`
	ResultStatus  string `json:"resultStatus"`
}

// Override is a separate audited human decision. The HTTP adapter holds current
// parent identity/session/member authority plus the occurrence lock through this
// transaction. No model tool receives this method or the parent transaction.
func OverrideDialogue(ctx context.Context, tx pgx.Tx, tenant, actor, occurrence, attempt, requestID, reason string, expected int64, accepted bool) (receipt DialogueOverrideReceipt, err error) {
	if requestID == "" || len(requestID) > 128 || expected < 1 || len([]rune(strings.TrimSpace(reason))) < 8 || len([]rune(reason)) > 1000 {
		return receipt, ErrDialogueContext
	}
	var student string
	if err = tx.QueryRow(ctx, `SELECT student_id FROM task_occurrences WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenant, occurrence).Scan(&student); err != nil {
		return receipt, ErrDialogueContext
	}
	if err = tx.QueryRow(ctx, `SELECT student_id FROM dialogue_attempts WHERE tenant_id=$1 AND occurrence_id=$2 AND attempt_id=$3 FOR UPDATE`, tenant, occurrence, attempt).Scan(&student); err != nil {
		return receipt, ErrDialogueContext
	}
	var oldActor, oldReason, oldAttempt string
	var oldExpected int64
	err = tx.QueryRow(ctx, `SELECT id,attempt_id,actor_id,reason,expected_version,accepted,result_version,result_status FROM verification_overrides WHERE tenant_id=$1 AND occurrence_id=$2 AND client_request_id=$3`, tenant, occurrence, requestID).Scan(&receipt.OverrideID, &oldAttempt, &oldActor, &oldReason, &oldExpected, &receipt.Accepted, &receipt.ResultVersion, &receipt.ResultStatus)
	if err == nil {
		if oldActor != actor || oldAttempt != attempt || oldReason != reason || oldExpected != expected || receipt.Accepted != accepted {
			return DialogueOverrideReceipt{}, ErrDialogueConflict
		}
		receipt.OccurrenceID, receipt.AttemptID = occurrence, attempt
		return receipt, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return receipt, err
	}
	s, err := LoadDialogueState(ctx, tx, StudentAuthority{TenantID: tenant, StudentID: student}, occurrence, attempt)
	if err != nil {
		return receipt, err
	}
	if s.Version != expected {
		return receipt, ErrDialogueConflict
	}
	if s.OccurrenceStatus == "completed" && !accepted {
		return receipt, ErrDialogueReversal
	}
	if s.OccurrenceStatus == "canceled" || s.OccurrenceStatus == "excused" {
		return receipt, ErrDialogueTerminal
	}
	receipt = DialogueOverrideReceipt{OccurrenceID: occurrence, AttemptID: attempt, OverrideID: uuid.NewString(), Accepted: accepted, ResultVersion: s.Version + 1, ResultStatus: "pending"}
	// Derive the prospective all-requirements result without pretending that
	// another driver's missing decision was accepted by this override.
	rows, err := tx.Query(ctx, `SELECT r.id FROM verification_requirements r JOIN task_occurrences o ON o.tenant_id=r.tenant_id AND o.revision_id=r.revision_id WHERE o.tenant_id=$1 AND o.id=$2 ORDER BY r.ordinal`, tenant, occurrence)
	if err != nil {
		return receipt, err
	}
	var required []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		required = append(required, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return receipt, err
	}
	outcomes := make([]RequirementOutcome, 0, len(required))
	for _, id := range required {
		ok := accepted
		if id != s.Context.RequirementID {
			if err = tx.QueryRow(ctx, `SELECT COALESCE((SELECT COALESCE((SELECT ov.accepted FROM verification_overrides ov WHERE ov.tenant_id=a.tenant_id AND ov.attempt_id=a.id ORDER BY ov.result_version DESC LIMIT 1),d.accepted,false) FROM verification_attempts a LEFT JOIN verification_decisions d ON d.tenant_id=a.tenant_id AND d.attempt_id=a.id WHERE a.tenant_id=$1 AND a.occurrence_id=$2 AND a.requirement_id=$3 ORDER BY a.number DESC LIMIT 1),false)`, tenant, occurrence, id).Scan(&ok); err != nil {
				return receipt, err
			}
		}
		outcomes = append(outcomes, RequirementOutcome{RequirementID: id, Accepted: ok})
	}
	all, err := AllRequirementsAccepted(required, outcomes)
	if err != nil {
		return receipt, err
	}
	if accepted {
		receipt.ResultStatus = "awaiting_verification"
		if all || s.OccurrenceStatus == "completed" {
			receipt.ResultStatus = "completed"
		}
	}
	if !domain.CanTransition(domain.OccurrenceStatus(s.OccurrenceStatus), domain.OccurrenceStatus(receipt.ResultStatus)) {
		return receipt, ErrDialogueConflict
	}
	_, err = tx.Exec(ctx, `INSERT INTO verification_overrides(id,tenant_id,occurrence_id,requirement_id,attempt_id,client_request_id,expected_version,result_version,result_status,accepted,reason,actor_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, receipt.OverrideID, tenant, occurrence, s.Context.RequirementID, attempt, requestID, expected, receipt.ResultVersion, receipt.ResultStatus, accepted, reason, actor)
	if err != nil {
		return receipt, err
	}
	if _, err = tx.Exec(ctx, `UPDATE dialogue_attempts SET version=$3,updated_at=clock_timestamp() WHERE tenant_id=$1 AND attempt_id=$2`, tenant, attempt, receipt.ResultVersion); err != nil {
		return receipt, err
	}
	attemptStatus := "rejected"
	if accepted {
		attemptStatus = "accepted"
	}
	if _, err = tx.Exec(ctx, `UPDATE verification_attempts SET status=$3 WHERE tenant_id=$1 AND id=$2`, tenant, attempt, attemptStatus); err != nil {
		return receipt, err
	}
	tag, err := tx.Exec(ctx, `UPDATE task_occurrences SET status=$3 WHERE tenant_id=$1 AND id=$2 AND status=$4`, tenant, occurrence, receipt.ResultStatus, s.OccurrenceStatus)
	if err != nil {
		return receipt, err
	}
	if tag.RowsAffected() != 1 {
		return receipt, ErrDialogueConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE verification_jobs SET status='failed',lease_owner=NULL,lease_until=NULL,last_error='exhausted',updated_at=clock_timestamp() WHERE tenant_id=$1 AND attempt_id=$2 AND status IN ('queued','running')`, tenant, attempt); err != nil {
		return receipt, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_records(tenant_id,subject_ref,action,entity_id,metadata) VALUES($1,$2,'verification_override',$3,jsonb_build_object('overrideId',$4::text,'attemptId',$5::text))`, tenant, actor, occurrence, receipt.OverrideID, attempt); err != nil {
		return receipt, err
	}
	ready, err := DialogueEvidence(s)
	if err != nil {
		return receipt, err
	}
	s.Version = receipt.ResultVersion
	event := DialogueEvent{Kind: "override", Status: attemptStatus, OccurrenceStatus: receipt.ResultStatus, DecisionID: receipt.OverrideID, DecisionSource: "parent_override", AcceptedCount: ready.AcceptedCount, RequiredCount: 3}
	if err = appendDialogueEvent(ctx, tx, s, "override:"+receipt.OverrideID, event); err != nil {
		return receipt, err
	}
	if receipt.ResultStatus == "completed" && s.OccurrenceStatus != "completed" {
		event.Kind = "complete"
		if err = appendDialogueEvent(ctx, tx, s, "completion", event); err != nil {
			return receipt, err
		}
	}
	return receipt, nil
}

func (e DialogueEngine) Progress(ctx context.Context, a StudentAuthority, occurrence, attempt string, lease DialogueLease, phase string) error {
	if phase != "thinking" && phase != "evaluating" {
		return ErrDialogueContext
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = LockDialogueAccess(ctx, tx, a, occurrence, attempt, true); err != nil {
		return err
	}
	if _, _, err = lockDialogueLease(ctx, tx, a, attempt, lease); err != nil {
		return err
	}
	s, err := LoadDialogueState(ctx, tx, a, occurrence, attempt)
	if err != nil {
		return err
	}
	if s.Terminal {
		return ErrDialogueTerminal
	}
	if err = appendDialogueEvent(ctx, tx, s, fmt.Sprintf("%s:%d:%s", lease.JobID, lease.Generation, phase), DialogueEvent{Kind: "progress", Phase: phase, RequiredCount: 3}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
