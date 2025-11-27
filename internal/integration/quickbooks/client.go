package quickbooks

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/flexprice/flexprice/internal/domain/connection"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/security"
	"github.com/flexprice/flexprice/internal/types"
)

// QuickBooksClient defines the interface for QuickBooks API operations
type QuickBooksClient interface {
	// Configuration and initialization
	GetQuickBooksConfig(ctx context.Context) (*QuickBooksConfig, error)
	GetDecryptedQuickBooksConfig(conn *connection.Connection) (*QuickBooksConfig, error)
	HasQuickBooksConnection(ctx context.Context) bool
	GetConnection(ctx context.Context) (*connection.Connection, error)

	// Customer API wrappers
	CreateCustomer(ctx context.Context, req *CustomerCreateRequest) (*CustomerResponse, error)
	QueryCustomerByEmail(ctx context.Context, email string) (*CustomerResponse, error)
	QueryCustomerByName(ctx context.Context, name string) (*CustomerResponse, error)

	// Item API wrappers
	CreateItem(ctx context.Context, req *ItemCreateRequest) (*ItemResponse, error)
	QueryItemByName(ctx context.Context, name string) (*ItemResponse, error)

	// Invoice API wrappers
	CreateInvoice(ctx context.Context, req *InvoiceCreateRequest) (*InvoiceResponse, error)

	// Token management
	RefreshAccessToken(ctx context.Context) error
}

// Client handles QuickBooks API client setup and configuration
type Client struct {
	connectionRepo    connection.Repository
	encryptionService security.EncryptionService
	logger            *logger.Logger
	httpClient        *http.Client
	minorVersion      string
}

// QuickBooksConfig holds decrypted QuickBooks configuration
type QuickBooksConfig struct {
	ClientID       string
	ClientSecret   string
	AccessToken    string
	RefreshToken   string
	RealmID        string
	Environment    string // "sandbox" or "production"
	TokenExpiresAt int64
}

// NewClient creates a new QuickBooks client
func NewClient(
	connectionRepo connection.Repository,
	encryptionService security.EncryptionService,
	logger *logger.Logger,
) QuickBooksClient {
	return &Client{
		connectionRepo:    connectionRepo,
		encryptionService: encryptionService,
		logger:            logger,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		minorVersion: "70",
	}
}

// GetQuickBooksConfig retrieves and decrypts QuickBooks configuration for the current environment.
// Validates that required fields (RealmID and AccessToken) are present.
// Returns decrypted configuration ready for API calls.
func (c *Client) GetQuickBooksConfig(ctx context.Context) (*QuickBooksConfig, error) {
	conn, err := c.connectionRepo.GetByProvider(ctx, types.SecretProviderQuickBooks)
	if err != nil {
		return nil, ierr.NewError("failed to get QuickBooks connection").
			WithHint("QuickBooks connection not configured for this environment").
			WithReportableDetails(map[string]interface{}{
				"error": err.Error(),
			}).
			Mark(ierr.ErrNotFound)
	}

	// Decrypt connection metadata to get usable credentials
	qbConfig, err := c.GetDecryptedQuickBooksConfig(conn)
	if err != nil {
		return nil, ierr.NewError("failed to get QuickBooks configuration").
			WithHint("Invalid QuickBooks configuration").
			Mark(ierr.ErrValidation)
	}

	// RealmID is required - it's the QuickBooks Company ID used in all API calls
	if qbConfig.RealmID == "" {
		return nil, ierr.NewError("missing QuickBooks realm ID").
			WithHint("Configure QuickBooks Company ID (realm ID) in the connection settings").
			Mark(ierr.ErrValidation)
	}

	// AccessToken is required for authenticated API calls
	if qbConfig.AccessToken == "" {
		return nil, ierr.NewError("missing QuickBooks access token").
			WithHint("Configure QuickBooks OAuth access token in the connection settings").
			Mark(ierr.ErrValidation)
	}

	return qbConfig, nil
}

