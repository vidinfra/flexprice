package chartmogul

import (
	"fmt"

	cm "github.com/chartmogul/chartmogul-go/v4"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/logger"
)

type ChartMogulService struct {
	client *cm.API
	cfg    *config.Configuration
	logger *logger.Logger
}

func NewChartMogulService(cfg *config.Configuration, logger *logger.Logger) (*ChartMogulService, error) {
	apiKey := cfg.ChartMogul.APIKey
	if apiKey == "" {
		return nil, fmt.Errorf("CHARTMOGUL_API_KEY not set")
	}
	client := &cm.API{
		ApiKey: apiKey,
	}
	return &ChartMogulService{
		client: client,
		cfg:    cfg,
		logger: logger,
	}, nil
}

func (s *ChartMogulService) Ping() (bool, error) {
	return s.client.Ping()
}

func (s *ChartMogulService) CreatePlan(dataSourceUUID, name string, intervalCount int, intervalUnit, externalID string) (*cm.Plan, error) {
	plan := &cm.Plan{
		DataSourceUUID: dataSourceUUID,
		Name:           name,
		IntervalCount:  uint32(intervalCount),
		IntervalUnit:   intervalUnit,
		ExternalID:     externalID,
	}
	createdPlan, err := s.client.CreatePlan(plan)
	if err != nil {
		s.logger.Errorw("ChartMogul CreatePlan failed", "error", err, "plan", plan)
		return nil, err
	}
	return createdPlan, nil
}

func (s *ChartMogulService) UpdatePlan(plan *cm.Plan, planUUID string) (*cm.Plan, error) {
	updatedPlan, err := s.client.UpdatePlan(plan, planUUID)
	if err != nil {
		s.logger.Errorw("ChartMogul UpdatePlan failed", "error", err, "planUUID", planUUID)
		return nil, err
	}
	return updatedPlan, nil
}

func (s *ChartMogulService) DeletePlan(planUUID string) error {
	err := s.client.DeletePlan(planUUID)
	if err != nil {
		s.logger.Errorw("ChartMogul DeletePlan failed", "error", err, "planUUID", planUUID)
	}
	return err
}

func (s *ChartMogulService) CreateCustomer(newCustomer *cm.NewCustomer) (*cm.Customer, error) {
	customer, err := s.client.CreateCustomer(newCustomer)
	if err != nil {
		s.logger.Errorw("ChartMogul CreateCustomer failed", "error", err, "customer", newCustomer)
		return nil, err
	}
	return customer, nil
}

func (s *ChartMogulService) UpdateCustomer(customer *cm.Customer, customerUUID string) (*cm.Customer, error) {
	updatedCustomer, err := s.client.UpdateCustomer(customer, customerUUID)
	if err != nil {
		s.logger.Errorw("ChartMogul UpdateCustomer failed", "error", err, "customerUUID", customerUUID)
		return nil, err
	}
	return updatedCustomer, nil
}

func (s *ChartMogulService) DeleteCustomer(customerUUID string) error {
	err := s.client.DeleteCustomer(customerUUID)
	if err != nil {
		s.logger.Errorw("ChartMogul DeleteCustomer failed", "error", err, "customerUUID", customerUUID)
	}
	return err
}

func (s *ChartMogulService) CreateSubscriptionEvent(event *cm.SubscriptionEvent) (*cm.SubscriptionEvent, error) {
	createdEvent, err := s.client.CreateSubscriptionEvent(event)
	if err != nil {
		s.logger.Errorw("ChartMogul CreateSubscriptionEvent failed", "error", err, "event", event)
		return nil, err
	}
	return createdEvent, nil
}

func (s *ChartMogulService) UpdateSubscriptionEvent(event *cm.SubscriptionEvent) (*cm.SubscriptionEvent, error) {
	updatedEvent, err := s.client.UpdateSubscriptionEvent(event)
	if err != nil {
		s.logger.Errorw("ChartMogul UpdateSubscriptionEvent failed", "error", err, "event", event)
		return nil, err
	}
	return updatedEvent, nil
}

func (s *ChartMogulService) DeleteSubscriptionEvent(deleteEvent *cm.DeleteSubscriptionEvent) error {
	err := s.client.DeleteSubscriptionEvent(deleteEvent)
	if err != nil {
		s.logger.Errorw("ChartMogul DeleteSubscriptionEvent failed", "error", err, "deleteEvent", deleteEvent)
	}
	return err
}

func (s *ChartMogulService) CreateTransaction(tx *cm.Transaction, invoiceUUID string) (*cm.Transaction, error) {
	createdTx, err := s.client.CreateTransaction(tx, invoiceUUID)
	if err != nil {
		s.logger.Errorw("ChartMogul CreateTransaction failed", "error", err, "invoiceUUID", invoiceUUID)
		return nil, err
	}
	return createdTx, nil
}

func (s *ChartMogulService) UpdateTransactionAmount(transactionUUID string, amountInCents int) error {
	// The SDK may not have a direct method for PATCH /transactions/{uuid}, so use the generic Patch method if available, or implement custom logic.
	// This is a placeholder for custom PATCH logic.
	// ...implement custom PATCH logic here if needed...
	return fmt.Errorf("UpdateTransactionAmount not implemented: use custom HTTP PATCH if SDK does not support")
}

func (s *ChartMogulService) SetTransactionDisabledState(transactionUUID string, disabled bool) error {
	// The SDK may not have a direct method for PATCH /transactions/{uuid}/disabled_state, so use the generic Patch method if available, or implement custom logic.
	// This is a placeholder for custom PATCH logic.
	// ...implement custom PATCH logic here if needed...
	return fmt.Errorf("SetTransactionDisabledState not implemented: use custom HTTP PATCH if SDK does not support")
}

func (s *ChartMogulService) DeleteTransaction(transactionUUID string) error {
	return fmt.Errorf("DeleteTransaction not implemented: use custom HTTP DELETE if SDK does not support")
}
