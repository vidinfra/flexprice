package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/cache"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/security"
	"github.com/flexprice/flexprice/internal/types"
)

const (
	// OAuthSessionTTL is the lifetime of an OAuth session (5 minutes)
	// This matches typical OAuth authorization code expiry times
	OAuthSessionTTL = 5 * time.Minute

	// Cache key prefix for OAuth sessions
	oauthSessionPrefix = "oauth:session:"
)

// OAuthService manages temporary OAuth sessions during OAuth flows for multiple providers
type OAuthService interface {
	// StoreOAuthSession stores an OAuth session in cache with automatic expiration
	// Credentials in the session will be encrypted before storage
	StoreOAuthSession(ctx context.Context, session *types.OAuthSession) error

	// GetOAuthSession retrieves and decrypts an OAuth session from cache
	GetOAuthSession(ctx context.Context, sessionID string) (*types.OAuthSession, error)

	// DeleteOAuthSession removes an OAuth session from cache (cleanup)
	DeleteOAuthSession(ctx context.Context, sessionID string) error

	// GenerateSessionID generates a cryptographically secure random session ID
	GenerateSessionID() (string, error)

	// GenerateCSRFState generates a cryptographically secure random CSRF state token
	GenerateCSRFState() (string, error)

	// BuildOAuthURL builds the provider-specific OAuth authorization URL
	BuildOAuthURL(provider types.OAuthProvider, clientID, redirectURI, state string, metadata map[string]string) (string, error)

	// ExchangeCodeForConnection exchanges the authorization code for tokens and creates a connection
	ExchangeCodeForConnection(ctx context.Context, session *types.OAuthSession, code, realmID string) (connectionID string, err error)
}

type oauthService struct {
	cache             cache.Cache
	encryptionService security.EncryptionService
	connectionService ConnectionService
	logger            *logger.Logger
}

// NewOAuthService creates a new OAuth service (renamed from NewOAuthSessionService)
func NewOAuthService(
	cache cache.Cache,
	encryptionService security.EncryptionService,
	connectionService ConnectionService,
	logger *logger.Logger,
) OAuthService {
	return &oauthService{
		cache:             cache,
		encryptionService: encryptionService,
		connectionService: connectionService,
		logger:            logger,
	}
}

// StoreOAuthSession stores an OAuth session in cache with encryption and TTL
func (s *oauthService) StoreOAuthSession(ctx context.Context, session *types.OAuthSession) error {
	// Validate session
	if err := session.Validate(); err != nil {
		return ierr.WithError(err).
			WithHint("OAuth session validation failed").
			Mark(ierr.ErrValidation)
	}

	// Check expiration
	if session.IsExpired() {
		return ierr.NewError("OAuth session has already expired").
			WithHint("Session must have a future expiration time").
			Mark(ierr.ErrValidation)
	}

	// CRITICAL: Encrypt all credentials before caching
	// This provides double encryption: encrypted in cache AND in database later
	encryptedCredentials := make(map[string]string)
	for key, value := range session.Credentials {
		encrypted, err := s.encryptionService.Encrypt(value)
		if err != nil {
			return ierr.WithError(err).
				WithHint(fmt.Sprintf("Failed to encrypt credential '%s' for OAuth session", key)).
				Mark(ierr.ErrInternal)
		}
		encryptedCredentials[key] = encrypted
	}

	// Create a copy with encrypted credentials
	sessionToStore := *session
	sessionToStore.Credentials = encryptedCredentials

	// Store in cache with TTL
	cacheKey := oauthSessionPrefix + session.SessionID
	s.cache.Set(ctx, cacheKey, &sessionToStore, OAuthSessionTTL)

	s.logger.Infow("stored OAuth session in cache",
		"session_id", session.SessionID,
		"provider", session.Provider,
		"tenant_id", session.TenantID,
		"expires_at", session.ExpiresAt)

	return nil
}

// GetOAuthSession retrieves and decrypts an OAuth session from cache
func (s *oauthService) GetOAuthSession(ctx context.Context, sessionID string) (*types.OAuthSession, error) {
	if sessionID == "" {
		return nil, ierr.NewError("session_id is required").
			WithHint("Provide a valid session_id from the OAuth init response").
			Mark(ierr.ErrValidation)
	}

	// Retrieve from cache
	cacheKey := oauthSessionPrefix + sessionID
	value, found := s.cache.Get(ctx, cacheKey)
	if !found {
		return nil, ierr.NewError("OAuth session not found or expired").
			WithHint("The OAuth session may have expired (5 minute limit). Please restart the OAuth flow.").
			Mark(ierr.ErrNotFound)
	}

	// Type assertion
	session, ok := value.(*types.OAuthSession)
	if !ok {
		s.logger.Errorw("invalid OAuth session type in cache",
			"session_id", sessionID,
			"type", fmt.Sprintf("%T", value))
		return nil, ierr.NewError("invalid OAuth session data").
			WithHint("OAuth session data is corrupted. Please restart the OAuth flow.").
			Mark(ierr.ErrInternal)
	}

	// Check expiration (belt-and-suspenders - cache should auto-expire)
	if session.IsExpired() {
		s.cache.Delete(ctx, cacheKey) // Cleanup expired session
		return nil, ierr.NewError("OAuth session has expired").
			WithHint("The OAuth session expired. Please restart the OAuth flow.").
			Mark(ierr.ErrNotFound)
	}

	// CRITICAL: Decrypt all credentials after retrieval
	decryptedCredentials := make(map[string]string)
	for key, encryptedValue := range session.Credentials {
		decrypted, err := s.encryptionService.Decrypt(encryptedValue)
		if err != nil {
			return nil, ierr.WithError(err).
				WithHint(fmt.Sprintf("Failed to decrypt credential '%s' from OAuth session", key)).
				Mark(ierr.ErrInternal)
		}
		decryptedCredentials[key] = decrypted
	}

	// Return session with decrypted credentials
	decryptedSession := *session
	decryptedSession.Credentials = decryptedCredentials

	s.logger.Debugw("retrieved OAuth session from cache",
		"session_id", sessionID,
		"provider", session.Provider,
		"tenant_id", session.TenantID)

	return &decryptedSession, nil
}

