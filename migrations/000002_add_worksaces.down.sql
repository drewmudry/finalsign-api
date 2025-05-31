-- Migration Down: Remove Workspaces and Memberships
-- This reverses the add_workspaces.up.sql migration

-- Drop triggers first (they depend on functions and tables)
DROP TRIGGER IF EXISTS trigger_ensure_workspace_slug ON workspaces;
DROP TRIGGER IF EXISTS trigger_check_workspace_owner ON workspace_memberships;
DROP TRIGGER IF EXISTS update_workspaces_updated_at ON workspaces;

-- Drop views (they depend on tables)
DROP VIEW IF EXISTS user_workspaces;

-- Drop functions (they might be referenced by triggers or constraints)
DROP FUNCTION IF EXISTS ensure_workspace_slug();
DROP FUNCTION IF EXISTS generate_workspace_slug();
DROP FUNCTION IF EXISTS check_workspace_has_owner();
DROP FUNCTION IF EXISTS update_updated_at_column();

-- Drop tables in reverse dependency order
-- workspace_memberships references workspaces, so drop it first
DROP TABLE IF EXISTS workspace_memberships;
DROP TABLE IF EXISTS workspaces;

-- Drop custom types (enums) last
DROP TYPE IF EXISTS membership_status;
DROP TYPE IF EXISTS membership_role;
DROP TYPE IF EXISTS workspace_plan;