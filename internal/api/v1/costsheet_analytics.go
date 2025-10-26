package v1

import (
	"net/http"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/config"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/interfaces"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/gin-gonic/gin"
)

type CostsheetAnalyticsHandler struct {
	costsheetAnalyticsService interfaces.CostsheetAnalyticsService
	config                    *config.Configuration
	Logger                    *logger.Logger
}

func NewCostsheetAnalyticsHandler(
	costsheetAnalyticsService interfaces.CostsheetAnalyticsService,
	config *config.Configuration,
	logger *logger.Logger,
) *CostsheetAnalyticsHandler {
	return &CostsheetAnalyticsHandler{
		costsheetAnalyticsService: costsheetAnalyticsService,
		config:                    config,
		Logger:                    logger,
	}
}

// GetCostAnalytics retrieves cost analytics for customers and costsheets
// @Summary Get cost analytics
// @Description Retrieve cost analytics with breakdown by meter, customer, and time. If start_time and end_time are not provided, defaults to last 7 days.
// @Tags Cost Analytics
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body dto.GetCostAnalyticsRequest true "Cost analytics request (start_time/end_time optional - defaults to last 7 days)"
// @Success 200 {object} dto.GetCostAnalyticsResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /analytics/cost [post]
func (h *CostsheetAnalyticsHandler) GetCostAnalytics(c *gin.Context) {
	ctx := c.Request.Context()

	var req dto.GetCostAnalyticsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ierr.WithError(err).
			WithHint("Please check the request payload").
			Mark(ierr.ErrValidation))
		return
	}

	response, err := h.costsheetAnalyticsService.GetCostAnalytics(ctx, &req)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetCombinedAnalytics retrieves combined cost and revenue analytics with derived metrics
// @Summary Get combined revenue and cost analytics
// @Description Retrieve combined analytics with ROI, margin, and detailed breakdowns. If start_time and end_time are not provided, defaults to last 7 days.
// @Tags Analytics
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body dto.GetCombinedAnalyticsRequest true "Combined analytics request (start_time/end_time optional - defaults to last 7 days)"
// @Success 200 {object} dto.GetCombinedAnalyticsResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /analytics/combined [post]
func (h *CostsheetAnalyticsHandler) GetCombinedAnalytics(c *gin.Context) {
	ctx := c.Request.Context()

	var req dto.GetCombinedAnalyticsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ierr.WithError(err).
			WithHint("Please check the request payload").
			Mark(ierr.ErrValidation))
		return
	}

	response, err := h.costsheetAnalyticsService.GetCombinedAnalytics(ctx, &req)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, response)
}
