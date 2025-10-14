package v1

import (
	"net/http"
	"time"

	"github.com/flexprice/flexprice/internal/domain/scheduledjob"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/service/sync/export"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/gin-gonic/gin"
)

type TestUsageExportHandler struct {
	exportService    *export.ExportService
	scheduledJobRepo scheduledjob.Repository
	logger           *logger.Logger
}

func NewTestUsageExportHandler(
	exportService *export.ExportService,
	scheduledJobRepo scheduledjob.Repository,
	logger *logger.Logger,
) *TestUsageExportHandler {
	return &TestUsageExportHandler{
		exportService:    exportService,
		scheduledJobRepo: scheduledJobRepo,
		logger:           logger,
	}
}

// TestUsageExport godoc
// @Summary Test feature usage export
// @Description Export feature usage data from last 24 hours to S3
// @Tags test
// @Success 200 {object} map[string]interface{}
// @Router /test/export-usage [post]
func (h *TestUsageExportHandler) TestUsageExport(c *gin.Context) {
	ctx := c.Request.Context()

	// Get tenant and environment from headers (or context)
	var tenantID, envID string

	// Try to get from context first (set by auth middleware)
	if tid, exists := c.Get("tenant_id"); exists && tid != nil {
		tenantID = tid.(string)
	} else {
		// Fallback to reading from headers directly
		tenantID = c.GetHeader("X-Tenant-Id")
	}

	if eid, exists := c.Get("environment_id"); exists && eid != nil {
		envID = eid.(string)
	} else {
		// Fallback to reading from headers directly
		envID = c.GetHeader("X-Environment-Id")
	}

	// Validate we have both
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "X-Tenant-Id header is required",
		})
		return
	}

	if envID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "X-Environment-Id header is required",
		})
		return
	}

	h.logger.Infow("test usage export requested",
		"tenant_id", tenantID,
		"environment_id", envID)

	// Get scheduled job for events entity type
	// This will fetch the S3 configuration from the scheduled_jobs table
	scheduledJobs, err := h.scheduledJobRepo.GetByEntityType(ctx, string(types.ScheduledJobEntityTypeEvents))
	if err != nil {
		h.logger.Errorw("failed to get scheduled job for events", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "No scheduled job configured for events export",
			"details": err.Error(),
		})
		return
	}

	if len(scheduledJobs) == 0 {
		h.logger.Warnw("no scheduled job found for events export")
		c.JSON(http.StatusNotFound, gin.H{
			"error": "No scheduled job configured for events export. Please create a scheduled job first.",
		})
		return
	}

	// Use the first enabled job
	var jobConfig *types.S3JobConfig
	var configErr error
	for _, job := range scheduledJobs {
		if job.Enabled {
			jobConfig, configErr = job.GetS3JobConfig()
			if configErr != nil {
				h.logger.Errorw("failed to get job config", "job_id", job.ID, "error", configErr)
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":   "Invalid job configuration",
					"details": configErr.Error(),
				})
				return
			}
			break
		}
	}

	if jobConfig == nil {
		h.logger.Warnw("no enabled scheduled job found for feature usage export")
		c.JSON(http.StatusNotFound, gin.H{
			"error": "No enabled scheduled job found for feature usage export. Please enable a scheduled job.",
		})
		return
	}

	// Export last 24 hours of data
	endTime := time.Now()
	startTime := endTime.Add(-24 * time.Hour)

	request := &export.ExportRequest{
		EntityType: types.ExportEntityTypeEvents,
		TenantID:   tenantID,
		EnvID:      envID,
		StartTime:  startTime,
		EndTime:    endTime,
		JobConfig:  jobConfig,
	}

	// Call export service
	response, err := h.exportService.Export(ctx, request)
	if err != nil {
		h.logger.Errorw("failed to export feature usage", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to export feature usage",
			"details": err.Error(),
		})
		return
	}

	h.logger.Infow("successfully exported feature usage",
		"file_url", response.FileURL,
		"record_count", response.RecordCount)

	// Success!
	c.JSON(http.StatusOK, gin.H{
		"success":         true,
		"message":         "Feature usage exported successfully!",
		"entity_type":     response.EntityType,
		"record_count":    response.RecordCount,
		"file_url":        response.FileURL,
		"file_size_bytes": response.FileSizeBytes,
		"exported_at":     response.ExportedAt,
		"time_range": gin.H{
			"start": startTime.Format(time.RFC3339),
			"end":   endTime.Format(time.RFC3339),
		},
	})
}
