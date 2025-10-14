package export

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/domain/events"
	ierr "github.com/flexprice/flexprice/internal/errors"
	s3Integration "github.com/flexprice/flexprice/internal/integration/s3"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
)

// Exporter defines the interface for entity-specific exporters
type Exporter interface {
	// PrepareData fetches and prepares data for export
	PrepareData(ctx context.Context, request *ExportRequest) ([]byte, int, error)

	// Export performs the complete export operation (fetch + upload)
	Export(ctx context.Context, request *ExportRequest) (*ExportResponse, error)
}

// ExportService handles export operations for different entity types
type ExportService struct {
	featureUsageRepo events.FeatureUsageRepository
	s3Client         *s3Integration.Client
	logger           *logger.Logger
}

// NewExportService creates a new export service
func NewExportService(
	featureUsageRepo events.FeatureUsageRepository,
	s3Client *s3Integration.Client,
	logger *logger.Logger,
) *ExportService {
	return &ExportService{
		featureUsageRepo: featureUsageRepo,
		s3Client:         s3Client,
		logger:           logger,
	}
}

// ExportRequest represents an export request
type ExportRequest struct {
	EntityType   types.ExportEntityType
	ConnectionID string // Connection ID for S3 credentials
	TenantID     string
	EnvID        string
	StartTime    time.Time
	EndTime      time.Time
	JobConfig    *types.S3JobConfig // S3 job configuration from scheduled_jobs
}

// ExportResponse represents the result of an export operation
type ExportResponse struct {
	EntityType    string
	RecordCount   int
	FileURL       string
	FileSizeBytes int64
	ExportedAt    time.Time
}

// Export routes the export request to the appropriate entity exporter
func (s *ExportService) Export(ctx context.Context, request *ExportRequest) (*ExportResponse, error) {
	s.logger.Infow("starting export",
		"entity_type", request.EntityType,
		"tenant_id", request.TenantID,
		"env_id", request.EnvID,
		"start_time", request.StartTime,
		"end_time", request.EndTime)

	// Get the appropriate exporter for the entity type
	exporter := s.getExporter(request.EntityType)
	if exporter == nil {
		return nil, ierr.NewError("unknown entity type").
			WithHintf("entity type '%s' is not supported", request.EntityType).
			Mark(ierr.ErrValidation)
	}

	// Delegate to the entity-specific exporter
	return exporter.Export(ctx, request)
}

// getExporter returns the appropriate exporter for the given entity type
func (s *ExportService) getExporter(entityType types.ExportEntityType) Exporter {
	switch entityType {
	case types.ExportEntityTypeEvents:
		return NewUsageExporter(s.featureUsageRepo, s.s3Client, s.logger)
	case types.ExportEntityTypeCustomer:
		// return NewCustomerExporter(...) // TODO: Implement
		return nil
	case types.ExportEntityTypeInvoice:
		// return NewInvoiceExporter(...) // TODO: Implement
		return nil
	case types.ExportEntityTypeSubscription:
		// return NewSubscriptionExporter(...) // TODO: Implement
		return nil
	case types.ExportEntityTypePrice:
		// return NewPriceExporter(...) // TODO: Implement
		return nil
	case types.ExportEntityTypeCreditNote:
		// return NewCreditNoteExporter(...) // TODO: Implement
		return nil
	default:
		return nil
	}
}
