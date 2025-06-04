package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"finalsign/internal/database"
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
		templates.POST("/:templateID/fields", tr.addFieldsToTemplateHandler)
		templates.GET("/:templateID/fields", tr.getTemplateFieldsHandler)
	}
}

// createTemplateHandler handles PDF upload and template creation
func (tr *TemplateRoutes) createTemplateHandler(c *gin.Context) {
	user := c.MustGet("user").(*database.User)
	workspace := c.MustGet("workspace").(*database.UserWorkspace)

	// Check if user has permission to create templates (member or higher)
	if workspace.Role == "viewer" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions to create templates"})
		return
	}

	// Parse multipart form
	err := c.Request.ParseMultipartForm(32 << 20) // 32 MB max
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse form data"})
		return
	}

	// Get form fields
	name := c.PostForm("name")
	description := c.PostForm("description")
	fieldsJSON := c.PostForm("fields") // Optional: fields can be added later

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

	// Upload to S3
	s3Service := tr.server.GetS3Service()
	uploadResult, err := s3Service.UploadTemplate(c.Request.Context(), file, header, user.ID, workspace.WorkspaceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload PDF"})
		return
	}

	// Parse fields if provided
	var fields []database.TemplateField
	if fieldsJSON != "" {
		var fieldRequests []FieldRequest
		if err := json.Unmarshal([]byte(fieldsJSON), &fieldRequests); err != nil {
			// Clean up uploaded file if field parsing fails
			s3Service.DeleteFile(c.Request.Context(), uploadResult.S3Key)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid fields JSON format"})
			return
		}

		// Validate and convert fields
		fields, err = tr.validateAndConvertFields(fieldRequests)
		if err != nil {
			// Clean up uploaded file if field validation fails
			s3Service.DeleteFile(c.Request.Context(), uploadResult.S3Key)
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
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
		CreatedBy:   user.ID,
		WorkspaceID: workspace.WorkspaceID,
		IsActive:    true,
		Version:     1,
	}

	db := tr.server.GetDB()
	createdTemplate, err := db.CreateTemplate(template, fields)
	if err != nil {
		// Clean up uploaded file if database creation fails
		s3Service.DeleteFile(c.Request.Context(), uploadResult.S3Key)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create template"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Template created successfully",
		"template": gin.H{
			"id":          createdTemplate.ID,
			"name":        createdTemplate.Name,
			"description": createdTemplate.Description,
			"file_size":   createdTemplate.FileSize,
			"field_count": len(fields),
			"created_at":  createdTemplate.CreatedAt,
		},
	})
}

// getWorkspaceTemplatesHandler returns all templates for a workspace
func (tr *TemplateRoutes) getWorkspaceTemplatesHandler(c *gin.Context) {
	fmt.Println("DEBUG: Entering getWorkspaceTemplatesHandler")

	user := c.MustGet("user").(*database.User)
	fmt.Printf("DEBUG: User ID: %d\n", user.ID)

	workspace := c.MustGet("workspace").(*database.UserWorkspace)
	fmt.Printf("DEBUG: Workspace ID: %s, Name: %s\n", workspace.WorkspaceID, workspace.WorkspaceName)

	db := tr.server.GetDB()
	fmt.Println("DEBUG: About to call GetWorkspaceTemplates")

	templates, err := db.GetWorkspaceTemplates(workspace.WorkspaceID, user.ID)
	if err != nil {
		fmt.Printf("DEBUG: Error from GetWorkspaceTemplates: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch templates"})
		return
	}

	fmt.Printf("DEBUG: Successfully got %d templates\n", len(templates))

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

// getTemplateHandler returns a specific template with its fields
func (tr *TemplateRoutes) getTemplateHandler(c *gin.Context) {
	user := c.MustGet("user").(*database.User)

	templateIDStr := c.Param("templateID")
	templateID, err := uuid.Parse(templateIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template ID"})
		return
	}

	db := tr.server.GetDB()
	template, err := db.GetTemplateWithFields(templateID, user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "access denied") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch template"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"template": template})
}

// updateTemplateHandler updates template metadata
func (tr *TemplateRoutes) updateTemplateHandler(c *gin.Context) {
	user := c.MustGet("user").(*database.User)

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

	db := tr.server.GetDB()
	err = db.UpdateTemplate(templateID, req.Name, req.Description, user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "access denied") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
			return
		}
		if strings.Contains(err.Error(), "insufficient permissions") {
			c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions to update template"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update template"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Template updated successfully"})
}

// deactivateTemplateHandler soft deletes a template
func (tr *TemplateRoutes) deactivateTemplateHandler(c *gin.Context) {
	user := c.MustGet("user").(*database.User)

	templateIDStr := c.Param("templateID")
	templateID, err := uuid.Parse(templateIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template ID"})
		return
	}

	db := tr.server.GetDB()
	err = db.DeactivateTemplate(templateID, user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "access denied") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
			return
		}
		if strings.Contains(err.Error(), "insufficient permissions") {
			c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions to delete template"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete template"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Template deleted successfully"})
}

