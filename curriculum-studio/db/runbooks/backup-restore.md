# Curriculum Studio backup/restore drill

This runbook is a disposable-environment proof, not a production backup
configuration. Production backup/PITR ownership remains with the deployment
platform.

## Logical restore drill

1. Start two disposable PostgreSQL databases: one seeded Studio database and
   one empty restore database.
2. Set `STUDIO_BACKUP_DSN` and `STUDIO_RESTORE_DSN` to those databases.
3. Run:

   ```bash
   curriculum-studio/db/scripts/backup_restore_drill.sh
   ```

The script uses `pg_dump` custom format and `pg_restore`, then checks the Studio
migration version table and restored `curriculum_studio` table inventory. It
never reads LMS, TV, Identity, or production credentials.

## Operational policy

- Take encrypted, access-controlled backups through the deployment platform.
- Restore into a separate database before any destructive recovery action.
- Verify `studio_goose_db_version` and representative tenant/workspace rows
  before declaring recovery successful.
- Do not store database credentials, snapshot JSON, OAuth tokens, or provider
  payloads in this repository or in backup-drill logs.
- PITR target selection and retention are platform-owned; this drill proves the
  database contents are restorable, not that a particular cloud provider's PITR
  configuration is correct.
