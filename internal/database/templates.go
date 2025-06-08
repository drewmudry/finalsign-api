package database

import (
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

// Template struct - add TotalPages field
type Template struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	S3Bucket    string    `json:"s3_bucket"`
	S3Key       string    `json:"s3_key"`
	PDFHash     string    `json:"pdf_hash"`
	FileSize    int64     `json:"file_size"`
	MimeType    string    `json:"mime_type"`
	TotalPages  int       `json:"total_pages"`
	CreatedBy   int       `json:"created_by"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Version     int       `json:"version"`
}

type TemplateListItem struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	FileSize      int64     `json:"file_size"`
	TotalPages    int       `json:"total_pages"`
	CreatedBy     int       `json:"created_by"`
	CreatorName   string    `json:"creator_name"`
	CreatorEmail  string    `json:"creator_email"`
	SignerCount   int       `json:"signer_count"`
	FieldCount    int       `json:"field_count"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// NEW: TemplateSigner struct to represent signers
type TemplateSigner struct {
	ID           uuid.UUID `json:"id"`
	TemplateID   uuid.UUID `json:"template_id"`
	SignerOrder  int       `json:"signer_order"`
	SignerName   string    `json:"signer_name"`
	SignerColor  string    `json:"signer_color"`
	CreatedAt    time.Time `json:"created_at"`
}

// TemplateField struct - add SignerID to link fields to signers
type TemplateField struct {
	ID              uuid.UUID `json:"id"`
	TemplateID      uuid.UUID `json:"template_id"`
	SignerID        uuid.UUID `json:"signer_id"`
	FieldName       string    `json:"field_name"`
	FieldType       string    `json:"field_type"`       // Maps to "type" in your JSON
	FieldLabel      string    `json:"field_label"`      // Maps to "label" in your JSON
	PlaceholderText string    `json:"placeholder_text"`
	PositionData    string    `json:"position_data"`    // JSON string of position + page
	ValidationRules string    `json:"validation_rules"` // JSON string
	Required        bool      `json:"required"`         // Maps to "required" in your JSON
	CreatedAt       time.Time `json:"created_at"`
	Version         int       `json:"version"`
}

// Updated: TemplateWithSignersAndFields to include signers
type TemplateWithSignersAndFields struct {
	Template
	Signers []TemplateSigner `json:"signers"`
	Fields  []TemplateField  `json:"fields"`
}

// Updated: UserTemplate to include signer count
type UserTemplate struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	FileSize      int64     `json:"file_size"`
	TotalPages    int       `json:"total_pages"`    // NEW: Include page count
	CreatedBy     int       `json:"created_by"`
	CreatorName   string    `json:"creator_name"`
	CreatorEmail  string    `json:"creator_email"`
	WorkspaceID   uuid.UUID `json:"workspace_id"`
	WorkspaceName string    `json:"workspace_name"`
	SignerCount   int       `json:"signer_count"`   // NEW: Count of signers
	FieldCount    int       `json:"field_count"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	Version       int       `json:"version"`
}

// CreateTemplateWithSignersAndFields creates a new template with signers and fields
func (s *service) CreateTemplateWithSignersAndFields(template *Template, signers []TemplateSigner, fields []TemplateField) (*Template, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert template
	templateQuery := `
		INSERT INTO templates (name, description, s3_bucket, s3_key, pdf_hash, file_size, 
			mime_type, total_pages, created_by, workspace_id, is_active, created_at, updated_at, version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW(), NOW(), $12)
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
		template.TotalPages,
		template.CreatedBy,
		template.WorkspaceID,
		template.IsActive,
		template.Version,
	).Scan(&template.ID, &template.CreatedAt, &template.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create template: %w", err)
	}

	// Create map to store signer order -> signer ID mapping
	signerOrderToID := make(map[int]uuid.UUID)

	// Insert signers
	if len(signers) > 0 {
		signerQuery := `
			INSERT INTO template_signers (template_id, signer_order, signer_name, signer_color, created_at)
			VALUES ($1, $2, $3, $4, NOW())
			RETURNING id`

		for i := range signers {
			signers[i].TemplateID = template.ID
			err = tx.QueryRow(
				signerQuery,
				signers[i].TemplateID,
				signers[i].SignerOrder,
				signers[i].SignerName,
				signers[i].SignerColor,
			).Scan(&signers[i].ID)
			if err != nil {
				return nil, fmt.Errorf("failed to create template signer %s: %w", signers[i].SignerName, err)
			}
			
			// Map signer order to ID for field creation
			signerOrderToID[signers[i].SignerOrder] = signers[i].ID
		}
	}

	// Insert fields if any
	if len(fields) > 0 {
		fieldQuery := `
			INSERT INTO template_fields (template_id, signer_id, field_name, field_type, field_label, 
				placeholder_text, position_data, validation_rules, required, created_at, version)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), $10)`

		for _, field := range fields {
			_, err = tx.Exec(
				fieldQuery,
				template.ID,
				field.SignerID, // This should be set by the calling code using signerOrderToID
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

	auditDetails := fmt.Sprintf(`{"template_name": "%s", "signer_count": %d, "field_count": %d}`, 
		template.Name, len(signers), len(fields))
	_, err = tx.Exec(auditQuery, template.ID, template.CreatedBy, auditDetails)
	if err != nil {
		// Don't fail the whole operation for audit log errors
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
			   t.file_size, t.mime_type, t.total_pages, t.created_by, t.workspace_id, t.is_active, 
			   t.created_at, t.updated_at, t.version
		FROM templates t
		JOIN workspace_memberships wm ON t.workspace_id = wm.workspace_id
		WHERE t.id = $1 AND wm.user_id = $2 AND wm.status = 'active' AND t.is_active = true`

	err := s.db.QueryRow(query, templateID, userID).Scan(
		&template.ID, &template.Name, &template.Description, &template.S3Bucket,
		&template.S3Key, &template.PDFHash, &template.FileSize, &template.MimeType,
		&template.TotalPages, &template.CreatedBy, &template.WorkspaceID, &template.IsActive,
		&template.CreatedAt, &template.UpdatedAt, &template.Version,
	)

	if err != nil {
		return nil, fmt.Errorf("template not found or access denied: %w", err)
	}

	return template, nil
}

// GetTemplateWithSignersAndFields retrieves a template with all signers and fields
func (s *service) GetTemplateWithSignersAndFields(templateID uuid.UUID, userID int) (*TemplateWithSignersAndFields, error) {
	// First get the template
	template, err := s.GetTemplateByID(templateID, userID)
	if err != nil {
		return nil, err
	}

	// Get the signers
	signers, err := s.GetTemplateSigners(templateID, userID)
	if err != nil {
		return nil, err
	}

	// Get the fields
	fields, err := s.GetTemplateFields(templateID, userID)
	if err != nil {
		return nil, err
	}

	return &TemplateWithSignersAndFields{
		Template: *template,
		Signers:  signers,
		Fields:   fields,
	}, nil
}

// GetTemplateSigners retrieves all signers for a template
func (s *service) GetTemplateSigners(templateID uuid.UUID, userID int) ([]TemplateSigner, error) {
	// First verify user has access to this template
	_, err := s.GetTemplateByID(templateID, userID)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT id, template_id, signer_order, signer_name, signer_color, created_at
		FROM template_signers
		WHERE template_id = $1
		ORDER BY signer_order ASC`

	rows, err := s.db.Query(query, templateID)
	if err != nil {
		return nil, fmt.Errorf("failed to get template signers: %w", err)
	}
	defer rows.Close()

	var signers []TemplateSigner
	for rows.Next() {
		var signer TemplateSigner
		err := rows.Scan(
			&signer.ID, &signer.TemplateID, &signer.SignerOrder,
			&signer.SignerName, &signer.SignerColor, &signer.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan template signer: %w", err)
		}
		signers = append(signers, signer)
	}

	return signers, nil
}

// GetTemplateFields retrieves all fields for a template
func (s *service) GetTemplateFields(templateID uuid.UUID, userID int) ([]TemplateField, error) {
	// First verify user has access to this template
	_, err := s.GetTemplateByID(templateID, userID)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT tf.id, tf.template_id, tf.signer_id, tf.field_name, tf.field_type, tf.field_label, 
			   tf.placeholder_text, tf.position_data, tf.validation_rules, tf.required, 
			   tf.created_at, tf.version
		FROM template_fields tf
		JOIN template_signers ts ON tf.signer_id = ts.id
		WHERE tf.template_id = $1
		ORDER BY ts.signer_order ASC, tf.created_at ASC`

	rows, err := s.db.Query(query, templateID)
	if err != nil {
		return nil, fmt.Errorf("failed to get template fields: %w", err)
	}
	defer rows.Close()

	var fields []TemplateField
	for rows.Next() {
		var field TemplateField
		err := rows.Scan(
			&field.ID, &field.TemplateID, &field.SignerID, &field.FieldName, &field.FieldType,
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
func (s *service) GetWorkspaceTemplatesList(workspaceID uuid.UUID, userID int) ([]TemplateListItem, error) {

	// First check if user has access to this workspace
	accessQuery := `
		SELECT role FROM workspace_memberships 
		WHERE workspace_id = $1 AND user_id = $2 AND status = 'active'`

	var userRole string
	err := s.db.QueryRow(accessQuery, workspaceID, userID).Scan(&userRole)
	if err != nil {
		return nil, fmt.Errorf("user does not have access to this workspace: %v", err)
	}

	// Simplified query - only select what we need for the list view
	query := `
		SELECT 
			t.id, 
			t.name, 
			t.description, 
			t.file_size, 
			t.total_pages, 
			t.created_by,
			u.name as creator_name, 
			u.email as creator_email,
			COALESCE(signer_counts.signer_count, 0) as signer_count,
			COALESCE(field_counts.field_count, 0) as field_count,
			t.is_active, 
			t.created_at, 
			t.updated_at
		FROM templates t
		JOIN users u ON t.created_by = u.id
		LEFT JOIN (
			SELECT template_id, COUNT(*) as signer_count
			FROM template_signers
			GROUP BY template_id
		) signer_counts ON t.id = signer_counts.template_id
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
	var templates []TemplateListItem
	rowCount := 0
	
	for rows.Next() {
		rowCount++
		log.Printf("DEBUG: Processing row %d", rowCount)
		
		var template TemplateListItem
		err := rows.Scan(
			&template.ID,               // 1. t.id
			&template.Name,             // 2. t.name  
			&template.Description,      // 3. t.description
			&template.FileSize,         // 4. t.file_size
			&template.TotalPages,       // 5. t.total_pages
			&template.CreatedBy,        // 6. t.created_by
			&template.CreatorName,      // 7. u.name as creator_name
			&template.CreatorEmail,     // 8. u.email as creator_email
			&template.SignerCount,      // 9. signer_count
			&template.FieldCount,       // 10. field_count
			&template.IsActive,         // 11. t.is_active
			&template.CreatedAt,        // 12. t.created_at
			&template.UpdatedAt,        // 13. t.updated_at
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan template at row %d: %w", rowCount, err)
		}
		templates = append(templates, template)
	}

	// Check for any errors that occurred during iteration
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error during row iteration: %w", err)
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

// ReplaceTemplateFields removes all existing fields and replaces them with new ones
func (s *service) ReplaceTemplateFields(templateID uuid.UUID, fields []TemplateField, userID int) error {
	// Check permissions first
	template, err := s.GetTemplateByID(templateID, userID)
	if err != nil {
		return err
	}

	// Check if user has permission to modify this template
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

	// Delete all existing fields
	_, err = tx.Exec(`DELETE FROM template_fields WHERE template_id = $1`, templateID)
	if err != nil {
		return fmt.Errorf("failed to delete existing fields: %w", err)
	}

	// Insert new fields if any
	if len(fields) > 0 {
		fieldQuery := `
			INSERT INTO template_fields (template_id, signer_id, field_name, field_type, field_label, 
				placeholder_text, position_data, validation_rules, required, created_at, version)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), $10)`

		for _, field := range fields {
			_, err = tx.Exec(
				fieldQuery,
				templateID,
				field.SignerID,
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
				return fmt.Errorf("failed to create template field %s: %w", field.FieldName, err)
			}
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

// ReplaceTemplateSigners removes all existing signers and replaces them with new ones
// WARNING: This will also delete all fields since fields are linked to signers
func (s *service) ReplaceTemplateSigners(templateID uuid.UUID, signers []TemplateSigner, userID int) error {
	// Check permissions first
	template, err := s.GetTemplateByID(templateID, userID)
	if err != nil {
		return err
	}

	// Check if user has permission to modify this template
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

	// Delete all existing fields first (due to foreign key constraint)
	_, err = tx.Exec(`DELETE FROM template_fields WHERE template_id = $1`, templateID)
	if err != nil {
		return fmt.Errorf("failed to delete existing fields: %w", err)
	}

	// Delete all existing signers
	_, err = tx.Exec(`DELETE FROM template_signers WHERE template_id = $1`, templateID)
	if err != nil {
		return fmt.Errorf("failed to delete existing signers: %w", err)
	}

	// Insert new signers if any
	if len(signers) > 0 {
		signerQuery := `
			INSERT INTO template_signers (template_id, signer_order, signer_name, signer_color, created_at)
			VALUES ($1, $2, $3, $4, NOW())
			RETURNING id`

		for i := range signers {
			signers[i].TemplateID = templateID
			err = tx.QueryRow(
				signerQuery,
				signers[i].TemplateID,
				signers[i].SignerOrder,
				signers[i].SignerName,
				signers[i].SignerColor,
			).Scan(&signers[i].ID)
			if err != nil {
				return fmt.Errorf("failed to create template signer %s: %w", signers[i].SignerName, err)
			}
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