-- migrations/000004_add_document_signing.down.sql

-- Drop triggers first
DROP TRIGGER IF EXISTS set_document_access_token ON documents;
DROP TRIGGER IF EXISTS update_documents_updated_at ON documents;
DROP TRIGGER IF EXISTS update_templates_updated_at ON templates;

-- Drop functions
DROP FUNCTION IF EXISTS generate_document_access_token();
DROP FUNCTION IF EXISTS update_updated_at_column();

-- Drop indexes
DROP INDEX IF EXISTS idx_document_audit_log_created_at;
DROP INDEX IF EXISTS idx_document_audit_log_action;
DROP INDEX IF EXISTS idx_document_audit_log_user_id;
DROP INDEX IF EXISTS idx_document_audit_log_template_id;
DROP INDEX IF EXISTS idx_document_audit_log_document_id;

DROP INDEX IF EXISTS idx_digital_signatures_signed_at;
DROP INDEX IF EXISTS idx_digital_signatures_signer_email;
DROP INDEX IF EXISTS idx_digital_signatures_document_id;

DROP INDEX IF EXISTS idx_form_submissions_submitted_at;
DROP INDEX IF EXISTS idx_form_submissions_field_id;
DROP INDEX IF EXISTS idx_form_submissions_document_id;

DROP INDEX IF EXISTS idx_documents_created_at;
DROP INDEX IF EXISTS idx_documents_expires_at;
DROP INDEX IF EXISTS idx_documents_access_token;
DROP INDEX IF EXISTS idx_documents_recipient_email;
DROP INDEX IF EXISTS idx_documents_status;
DROP INDEX IF EXISTS idx_documents_created_by;
DROP INDEX IF EXISTS idx_documents_workspace_id;
DROP INDEX IF EXISTS idx_documents_template_id;

DROP INDEX IF EXISTS idx_template_fields_type;
DROP INDEX IF EXISTS idx_template_fields_template_id;

DROP INDEX IF EXISTS idx_templates_created_at;
DROP INDEX IF EXISTS idx_templates_active;
DROP INDEX IF EXISTS idx_templates_created_by;
DROP INDEX IF EXISTS idx_templates_workspace_id;

-- Drop tables in reverse dependency order
DROP TABLE IF EXISTS document_audit_log;
DROP TABLE IF EXISTS digital_signatures;
DROP TABLE IF EXISTS form_submissions;
DROP TABLE IF EXISTS documents;
DROP TABLE IF EXISTS template_fields;
DROP TABLE IF EXISTS templates;