package database

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Template struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	S3Bucket    string    `json:"s3_bucket"`
	S3Key       string    `json:"s3_key"`
	PDFHash     string    `json:"pdf_hash"`
	FileSize    int64     `json:"file_size"`
	MimeType    string    `json:"mime_type"`
	CreatedBy   int       `json:"created_by"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Version     int       `json:"version"`
}

type TemplateField struct {
	ID              uuid.UUID `json:"id"`
	TemplateID      uuid.UUID `json:"template_id"`
	FieldName       string    `json:"field_name"`
	FieldType       string    `json:"field_type"`
	FieldLabel      string    `json:"field_label"`
	PlaceholderText string    `json:"placeholder_text"`
	PositionData    string    `json:"position_data"`    // JSON string
	ValidationRules string    `json:"validation_rules"` // JSON string
	Required        bool      `json:"required"`
	CreatedAt       time.Time `json:"created_at"`
	Version         int       `json:"version"`
}

type TemplateWithFields struct {
	Template
	Fields []TemplateField `json:"fields"`
}

type UserTemplate struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	FileSize      int64     `json:"file_size"`
	CreatedBy     int       `json:"created_by"`
	CreatorName   string    `json:"creator_name"`
	CreatorEmail  string    `json:"creator_email"`
	WorkspaceID   uuid.UUID `json:"workspace_id"`
	WorkspaceName string    `json:"workspace_name"`
	FieldCount    int       `json:"field_count"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	Version       int       `json:"version"`
}

