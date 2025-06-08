package routes

import (
	"encoding/json"
	"finalsign/internal/database"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type TemplateRoutes struct {
	server ServerInterface
}

func NewTemplateRoutes(server ServerInterface) *TemplateRoutes {
	return &TemplateRoutes{server: server}
}

func (tr *TemplateRoutes) RegisterRoutes(r *gin.Engine) {
	// Create middleware instance
	middleware := NewMiddleware(tr.server)

	// Template routes - all require authentication and workspace context
	templates := r.Group("/workspaces/:slug/templates")
	templates.Use(middleware.AuthMiddleware())
	templates.Use(middleware.WorkspaceMiddleware())
	{
		templates.POST("", tr.createTemplateHandler)
		templates.GET("", tr.getWorkspaceTemplatesHandler)
		templates.GET("/:templateID", tr.getTemplateHandler)
		templates.PUT("/:templateID", tr.updateTemplateHandler)
		templates.DELETE("/:templateID", tr.deactivateTemplateHandler)
		templates.PUT("/:templateID/fields", tr.UpdateTemplateFieldsHandler)
		templates.PUT("/:templateID/signers", tr.UpdateTemplateSignersHandler)
	}
}

type CreateTemplateRequest struct {
	Document   string         `json:"document"`
	TotalPages int            `json:"totalPages"`
	Fields     []FieldRequest `json:"fields"`
	Signers    []SignerRequest `json:"signers"`
}

type SignerRequest struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Color      string `json:"color"`
	FieldCount int    `json:"fieldCount"`
}

type FieldRequest struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Page       int                    `json:"page"`
	Position   map[string]interface{} `json:"position"`
	Label      string                 `json:"label"`
	Required   bool                   `json:"required"`
	Signer     int                    `json:"signer"`
	SignerName string                 `json:"signerName"`
}

func (tr *TemplateRoutes) createTemplateHandler(c *gin.Context) {
	user := c.MustGet("user").(*database.User)
	workspace := c.MustGet("workspace").(*database.UserWorkspace)

	// Check if user has permission to create templates (member or higher)
	if workspace.Role == "viewer" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions to create templates"})
		return
	}

	err := c.Request.ParseMultipartForm(32 << 20) // 32 MB max
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse form data"})
		return
	}

	// Get form fields
	name := c.PostForm("name")
	description := c.PostForm("description")
	templateDataJSON := c.PostForm("templateData") // The JSON you provided

	// Validate basic fields
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Template name is required"})
		return
	}

	if len(name) > 255 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Template name must be 255 characters or less"})
		return
	}

	if len(description) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Description must be 500 characters or less"})
		return
	}

	// Get uploaded file
	file, header, err := c.Request.FormFile("pdf")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "PDF file is required"})
		return
	}
	defer file.Close()

	// Validate file type
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".pdf") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Only PDF files are allowed"})
		return
	}

	// Validate file content
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read file"})
		return
	}
	if len(fileBytes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File is empty"})
		return
	}

	// Reset file position for S3 upload
	if seeker, ok := file.(io.Seeker); ok {
		seeker.Seek(0, io.SeekStart)
	}

	// Parse template data JSON
	var templateReq CreateTemplateRequest
	if templateDataJSON != "" {
		if err := json.Unmarshal([]byte(templateDataJSON), &templateReq); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template data JSON format"})
			return
		}
	}

	// Use totalPages from JSON, fallback to 1 if not provided
	totalPages := 1
	if templateReq.TotalPages > 0 {
		totalPages = templateReq.TotalPages
	}

	// Upload to S3
	s3Service := tr.server.GetS3Service()
	uploadResult, err := s3Service.UploadTemplate(c.Request.Context(), file, header, user.ID, workspace.WorkspaceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload PDF"})
		return
	}

	// Convert and validate signers
	signers, signerOrderToID, err := tr.convertAndValidateSigners(templateReq.Signers)
	if err != nil {
		// Clean up uploaded file if signer validation fails
		s3Service.DeleteFile(c.Request.Context(), uploadResult.S3Key)
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid signers: %v", err)})
		return
	}

	// Convert and validate fields
	fields, err := tr.convertAndValidateFields(templateReq.Fields, signerOrderToID)
	if err != nil {
		// Clean up uploaded file if field validation fails
		s3Service.DeleteFile(c.Request.Context(), uploadResult.S3Key)
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid fields: %v", err)})
		return
	}

	// Create template in database
	template := &database.Template{
		Name:        name,
		Description: description,
		S3Bucket:    uploadResult.S3Bucket,
		S3Key:       uploadResult.S3Key,
		PDFHash:     uploadResult.FileHash,
		FileSize:    uploadResult.FileSize,
		MimeType:    uploadResult.MimeType,
		TotalPages:  totalPages,
		CreatedBy:   user.ID,
		WorkspaceID: workspace.WorkspaceID,
		IsActive:    true,
		Version:     1,
	}

	db := tr.server.GetDB()
	createdTemplate, err := db.CreateTemplateWithSignersAndFields(template, signers, fields)
	if err != nil {
		// Clean up uploaded file if database creation fails
		s3Service.DeleteFile(c.Request.Context(), uploadResult.S3Key)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create template"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Template created successfully",
		"template": gin.H{
			"id":           createdTemplate.ID,
			"name":         createdTemplate.Name,
			"description":  createdTemplate.Description,
			"file_size":    createdTemplate.FileSize,
			"total_pages":  createdTemplate.TotalPages,
			"signer_count": len(signers),
			"field_count":  len(fields),
			"created_at":   createdTemplate.CreatedAt,
		},
	})
}


