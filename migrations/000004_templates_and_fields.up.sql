-- migrations/000004_add_document_signing.up.sql

-- Templates table (metadata only, PDFs in S3)
CREATE TABLE templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    s3_bucket VARCHAR(255) NOT NULL,
    s3_key VARCHAR(255) NOT NULL,        -- Path to encrypted PDF in S3
    pdf_hash VARCHAR(64) NOT NULL,       -- SHA-256 of original PDF
    file_size BIGINT NOT NULL,           -- Original file size in bytes
    mime_type VARCHAR(100) DEFAULT 'application/pdf',
    created_by INTEGER NOT NULL REFERENCES users(id),
    workspace_id UUID NOT NULL REFERENCES workspaces(id),
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    version INTEGER DEFAULT 1,
    
    -- Constraints
    CONSTRAINT templates_name_length CHECK (char_length(name) >= 1),
    CONSTRAINT templates_positive_file_size CHECK (file_size > 0)
);

-- Fields for templates (define fillable areas on PDFs)
CREATE TABLE template_fields (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    recipient INTEGER DEFAULT 1,
    field_name VARCHAR(255) NOT NULL,    -- "signature_field_1", "name_field", etc.
    field_type VARCHAR(50) NOT NULL,     -- text, signature, date, checkbox, dropdown
    field_label VARCHAR(255),            -- Human-readable label
    placeholder_text VARCHAR(255),       -- "Sign here", "Enter your name", etc.
    position_data JSONB NOT NULL,        -- {x: 100, y: 200, width: 150, height: 30, page: 1}
    validation_rules JSONB,              -- {required: true, min_length: 1, max_length: 100, pattern: "regex"}
    required BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT NOW(),
    version INTEGER DEFAULT 1,
    
    -- Constraints
    CONSTRAINT template_fields_valid_type CHECK (
        field_type IN ('text', 'signature', 'date', 'checkbox', 'email', 'phone')
    ),
    CONSTRAINT template_fields_name_length CHECK (char_length(field_name) >= 1),
    CONSTRAINT template_fields_unique_name_per_template UNIQUE (template_id, field_name)
);

-- Documents (template + fields for specific recipient)
CREATE TABLE documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    template_id UUID NOT NULL REFERENCES templates(id),
    name VARCHAR(255) NOT NULL,
    s3_bucket VARCHAR(255),              -- Final signed PDF location (null until completed)
    s3_key VARCHAR(255),                 -- Final signed PDF key (null until completed)
    template_snapshot_hash VARCHAR(64) NOT NULL, -- Hash of template at document creation time
    final_document_hash VARCHAR(64),     -- Hash of final signed PDF (null until completed)
    created_by INTEGER NOT NULL REFERENCES users(id),
    workspace_id UUID NOT NULL REFERENCES workspaces(id),
    recipient_email VARCHAR(255) NOT NULL,
    recipient_name VARCHAR(255),
    status VARCHAR(50) DEFAULT 'draft',  -- draft, scheduled, sent, in_progress, completed, expired, cancelled
    access_token VARCHAR(255) UNIQUE,    -- Token for recipient access (generated)
    expires_at TIMESTAMP,
    sent_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    completed_at TIMESTAMP,
    
    -- Constraints
    CONSTRAINT documents_valid_status CHECK (
        status IN ('draft', 'scheduled', 'sent', 'in_progress', 'completed', 'expired', 'cancelled')
    ),
    CONSTRAINT documents_name_length CHECK (char_length(name) >= 1),
    CONSTRAINT documents_valid_email CHECK (recipient_email ~* '^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$')
);

-- Form submissions (user input for specific document fields)
CREATE TABLE form_submissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    field_id UUID NOT NULL REFERENCES template_fields(id),
    field_name VARCHAR(255) NOT NULL,    -- Denormalized for faster queries
    field_type VARCHAR(50) NOT NULL,     -- Denormalized for faster queries
    encrypted_value TEXT,                -- User input, encrypted (null for empty optional fields)
    encryption_key_id VARCHAR(255),      -- Key ID for encryption/decryption
    submitted_by VARCHAR(255),           -- Email of person who submitted (usually recipient)
    submitted_at TIMESTAMP DEFAULT NOW(),
    ip_address INET,
    user_agent TEXT,
    
    -- Constraints
    CONSTRAINT form_submissions_unique_field_per_document UNIQUE (document_id, field_id)
);