// GetDecryptedQuickBooksConfig decrypts and returns QuickBooks configuration from connection.
// Decrypts encrypted fields (ClientID, ClientSecret, AccessToken, RefreshToken) using encryption service.
// RealmID and Environment are stored unencrypted.
// Defaults to "production" environment if not specified.
func (c *Client) GetDecryptedQuickBooksConfig(conn *connection.Connection) (*QuickBooksConfig, error) {
	decryptedMetadata, err := c.decryptConnectionMetadata(conn)
	if err != nil {
		return nil, err
	}

	qbConfig := &QuickBooksConfig{}

	if clientID, exists := decryptedMetadata["client_id"]; exists {
		qbConfig.ClientID = clientID
	}

	if clientSecret, exists := decryptedMetadata["client_secret"]; exists {
		qbConfig.ClientSecret = clientSecret
	}

	if accessToken, exists := decryptedMetadata["access_token"]; exists {
		qbConfig.AccessToken = accessToken
	}

	if refreshToken, exists := decryptedMetadata["refresh_token"]; exists {
		qbConfig.RefreshToken = refreshToken
	}

	// RealmID is not encrypted - it's the QuickBooks Company ID
	if realmID, exists := decryptedMetadata["realm_id"]; exists {
		qbConfig.RealmID = realmID
	}

	// Environment defaults to "production" if not specified
	if environment, exists := decryptedMetadata["environment"]; exists {
		qbConfig.Environment = environment
	} else {
		qbConfig.Environment = "production"
	}

	return qbConfig, nil
}

// decryptConnectionMetadata decrypts the connection encrypted secret data
func (c *Client) decryptConnectionMetadata(conn *connection.Connection) (types.Metadata, error) {
	if conn.EncryptedSecretData.QuickBooks == nil {
		return types.Metadata{}, nil
	}

	if conn.ProviderType == types.SecretProviderQuickBooks {

		clientID, err := c.encryptionService.Decrypt(conn.EncryptedSecretData.QuickBooks.ClientID)
		if err != nil {
			return nil, ierr.NewError("failed to decrypt client_id").Mark(ierr.ErrInternal)
		}

		clientSecret, err := c.encryptionService.Decrypt(conn.EncryptedSecretData.QuickBooks.ClientSecret)
		if err != nil {
			return nil, ierr.NewError("failed to decrypt client_secret").Mark(ierr.ErrInternal)
		}

		accessToken, err := c.encryptionService.Decrypt(conn.EncryptedSecretData.QuickBooks.AccessToken)
		if err != nil {
			return nil, ierr.NewError("failed to decrypt access_token").Mark(ierr.ErrInternal)
		}

		refreshToken, err := c.encryptionService.Decrypt(conn.EncryptedSecretData.QuickBooks.RefreshToken)
		if err != nil {
			return nil, ierr.NewError("failed to decrypt refresh_token").Mark(ierr.ErrInternal)
		}

		decryptedMetadata := types.Metadata{
			"client_id":        clientID,
			"client_secret":    clientSecret,
			"access_token":     accessToken,
			"refresh_token":    refreshToken,
			"realm_id":         conn.EncryptedSecretData.QuickBooks.RealmID, // Not encrypted
			"environment":      conn.EncryptedSecretData.QuickBooks.Environment,
			"token_expires_at": fmt.Sprintf("%d", conn.EncryptedSecretData.QuickBooks.TokenExpiresAt),
		}

		return decryptedMetadata, nil
	}

	return types.Metadata{}, nil
}

// HasQuickBooksConnection checks if QuickBooks connection exists for the current environment
func (c *Client) HasQuickBooksConnection(ctx context.Context) bool {
	conn, err := c.connectionRepo.GetByProvider(ctx, types.SecretProviderQuickBooks)
	return err == nil && conn != nil && conn.Status == types.StatusPublished
}

