# S3 Integration

This package provides S3 integration for exporting data from FlexPrice to AWS S3 or S3-compatible services.

## Overview

The S3 integration allows FlexPrice to export various entity types (feature usage, invoices, customers, etc.) to S3 buckets in different formats (CSV, JSON, Parquet).

## Components

### 1. `config.go`
Defines the S3 configuration structure and provides a helper function to create a config from connection metadata.

**Key Features:**
- Bucket and region configuration
- AWS credentials (encrypted)
- Optional key prefix for organizing exports
- Compression support (gzip, none)
- Encryption support (AES256, aws:kms)
- Custom endpoint URL for S3-compatible services
- Max file size limits

### 2. `client.go`
Creates and manages the S3 client connection.

**Key Features:**
- Creates AWS S3 client from configuration
- Supports custom endpoints for S3-compatible services
- Validates connection by checking bucket access
- Handles credential loading and authentication

### 3. `export.go`
Handles data export operations to S3.

**Key Features:**
- Upload files in various formats (CSV, JSON, Parquet)
- Automatic compression (gzip) if enabled
- Server-side encryption
- Organized file structure: `{prefix}/{entity_type}/{year}/{month}/{day}/{filename}.{ext}`
- File size validation
- Returns upload metadata (URL, size, timestamps)

## Usage

### Creating a Connection

To use the S3 integration, first create a connection with S3 credentials:

```json
POST /api/v1/connections
{
  "name": "My S3 Export Connection",
  "provider_type": "s3",
  "encrypted_secret_data": {
    "aws_access_key_id": "AKIA...",
    "aws_secret_access_key": "secret...",
    "aws_session_token": "SESSION_TOKEN..."
  },
  "sync_config": {
    "feature_usage": {
      "outbound": true
    },
    "customer": {
      "outbound": true
    },
    "s3": {
      "bucket": "my-export-bucket",
      "region": "us-west-2",
      "key_prefix": "flexprice-exports",
      "compression": "gzip",
      "encryption": "AES256",
      "max_file_size_mb": 100,
      "interval": "daily",
      "entity_types": ["feature_usage", "customer"],
      "sync_active": true
    }
  },
  "status": "published"
}
```

### Using the Client

```go
import (
    "context"
    s3Integration "github.com/flexprice/flexprice/internal/integration/s3"
    "github.com/flexprice/flexprice/internal/types"
)

// Assuming you have connection data loaded from the database
// connectionMetadata.S3 contains the decrypted AWS credentials
// syncConfig.S3 contains the bucket and export settings

// Create config from connection metadata and sync config
config := s3Integration.NewConfigFromConnection(
    connectionMetadata.S3,      // Contains aws_access_key_id and aws_secret_access_key
    syncConfig.S3,              // Contains bucket, region, and other settings
)

// Create S3 client
client, err := s3Integration.NewClient(ctx, config, logger)
if err != nil {
    // Handle error
}

// Validate connection
if err := client.ValidateConnection(ctx); err != nil {
    // Handle error
}

// Upload CSV data
csvData := []byte("id,name,value\n1,test,100\n")
response, err := client.UploadCSV(ctx, "feature_usage_export", csvData, "feature_usage")
if err != nil {
    // Handle error
}

fmt.Printf("Uploaded to: %s\n", response.FileURL)
fmt.Printf("File size: %d bytes\n", response.FileSizeBytes)
```

## Configuration Options

### Encrypted Secret Data (in `encrypted_secret_data`)
These are sensitive credentials that are encrypted in the database:
- `aws_access_key_id`: AWS access key ID (required, encrypted)
- `aws_secret_access_key`: AWS secret access key (required, encrypted)
- `aws_session_token`: AWS session token for temporary credentials (optional, encrypted)

### Sync Config (in `sync_config.s3`)
These are non-sensitive configuration settings:
- `bucket`: S3 bucket name (required)
- `region`: AWS region (required, e.g., "us-west-2")
- `key_prefix`: Prefix for all S3 keys (optional, default: "")
- `compression`: Compression type - "gzip" or "none" (optional, default: "gzip")
- `encryption`: Encryption type - "AES256" or "aws:kms" (optional, default: "AES256")
- `endpoint_url`: Custom endpoint URL for S3-compatible services (optional)
- `virtual_host_style`: Use virtual-hosted-style URLs (optional, default: false)
- `max_file_size_mb`: Maximum file size in MB (optional, default: 100)
- `interval`: Sync interval - "daily", "weekly", or "monthly" (optional, default: "daily")
- `entity_types`: Array of entity types to export (optional, e.g., ["feature_usage", "customer", "invoice"])
- `sync_active`: Whether the sync is active (required, boolean, default: false)

## File Organization

Files are automatically organized in S3 with the following structure:

```
{key_prefix}/{entity_type}/{year}/{month}/{day}/{filename}.{extension}
```

Example:
```
flexprice-exports/feature_usage/2025/10/11/export_20251011_143022.csv.gz
```

## Supported Formats

- **CSV**: Comma-separated values (`.csv`)
- **JSON**: JSON format (`.json`)
- **Parquet**: Apache Parquet format (`.parquet`)

Files can be optionally compressed with gzip (adds `.gz` extension).

## Security

### Credentials Storage
- AWS credentials are encrypted in the database using AES-GCM encryption
- Encryption service handles encryption/decryption transparently
- Credentials are only decrypted when creating S3 client

### Data Encryption
- Server-side encryption is applied to all uploaded files
- Supports AES256 (AWS-managed keys) and aws:kms (customer-managed keys)

### Access Control
- Only authenticated users with proper permissions can create connections
- Connection credentials are tenant-scoped
- Environment-scoped for multi-environment setups

## Error Handling

The integration uses FlexPrice's standard error handling:

```go
if err != nil {
    // Errors are wrapped with context
    // Examples:
    // - ErrValidation: Invalid configuration or request
    // - ErrHTTPClient: S3 API errors
    // - ErrSystem: Internal errors (compression, etc.)
}
```

## Future Enhancements

- Support for Parquet format
- Incremental exports with change tracking
- Export scheduling and automation
- Multi-part upload for large files
- Pre-signed URL generation for downloads
- Export to other providers (Athena, BigQuery, Snowflake)