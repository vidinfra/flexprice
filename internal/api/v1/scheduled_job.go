package v1

import (
	"net/http"

	"github.com/flexprice/flexprice/internal/api/dto"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/service"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/gin-gonic/gin"
)

type ScheduledJobHandler struct {
	service service.ScheduledJobService
	logger  *logger.Logger
}

func NewScheduledJobHandler(
	service service.ScheduledJobService,
	logger *logger.Logger,
) *ScheduledJobHandler {
	return &ScheduledJobHandler{
		service: service,
		logger:  logger,
	}
}

// @Summary Create a scheduled job
// @Description Create a new scheduled job for data export
// @Tags ScheduledJobs
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param scheduled_job body dto.CreateScheduledJobRequest true "Scheduled Job"
// @Success 201 {object} dto.ScheduledJobResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /scheduled-jobs [post]
func (h *ScheduledJobHandler) CreateScheduledJob(c *gin.Context) {
	var req dto.CreateScheduledJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Errorw("failed to bind request", "error", err)
		c.Error(ierr.WithError(err).
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	// Default enabled to true if not provided
	if !req.Enabled {
		req.Enabled = true
	}

	resp, err := h.service.CreateScheduledJob(c.Request.Context(), req)
	if err != nil {
		h.logger.Errorw("failed to create scheduled job", "error", err)
		c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, resp)
}

// @Summary Get a scheduled job
// @Description Get a scheduled job by ID
// @Tags ScheduledJobs
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Scheduled Job ID"
// @Success 200 {object} dto.ScheduledJobResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /scheduled-jobs/{id} [get]
func (h *ScheduledJobHandler) GetScheduledJob(c *gin.Context) {
	id := c.Param("id")

	resp, err := h.service.GetScheduledJob(c.Request.Context(), id)
	if err != nil {
		h.logger.Errorw("failed to get scheduled job", "id", id, "error", err)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// @Summary List scheduled jobs
// @Description Get a list of scheduled jobs
// @Tags ScheduledJobs
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param limit query int false "Limit"
// @Param offset query int false "Offset"
// @Success 200 {object} dto.ListScheduledJobsResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /scheduled-jobs [get]
func (h *ScheduledJobHandler) ListScheduledJobs(c *gin.Context) {
	var filter types.QueryFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.Error(ierr.WithError(err).
			WithHint("Invalid filter parameters").
			Mark(ierr.ErrValidation))
		return
	}

	resp, err := h.service.ListScheduledJobs(c.Request.Context(), &filter)
	if err != nil {
		h.logger.Errorw("failed to list scheduled jobs", "error", err)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// @Summary Update a scheduled job
// @Description Update a scheduled job by ID
// @Tags ScheduledJobs
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Scheduled Job ID"
// @Param scheduled_job body dto.UpdateScheduledJobRequest true "Scheduled Job"
// @Success 200 {object} dto.ScheduledJobResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /scheduled-jobs/{id} [put]
func (h *ScheduledJobHandler) UpdateScheduledJob(c *gin.Context) {
	id := c.Param("id")

	var req dto.UpdateScheduledJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Errorw("failed to bind request", "error", err)
		c.Error(ierr.WithError(err).
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	resp, err := h.service.UpdateScheduledJob(c.Request.Context(), id, req)
	if err != nil {
		h.logger.Errorw("failed to update scheduled job", "id", id, "error", err)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// @Summary Delete a scheduled job
// @Description Delete a scheduled job by ID
// @Tags ScheduledJobs
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Scheduled Job ID"
// @Success 204
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /scheduled-jobs/{id} [delete]
func (h *ScheduledJobHandler) DeleteScheduledJob(c *gin.Context) {
	id := c.Param("id")

	err := h.service.DeleteScheduledJob(c.Request.Context(), id)
	if err != nil {
		h.logger.Errorw("failed to delete scheduled job", "id", id, "error", err)
		c.Error(err)
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary Trigger manual sync
// @Description Trigger a manual export sync immediately for a scheduled job
// @Tags ScheduledJobs
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Scheduled Job ID"
// @Success 200 {object} map[string]string "Returns workflow_id"
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /scheduled-jobs/{id}/sync [post]
func (h *ScheduledJobHandler) TriggerManualSync(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.Error(ierr.NewError("id is required").
			WithHint("Scheduled job ID must be provided").
			Mark(ierr.ErrValidation))
		return
	}

	workflowID, err := h.service.TriggerManualSync(c.Request.Context(), id)
	if err != nil {
		h.logger.Errorw("failed to trigger manual sync", "id", id, "error", err)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"workflow_id": workflowID,
		"message":     "Manual sync triggered successfully",
	})
}