// GetConnection retrieves the QuickBooks connection for the current context
func (c *Client) GetConnection(ctx context.Context) (*connection.Connection, error) {
	conn, err := c.connectionRepo.GetByProvider(ctx, types.SecretProviderQuickBooks)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to get QuickBooks connection").
			Mark(ierr.ErrDatabase)
	}
	if conn == nil {
		return nil, ierr.NewError("QuickBooks connection not found").
			WithHint("QuickBooks connection not configured for this environment").
			Mark(ierr.ErrNotFound)
	}
	return conn, nil
}

// GetBaseURL returns the QuickBooks API base URL based on environment
func (c *Client) GetBaseURL(environment string) string {
	if environment == "sandbox" {
		return "https://sandbox-quickbooks.api.intuit.com"
	}
	return "https://quickbooks.api.intuit.com"
}

// makeRequest makes an HTTP request to QuickBooks API
func (c *Client) makeRequest(ctx context.Context, method, endpoint string, body interface{}) (*http.Response, error) {
	qbConfig, err := c.GetQuickBooksConfig(ctx)
	if err != nil {
		return nil, err
	}

	baseURL := c.GetBaseURL(qbConfig.Environment)
	fullURL := fmt.Sprintf("%s/v3/company/%s/%s?minorversion=%s", baseURL, qbConfig.RealmID, endpoint, c.minorVersion)

	var bodyReader io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, ierr.NewError("failed to marshal request body").
				Mark(ierr.ErrSystem)
		}
		bodyReader = strings.NewReader(string(jsonData))
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return nil, ierr.NewError("failed to create request").
			Mark(ierr.ErrSystem)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", qbConfig.AccessToken))
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, ierr.NewError("failed to make request to QuickBooks API").
			WithHint("Network error connecting to QuickBooks API").
			Mark(ierr.ErrSystem)
	}

	return resp, nil
}

// parseErrorResponse parses QuickBooks error response
func (c *Client) parseErrorResponse(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ierr.NewError("failed to read error response").
			Mark(ierr.ErrSystem)
	}

	var fault struct {
		Fault struct {
			Type  string `json:"type"`
			Error []struct {
				Detail  string `json:"Detail"`
				Code    string `json:"code"`
				Element string `json:"element,omitempty"`
			} `json:"Error"`
		} `json:"fault"`
	}

	if err := json.Unmarshal(body, &fault); err == nil && len(fault.Fault.Error) > 0 {
		errorCode := fault.Fault.Error[0].Code
		errorDetail := fault.Fault.Error[0].Detail

		// Handle token expiration specifically
		if errorCode == "3200" && (strings.Contains(errorDetail, "Token expired") || strings.Contains(errorDetail, "token expired")) {
			return ierr.NewError("QuickBooks access token expired").
				WithHint("Your QuickBooks access token has expired. Please refresh the token using your refresh_token and update the connection with the new access_token.").
				WithReportableDetails(map[string]interface{}{
					"code":   errorCode,
					"detail": errorDetail,
					"action": "refresh_token_required",
				}).
				Mark(ierr.ErrHTTPClient)
		}

		return ierr.NewError("QuickBooks API error").
			WithHint(errorDetail).
			WithReportableDetails(map[string]interface{}{
				"code":   errorCode,
				"detail": errorDetail,
			}).
			Mark(ierr.ErrHTTPClient)
	}

	return ierr.NewError("QuickBooks API error").
		WithHint(fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body))).
		Mark(ierr.ErrHTTPClient)
}

// queryEntities performs a QuickBooks query with automatic token refresh
func (c *Client) queryEntities(ctx context.Context, entityType, query string) ([]byte, error) {
	return c.queryEntitiesWithRetry(ctx, entityType, query, 0)
}

