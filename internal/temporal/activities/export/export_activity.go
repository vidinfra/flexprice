package export

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/domain/events"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/integration"
	"github.com/flexprice/flexprice/internal/logger"
	syncExport "github.com/flexprice/flexprice/internal/service/sync/export"
	"github.com/flexprice/flexprice/internal/types"
)

// ExportActivity handles the actual export operations
type ExportActivity struct {
	featureUsageRepo   events.FeatureUsageRepository
	integrationFactory *integration.Factory
	logger             *logger.Logger
}

// NewExportActivity creates a new export activity
func NewExportActivity(
	featureUsageRepo events.FeatureUsageRepository,
	integrationFactory *integration.Factory,
	logger *logger.Logger,
) *ExportActivity {
	return &ExportActivity{
		featureUsageRepo:   featureUsageRepo,
		integrationFactory: integrationFactory,
		logger:             logger,
	}
}

// ExportDataInput represents input for exporting data
type ExportDataInput struct {
	EntityType   types.ExportEntityType
	ConnectionID string
	TenantID     string
	EnvID        string
	StartTime    time.Time
	EndTime      time.Time
	JobConfig    *types.S3JobConfig
}

// ExportDataOutput represents output from export
type ExportDataOutput struct {
	FileURL       string
	RecordCount   int
	FileSizeBytes int64
}

// ExportData performs the complete export: prepare data, generate CSV, upload to S3
func (a *ExportActivity) ExportData(ctx context.Context, input ExportDataInput) (*ExportDataOutput, error) {
	a.logger.Infow("starting data export",
		"entity_type", input.EntityType,
		"tenant_id", input.TenantID,
		"env_id", input.EnvID,
		"start_time", input.StartTime,
		"end_time", input.EndTime)

	// Add tenant and environment to context for repository queries
	ctx = types.SetTenantID(ctx, input.TenantID)
	ctx = types.SetEnvironmentID(ctx, input.EnvID)

	// Create export request
	request := &syncExport.ExportRequest{
		EntityType:   input.EntityType,
		ConnectionID: input.ConnectionID,
		TenantID:     input.TenantID,
		EnvID:        input.EnvID,
		StartTime:    input.StartTime,
		EndTime:      input.EndTime,
		JobConfig:    input.JobConfig,
	}

	// Get the appropriate exporter based on entity type
	var response *syncExport.ExportResponse
	var err error

	switch input.EntityType {
	case types.ExportEntityTypeEvents:
		exporter := syncExport.NewUsageExporter(a.featureUsageRepo, a.integrationFactory, a.logger)
		response, err = exporter.Export(ctx, request)
	// Add more entity types as needed
	// case types.ExportEntityTypeCustomer:
	default:
		return nil, ierr.NewError("unsupported entity type").
			WithHintf("Entity type '%s' is not supported for export", input.EntityType).
			Mark(ierr.ErrValidation)
	}

	if err != nil {
		a.logger.Errorw("export failed", "error", err, "entity_type", input.EntityType)
		return nil, err
	}

	a.logger.Infow("export completed successfully",
		"entity_type", input.EntityType,
		"file_url", response.FileURL,
		"record_count", response.RecordCount,
		"file_size", response.FileSizeBytes)

	return &ExportDataOutput{
		FileURL:       response.FileURL,
		RecordCount:   response.RecordCount,
		FileSizeBytes: response.FileSizeBytes,
	}, nil
}
