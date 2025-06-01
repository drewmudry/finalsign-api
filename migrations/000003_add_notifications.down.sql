-- Migration 003 Down: Remove workspace invitations and notifications system

-- Drop views first (they depend on tables)
DROP VIEW IF EXISTS user_notifications;
DROP VIEW IF EXISTS pending_invitations;

-- Drop triggers (they depend on functions)
DROP TRIGGER IF EXISTS trigger_update_invitation_timestamps ON workspace_invitations;
DROP TRIGGER IF EXISTS trigger_ensure_invitation_token ON workspace_invitations;

-- Drop functions
DROP FUNCTION IF EXISTS cleanup_old_notifications();
DROP FUNCTION IF EXISTS cleanup_expired_invitations();
DROP FUNCTION IF EXISTS update_invitation_timestamps();
DROP FUNCTION IF EXISTS ensure_invitation_token();
DROP FUNCTION IF EXISTS generate_invitation_token();

-- Drop indexes
DROP INDEX IF EXISTS idx_notifications_expires_at;
DROP INDEX IF EXISTS idx_notifications_created_at;
DROP INDEX IF EXISTS idx_notifications_is_read;
DROP INDEX IF EXISTS idx_notifications_type;
DROP INDEX IF EXISTS idx_notifications_user_id;

DROP INDEX IF EXISTS idx_workspace_invitations_expires_at;
DROP INDEX IF EXISTS idx_workspace_invitations_token;
DROP INDEX IF EXISTS idx_workspace_invitations_status;
DROP INDEX IF EXISTS idx_workspace_invitations_invitee_id;
DROP INDEX IF EXISTS idx_workspace_invitations_invitee_email;
DROP INDEX IF EXISTS idx_workspace_invitations_workspace_id;

-- Drop unique constraint index
DROP INDEX IF EXISTS idx_unique_pending_invitation;

-- Drop tables (notifications first since it doesn't have foreign key dependencies from other tables)
DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS workspace_invitations;

-- Drop enums (drop in reverse order of creation to avoid dependency issues)
DROP TYPE IF EXISTS notification_type;
DROP TYPE IF EXISTS invitation_status;