func (tr *TemplateRoutes) convertAndValidateSigners(signerRequests []SignerRequest) ([]database.TemplateSigner, map[int]uuid.UUID, error) {
	if len(signerRequests) == 0 {
		return nil, nil, fmt.Errorf("at least one signer is required")
	}

	var signers []database.TemplateSigner
	signerOrderToID := make(map[int]uuid.UUID)
	usedOrders := make(map[int]bool)

	for i, req := range signerRequests {
		// Validate signer order (ID)
		if req.ID <= 0 {
			return nil, nil, fmt.Errorf("signer ID must be positive at index %d", i)
		}

		if usedOrders[req.ID] {
			return nil, nil, fmt.Errorf("duplicate signer ID %d", req.ID)
		}
		usedOrders[req.ID] = true

		// Validate signer name
		if strings.TrimSpace(req.Name) == "" {
			return nil, nil, fmt.Errorf("signer name is required at index %d", i)
		}

		if len(req.Name) > 255 {
			return nil, nil, fmt.Errorf("signer name too long at index %d", i)
		}

		// Validate color format (hex color)
		if !isValidHexColor(req.Color) {
			return nil, nil, fmt.Errorf("invalid color format '%s' for signer at index %d, must be hex color like #3B82F6", req.Color, i)
		}

		// Create signer (ID will be generated by database)
		signerID := uuid.New() // Generate UUID for mapping
		signer := database.TemplateSigner{
			ID:          signerID,
			SignerOrder: req.ID,
			SignerName:  strings.TrimSpace(req.Name),
			SignerColor: req.Color,
		}

		signers = append(signers, signer)
		signerOrderToID[req.ID] = signerID
	}

	return signers, signerOrderToID, nil
}


