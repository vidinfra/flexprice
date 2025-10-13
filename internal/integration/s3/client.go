package s3

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/flexprice/flexprice/internal/domain/connection"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/security"
	"github.com/flexprice/flexprice/internal/types"
)

// Client represents an S3 integration client
type Client struct {
	connectionRepo    connection.Repository
	encryptionService security.EncryptionService
	logger            *logger.Logger
}

// NewClient creates a new S3 client
func NewClient(
	connectionRepo connection.Repository,
	encryptionService security.EncryptionService,
	logger *logger.Logger,
) *Client {
	return &Client{
		connectionRepo:    connectionRepo,
		encryptionService: encryptionService,
		logger:            logger,
	}
}

// S3Config holds decrypted S3 configuration
type S3Config struct {
	AWSAccessKeyID     string
	AWSSecretAccessKey string
	AWSSessionToken    string // Optional: for temporary credentials
	Bucket             string
	Region             string
	KeyPrefix          string
	Compression        string
	Encryption         string
	EndpointURL        string
	VirtualHostStyle   bool
	MaxFileSizeMB      int
	Interval           types.SyncInterval
	EntityTypes        []types.ExportEntityType
}

// GetS3Client returns a configured S3 client with the provided job config
func (c *Client) GetS3Client(ctx context.Context, jobConfig *types.S3JobConfig) (*s3Client, *S3Config, error) {
	// Get S3 connection for this environment
	conn, err := c.connectionRepo.GetByProvider(ctx, types.SecretProviderS3)
	if err != nil {
		return nil, nil, ierr.NewError("failed to get S3 connection").
			WithHint("S3 connection not configured for this environment").
			Mark(ierr.ErrNotFound)
	}

	s3Config, err := c.GetDecryptedS3Config(conn, jobConfig)
	if err != nil {
		return nil, nil, ierr.NewError("failed to get S3 configuration").
			WithHint("Invalid S3 configuration").
			Mark(ierr.ErrValidation)
	}

	// Create AWS config with explicit credentials (including session token for temporary credentials)
	awsCfg, err := awsConfig.LoadDefaultConfig(ctx,
		awsConfig.WithRegion(s3Config.Region),
		awsConfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			s3Config.AWSAccessKeyID,
			s3Config.AWSSecretAccessKey,
			s3Config.AWSSessionToken, // session token for temporary credentials (ASIA keys)
		)),
	)
	if err != nil {
		return nil, nil, ierr.WithError(err).
			WithHint("failed to load AWS config").
			Mark(ierr.ErrHTTPClient)
	}

	// Create S3 client options
	s3Options := []func(*s3.Options){
		func(o *s3.Options) {
			o.Region = s3Config.Region
		},
	}

	// Add custom endpoint if provided (for S3-compatible services)
	if s3Config.EndpointURL != "" {
		s3Options = append(s3Options, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(s3Config.EndpointURL)
			o.UsePathStyle = !s3Config.VirtualHostStyle
		})
	}

	// Create S3 client
	awsS3Client := s3.NewFromConfig(awsCfg, s3Options...)

	c.logger.Infow("S3 client created successfully",
		"bucket", s3Config.Bucket,
		"region", s3Config.Region,
		"key_prefix", s3Config.KeyPrefix,
	)

	return &s3Client{
		s3Client: awsS3Client,
		config:   s3Config,
		logger:   c.logger,
	}, s3Config, nil
}

