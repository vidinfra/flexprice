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

type ScheduledTaskHandler struct {
	service service.ScheduledTaskService
	logger  *logger.Logger
}

func NewScheduledTaskHandler(
	service service.ScheduledTaskService,
	logger *logger.Logger,
) *ScheduledTaskHandler {
	return &ScheduledTaskHandler{
		service: service,
		logger:  logger,
	}
}

// @Summary Create a scheduled task
// @Description Create a new scheduled task for data export
// @Tags ScheduledTasks
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param scheduled_task body dto.CreateScheduledTaskRequest true "Scheduled Task"
// @Success 201 {object} dto.ScheduledTaskResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /tasks/scheduled [post]
func (h *ScheduledTaskHandler) CreateScheduledTask(c *gin.Context) {
	var req dto.CreateScheduledTaskRequest
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

	resp, err := h.service.CreateScheduledTask(c.Request.Context(), req)
	if err != nil {
		h.logger.Errorw("failed to create scheduled task", "error", err)
		c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, resp)
}

// @Summary Get a scheduled task
// @Description Get a scheduled task by ID
// @Tags ScheduledTasks
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Scheduled Task ID"
// @Success 200 {object} dto.ScheduledTaskResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /tasks/scheduled/{id} [get]
func (h *ScheduledTaskHandler) GetScheduledTask(c *gin.Context) {
	id := c.Param("id")

	resp, err := h.service.GetScheduledTask(c.Request.Context(), id)
	if err != nil {
		h.logger.Errorw("failed to get scheduled task", "id", id, "error", err)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// @Summary List scheduled tasks
// @Description Get a list of scheduled tasks
// @Tags ScheduledTasks
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param limit query int false "Limit"
// @Param offset query int false "Offset"
// @Success 200 {object} dto.ListScheduledTasksResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /tasks/scheduled [get]
func (h *ScheduledTaskHandler) ListScheduledTasks(c *gin.Context) {
	var filter types.QueryFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.Error(ierr.WithError(err).
			WithHint("Invalid filter parameters").
			Mark(ierr.ErrValidation))
		return
	}

	resp, err := h.service.ListScheduledTasks(c.Request.Context(), &filter)
	if err != nil {
		h.logger.Errorw("failed to list scheduled tasks", "error", err)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// @Summary Update a scheduled task
// @Description Update a scheduled task by ID
// @Tags ScheduledTasks
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Scheduled Task ID"
// @Param scheduled_task body dto.UpdateScheduledTaskRequest true "Scheduled Task"
// @Success 200 {object} dto.ScheduledTaskResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /tasks/scheduled/{id} [put]
func (h *ScheduledTaskHandler) UpdateScheduledTask(c *gin.Context) {
	id := c.Param("id")

	var req dto.UpdateScheduledTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Errorw("failed to bind request", "error", err)
		c.Error(ierr.WithError(err).
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	resp, err := h.service.UpdateScheduledTask(c.Request.Context(), id, req)
	if err != nil {
		h.logger.Errorw("failed to update scheduled task", "id", id, "error", err)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// @Summary Delete a scheduled task
// @Description Delete a scheduled task by ID
// @Tags ScheduledTasks
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Scheduled Task ID"
// @Success 204
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /tasks/scheduled/{id} [delete]
func (h *ScheduledTaskHandler) DeleteScheduledTask(c *gin.Context) {
	id := c.Param("id")

	err := h.service.DeleteScheduledTask(c.Request.Context(), id)
	if err != nil {
		h.logger.Errorw("failed to delete scheduled task", "id", id, "error", err)
		c.Error(err)
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary Trigger manual sync
// @Description Trigger a manual export sync immediately for a scheduled task
// @Tags ScheduledTasks
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Scheduled Task ID"
// @Success 200 {object} map[string]string "Returns workflow_id"
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /tasks/scheduled/{id}/sync [post]
func (h *ScheduledTaskHandler) TriggerManualSync(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.Error(ierr.NewError("id is required").
			WithHint("Scheduled task ID must be provided").
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