func (tr *TemplateRoutes) convertAndValidateFields(fieldRequests []FieldRequest, signerOrderToID map[int]uuid.UUID) ([]database.TemplateField, error) {
	validFieldTypes := map[string]bool{
		"text":      true,
		"signature": true,
		"date":      true,
		"checkbox":  true,
		"email":     true,
		"phone":     true,
	}

	var fields []database.TemplateField
	fieldNames := make(map[string]bool)

	for i, req := range fieldRequests {
		// Validate field type
		if !validFieldTypes[req.Type] {
			return nil, fmt.Errorf("invalid field type '%s' at index %d. Must be one of: text, signature, date, checkbox, email, phone", req.Type, i)
		}

		// Validate field ID (will be used as field_name)
		if strings.TrimSpace(req.ID) == "" {
			return nil, fmt.Errorf("field ID is required at index %d", i)
		}

		// Check for duplicate field IDs
		if fieldNames[req.ID] {
			return nil, fmt.Errorf("duplicate field ID '%s' at index %d", req.ID, i)
		}
		fieldNames[req.ID] = true

		// Validate signer reference
		signerID, exists := signerOrderToID[req.Signer]
		if !exists {
			return nil, fmt.Errorf("field at index %d references non-existent signer %d", i, req.Signer)
		}

		// Validate position data
		if err := tr.validatePositionData(req.Position); err != nil {
			return nil, fmt.Errorf("invalid position data for field '%s' at index %d: %v", req.ID, i, err)
		}

		// Add page to position data
		positionWithPage := make(map[string]interface{})
		for k, v := range req.Position {
			positionWithPage[k] = v
		}
		positionWithPage["page"] = req.Page

		// Convert position data to JSON string
		positionBytes, err := json.Marshal(positionWithPage)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal position data for field '%s': %v", req.ID, err)
		}

		// Create database field
		field := database.TemplateField{
			SignerID:        signerID,
			FieldName:       req.ID,
			FieldType:       req.Type,
			FieldLabel:      req.Label,
			PlaceholderText: "", // Could be derived from label or left empty
			PositionData:    string(positionBytes),
			ValidationRules: "{}",
			Required:        req.Required,
			Version:         1,
		}

		fields = append(fields, field)
	}

	return fields, nil
}


func (tr *TemplateRoutes) validatePositionData(positionData map[string]interface{}) error {
	required := []string{"x", "y", "width", "height"}

	for _, field := range required {
		value, exists := positionData[field]
		if !exists {
			return fmt.Errorf("missing required field '%s'", field)
		}

		// Validate numeric values
		if num, ok := value.(float64); !ok {
			return fmt.Errorf("field '%s' must be a number", field)
		} else if field == "width" || field == "height" {
			if num <= 0 {
				return fmt.Errorf("field '%s' must be greater than 0", field)
			}
		} else if num < 0 {
			return fmt.Errorf("field '%s' must be non-negative", field)
		}
	}

	return nil
}


func isValidHexColor(color string) bool {
	if len(color) != 7 || color[0] != '#' {
		return false
	}
	
	for i := 1; i < len(color); i++ {
		c := color[i]
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'F') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	
	return true
}




func (tr *TemplateRoutes) getWorkspaceTemplatesHandler(c *gin.Context) {
	user := c.MustGet("user").(*database.User)
	workspace := c.MustGet("workspace").(*database.UserWorkspace)
	db := tr.server.GetDB()
	templates, err := db.GetWorkspaceTemplatesList(workspace.WorkspaceID, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch templates",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"templates": templates,
		"total":     len(templates),
		"workspace": gin.H{
			"id":   workspace.WorkspaceID,
			"name": workspace.WorkspaceName,
			"slug": workspace.WorkspaceSlug,
		},
	})
}

func (tr *TemplateRoutes) getTemplateHandler(c *gin.Context) {
	user := c.MustGet("user").(*database.User)
	workspace := c.MustGet("workspace").(*database.UserWorkspace)

	templateIDStr := c.Param("templateID")
	templateID, err := uuid.Parse(templateIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template ID"})
		return
	}

	db := tr.server.GetDB()
	template, err := db.GetTemplateWithSignersAndFields(templateID, user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "access denied") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch template"})
		return
	}

	// Additional validation: ensure template belongs to the current workspace
	if template.WorkspaceID != workspace.WorkspaceID {
		c.JSON(http.StatusNotFound, gin.H{"error": "Template not found in this workspace"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"template": template})
}

