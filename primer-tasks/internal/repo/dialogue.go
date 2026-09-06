// Package repo contains the SQL persistence boundary for verification.
package repo

// Selectively recovered from original P4 repo/dialogue.go. The canonical
// Database seam supports real transactions/savepoints. This repository stores
// policy/evidence, never decides or writes occurrence completion.
import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	tasksdb "primer-tasks/internal/db"
	"primer-tasks/internal/domain"
)

type DialogueRepository struct{ DB tasksdb.Database }

func NewDialogueRepository(db tasksdb.Database) *DialogueRepository {
	return &DialogueRepository{DB: db}
}

// PublishRevisionPolicies runs inside the ordinary publication transaction.
// Config is read from the canonical requirement-array envelope, not accepted
// as an alternate model/client source. The returned policies are retained
// immutable records; later starts must copy these, not resolve template-latest.
func (r *DialogueRepository) PublishRevisionPolicies(ctx context.Context, tenant, revision string) error {
	if r == nil || r.DB == nil || tenant == "" || revision == "" {
		return domain.ErrInvalidDialogueConfig
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var version int
	if err = tx.QueryRow(ctx, `SELECT version FROM task_revisions WHERE tenant_id=$1 AND id=$2 AND status='published' FOR UPDATE`, tenant, revision).Scan(&version); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT id,kind,config_version,interaction,executor,config FROM verification_requirements WHERE tenant_id=$1 AND revision_id=$2 ORDER BY ordinal`, tenant, revision)
	if err != nil {
		return err
	}
	var snapshots []domain.DialogueSnapshot
	count := 0
	for rows.Next() {
		count++
		var requirement domain.VerificationRequirement
		var raw []byte
		if err = rows.Scan(&requirement.ID, &requirement.Kind, &requirement.ConfigVersion, &requirement.Interaction, &requirement.Executor, &raw); err != nil {
			break
		}
		if requirement.Kind == "parent_approval" && requirement.ConfigVersion == 1 {
			continue // Preserve the existing manual driver's wire/storage contract.
		}
		if requirement.Kind != domain.AgentDialogueKind {
			err = domain.ErrInvalidDialogueConfig
			break
		}
		var config domain.DialogueConfig
		config, err = domain.DialogueConfigFromRequirementJSON(raw)
		if err != nil {
			break
		}
		requirement.Config, err = domain.SnapshotDialogueConfig(config)
		if err == nil {
			err = domain.ValidateDialogueRequirement(requirement)
		}
		if err != nil {
			break
		}
		var snapshot domain.DialogueSnapshot
		snapshot, err = domain.NewDialogueSnapshot(revision, requirement.ID, version, config)
		if err != nil {
			break
		}
		snapshots = append(snapshots, snapshot)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	if count == 0 {
		return domain.ErrInvalidDialogueConfig
	}
	for _, snapshot := range snapshots {
		encoded, err := json.Marshal(snapshot)
		if err != nil {
			return err
		}
		result, err := tx.Exec(ctx, `INSERT INTO dialogue_revision_policies(tenant_id,revision_id,requirement_id,snapshot_digest,snapshot) VALUES($1,$2,$3,$4,$5) ON CONFLICT(tenant_id,requirement_id) DO NOTHING`, tenant, revision, snapshot.RequirementID, snapshot.Digest, encoded)
		if err != nil {
			return err
		}
		if result.RowsAffected() == 0 {
			var digest string
			if err = tx.QueryRow(ctx, `SELECT snapshot_digest FROM dialogue_revision_policies WHERE tenant_id=$1 AND revision_id=$2 AND requirement_id=$3`, tenant, revision, snapshot.RequirementID).Scan(&digest); err != nil {
				return err
			}
			if digest != snapshot.Digest {
				return domain.ErrInvalidDialogueConfig
			}
		}
	}
	return tx.Commit(ctx)
}

// RevisionPolicy returns one server-authored policy from the issued revision.
// It does not substitute another revision/reference when one is unavailable.
func (r *DialogueRepository) RevisionPolicy(ctx context.Context, tenant, revision, requirement string) (domain.DialogueSnapshot, error) {
	var snapshot domain.DialogueSnapshot
	if r == nil || r.DB == nil || tenant == "" || revision == "" || requirement == "" {
		return snapshot, domain.ErrInvalidDialogueConfig
	}
	var raw []byte
	err := r.DB.QueryRow(ctx, `SELECT snapshot FROM dialogue_revision_policies WHERE tenant_id=$1 AND revision_id=$2 AND requirement_id=$3`, tenant, revision, requirement).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return snapshot, domain.ErrInvalidDialogueConfig
	}
	if err != nil {
		return snapshot, err
	}
	if err = json.Unmarshal(raw, &snapshot); err != nil {
		return snapshot, domain.ErrInvalidDialogueConfig
	}
	if snapshot.RevisionID != revision || snapshot.RequirementID != requirement {
		return snapshot, domain.ErrInvalidDialogueConfig
	}
	return snapshot, snapshot.Validate()
}