// DeleteOAuthSession removes an OAuth session from cache
func (s *oauthService) DeleteOAuthSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil // Nothing to delete
	}

	cacheKey := oauthSessionPrefix + sessionID
	s.cache.Delete(ctx, cacheKey)

	s.logger.Debugw("deleted OAuth session from cache",
		"session_id", sessionID)

	return nil
}

// GenerateSessionID generates a cryptographically secure random session ID (32 bytes = 64 hex chars)
func (s *oauthService) GenerateSessionID() (string, error) {
	return generateSecureRandomHex(32)
}

// GenerateCSRFState generates a cryptographically secure random CSRF state token (32 bytes = 64 hex chars)
func (s *oauthService) GenerateCSRFState() (string, error) {
	return generateSecureRandomHex(32)
}

// generateSecureRandomHex generates a cryptographically secure random hex string
func generateSecureRandomHex(byteLength int) (string, error) {
	bytes := make([]byte, byteLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", ierr.WithError(err).
			WithHint("Failed to generate secure random token").
			Mark(ierr.ErrInternal)
	}
	return hex.EncodeToString(bytes), nil
}

// BuildOAuthURL builds the provider-specific OAuth authorization URL
func (s *oauthService) BuildOAuthURL(provider types.OAuthProvider, clientID, redirectURI, state string, metadata map[string]string) (string, error) {
	switch provider {
	case types.OAuthProviderQuickBooks:
		params := url.Values{}
		params.Set("client_id", clientID)
		params.Set("redirect_uri", redirectURI)
		params.Set("response_type", "code")
		params.Set("scope", "com.intuit.quickbooks.accounting")
		params.Set("state", state)
		return fmt.Sprintf("https://appcenter.intuit.com/connect/oauth2?%s", params.Encode()), nil

	// Add more providers here:
	// case types.OAuthProviderStripe:
	//     return buildStripeOAuthURL(clientID, redirectURI, state, metadata), nil

	default:
		return "", ierr.NewError(fmt.Sprintf("unsupported OAuth provider: %s", provider)).
			WithHint("Supported providers: quickbooks").
			Mark(ierr.ErrValidation)
	}
}

// ExchangeCodeForConnection exchanges the authorization code for tokens and creates a connection
func (s *oauthService) ExchangeCodeForConnection(
	ctx context.Context,
	session *types.OAuthSession,
	code, realmID string,
) (string, error) {
	switch session.Provider {
	case types.OAuthProviderQuickBooks:
		// Create connection using existing connection service
		// This will encrypt credentials, exchange auth code for tokens, and store in DB
		connectionReq := dto.CreateConnectionRequest{
			Name:         session.Name,
			ProviderType: types.SecretProviderQuickBooks,
			EncryptedSecretData: types.ConnectionMetadata{
				QuickBooks: &types.QuickBooksConnectionMetadata{
					ClientID:        session.GetCredential(types.OAuthCredentialClientID),
					ClientSecret:    session.GetCredential(types.OAuthCredentialClientSecret),
					RealmID:         realmID,
					Environment:     session.GetMetadata(types.OAuthMetadataEnvironment),
					AuthCode:        code, // Will be exchanged for tokens immediately by connectionService
					RedirectURI:     session.GetMetadata(types.OAuthMetadataRedirectURI),
					IncomeAccountID: session.GetMetadata(types.OAuthMetadataIncomeAccountID),
				},
			},
			SyncConfig: session.SyncConfig, // Include sync configuration from OAuth session
		}

		// Create connection (this will exchange auth_code for tokens automatically)
		connectionResp, err := s.connectionService.CreateConnection(ctx, connectionReq)
		if err != nil {
			return "", ierr.WithError(err).
				WithHint("Failed to create QuickBooks connection. The authorization may have expired or been revoked.").
				Mark(ierr.ErrInternal)
		}

		s.logger.Infow("QuickBooks OAuth connection created successfully",
			"connection_id", connectionResp.ID,
			"realm_id", realmID)

		return connectionResp.ID, nil

	// Add more providers here:
	// case types.OAuthProviderStripe:
	//     return s.exchangeStripeCode(ctx, session, code)

	default:
		return "", ierr.NewError(fmt.Sprintf("unsupported OAuth provider: %s", session.Provider)).
			WithHint("Supported providers: quickbooks").
			Mark(ierr.ErrValidation)
	}
}