// queryEntitiesWithRetry performs a QuickBooks query with retry on token expiration
func (c *Client) queryEntitiesWithRetry(ctx context.Context, entityType, query string, retryCount int) ([]byte, error) {
	const maxRetries = 1

	// Get fresh config on each attempt to ensure we have the latest token after refresh
	qbConfig, err := c.GetQuickBooksConfig(ctx)
	if err != nil {
		return nil, err
	}

	baseURL := c.GetBaseURL(qbConfig.Environment)
	queryURL := fmt.Sprintf("%s/v3/company/%s/query?minorversion=%s&query=%s",
		baseURL, qbConfig.RealmID, c.minorVersion, url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, "GET", queryURL, nil)
	if err != nil {
		return nil, ierr.NewError("failed to create request").Mark(ierr.ErrSystem)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", qbConfig.AccessToken))
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, ierr.NewError("failed to query QuickBooks").
			Mark(ierr.ErrSystem)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, ierr.NewError("failed to read response").Mark(ierr.ErrSystem)
	}

	if resp.StatusCode != http.StatusOK {
		// Create a response copy for error parsing
		respCopy := &http.Response{
			StatusCode: resp.StatusCode,
			Body:       io.NopCloser(strings.NewReader(string(body))),
		}
		err = c.parseErrorResponse(respCopy)

		// Check if token expired and retry
		if c.isTokenExpiredError(err) && retryCount < maxRetries {
			if refreshErr := c.RefreshAccessToken(ctx); refreshErr != nil {
				return nil, ierr.WithError(refreshErr).
					WithHint("Token expired and automatic refresh failed. Please check your QuickBooks connection credentials.").
					Mark(ierr.ErrHTTPClient)
			}

			// Retry the query - will get fresh config with new token
			return c.queryEntitiesWithRetry(ctx, entityType, query, retryCount+1)
		}

		return nil, err
	}

	return body, nil
}

// CreateCustomer creates a customer in QuickBooks
func (c *Client) CreateCustomer(ctx context.Context, req *CustomerCreateRequest) (*CustomerResponse, error) {
	payload := map[string]interface{}{
		"DisplayName": req.DisplayName,
	}

	if req.PrimaryEmailAddr != nil && req.PrimaryEmailAddr.Address != "" {
		payload["PrimaryEmailAddr"] = map[string]string{
			"Address": req.PrimaryEmailAddr.Address,
		}
	}

	if req.BillAddr != nil {
		addr := make(map[string]string)
		if req.BillAddr.Line1 != "" {
			addr["Line1"] = req.BillAddr.Line1
		}
		if req.BillAddr.Line2 != "" {
			addr["Line2"] = req.BillAddr.Line2
		}
		if req.BillAddr.City != "" {
			addr["City"] = req.BillAddr.City
		}
		if req.BillAddr.CountrySubDivisionCode != "" {
			addr["CountrySubDivisionCode"] = req.BillAddr.CountrySubDivisionCode
		}
		if req.BillAddr.PostalCode != "" {
			addr["PostalCode"] = req.BillAddr.PostalCode
		}
		if req.BillAddr.Country != "" {
			addr["Country"] = req.BillAddr.Country
		}
		if len(addr) > 0 {
			payload["BillAddr"] = addr
		}
	}

	resp, err := c.makeRequestWithRetry(ctx, "POST", "customer", payload, 0)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, c.parseErrorResponse(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, ierr.NewError("failed to read response").Mark(ierr.ErrSystem)
	}

	var result struct {
		Customer CustomerResponse `json:"Customer"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, ierr.NewError("failed to parse QuickBooks response").
			Mark(ierr.ErrSystem)
	}

	return &result.Customer, nil
}

// QueryCustomerByEmail queries a customer by email
func (c *Client) QueryCustomerByEmail(ctx context.Context, email string) (*CustomerResponse, error) {
	escapedEmail := strings.ReplaceAll(email, "'", "''")
	query := fmt.Sprintf("SELECT * FROM Customer WHERE PrimaryEmailAddr = '%s'", escapedEmail)

	body, err := c.queryEntities(ctx, "Customer", query)
	if err != nil {
		return nil, err
	}

	var queryResp QueryResponse
	if err := json.Unmarshal(body, &queryResp); err != nil {
		return nil, ierr.NewError("failed to parse QuickBooks response").Mark(ierr.ErrSystem)
	}

	if len(queryResp.QueryResponse.Customer) == 0 {
		return nil, ierr.NewError("customer not found").Mark(ierr.ErrNotFound)
	}

	return &queryResp.QueryResponse.Customer[0], nil
}

// QueryCustomerByName queries a customer by display name
func (c *Client) QueryCustomerByName(ctx context.Context, name string) (*CustomerResponse, error) {
	escapedName := strings.ReplaceAll(name, "'", "''")
	query := fmt.Sprintf("SELECT * FROM Customer WHERE DisplayName = '%s'", escapedName)

	body, err := c.queryEntities(ctx, "Customer", query)
	if err != nil {
		return nil, err
	}

	var queryResp QueryResponse
	if err := json.Unmarshal(body, &queryResp); err != nil {
		return nil, ierr.NewError("failed to parse QuickBooks response").Mark(ierr.ErrSystem)
	}

	if len(queryResp.QueryResponse.Customer) == 0 {
		return nil, ierr.NewError("customer not found").Mark(ierr.ErrNotFound)
	}

	return &queryResp.QueryResponse.Customer[0], nil
}

// CreateItem creates an item in QuickBooks
func (c *Client) CreateItem(ctx context.Context, req *ItemCreateRequest) (*ItemResponse, error) {
	incomeAccountRef := map[string]string{
		"value": req.IncomeAccountRef.Value,
	}
	if req.IncomeAccountRef.Name != "" {
		incomeAccountRef["name"] = req.IncomeAccountRef.Name
	}

	payload := map[string]interface{}{
		"Name":             req.Name,
		"Type":             req.Type,
		"Active":           req.Active,
		"IncomeAccountRef": incomeAccountRef,
	}

	if req.Description != "" {
		payload["Description"] = req.Description
	}

	resp, err := c.makeRequestWithRetry(ctx, "POST", "item", payload, 0)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, c.parseErrorResponse(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, ierr.NewError("failed to read response").Mark(ierr.ErrSystem)
	}

	var result struct {
		Item ItemResponse `json:"Item"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, ierr.NewError("failed to parse QuickBooks response").
			Mark(ierr.ErrSystem)
	}

	return &result.Item, nil
}