// updateTemplateHandler updates template metadata (name, description)
func (tr *TemplateRoutes) updateTemplateHandler(c *gin.Context) {
	user := c.MustGet("user").(*database.User)
	workspace := c.MustGet("workspace").(*database.UserWorkspace)

	templateIDStr := c.Param("templateID")
	templateID, err := uuid.Parse(templateIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template ID"})
		return
	}

	var req struct {
		Name        string `json:"name" binding:"required,min=1,max=255"`
		Description string `json:"description" binding:"max=500"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// First verify template exists and belongs to this workspace
	db := tr.server.GetDB()
	template, err := db.GetTemplateByID(templateID, user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "access denied") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch template"})
		return
	}

	// Ensure template belongs to the current workspace
	if template.WorkspaceID != workspace.WorkspaceID {
		c.JSON(http.StatusNotFound, gin.H{"error": "Template not found in this workspace"})
		return
	}

	err = db.UpdateTemplate(templateID, req.Name, req.Description, user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "insufficient permissions") {
			c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions to update template"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update template"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Template updated successfully"})
}


func (tr *TemplateRoutes) deactivateTemplateHandler(c *gin.Context) {
	user := c.MustGet("user").(*database.User)
	workspace := c.MustGet("workspace").(*database.UserWorkspace)

	templateIDStr := c.Param("templateID")
	templateID, err := uuid.Parse(templateIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template ID"})
		return
	}

	// First verify template exists and belongs to this workspace
	db := tr.server.GetDB()
	template, err := db.GetTemplateByID(templateID, user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "access denied") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch template"})
		return
	}

	// Ensure template belongs to the current workspace
	if template.WorkspaceID != workspace.WorkspaceID {
		c.JSON(http.StatusNotFound, gin.H{"error": "Template not found in this workspace"})
		return
	}

	err = db.DeactivateTemplate(templateID, user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "insufficient permissions") {
			c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions to delete template"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete template"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Template deleted successfully"})
}


func (tr *TemplateRoutes) UpdateTemplateFieldsHandler(c *gin.Context) {
	user := c.MustGet("user").(*database.User)
	workspace := c.MustGet("workspace").(*database.UserWorkspace)

	templateIDStr := c.Param("templateID")
	templateID, err := uuid.Parse(templateIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template ID"})
		return
	}

	var req struct {
		Fields []FieldRequest `json:"fields" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	db := tr.server.GetDB()

	template, err := db.GetTemplateByID(templateID, user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "access denied") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch template"})
		return
	}

	if template.WorkspaceID != workspace.WorkspaceID {
		c.JSON(http.StatusNotFound, gin.H{"error": "Template not found in this workspace"})
		return
	}

	// Get existing signers to build the mapping
	signers, err := db.GetTemplateSigners(templateID, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch template signers"})
		return
	}

	signerOrderToID := make(map[int]uuid.UUID)
	for _, signer := range signers {
		signerOrderToID[signer.SignerOrder] = signer.ID
	}


	fields, err := tr.convertAndValidateFields(req.Fields, signerOrderToID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid fields: %v", err)})
		return
	}

	// Replace all fields
	err = db.ReplaceTemplateFields(templateID, fields, user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "insufficient permissions") {
			c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions to modify template"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update template fields"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "Template fields updated successfully",
		"field_count": len(fields),
	})
}


func (tr *TemplateRoutes) UpdateTemplateSignersHandler(c *gin.Context) {
	user := c.MustGet("user").(*database.User)
	workspace := c.MustGet("workspace").(*database.UserWorkspace)

	templateIDStr := c.Param("templateID")
	templateID, err := uuid.Parse(templateIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template ID"})
		return
	}

	var req struct {
		Signers []SignerRequest `json:"signers" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}


	signers, _, err := tr.convertAndValidateSigners(req.Signers)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid signers: %v", err)})
		return
	}

	db := tr.server.GetDB()


	template, err := db.GetTemplateByID(templateID, user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "access denied") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch template"})
		return
	}


	if template.WorkspaceID != workspace.WorkspaceID {
		c.JSON(http.StatusNotFound, gin.H{"error": "Template not found in this workspace"})
		return
	}

	err = db.ReplaceTemplateSigners(templateID, signers, user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "insufficient permissions") {
			c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions to modify template"})
			return
		}
		if strings.Contains(err.Error(), "fields exist") {
			c.JSON(http.StatusConflict, gin.H{
				"error": "Cannot update signers while template has fields. Update fields first or use the complete template update endpoint.",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update template signers"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "Template signers updated successfully",
		"signer_count": len(signers),
		"note":         "All existing fields have been removed. Please update fields to reassign them to the new signers.",
	})
}