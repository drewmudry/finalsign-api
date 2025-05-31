-- Constraint: ensure each workspace has at least one owner
-- This is enforced at application level, but we can add a check-- Migration 001: Workspaces and Memberships
-- Run this migration to create the workspace system with proper enums

-- Create enums for better data integrity
CREATE TYPE workspace_plan AS ENUM ('free', 'pro', 'enterprise');
CREATE TYPE membership_role AS ENUM ('owner', 'admin', 'member', 'viewer');
CREATE TYPE membership_status AS ENUM ('active', 'suspended', 'invited');

-- Workspaces table - multi-tenant workspaces
CREATE TABLE workspaces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(6) UNIQUE NOT NULL, -- 6-character alphanumeric identifier
    description TEXT,
    plan workspace_plan DEFAULT 'free',
    settings JSONB DEFAULT '{}', -- Workspace-specific settings
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Workspace memberships - users can belong to multiple workspaces
CREATE TABLE workspace_memberships (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id INTEGER REFERENCES users(id) ON DELETE CASCADE, -- Your SERIAL user ID
    role membership_role NOT NULL DEFAULT 'member',
    status membership_status DEFAULT 'active',
    invited_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    invited_at TIMESTAMP WITH TIME ZONE,
    joined_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for performance
CREATE INDEX idx_workspaces_slug ON workspaces(slug);
CREATE INDEX idx_workspaces_is_active ON workspaces(is_active);
CREATE INDEX idx_workspaces_plan ON workspaces(plan);
CREATE INDEX idx_workspace_memberships_workspace_id ON workspace_memberships(workspace_id);
CREATE INDEX idx_workspace_memberships_user_id ON workspace_memberships(user_id);
CREATE INDEX idx_workspace_memberships_status ON workspace_memberships(status);
CREATE INDEX idx_workspace_memberships_role ON workspace_memberships(role);


-- Constraint: slug must be exactly 6 alphanumeric characters
ALTER TABLE workspaces ADD CONSTRAINT slug_format_check 
    CHECK (slug ~ '^[a-zA-Z0-9]{6}$');

-- Function to generate a random 6-character alphanumeric slug
CREATE OR REPLACE FUNCTION generate_workspace_slug() RETURNS VARCHAR(6) AS $$
DECLARE
    chars TEXT := 'abcdefghijklmnopqrstuvwxyz0123456789';
    result VARCHAR(6) := '';
    i INT;
BEGIN
    FOR i IN 1..6 LOOP
        result := result || substr(chars, floor(random() * length(chars) + 1)::int, 1);
    END LOOP;
    RETURN result;
END;
$$ LANGUAGE plpgsql;

-- Function to auto-generate unique slug if not provided
CREATE OR REPLACE FUNCTION ensure_workspace_slug() RETURNS TRIGGER AS $$
DECLARE
    new_slug VARCHAR(6);
    attempts INT := 0;
    max_attempts INT := 100;
BEGIN
    -- If slug is not provided or empty, generate one
    IF NEW.slug IS NULL OR NEW.slug = '' THEN
        LOOP
            new_slug := generate_workspace_slug();
            attempts := attempts + 1;
            
            -- Check if slug is unique
            IF NOT EXISTS (SELECT 1 FROM workspaces WHERE slug = new_slug) THEN
                NEW.slug := new_slug;
                EXIT;
            END IF;
            
            -- Prevent infinite loop
            IF attempts >= max_attempts THEN
                RAISE EXCEPTION 'Could not generate unique slug after % attempts', max_attempts;
            END IF;
        END LOOP;
    END IF;
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply slug generation trigger
CREATE TRIGGER trigger_ensure_workspace_slug
    BEFORE INSERT OR UPDATE ON workspaces
    FOR EACH ROW EXECUTE FUNCTION ensure_workspace_slug();

-- Function to ensure each workspace has at least one owner
CREATE OR REPLACE FUNCTION check_workspace_has_owner() 
RETURNS TRIGGER AS $$
BEGIN
    -- If deleting/updating an owner, ensure another owner exists
    IF (TG_OP = 'DELETE' OR (TG_OP = 'UPDATE' AND OLD.role = 'owner' AND NEW.role != 'owner')) THEN
        IF NOT EXISTS (
            SELECT 1 FROM workspace_memberships 
            WHERE workspace_id = COALESCE(OLD.workspace_id, NEW.workspace_id) 
            AND role = 'owner' 
            AND status = 'active'
            AND id != COALESCE(OLD.id, NEW.id)
        ) THEN
            RAISE EXCEPTION 'Workspace must have at least one active owner';
        END IF;
    END IF;
    
    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

-- Apply owner constraint trigger
CREATE TRIGGER trigger_check_workspace_owner
    BEFORE UPDATE OR DELETE ON workspace_memberships
    FOR EACH ROW EXECUTE FUNCTION check_workspace_has_owner();

-- Updated_at trigger function
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply updated_at trigger
CREATE TRIGGER update_workspaces_updated_at 
    BEFORE UPDATE ON workspaces 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Helpful views
CREATE VIEW user_workspaces AS
SELECT 
    u.id as user_id,
    u.email,
    u.name as user_name,
    w.id as workspace_id,
    w.name as workspace_name,
    w.slug as workspace_slug,
    wm.role,
    wm.status as membership_status,
    wm.joined_at,
    w.plan,
    w.is_active as workspace_active
FROM users u
JOIN workspace_memberships wm ON u.id = wm.user_id
JOIN workspaces w ON wm.workspace_id = w.id
WHERE wm.status = 'active' AND w.is_active = true;