// QueryItemByName queries an item by name
func (c *Client) QueryItemByName(ctx context.Context, name string) (*ItemResponse, error) {
	escapedName := strings.ReplaceAll(name, "'", "''")
	query := fmt.Sprintf("SELECT * FROM Item WHERE Name = '%s' AND Type = 'Service' AND Active = true", escapedName)

	body, err := c.queryEntities(ctx, "Item", query)
	if err != nil {
		return nil, err
	}

	var queryResp QueryResponse
	if err := json.Unmarshal(body, &queryResp); err != nil {
		return nil, ierr.NewError("failed to parse QuickBooks response").Mark(ierr.ErrSystem)
	}

	if len(queryResp.QueryResponse.Item) == 0 {
		return nil, ierr.NewError("item not found").Mark(ierr.ErrNotFound)
	}

	return &queryResp.QueryResponse.Item[0], nil
}

// CreateInvoice creates an invoice in QuickBooks
func (c *Client) CreateInvoice(ctx context.Context, req *InvoiceCreateRequest) (*InvoiceResponse, error) {
	payload := map[string]interface{}{
		"CustomerRef": map[string]string{
			"value": req.CustomerRef.Value,
		},
		"Line": req.Line,
	}

	resp, err := c.makeRequestWithRetry(ctx, "POST", "invoice", payload, 0)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, c.parseErrorResponse(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, ierr.NewError("failed to read response").Mark(ierr.ErrSystem)
	}

	var result struct {
		Invoice InvoiceResponse `json:"Invoice"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, ierr.NewError("failed to parse QuickBooks response").
			Mark(ierr.ErrSystem)
	}

	return &result.Invoice, nil
}

// RefreshAccessToken refreshes the QuickBooks access token using the refresh token.
// This method implements OAuth 2.0 token refresh flow:
// 1. Uses refresh_token to get new access_token and refresh_token from QuickBooks OAuth endpoint
// 2. Encrypts new tokens before saving
// 3. Updates connection in database with new encrypted tokens and expiration time
// This is called automatically when API calls detect token expiration (error code 3200).
func (c *Client) RefreshAccessToken(ctx context.Context) error {
	conn, err := c.GetConnection(ctx)
	if err != nil {
		return err
	}

	qbConfig, err := c.GetDecryptedQuickBooksConfig(conn)
	if err != nil {
		return err
	}

	if qbConfig.RefreshToken == "" {
		return ierr.NewError("refresh token not available").
			WithHint("QuickBooks refresh token is required to refresh access token").
			Mark(ierr.ErrValidation)
	}

	// QuickBooks OAuth token refresh endpoint
	refreshURL := "https://oauth.platform.intuit.com/oauth2/v1/tokens/bearer"

	// Prepare form data for OAuth 2.0 refresh token grant
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", qbConfig.RefreshToken)

	req, err := http.NewRequestWithContext(ctx, "POST", refreshURL, strings.NewReader(data.Encode()))
	if err != nil {
		return ierr.NewError("failed to create refresh token request").
			Mark(ierr.ErrSystem)
	}

	// OAuth 2.0 requires Basic Auth with client_id:client_secret for token refresh
	auth := fmt.Sprintf("%s:%s", qbConfig.ClientID, qbConfig.ClientSecret)
	req.Header.Set("Authorization", fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(auth))))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ierr.NewError("failed to refresh token").
			WithHint("Network error connecting to QuickBooks OAuth endpoint").
			Mark(ierr.ErrSystem)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		responseBody := string(body)

		if strings.Contains(responseBody, "invalid_grant") ||
			strings.Contains(responseBody, "invalid refresh token") ||
			strings.Contains(responseBody, "Incorrect or invalid refresh token") {
			return ierr.NewError("QuickBooks refresh token is invalid or expired").
				WithHint("The QuickBooks refresh token stored in your connection is invalid or expired. Please re-authenticate with QuickBooks and update your connection with new tokens.").
				WithReportableDetails(map[string]interface{}{
					"status_code": resp.StatusCode,
					"response":    responseBody,
				}).
				Mark(ierr.ErrHTTPClient)
		}

		return ierr.NewError("failed to refresh token").
			WithHint(fmt.Sprintf("QuickBooks OAuth returned status %d: %s", resp.StatusCode, responseBody)).
			Mark(ierr.ErrHTTPClient)
	}

	var tokenResponse struct {
		AccessToken           string `json:"access_token"`
		RefreshToken          string `json:"refresh_token"`
		ExpiresIn             int    `json:"expires_in"`               // seconds
		RefreshTokenExpiresIn int    `json:"refresh_token_expires_in"` // seconds
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ierr.NewError("failed to read token response").
			Mark(ierr.ErrSystem)
	}

	if err := json.Unmarshal(body, &tokenResponse); err != nil {
		return ierr.NewError("failed to parse token response").
			WithHint(fmt.Sprintf("Response: %s", string(body))).
			Mark(ierr.ErrSystem)
	}

	// Calculate new expiration time from expires_in (seconds)
	newExpiresAt := time.Now().Add(time.Duration(tokenResponse.ExpiresIn) * time.Second).Unix()

	// Encrypt new tokens before saving to database
	// All sensitive OAuth tokens must be encrypted at rest
	encryptedAccessToken, err := c.encryptionService.Encrypt(tokenResponse.AccessToken)
	if err != nil {
		return ierr.NewError("failed to encrypt new access token").
			Mark(ierr.ErrInternal)
	}

	encryptedRefreshToken, err := c.encryptionService.Encrypt(tokenResponse.RefreshToken)
	if err != nil {
		return ierr.NewError("failed to encrypt new refresh token").
			Mark(ierr.ErrInternal)
	}

	// Update connection with new encrypted tokens and expiration time
	conn.EncryptedSecretData.QuickBooks.AccessToken = encryptedAccessToken
	conn.EncryptedSecretData.QuickBooks.RefreshToken = encryptedRefreshToken
	conn.EncryptedSecretData.QuickBooks.TokenExpiresAt = newExpiresAt

	conn.UpdatedAt = time.Now()
	conn.UpdatedBy = types.GetUserID(ctx)

	if err := c.connectionRepo.Update(ctx, conn); err != nil {
		c.logger.Errorw("failed to update connection with new tokens",
			"connection_id", conn.ID,
			"error", err)
		return ierr.NewError("failed to update connection with new tokens").
			Mark(ierr.ErrDatabase)
	}

	c.logger.Infow("successfully updated connection with new tokens",
		"connection_id", conn.ID)

	return nil
}

// isTokenExpiredError checks if the error is a token expiration error (error code 3200).
// Returns false for invalid refresh token errors (which require re-authentication, not retry).
func (c *Client) isTokenExpiredError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := strings.ToLower(err.Error())

	// Don't retry if refresh token is invalid - user needs to re-authenticate
	// These errors indicate the refresh token itself is bad, not just expired
	if strings.Contains(errMsg, "invalid refresh token") ||
		strings.Contains(errMsg, "invalid_grant") ||
		strings.Contains(errMsg, "refresh token is invalid") ||
		strings.Contains(errMsg, "incorrect or invalid refresh token") {
		return false
	}

	// Check for access token expiration (error code 3200) - these can be retried with refresh
	if strings.Contains(errMsg, "token expired") ||
		strings.Contains(errMsg, "3200") {
		return true
	}

	return false
}

// makeRequestWithRetry makes an HTTP request to QuickBooks API with automatic token refresh on expiration.
// If a request fails with token expiration (error code 3200), this method:
// 1. Automatically refreshes the access token
// 2. Retries the original request with the new token
// This provides seamless token refresh without requiring manual intervention.
func (c *Client) makeRequestWithRetry(ctx context.Context, method, endpoint string, body interface{}, retryCount int) (*http.Response, error) {
	const maxRetries = 1 // Only retry once after token refresh to avoid infinite loops

	resp, err := c.makeRequest(ctx, method, endpoint, body)
	if err != nil {
		// Check if error is token expiration
		if c.isTokenExpiredError(err) && retryCount < maxRetries {
			if refreshErr := c.RefreshAccessToken(ctx); refreshErr != nil {
				return nil, ierr.WithError(refreshErr).
					WithHint("Token expired and automatic refresh failed. Please check your QuickBooks connection credentials.").
					Mark(ierr.ErrHTTPClient)
			}

			// Retry the request
			return c.makeRequestWithRetry(ctx, method, endpoint, body, retryCount+1)
		}
		return resp, err
	}

	// Check if response indicates token expiration
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		// Read response body to check error details
		bodyBytes, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()

		if readErr == nil {
			// Create a new response with the body for parsing
			respCopy := &http.Response{
				StatusCode: resp.StatusCode,
				Body:       io.NopCloser(strings.NewReader(string(bodyBytes))),
			}
			err = c.parseErrorResponse(respCopy)

			if c.isTokenExpiredError(err) && retryCount < maxRetries {
				if refreshErr := c.RefreshAccessToken(ctx); refreshErr != nil {
					return nil, ierr.WithError(refreshErr).
						WithHint("Token expired and automatic refresh failed. Please check your QuickBooks connection credentials.").
						Mark(ierr.ErrHTTPClient)
				}

				// Retry the request
				return c.makeRequestWithRetry(ctx, method, endpoint, body, retryCount+1)
			}
		}

		// Return error if not token expiration or retry limit reached
		if err == nil {
			err = ierr.NewError("QuickBooks API error").
				WithHint(fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))).
				Mark(ierr.ErrHTTPClient)
		}
		return nil, err
	}

	return resp, nil
}