// addFieldsToTemplateHandler adds new fields to an existing template
func (tr *TemplateRoutes) addFieldsToTemplateHandler(c *gin.Context) {
	user := c.MustGet("user").(*database.User)

	templateIDStr := c.Param("templateID")
	templateID, err := uuid.Parse(templateIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template ID"})
		return
	}

	var req struct {
		Fields []FieldRequest `json:"fields" binding:"required,min=1"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate and convert fields
	fields, err := tr.validateAndConvertFields(req.Fields)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	db := tr.server.GetDB()
	err = db.AddFieldsToTemplate(templateID, fields, user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "access denied") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
			return
		}
		if strings.Contains(err.Error(), "insufficient permissions") {
			c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions to modify template"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add fields to template"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "Fields added successfully",
		"field_count": len(fields),
	})
}

// getTemplateFieldsHandler returns all fields for a template
func (tr *TemplateRoutes) getTemplateFieldsHandler(c *gin.Context) {
	user := c.MustGet("user").(*database.User)

	templateIDStr := c.Param("templateID")
	templateID, err := uuid.Parse(templateIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template ID"})
		return
	}

	db := tr.server.GetDB()
	fields, err := db.GetTemplateFields(templateID, user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "access denied") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch template fields"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"fields": fields,
		"total":  len(fields),
	})
}

// Field request structure for API
type FieldRequest struct {
	FieldName       string                 `json:"field_name" binding:"required,min=1,max=255"`
	FieldType       string                 `json:"field_type" binding:"required"`
	FieldLabel      string                 `json:"field_label" binding:"max=255"`
	PlaceholderText string                 `json:"placeholder_text" binding:"max=255"`
	PositionData    map[string]interface{} `json:"position_data" binding:"required"`
	ValidationRules map[string]interface{} `json:"validation_rules"`
	Required        bool                   `json:"required"`
}

// Position data structure for validation
type PositionData struct {
	X      float64 `json:"x" binding:"required,min=0"`
	Y      float64 `json:"y" binding:"required,min=0"`
	Width  float64 `json:"width" binding:"required,min=1"`
	Height float64 `json:"height" binding:"required,min=1"`
	Page   int     `json:"page" binding:"required,min=1"`
}

// validateAndConvertFields validates field requests and converts them to database fields
func (tr *TemplateRoutes) validateAndConvertFields(fieldRequests []FieldRequest) ([]database.TemplateField, error) {
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
		if !validFieldTypes[req.FieldType] {
			return nil, fmt.Errorf("invalid field type '%s' at index %d. Must be one of: text, signature, date, checkbox, email, phone", req.FieldType, i)
		}

		// Check for duplicate field names
		if fieldNames[req.FieldName] {
			return nil, fmt.Errorf("duplicate field name '%s' at index %d", req.FieldName, i)
		}
		fieldNames[req.FieldName] = true

		// Validate position data structure
		if err := tr.validatePositionData(req.PositionData); err != nil {
			return nil, fmt.Errorf("invalid position data for field '%s' at index %d: %v", req.FieldName, i, err)
		}

		// Convert position data to JSON string
		positionBytes, err := json.Marshal(req.PositionData)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal position data for field '%s': %v", req.FieldName, err)
		}

		// Convert validation rules to JSON string
		var validationJSON string
		if req.ValidationRules != nil {
			validationBytes, err := json.Marshal(req.ValidationRules)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal validation rules for field '%s': %v", req.FieldName, err)
			}
			validationJSON = string(validationBytes)
		} else {
			validationJSON = "{}"
		}

		// Create database field
		field := database.TemplateField{
			FieldName:       req.FieldName,
			FieldType:       req.FieldType,
			FieldLabel:      req.FieldLabel,
			PlaceholderText: req.PlaceholderText,
			PositionData:    string(positionBytes),
			ValidationRules: validationJSON,
			Required:        req.Required,
			Version:         1,
		}

		fields = append(fields, field)
	}

	return fields, nil
}

// validatePositionData validates the position data structure
func (tr *TemplateRoutes) validatePositionData(positionData map[string]interface{}) error {
	required := []string{"x", "y", "width", "height", "page"}

	for _, field := range required {
		value, exists := positionData[field]
		if !exists {
			return fmt.Errorf("missing required field '%s'", field)
		}

		// Validate numeric values
		switch field {
		case "x", "y", "width", "height":
			if num, ok := value.(float64); !ok {
				return fmt.Errorf("field '%s' must be a number", field)
			} else if field == "width" || field == "height" {
				if num <= 0 {
					return fmt.Errorf("field '%s' must be greater than 0", field)
				}
			} else if num < 0 {
				return fmt.Errorf("field '%s' must be non-negative", field)
			}
		case "page":
			if num, ok := value.(float64); !ok {
				return fmt.Errorf("field 'page' must be a number")
			} else if num < 1 {
				return fmt.Errorf("field 'page' must be at least 1")
			}
		}
	}

	return nil
}
