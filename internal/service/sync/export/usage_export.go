package export

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/domain/events"
	ierr "github.com/flexprice/flexprice/internal/errors"
	s3Integration "github.com/flexprice/flexprice/internal/integration/s3"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
)

// UsageExporter handles feature usage export operations
type UsageExporter struct {
	featureUsageRepo events.FeatureUsageRepository
	s3Client         *s3Integration.Client
	logger           *logger.Logger
}

// NewUsageExporter creates a new usage exporter
func NewUsageExporter(
	featureUsageRepo events.FeatureUsageRepository,
	s3Client *s3Integration.Client,
	logger *logger.Logger,
) *UsageExporter {
	return &UsageExporter{
		featureUsageRepo: featureUsageRepo,
		s3Client:         s3Client,
		logger:           logger,
	}
}

// PrepareData fetches feature usage data in batches and converts it to CSV format
func (e *UsageExporter) PrepareData(ctx context.Context, request *ExportRequest) ([]byte, int, error) {
	const batchSize = 50

	e.logger.Infow("starting batched feature usage data fetch",
		"tenant_id", request.TenantID,
		"env_id", request.EnvID,
		"start_time", request.StartTime,
		"end_time", request.EndTime,
		"batch_size", batchSize)

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Write CSV header once
	header := []string{
		"id",
		"tenant_id",
		"environment_id",
		"external_customer_id",
		"customer_id",
		"subscription_id",
		"sub_line_item_id",
		"price_id",
		"meter_id",
		"feature_id",
		"event_name",
		"source",
		"timestamp",
		"ingested_at",
		"period_id",
		"qty_total",
		"properties",
		"unique_hash",
	}

	if err := writer.Write(header); err != nil {
		return nil, 0, ierr.WithError(err).
			WithHint("Failed to write CSV header").
			Mark(ierr.ErrInternal)
	}

	totalRecords := 0
	offset := 0

	// Fetch and process data in batches
	for {
		e.logger.Debugw("fetching batch",
			"offset", offset,
			"batch_size", batchSize)

		usageData, err := e.featureUsageRepo.GetFeatureUsageForExport(
			ctx,
			request.TenantID,
			request.EnvID,
			request.StartTime,
			request.EndTime,
			batchSize,
			offset,
		)
		if err != nil {
			return nil, 0, ierr.WithError(err).
				WithHint("Failed to fetch feature usage data batch").
				WithReportableDetails(map[string]interface{}{
					"offset":     offset,
					"batch_size": batchSize,
				}).
				Mark(ierr.ErrDatabase)
		}

		// If no data returned, we've reached the end
		if len(usageData) == 0 {
			break
		}

		e.logger.Debugw("fetched batch",
			"offset", offset,
			"records_in_batch", len(usageData),
			"total_so_far", totalRecords+len(usageData))

		// Write batch data to CSV
		for _, usage := range usageData {
			// Convert properties map to JSON string
			propertiesJSON, err := json.Marshal(usage.Properties)
			if err != nil {
				e.logger.Warnw("failed to marshal properties, skipping",
					"usage_id", usage.ID,
					"error", err)
				propertiesJSON = []byte("{}")
			}

			row := []string{
				usage.ID,
				usage.TenantID,
				usage.EnvironmentID,
				usage.ExternalCustomerID,
				usage.CustomerID,
				usage.SubscriptionID,
				usage.SubLineItemID,
				usage.PriceID,
				usage.MeterID,
				usage.FeatureID,
				usage.EventName,
				usage.Source,
				usage.Timestamp.Format(time.RFC3339),
				usage.IngestedAt.Format(time.RFC3339),
				fmt.Sprintf("%d", usage.PeriodID),
				usage.QtyTotal.String(),
				string(propertiesJSON),
				usage.UniqueHash,
			}

			if err := writer.Write(row); err != nil {
				return nil, 0, ierr.WithError(err).
					WithHintf("Failed to write CSV row for usage ID: %s", usage.ID).
					Mark(ierr.ErrInternal)
			}
		}

		totalRecords += len(usageData)
		offset += batchSize

		// If we got fewer records than batch size, we've reached the end
		if len(usageData) < batchSize {
			break
		}
	}

	if totalRecords == 0 {
		e.logger.Warnw("no feature usage data found for export",
			"tenant_id", request.TenantID,
			"env_id", request.EnvID)
		return nil, 0, ierr.NewError("no data found for export").
			WithHint("No feature usage data found for the specified time range").
			Mark(ierr.ErrNotFound)
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, 0, ierr.WithError(err).
			WithHint("Failed to flush CSV writer").
			Mark(ierr.ErrInternal)
	}

	csvBytes := buf.Bytes()

	e.logger.Infow("completed batched data fetch and CSV conversion",
		"total_records", totalRecords,
		"csv_size_bytes", len(csvBytes))

	return csvBytes, totalRecords, nil
}

// Export fetches feature usage data, converts to CSV, and uploads to S3
func (e *UsageExporter) Export(ctx context.Context, request *ExportRequest) (*ExportResponse, error) {
	// Validate job config is provided
	if request.JobConfig == nil {
		return nil, ierr.NewError("job configuration is required").
			WithHint("S3 job configuration must be provided in the export request").
			Mark(ierr.ErrValidation)
	}

	// Step 1: Prepare data (fetch + convert to CSV)
	csvBytes, recordCount, err := e.PrepareData(ctx, request)
	if err != nil {
		return nil, err
	}

	// Step 2: Upload to S3 with job config
	e.logger.Infow("obtaining S3 client for upload",
		"connection_id", request.ConnectionID,
		"bucket", request.JobConfig.Bucket,
		"region", request.JobConfig.Region)

	// Add tenant and environment to context for connection lookup
	ctx = types.SetTenantID(ctx, request.TenantID)
	ctx = types.SetEnvironmentID(ctx, request.EnvID)

	s3Client, _, err := e.s3Client.GetS3Client(ctx, request.JobConfig, request.ConnectionID)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to get S3 client").
			Mark(ierr.ErrHTTPClient)
	}

	// Generate filename with timestamp
	filename := fmt.Sprintf("feature_usage_%s", time.Now().Format("20060102_150405"))

	uploadResponse, err := s3Client.UploadCSV(ctx, filename, csvBytes, "feature_usage")
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to upload CSV to S3").
			Mark(ierr.ErrHTTPClient)
	}

	e.logger.Infow("successfully uploaded to S3",
		"file_url", uploadResponse.FileURL,
		"file_size_bytes", uploadResponse.FileSizeBytes)

	return &ExportResponse{
		EntityType:    string(request.EntityType),
		RecordCount:   recordCount,
		FileURL:       uploadResponse.FileURL,
		FileSizeBytes: uploadResponse.FileSizeBytes,
		ExportedAt:    uploadResponse.UploadedAt,
	}, nil
}