// GetDecryptedS3Config decrypts credentials and combines with job configuration
func (c *Client) GetDecryptedS3Config(conn *connection.Connection, jobConfig *types.S3JobConfig) (*S3Config, error) {
	// Check if we have S3 credentials
	if conn.EncryptedSecretData.S3 == nil {
		return nil, ierr.NewError("no S3 credentials found").
			WithHint("S3 credentials not configured").
			Mark(ierr.ErrValidation)
	}

	// Decrypt credentials
	c.logger.Infow("Decrypting S3 credentials",
		"connection_id", conn.ID,
		"encrypted_access_key_length", len(conn.EncryptedSecretData.S3.AWSAccessKeyID),
		"encrypted_secret_key_length", len(conn.EncryptedSecretData.S3.AWSSecretAccessKey),
	)

	accessKey, err := c.encryptionService.Decrypt(conn.EncryptedSecretData.S3.AWSAccessKeyID)
	if err != nil {
		c.logger.Errorw("failed to decrypt AWS access key", "connection_id", conn.ID, "error", err)
		return nil, ierr.NewError("failed to decrypt AWS access key").Mark(ierr.ErrInternal)
	}

	secretKey, err := c.encryptionService.Decrypt(conn.EncryptedSecretData.S3.AWSSecretAccessKey)
	if err != nil {
		c.logger.Errorw("failed to decrypt AWS secret key", "connection_id", conn.ID, "error", err)
		return nil, ierr.NewError("failed to decrypt AWS secret key").Mark(ierr.ErrInternal)
	}

	// Decrypt session token if present (for temporary credentials)
	var sessionToken string
	if conn.EncryptedSecretData.S3.AWSSessionToken != "" {
		sessionToken, err = c.encryptionService.Decrypt(conn.EncryptedSecretData.S3.AWSSessionToken)
		if err != nil {
			c.logger.Errorw("failed to decrypt AWS session token", "connection_id", conn.ID, "error", err)
			return nil, ierr.NewError("failed to decrypt AWS session token").Mark(ierr.ErrInternal)
		}
	}

	c.logger.Infow("Decrypted S3 credentials",
		"connection_id", conn.ID,
		"decrypted_access_key_length", len(accessKey),
		"decrypted_secret_key_length", len(secretKey),
		"has_session_token", sessionToken != "",
		"access_key_starts_with", accessKey[:min(4, len(accessKey))],
	)

	// Validate job config is provided
	if jobConfig == nil {
		return nil, ierr.NewError("no job configuration provided").
			WithHint("S3 job configuration is required").
			Mark(ierr.ErrValidation)
	}

	// Combine decrypted credentials with job config
	s3Config := &S3Config{
		AWSAccessKeyID:     accessKey,
		AWSSecretAccessKey: secretKey,
		AWSSessionToken:    sessionToken,
		Bucket:             jobConfig.Bucket,
		Region:             jobConfig.Region,
		KeyPrefix:          jobConfig.KeyPrefix,
		Compression:        jobConfig.Compression,
		Encryption:         jobConfig.Encryption,
		EndpointURL:        jobConfig.EndpointURL,
		VirtualHostStyle:   jobConfig.VirtualHostStyle,
		MaxFileSizeMB:      jobConfig.MaxFileSizeMB,
	}

	c.logger.Infow("successfully created S3 configuration",
		"connection_id", conn.ID,
		"bucket", s3Config.Bucket,
		"region", s3Config.Region,
	)

	return s3Config, nil
}

// s3Client is the actual S3 client with AWS SDK
type s3Client struct {
	s3Client *s3.Client
	config   *S3Config
	logger   *logger.Logger
}

// GetAWSS3Client returns the underlying AWS S3 client
func (c *s3Client) GetAWSS3Client() *s3.Client {
	return c.s3Client
}

// ValidateConnection validates the S3 connection by checking bucket access
func (c *s3Client) ValidateConnection(ctx context.Context) error {
	// Try to head bucket to validate connection
	_, err := c.s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(c.config.Bucket),
	})
	if err != nil {
		return ierr.WithError(err).
			WithHint("failed to validate S3 connection - check credentials and bucket name").
			WithMessagef("bucket: %s, region: %s", c.config.Bucket, c.config.Region).
			Mark(ierr.ErrHTTPClient)
	}

	c.logger.Infow("S3 connection validated successfully",
		"bucket", c.config.Bucket,
		"region", c.config.Region,
	)

	return nil
}

// HasS3Connection checks if the tenant has an S3 connection available
func (c *Client) HasS3Connection(ctx context.Context) bool {
	conn, err := c.connectionRepo.GetByProvider(ctx, types.SecretProviderS3)
	return err == nil && conn != nil && conn.Status == types.StatusPublished
}
