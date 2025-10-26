package v1

import (
	"net/http"

	"github.com/flexprice/flexprice/internal/api/dto"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/service"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

// CostsheetV2Handler handles HTTP requests for costsheet v2 operations.
type CostsheetV2Handler struct {
	service service.CostsheetV2Service
	log     *logger.Logger
}

// NewCostsheetV2Handler creates a new instance of CostsheetV2Handler.
func NewCostsheetV2Handler(service service.CostsheetV2Service, log *logger.Logger) *CostsheetV2Handler {
	return &CostsheetV2Handler{
		service: service,
		log:     log,
	}
}

// @Summary Create a new costsheet v2
// @Description Create a new costsheet v2 with the specified name
// @Tags CostsheetV2
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param costsheet body dto.CreateCostsheetV2Request true "Costsheet v2 configuration"
// @Success 201 {object} dto.CreateCostsheetV2Response
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 409 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /costsheets-v2 [post]
func (h *CostsheetV2Handler) CreateCostsheetV2(c *gin.Context) {
	var req dto.CreateCostsheetV2Request
	if err := c.ShouldBindJSON(&req); err != nil {
		h.log.Error("Failed to bind JSON", "error", err)
		c.Error(ierr.WithError(err).
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	resp, err := h.service.CreateCostsheetV2(c.Request.Context(), req)
	if err != nil {
		h.log.Error("Failed to create costsheet v2", "error", err)
		c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, resp)
}

// @Summary Get a costsheet v2 by ID
// @Description Get a costsheet v2 by ID with optional price expansion
// @Tags CostsheetV2
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Costsheet V2 ID"
// @Param expand query string false "Comma-separated list of fields to expand (e.g., 'prices')"
// @Success 200 {object} dto.GetCostsheetV2Response
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /costsheets-v2/{id} [get]
func (h *CostsheetV2Handler) GetCostsheetV2(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.Error(ierr.NewError("id is required").
			WithHint("Costsheet V2 ID is required").
			Mark(ierr.ErrValidation))
		return
	}

	resp, err := h.service.GetCostsheetV2(c.Request.Context(), id)
	if err != nil {
		h.log.Error("Failed to get costsheet v2", "error", err)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// @Summary Update a costsheet v2
// @Description Update a costsheet v2 with the specified configuration
// @Tags CostsheetV2
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Costsheet V2 ID"
// @Param costsheet body dto.UpdateCostsheetV2Request true "Costsheet v2 configuration"
// @Success 200 {object} dto.UpdateCostsheetV2Response
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 409 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /costsheets-v2/{id} [put]
func (h *CostsheetV2Handler) UpdateCostsheetV2(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.Error(ierr.NewError("id is required").
			WithHint("Costsheet V2 ID is required").
			Mark(ierr.ErrValidation))
		return
	}

	var req dto.UpdateCostsheetV2Request
	if err := c.ShouldBindJSON(&req); err != nil {
		h.log.Error("Failed to bind JSON", "error", err)
		c.Error(ierr.WithError(err).
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	resp, err := h.service.UpdateCostsheetV2(c.Request.Context(), id, req)
	if err != nil {
		h.log.Error("Failed to update costsheet v2", "error", err)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// @Summary Delete a costsheet v2
// @Description Soft delete a costsheet v2 by setting its status to deleted
// @Tags CostsheetV2
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Costsheet V2 ID"
// @Success 200 {object} dto.DeleteCostsheetV2Response
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /costsheets-v2/{id} [delete]
func (h *CostsheetV2Handler) DeleteCostsheetV2(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.Error(ierr.NewError("id is required").
			WithHint("Costsheet V2 ID is required").
			Mark(ierr.ErrValidation))
		return
	}

	resp, err := h.service.DeleteCostsheetV2(c.Request.Context(), id)
	if err != nil {
		h.log.Error("Failed to delete costsheet v2", "error", err)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// @Summary List costsheet v2 by filter
// @Description List costsheet v2 records by filter with POST body
// @Tags CostsheetV2
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param filter body types.CostsheetV2Filter true "Filter"
// @Success 200 {object} dto.ListCostsheetV2Response
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /costsheets-v2/search [post]
func (h *CostsheetV2Handler) ListCostsheetV2ByFilter(c *gin.Context) {
	var filter types.CostsheetV2Filter
	if err := c.ShouldBindJSON(&filter); err != nil {
		c.Error(ierr.WithError(err).
			WithHint("Invalid filter parameters").
			Mark(ierr.ErrValidation))
		return
	}

	// Initialize QueryFilter if not set and set default limit if not provided
	if filter.QueryFilter == nil {
		filter.QueryFilter = types.NewDefaultQueryFilter()
	}

	// Set default limit if not provided (following plan pattern)
	if filter.GetLimit() == 0 {
		filter.QueryFilter.Limit = lo.ToPtr(types.GetDefaultFilter().Limit)
	}

	resp, err := h.service.GetCostsheetV2s(c.Request.Context(), &filter)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}
