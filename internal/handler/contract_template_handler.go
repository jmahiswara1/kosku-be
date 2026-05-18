package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kosku/backend/internal/dto"
	"github.com/kosku/backend/internal/service"
)

// ContractTemplateServicer is the interface that ContractTemplateHandler depends on.
type ContractTemplateServicer interface {
	ListTemplates(ctx context.Context, ownerID uuid.UUID) ([]dto.ContractTemplateResponse, error)
	CreateTemplate(ctx context.Context, ownerID uuid.UUID, req dto.CreateContractTemplateRequest) (dto.ContractTemplateResponse, error)
	UpdateTemplate(ctx context.Context, ownerID, templateID uuid.UUID, req dto.UpdateContractTemplateRequest) (dto.ContractTemplateResponse, error)
	GenerateContract(ctx context.Context, ownerID, templateID uuid.UUID, req dto.GenerateContractRequest) (dto.GenerateContractResponse, error)
}

// Ensure *service.ContractTemplateService satisfies ContractTemplateServicer at compile time.
var _ ContractTemplateServicer = (*service.ContractTemplateService)(nil)

// ContractTemplateHandler holds the dependencies for contract template HTTP handlers.
type ContractTemplateHandler struct {
	svc ContractTemplateServicer
}

// NewContractTemplateHandler creates a new ContractTemplateHandler.
func NewContractTemplateHandler(svc *service.ContractTemplateService) *ContractTemplateHandler {
	return &ContractTemplateHandler{svc: svc}
}

// NewContractTemplateHandlerWithService creates a new ContractTemplateHandler with any
// ContractTemplateServicer. Intended for use in tests.
func NewContractTemplateHandlerWithService(svc ContractTemplateServicer) *ContractTemplateHandler {
	return &ContractTemplateHandler{svc: svc}
}

// ListTemplates handles GET /v1/contract-templates.
// Returns all contract templates owned by the authenticated owner.
//
//	@Summary		List contract templates
//	@Description	Returns all contract templates owned by the authenticated owner.
//	@Tags			contract-templates
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}
//	@Failure		401	{object}	map[string]interface{}
//	@Router			/contract-templates [get]
//	@Security		BearerAuth
func (h *ContractTemplateHandler) ListTemplates(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	templates, err := h.svc.ListTemplates(c.Request.Context(), ownerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("LIST_TEMPLATES_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    templates,
		"meta":    gin.H{"total": len(templates)},
	})
}

// CreateTemplate handles POST /v1/contract-templates.
// Creates a new contract template for the authenticated owner.
//
//	@Summary		Create contract template
//	@Description	Creates a new contract template with placeholder support.
//	@Tags			contract-templates
//	@Accept			json
//	@Produce		json
//	@Param			body	body		dto.CreateContractTemplateRequest	true	"Template data"
//	@Success		201		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		401		{object}	map[string]interface{}
//	@Router			/contract-templates [post]
//	@Security		BearerAuth
func (h *ContractTemplateHandler) CreateTemplate(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	var req dto.CreateContractTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", err.Error()))
		return
	}

	tmpl, err := h.svc.CreateTemplate(c.Request.Context(), ownerID, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("CREATE_TEMPLATE_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusCreated, successResponse(tmpl))
}

// UpdateTemplate handles PUT /v1/contract-templates/:id.
// Updates an existing contract template. Only the owner may update their templates.
//
//	@Summary		Update contract template
//	@Description	Updates the name and content of an existing contract template.
//	@Tags			contract-templates
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string								true	"Template UUID"
//	@Param			body	body		dto.UpdateContractTemplateRequest	true	"Updated template data"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		403		{object}	map[string]interface{}
//	@Failure		404		{object}	map[string]interface{}
//	@Router			/contract-templates/{id} [put]
//	@Security		BearerAuth
func (h *ContractTemplateHandler) UpdateTemplate(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	templateID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid template ID"))
		return
	}

	var req dto.UpdateContractTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", err.Error()))
		return
	}

	tmpl, err := h.svc.UpdateTemplate(c.Request.Context(), ownerID, templateID, req)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Contract template not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this template"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("UPDATE_TEMPLATE_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, successResponse(tmpl))
}

// GenerateContract handles POST /v1/contract-templates/:id/generate.
// Populates the template with tenant/contract data, generates PDF and DOCX files,
// uploads them to the documents bucket, and returns the download URLs.
//
//	@Summary		Generate contract document
//	@Description	Populates a contract template with tenant data, generates PDF and DOCX files, uploads to storage, and returns download URLs.
//	@Tags			contract-templates
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string							true	"Template UUID"
//	@Param			body	body		dto.GenerateContractRequest		true	"Generation parameters (contract_id or tenant_id)"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		403		{object}	map[string]interface{}
//	@Failure		404		{object}	map[string]interface{}
//	@Router			/contract-templates/{id}/generate [post]
//	@Security		BearerAuth
func (h *ContractTemplateHandler) GenerateContract(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	templateID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid template ID"))
		return
	}

	var req dto.GenerateContractRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", err.Error()))
		return
	}

	if req.ContractID == "" && req.TenantID == "" {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", "Either contract_id or tenant_id is required"))
		return
	}

	result, err := h.svc.GenerateContract(c.Request.Context(), ownerID, templateID, req)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Template, contract, or tenant not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this template or contract"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("GENERATE_CONTRACT_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, successResponse(result))
}
