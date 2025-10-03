// internal/integration/base/manager.go
package base

import (
	"context"
	"fmt"
	"sync"
)

// integrationManager implements IntegrationManager interface
type integrationManager struct {
	integrations map[IntegrationType]Integration
	mu           sync.RWMutex
	config       *ManagerConfig
}

// ManagerConfig contains manager configuration
type ManagerConfig struct {
	DefaultIntegration  IntegrationType
	EnabledIntegrations []IntegrationType
}

// NewIntegrationManager creates a new integration manager
func NewIntegrationManager(config *ManagerConfig) IntegrationManager {
	return &integrationManager{
		integrations: make(map[IntegrationType]Integration),
		config:       config,
	}
}

// RegisterIntegration registers a new integration
func (m *integrationManager) RegisterIntegration(integration Integration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.integrations[integration.Type]; exists {
		return fmt.Errorf("integration %s already registered", integration.Type)
	}

	m.integrations[integration.Type] = integration
	return nil
}

// GetClient retrieves a client for a specific integration type
func (m *integrationManager) GetClient(integrationType IntegrationType) (GenericClient, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	integration, exists := m.integrations[integrationType]
	if !exists {
		return nil, fmt.Errorf("integration %s not found", integrationType)
	}

	if integration.Client == nil {
		return nil, fmt.Errorf("client not configured for integration %s", integrationType)
	}

	return integration.Client, nil
}

// GetWebhookHandler retrieves a webhook handler for a specific integration type
func (m *integrationManager) GetWebhookHandler(integrationType IntegrationType) (WebhookHandler, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	integration, exists := m.integrations[integrationType]
	if !exists {
		return nil, fmt.Errorf("integration %s not found", integrationType)
	}

	if integration.WebhookHandler == nil {
		return nil, fmt.Errorf("webhook handler not configured for integration %s", integrationType)
	}

	return integration.WebhookHandler, nil
}

// GetIntegration retrieves a complete integration
func (m *integrationManager) GetIntegration(integrationType IntegrationType) (Integration, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	integration, exists := m.integrations[integrationType]
	if !exists {
		return Integration{}, fmt.Errorf("integration %s not found", integrationType)
	}

	return integration, nil
}

// ListIntegrations returns all registered integration types
func (m *integrationManager) ListIntegrations() []IntegrationType {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var types []IntegrationType
	for integrationType := range m.integrations {
		types = append(types, integrationType)
	}

	return types
}

// HealthCheck performs health check on all integrations
func (m *integrationManager) HealthCheck(ctx context.Context) map[IntegrationType]error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := make(map[IntegrationType]error)

	for integrationType, integration := range m.integrations {
		if integration.Client != nil {
			results[integrationType] = integration.Client.IsHealthy(ctx)
		} else {
			results[integrationType] = fmt.Errorf("client not configured")
		}
	}

	return results
}
