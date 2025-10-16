package v1

import (
	"net/http"

	"github.com/flexprice/flexprice/internal/integration"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/gin-gonic/gin"
)

type TestExportHandler struct {
	integrationFactory *integration.Factory
	logger             *logger.Logger
}

func NewTestExportHandler(integrationFactory *integration.Factory, logger *logger.Logger) *TestExportHandler {
	return &TestExportHandler{
		integrationFactory: integrationFactory,
		logger:             logger,
	}
}

// TestExport godoc
// @Summary Test S3 export
// @Description Upload a test CSV to S3
// @Tags test
// @Success 200 {object} map[string]interface{}
// @Router /test/export [post]
func (h *TestExportHandler) TestExport(c *gin.Context) {
	ctx := c.Request.Context()

	// Create test S3 job config (using default values from connection)
	testJobConfig := &types.S3JobConfig{
		Bucket:        "flexprice-dev-testing", // Default test bucket
		Region:        "ap-south-1",            // Default region
		KeyPrefix:     "test-exports",
		Compression:   "gzip",
		Encryption:    "AES256",
		MaxFileSizeMB: 100,
	}

	// 1. Get S3 client with decrypted credentials and test config
	s3IntegrationClient, err := h.integrationFactory.GetS3Client(ctx)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to get S3 integration client", "details": err.Error()})
		return
	}

	s3Client, s3Config, err := s3IntegrationClient.GetS3Client(ctx, testJobConfig)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to get S3 client", "details": err.Error()})
		return
	}

	h.logger.Infow("Got S3 client with decrypted credentials",
		"bucket", s3Config.Bucket,
		"region", s3Config.Region,
	)

	// 2. Test CSV data
	testCSV := `id,customer_name,amount,date
1,John Doe,100.50,2025-01-15
2,Jane Smith,250.75,2025-01-16
3,Bob Johnson,175.25,2025-01-17`

	// 3. Upload to S3
	h.logger.Infow("Uploading CSV to S3", "file_name", "test_export", "data_size", len(testCSV))
	response, err := s3Client.UploadCSV(ctx, "test_export", []byte(testCSV), "test_data")
	if err != nil {
		h.logger.Errorw("Failed to upload to S3", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload to S3", "details": err.Error()})
		return
	}

	h.logger.Infow("Successfully uploaded to S3", "file_url", response.FileURL)

	// 4. Success!
	c.JSON(http.StatusOK, gin.H{
		"success":         true,
		"message":         "CSV uploaded to S3 successfully!",
		"file_url":        response.FileURL,
		"bucket":          response.Bucket,
		"key":             response.Key,
		"file_size_bytes": response.FileSizeBytes,
		"compressed_size": response.CompressedSize,
		"uploaded_at":     response.UploadedAt,
	})
}