-- Digital signatures for completed documents
CREATE TABLE digital_signatures (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL REFERENCES documents(id),
    signer_email VARCHAR(255) NOT NULL,
    signer_name VARCHAR(255),
    final_document_hash VARCHAR(64) NOT NULL, -- Hash of completed PDF
    digital_signature TEXT NOT NULL,          -- Cryptographic signature (base64 encoded)
    certificate TEXT,                         -- Public key certificate (PEM format)
    signature_algorithm VARCHAR(100) DEFAULT 'RSA-SHA256',
    signed_at TIMESTAMP DEFAULT NOW(),
    ip_address INET,
    user_agent TEXT,
    
    -- Constraints
    CONSTRAINT digital_signatures_valid_email CHECK (signer_email ~* '^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$')
);

-- Audit log for document actions
CREATE TABLE document_audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID REFERENCES documents(id),
    template_id UUID REFERENCES templates(id),
    user_id INTEGER REFERENCES users(id),
    action VARCHAR(100) NOT NULL,         -- created, sent, viewed, field_filled, signed, completed, expired
    details JSONB,                        -- Additional action-specific data
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    
    -- Constraints
    CONSTRAINT document_audit_log_valid_action CHECK (
        action IN ('template_created', 'template_updated', 'document_created', 'document_sent', 
                   'document_viewed', 'field_filled', 'document_signed', 'document_completed', 
                   'document_expired', 'document_cancelled')
    )
);

-- Indexes for performance
CREATE INDEX idx_templates_workspace_id ON templates(workspace_id);
CREATE INDEX idx_templates_created_by ON templates(created_by);
CREATE INDEX idx_templates_active ON templates(is_active) WHERE is_active = true;
CREATE INDEX idx_templates_created_at ON templates(created_at);

CREATE INDEX idx_template_fields_template_id ON template_fields(template_id);
CREATE INDEX idx_template_fields_type ON template_fields(field_type);

CREATE INDEX idx_documents_template_id ON documents(template_id);
CREATE INDEX idx_documents_workspace_id ON documents(workspace_id);
CREATE INDEX idx_documents_created_by ON documents(created_by);
CREATE INDEX idx_documents_status ON documents(status);
CREATE INDEX idx_documents_recipient_email ON documents(recipient_email);
CREATE INDEX idx_documents_access_token ON documents(access_token) WHERE access_token IS NOT NULL;
CREATE INDEX idx_documents_expires_at ON documents(expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX idx_documents_created_at ON documents(created_at);

CREATE INDEX idx_form_submissions_document_id ON form_submissions(document_id);
CREATE INDEX idx_form_submissions_field_id ON form_submissions(field_id);
CREATE INDEX idx_form_submissions_submitted_at ON form_submissions(submitted_at);

CREATE INDEX idx_digital_signatures_document_id ON digital_signatures(document_id);
CREATE INDEX idx_digital_signatures_signer_email ON digital_signatures(signer_email);
CREATE INDEX idx_digital_signatures_signed_at ON digital_signatures(signed_at);

CREATE INDEX idx_document_audit_log_document_id ON document_audit_log(document_id);
CREATE INDEX idx_document_audit_log_template_id ON document_audit_log(template_id);
CREATE INDEX idx_document_audit_log_user_id ON document_audit_log(user_id);
CREATE INDEX idx_document_audit_log_action ON document_audit_log(action);
CREATE INDEX idx_document_audit_log_created_at ON document_audit_log(created_at);

-- Triggers for updated_at timestamps
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_templates_updated_at BEFORE UPDATE ON templates
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_documents_updated_at BEFORE UPDATE ON documents
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Function to generate secure access tokens for documents
CREATE OR REPLACE FUNCTION generate_document_access_token()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.access_token IS NULL THEN
        NEW.access_token = encode(gen_random_bytes(32), 'base64url');
    END IF;
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER set_document_access_token BEFORE INSERT ON documents
    FOR EACH ROW EXECUTE FUNCTION generate_document_access_token();