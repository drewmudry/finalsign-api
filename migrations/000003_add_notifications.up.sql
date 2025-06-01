-- Migration 003: Workspace Invitations and Notifications
-- Run this migration to add invitation system

-- Create enums for invitation system
CREATE TYPE invitation_status AS ENUM ('pending', 'accepted', 'declined', 'expired', 'cancelled');
CREATE TYPE notification_type AS ENUM ('workspace_invitation', 'invitation_accepted', 'invitation_declined', 'member_joined', 'member_left');

-- Workspace invitations table
CREATE TABLE workspace_invitations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID REFERENCES workspaces(id) ON DELETE CASCADE,
    inviter_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    invitee_email VARCHAR(255) NOT NULL,
    invitee_id INTEGER REFERENCES users(id) ON DELETE CASCADE, -- NULL if user doesn't exist yet
    role membership_role NOT NULL DEFAULT 'member',
    status invitation_status DEFAULT 'pending',
    token VARCHAR(255) UNIQUE NOT NULL, -- For secure invitation links
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT (NOW() + INTERVAL '7 days'),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    accepted_at TIMESTAMP WITH TIME ZONE,
    declined_at TIMESTAMP WITH TIME ZONE
);

-- Notifications table for system notifications
CREATE TABLE notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    type notification_type NOT NULL,
    title VARCHAR(255) NOT NULL,
    message TEXT NOT NULL,
    data JSONB DEFAULT '{}', -- Additional structured data
    is_read BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE -- For auto-cleanup of old notifications
);

-- Indexes for performance
CREATE INDEX idx_workspace_invitations_workspace_id ON workspace_invitations(workspace_id);
CREATE INDEX idx_workspace_invitations_invitee_email ON workspace_invitations(invitee_email);
CREATE INDEX idx_workspace_invitations_invitee_id ON workspace_invitations(invitee_id);
CREATE INDEX idx_workspace_invitations_status ON workspace_invitations(status);
CREATE INDEX idx_workspace_invitations_token ON workspace_invitations(token);
CREATE INDEX idx_workspace_invitations_expires_at ON workspace_invitations(expires_at);

CREATE INDEX idx_notifications_user_id ON notifications(user_id);
CREATE INDEX idx_notifications_type ON notifications(type);
CREATE INDEX idx_notifications_is_read ON notifications(is_read);
CREATE INDEX idx_notifications_created_at ON notifications(created_at);
CREATE INDEX idx_notifications_expires_at ON notifications(expires_at);

-- Constraint: only one pending invitation per email per workspace
CREATE UNIQUE INDEX idx_unique_pending_invitation 
ON workspace_invitations(workspace_id, invitee_email) 
WHERE status = 'pending';

-- Function to generate secure invitation token
CREATE OR REPLACE FUNCTION generate_invitation_token() RETURNS VARCHAR(255) AS $$
DECLARE
    chars TEXT := 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';
    result VARCHAR(255) := '';
    i INT;
BEGIN
    FOR i IN 1..64 LOOP
        result := result || substr(chars, floor(random() * length(chars) + 1)::int, 1);
    END LOOP;
    RETURN result;
END;
$$ LANGUAGE plpgsql;

-- Function to auto-generate invitation token
CREATE OR REPLACE FUNCTION ensure_invitation_token() RETURNS TRIGGER AS $$
DECLARE
    new_token VARCHAR(255);
    attempts INT := 0;
    max_attempts INT := 100;
BEGIN
    -- If token is not provided, generate one
    IF NEW.token IS NULL OR NEW.token = '' THEN
        LOOP
            new_token := generate_invitation_token();
            attempts := attempts + 1;
            
            -- Check if token is unique
            IF NOT EXISTS (SELECT 1 FROM workspace_invitations WHERE token = new_token) THEN
                NEW.token := new_token;
                EXIT;
            END IF;
            
            -- Prevent infinite loop
            IF attempts >= max_attempts THEN
                RAISE EXCEPTION 'Could not generate unique invitation token after % attempts', max_attempts;
            END IF;
        END LOOP;
    END IF;
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply token generation trigger
CREATE TRIGGER trigger_ensure_invitation_token
    BEFORE INSERT ON workspace_invitations
    FOR EACH ROW EXECUTE FUNCTION ensure_invitation_token();

-- Function to update invitation timestamps
CREATE OR REPLACE FUNCTION update_invitation_timestamps() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    
    -- Set accepted_at when status changes to accepted
    IF NEW.status = 'accepted' AND OLD.status != 'accepted' THEN
        NEW.accepted_at = NOW();
    END IF;
    
    -- Set declined_at when status changes to declined
    IF NEW.status = 'declined' AND OLD.status != 'declined' THEN
        NEW.declined_at = NOW();
    END IF;
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply timestamp update trigger
CREATE TRIGGER trigger_update_invitation_timestamps
    BEFORE UPDATE ON workspace_invitations
    FOR EACH ROW EXECUTE FUNCTION update_invitation_timestamps();

-- Function to clean up expired invitations (run as scheduled job)
CREATE OR REPLACE FUNCTION cleanup_expired_invitations() RETURNS INTEGER AS $$
DECLARE
    expired_count INTEGER;
BEGIN
    UPDATE workspace_invitations 
    SET status = 'expired', updated_at = NOW()
    WHERE status = 'pending' AND expires_at < NOW();
    
    GET DIAGNOSTICS expired_count = ROW_COUNT;
    RETURN expired_count;
END;
$$ LANGUAGE plpgsql;

-- Function to clean up old notifications (run as scheduled job)
CREATE OR REPLACE FUNCTION cleanup_old_notifications() RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM notifications 
    WHERE expires_at IS NOT NULL AND expires_at < NOW();
    
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- Helpful views
CREATE VIEW pending_invitations AS
SELECT 
    wi.id,
    wi.workspace_id,
    w.name as workspace_name,
    w.slug as workspace_slug,
    wi.inviter_id,
    u_inviter.name as inviter_name,
    u_inviter.email as inviter_email,
    wi.invitee_email,
    wi.invitee_id,
    u_invitee.name as invitee_name,
    wi.role,
    wi.token,
    wi.expires_at,
    wi.created_at
FROM workspace_invitations wi
JOIN workspaces w ON wi.workspace_id = w.id
JOIN users u_inviter ON wi.inviter_id = u_inviter.id
LEFT JOIN users u_invitee ON wi.invitee_id = u_invitee.id
WHERE wi.status = 'pending' AND wi.expires_at > NOW();

CREATE VIEW user_notifications AS
SELECT 
    n.id,
    n.user_id,
    n.type,
    n.title,
    n.message,
    n.data,
    n.is_read,
    n.created_at,
    n.expires_at
FROM notifications n
WHERE n.expires_at IS NULL OR n.expires_at > NOW()
ORDER BY n.created_at DESC;