// CreateTemplate creates a new template with its fields
func (s *service) CreateTemplate(template *Template, fields []TemplateField) (*Template, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert template
	templateQuery := `
		INSERT INTO templates (name, description, s3_bucket, s3_key, pdf_hash, file_size, 
			mime_type, created_by, workspace_id, is_active, created_at, updated_at, version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW(), $11)
		RETURNING id, created_at, updated_at`

	err = tx.QueryRow(
		templateQuery,
		template.Name,
		template.Description,
		template.S3Bucket,
		template.S3Key,
		template.PDFHash,
		template.FileSize,
		template.MimeType,
		template.CreatedBy,
		template.WorkspaceID,
		template.IsActive,
		template.Version,
	).Scan(&template.ID, &template.CreatedAt, &template.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create template: %w", err)
	}

	// Insert fields if any
	if len(fields) > 0 {
		fieldQuery := `
			INSERT INTO template_fields (template_id, field_name, field_type, field_label, 
				placeholder_text, position_data, validation_rules, required, created_at, version)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), $9)`

		for _, field := range fields {
			_, err = tx.Exec(
				fieldQuery,
				template.ID,
				field.FieldName,
				field.FieldType,
				field.FieldLabel,
				field.PlaceholderText,
				field.PositionData,
				field.ValidationRules,
				field.Required,
				field.Version,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to create template field %s: %w", field.FieldName, err)
			}
		}
	}

	// Create audit log entry
	auditQuery := `
		INSERT INTO document_audit_log (template_id, user_id, action, details, created_at)
		VALUES ($1, $2, 'template_created', $3, NOW())`

	auditDetails := fmt.Sprintf(`{"template_name": "%s", "field_count": %d}`, template.Name, len(fields))
	_, err = tx.Exec(auditQuery, template.ID, template.CreatedBy, auditDetails)
	if err != nil {
		// Don't fail the whole operation for audit log errors, but log it
		// You might want to add proper logging here
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return template, nil
}

// GetTemplateByID retrieves a template by ID with workspace access check
func (s *service) GetTemplateByID(templateID uuid.UUID, userID int) (*Template, error) {
	template := &Template{}
	query := `
		SELECT t.id, t.name, t.description, t.s3_bucket, t.s3_key, t.pdf_hash, 
			   t.file_size, t.mime_type, t.created_by, t.workspace_id, t.is_active, 
			   t.created_at, t.updated_at, t.version
		FROM templates t
		JOIN workspace_memberships wm ON t.workspace_id = wm.workspace_id
		WHERE t.id = $1 AND wm.user_id = $2 AND wm.status = 'active' AND t.is_active = true`

	err := s.db.QueryRow(query, templateID, userID).Scan(
		&template.ID, &template.Name, &template.Description, &template.S3Bucket,
		&template.S3Key, &template.PDFHash, &template.FileSize, &template.MimeType,
		&template.CreatedBy, &template.WorkspaceID, &template.IsActive,
		&template.CreatedAt, &template.UpdatedAt, &template.Version,
	)

	if err != nil {
		return nil, fmt.Errorf("template not found or access denied: %w", err)
	}

	return template, nil
}

// GetTemplateWithFields retrieves a template with all its fields
func (s *service) GetTemplateWithFields(templateID uuid.UUID, userID int) (*TemplateWithFields, error) {
	// First get the template
	template, err := s.GetTemplateByID(templateID, userID)
	if err != nil {
		return nil, err
	}

	// Then get the fields
	fields, err := s.GetTemplateFields(templateID, userID)
	if err != nil {
		return nil, err
	}

	return &TemplateWithFields{
		Template: *template,
		Fields:   fields,
	}, nil
}

// GetTemplateFields retrieves all fields for a template
func (s *service) GetTemplateFields(templateID uuid.UUID, userID int) ([]TemplateField, error) {
	// First verify user has access to this template
	_, err := s.GetTemplateByID(templateID, userID)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT id, template_id, field_name, field_type, field_label, placeholder_text,
			   position_data, validation_rules, required, created_at, version
		FROM template_fields
		WHERE template_id = $1
		ORDER BY created_at ASC`

	rows, err := s.db.Query(query, templateID)
	if err != nil {
		return nil, fmt.Errorf("failed to get template fields: %w", err)
	}
	defer rows.Close()

	var fields []TemplateField
	for rows.Next() {
		var field TemplateField
		err := rows.Scan(
			&field.ID, &field.TemplateID, &field.FieldName, &field.FieldType,
			&field.FieldLabel, &field.PlaceholderText, &field.PositionData,
			&field.ValidationRules, &field.Required, &field.CreatedAt, &field.Version,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan template field: %w", err)
		}
		fields = append(fields, field)
	}

	return fields, nil
}

// GetWorkspaceTemplates retrieves all templates for a workspace that user has access to
func (s *service) GetWorkspaceTemplates(workspaceID uuid.UUID, userID int) ([]UserTemplate, error) {
	// First check if user has access to this workspace
	accessQuery := `
		SELECT role FROM workspace_memberships 
		WHERE workspace_id = $1 AND user_id = $2 AND status = 'active'`

	var userRole string
	err := s.db.QueryRow(accessQuery, workspaceID, userID).Scan(&userRole)
	if err != nil {
		return nil, fmt.Errorf("user does not have access to this workspace")
	}

	// Updated query with JOIN to get creator information and field count
	query := `
		SELECT 
			t.id, t.name, t.description, t.file_size, t.created_by,
			u.name as creator_name, u.email as creator_email,
			t.workspace_id, w.name as workspace_name,
			COALESCE(field_counts.field_count, 0) as field_count,
			t.is_active, t.created_at, t.updated_at, t.version
		FROM templates t
		JOIN users u ON t.created_by = u.id
		JOIN workspaces w ON t.workspace_id = w.id
		LEFT JOIN (
			SELECT template_id, COUNT(*) as field_count
			FROM template_fields
			GROUP BY template_id
		) field_counts ON t.id = field_counts.template_id
		WHERE t.workspace_id = $1 AND t.is_active = true
		ORDER BY t.created_at DESC`

	rows, err := s.db.Query(query, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace templates: %w", err)
	}
	defer rows.Close()

	var templates []UserTemplate
	for rows.Next() {
		var template UserTemplate
		err := rows.Scan(
			&template.ID, &template.Name, &template.Description, &template.FileSize,
			&template.CreatedBy, &template.CreatorName, &template.CreatorEmail,
			&template.WorkspaceID, &template.WorkspaceName, &template.FieldCount,
			&template.IsActive, &template.CreatedAt, &template.UpdatedAt, &template.Version,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan template: %w", err)
		}
		templates = append(templates, template)
	}

	return templates, nil
}

// UpdateTemplate updates template metadata (not the PDF file)
func (s *service) UpdateTemplate(templateID uuid.UUID, name, description string, userID int) error {
	// Check if user has permission (creator, workspace owner, or admin)
	permissionQuery := `
		SELECT t.created_by, wm.role
		FROM templates t
		JOIN workspace_memberships wm ON t.workspace_id = wm.workspace_id
		WHERE t.id = $1 AND wm.user_id = $2 AND wm.status = 'active' AND t.is_active = true`

	var createdBy int
	var role string
	err := s.db.QueryRow(permissionQuery, templateID, userID).Scan(&createdBy, &role)
	if err != nil {
		return fmt.Errorf("template not found or access denied")
	}

	// Check permissions: creator, owner, or admin can update
	if createdBy != userID && role != "owner" && role != "admin" {
		return fmt.Errorf("insufficient permissions to update template")
	}

	// Update the template
	updateQuery := `
		UPDATE templates 
		SET name = $1, description = $2, updated_at = NOW()
		WHERE id = $3`

	result, err := s.db.Exec(updateQuery, name, description, templateID)
	if err != nil {
		return fmt.Errorf("failed to update template: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("template not found")
	}

	// Create audit log entry
	auditQuery := `
		INSERT INTO document_audit_log (template_id, user_id, action, details, created_at)
		VALUES ($1, $2, 'template_updated', $3, NOW())`

	auditDetails := fmt.Sprintf(`{"template_name": "%s", "updated_by": %d}`, name, userID)
	s.db.Exec(auditQuery, templateID, userID, auditDetails)

	return nil
}

// DeactivateTemplate soft deletes a template
func (s *service) DeactivateTemplate(templateID uuid.UUID, userID int) error {
	// Check if user has permission (creator, workspace owner, or admin)
	permissionQuery := `
		SELECT t.created_by, wm.role
		FROM templates t
		JOIN workspace_memberships wm ON t.workspace_id = wm.workspace_id
		WHERE t.id = $1 AND wm.user_id = $2 AND wm.status = 'active' AND t.is_active = true`

	var createdBy int
	var role string
	err := s.db.QueryRow(permissionQuery, templateID, userID).Scan(&createdBy, &role)
	if err != nil {
		return fmt.Errorf("template not found or access denied")
	}

	// Check permissions: creator, owner, or admin can deactivate
	if createdBy != userID && role != "owner" && role != "admin" {
		return fmt.Errorf("insufficient permissions to deactivate template")
	}

	// Deactivate the template
	updateQuery := `
		UPDATE templates 
		SET is_active = false, updated_at = NOW()
		WHERE id = $1`

	result, err := s.db.Exec(updateQuery, templateID)
	if err != nil {
		return fmt.Errorf("failed to deactivate template: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("template not found")
	}

	return nil
}

// AddFieldsToTemplate adds new fields to an existing template
func (s *service) AddFieldsToTemplate(templateID uuid.UUID, fields []TemplateField, userID int) error {
	// Check if user has permission to modify this template
	template, err := s.GetTemplateByID(templateID, userID)
	if err != nil {
		return err
	}

	// Check if user is creator, workspace owner, or admin
	permissionQuery := `
		SELECT wm.role
		FROM workspace_memberships wm
		WHERE wm.workspace_id = $1 AND wm.user_id = $2 AND wm.status = 'active'`

	var role string
	err = s.db.QueryRow(permissionQuery, template.WorkspaceID, userID).Scan(&role)
	if err != nil {
		return fmt.Errorf("access denied")
	}

	if template.CreatedBy != userID && role != "owner" && role != "admin" {
		return fmt.Errorf("insufficient permissions to modify template")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert new fields
	fieldQuery := `
		INSERT INTO template_fields (template_id, field_name, field_type, field_label, 
			placeholder_text, position_data, validation_rules, required, created_at, version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), $9)`

	for _, field := range fields {
		_, err = tx.Exec(
			fieldQuery,
			templateID,
			field.FieldName,
			field.FieldType,
			field.FieldLabel,
			field.PlaceholderText,
			field.PositionData,
			field.ValidationRules,
			field.Required,
			field.Version,
		)
		if err != nil {
			return fmt.Errorf("failed to add template field %s: %w", field.FieldName, err)
		}
	}

	// Update template version and timestamp
	_, err = tx.Exec(`
		UPDATE templates 
		SET version = version + 1, updated_at = NOW()
		WHERE id = $1`, templateID)
	if err != nil {
		return fmt.Errorf("failed to update template version: %w", err)
	}

	return tx.Commit()
}
