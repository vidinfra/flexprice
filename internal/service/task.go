package service

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
)

type TaskService interface {
	CreateTask(ctx context.Context, req dto.CreateTaskRequest) (*dto.TaskResponse, error)
	GetTask(ctx context.Context, id string) (*dto.TaskResponse, error)
	ListTasks(ctx context.Context, filter *types.TaskFilter) (*dto.ListTasksResponse, error)
	UpdateTaskStatus(ctx context.Context, id string, status types.TaskStatus) error
	ProcessTaskWithStreaming(ctx context.Context, id string) error
}

type taskService struct {
	ServiceParams
	fileProcessor *FileProcessor
}

func NewTaskService(
	serviceParams ServiceParams,
) TaskService {
	return &taskService{
		ServiceParams: serviceParams,
		fileProcessor: NewFileProcessor(serviceParams.Client, serviceParams.Logger),
	}
}

func (s *taskService) CreateTask(ctx context.Context, req dto.CreateTaskRequest) (*dto.TaskResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	t := req.ToTask(ctx)
	if err := t.Validate(); err != nil {
		return nil, err
	}

	if err := s.TaskRepo.Create(ctx, t); err != nil {
		s.Logger.Error("failed to create task", "error", err)
		return nil, err
	}

	// Task is created and ready for processing
	// The API layer will handle starting the temporal workflow

	return dto.NewTaskResponse(t), nil
}

func (s *taskService) GetTask(ctx context.Context, id string) (*dto.TaskResponse, error) {
	t, err := s.TaskRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return dto.NewTaskResponse(t), nil
}

func (s *taskService) ListTasks(ctx context.Context, filter *types.TaskFilter) (*dto.ListTasksResponse, error) {
	tasks, err := s.TaskRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	count, err := s.TaskRepo.Count(ctx, filter)
	if err != nil {
		return nil, err
	}

	items := make([]*dto.TaskResponse, len(tasks))
	for i, t := range tasks {
		items[i] = dto.NewTaskResponse(t)
	}

	return &dto.ListTasksResponse{
		Items: items,
		Pagination: types.PaginationResponse{
			Total:  count,
			Limit:  filter.GetLimit(),
			Offset: filter.GetOffset(),
		},
	}, nil
}

func (s *taskService) UpdateTaskStatus(ctx context.Context, id string, status types.TaskStatus) error {
	t, err := s.TaskRepo.Get(ctx, id)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to get task").
			Mark(ierr.ErrValidation)
	}

	// Validate status transition
	if !isValidStatusTransition(t.TaskStatus, status) {
		return ierr.NewError(fmt.Sprintf("invalid status transition from %s to %s", t.TaskStatus, status)).
			WithHint("Invalid status transition").
			WithReportableDetails(map[string]interface{}{
				"from": t.TaskStatus,
				"to":   status,
			}).
			Mark(ierr.ErrValidation)
	}

	now := time.Now().UTC()
	t.TaskStatus = status
	switch status {
	case types.TaskStatusProcessing:
		t.StartedAt = &now
	case types.TaskStatusCompleted:
		t.CompletedAt = &now
	case types.TaskStatusFailed:
		t.FailedAt = &now
	}

	if err := s.TaskRepo.Update(ctx, t); err != nil {
		return ierr.WithError(err).
			WithHint("Failed to update task").
			Mark(ierr.ErrValidation)
	}

	return nil
}

// Helper functions

func isValidStatusTransition(from, to types.TaskStatus) bool {
	allowedTransitions := map[types.TaskStatus][]types.TaskStatus{
		types.TaskStatusPending: {
			types.TaskStatusProcessing,
			types.TaskStatusFailed,
		},
		types.TaskStatusProcessing: {
			types.TaskStatusCompleted,
			types.TaskStatusFailed,
		},
		types.TaskStatusFailed: {
			types.TaskStatusProcessing,
		},
	}

	allowed, ok := allowedTransitions[from]
	if !ok {
		return false
	}

	for _, status := range allowed {
		if status == to {
			return true
		}
	}

	return false
}

// ProcessTaskWithStreaming processes a task using streaming for large files
func (s *taskService) ProcessTaskWithStreaming(ctx context.Context, id string) error {
	// Update status to processing
	if err := s.UpdateTaskStatus(ctx, id, types.TaskStatusProcessing); err != nil {
		return ierr.WithError(err).
			WithHint("Failed to update task status").
			Mark(ierr.ErrValidation)
	}

	// Get task
	t, err := s.TaskRepo.Get(ctx, id)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to get task").
			Mark(ierr.ErrValidation)
	}

	// Create a context with extended timeout for streaming file processing
	// This ensures we have enough time for large file downloads and processing
	processingCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	// Use the file processor for streaming
	streamingProcessor := s.fileProcessor.StreamingProcessor

	// Process based on entity type
	var processor ChunkProcessor
	switch t.EntityType {
	case types.EntityTypeEvents:
		// Create event service for chunk processor
		eventSvc := NewEventService(
			s.EventRepo,
			s.MeterRepo,
			s.EventPublisher,
			s.Logger,
			s.Config,
		)
		processor = &EventsChunkProcessor{
			eventService: eventSvc,
			logger:       s.Logger,
		}
	case types.EntityTypeCustomers:
		// Create customer service for chunk processor
		customerSvc := NewCustomerService(s.ServiceParams)
		processor = &CustomersChunkProcessor{
			customerService: customerSvc,
			logger:          s.Logger,
		}
	default:
		return ierr.NewError("unsupported entity type").
			WithHint("Unsupported entity type").
			WithReportableDetails(map[string]interface{}{
				"entity_type": t.EntityType,
			}).
			Mark(ierr.ErrInvalidOperation)
	}

	// Process file with streaming using the extended context
	config := DefaultStreamingConfig()
	err = streamingProcessor.ProcessFileStream(processingCtx, t, processor, config)
	if err != nil {
		// Update task status to failed
		if updateErr := s.UpdateTaskStatus(ctx, id, types.TaskStatusFailed); updateErr != nil {
			s.Logger.Error("failed to update task status", "error", updateErr)
		}
		return err
	}

	// Update task status to completed
	if err := s.UpdateTaskStatus(ctx, id, types.TaskStatusCompleted); err != nil {
		s.Logger.Error("failed to update task status", "error", err)
		return err
	}

	return nil
}

// EventsChunkProcessor processes chunks of event data
type EventsChunkProcessor struct {
	eventService EventService
	logger       *logger.Logger
}

// ProcessChunk processes a chunk of event records
func (p *EventsChunkProcessor) ProcessChunk(ctx context.Context, chunk [][]string, headers []string, chunkIndex int) (*ChunkResult, error) {
	processedRecords := 0
	successfulRecords := 0
	failedRecords := 0
	var errors []string

	// Batch process events for better performance
	var eventRequests []*dto.IngestEventRequest

	// Parse all records in the chunk
	for i, record := range chunk {
		processedRecords++

		// Create event request
		eventReq := &dto.IngestEventRequest{
			Properties: make(map[string]interface{}),
		}

		// Map standard fields
		for j, header := range headers {
			if j >= len(record) {
				continue
			}
			value := record[j]
			switch header {
			case "event_name":
				eventReq.EventName = value
			case "external_customer_id":
				eventReq.ExternalCustomerID = value
			case "customer_id":
				eventReq.CustomerID = value
			case "timestamp":
				eventReq.TimestampStr = value
			case "source":
				eventReq.Source = value
			}
		}

		// Parse property fields (headers starting with "properties.")
		for j, header := range headers {
			if j >= len(record) {
				continue
			}
			if strings.HasPrefix(header, "properties.") {
				propertyName := strings.TrimPrefix(header, "properties.")
				eventReq.Properties[propertyName] = record[j]
			}
		}

		// Validate the event request
		if err := eventReq.Validate(); err != nil {
			errors = append(errors, fmt.Sprintf("Record %d: %v", i, err))
			failedRecords++
			continue
		}

		// Parse timestamp
		if err := p.parseTimestamp(eventReq); err != nil {
			errors = append(errors, fmt.Sprintf("Record %d: timestamp error: %v", i, err))
			failedRecords++
			continue
		}

		eventRequests = append(eventRequests, eventReq)
	}

	// Batch create events
	if len(eventRequests) > 0 {
		successCount, err := p.batchCreateEvents(ctx, eventRequests)
		if err != nil {
			// If batch creation fails, try individual creation
			p.logger.Warn("batch event creation failed, falling back to individual creation", "error", err)
			for _, eventReq := range eventRequests {
				if err := p.eventService.CreateEvent(ctx, eventReq); err != nil {
					errors = append(errors, fmt.Sprintf("Event creation failed: %v", err))
					failedRecords++
				} else {
					successfulRecords++
				}
			}
		} else {
			successfulRecords += successCount
			failedRecords += len(eventRequests) - successCount
		}
	}

	// Create error summary if there are errors
	var errorSummary *string
	if len(errors) > 0 {
		summary := strings.Join(errors, "; ")
		errorSummary = &summary
	}

	return &ChunkResult{
		ProcessedRecords:  processedRecords,
		SuccessfulRecords: successfulRecords,
		FailedRecords:     failedRecords,
		ErrorSummary:      errorSummary,
	}, nil
}

// parseTimestamp parses the timestamp string
func (p *EventsChunkProcessor) parseTimestamp(eventReq *dto.IngestEventRequest) error {
	if eventReq.TimestampStr != "" {
		timestamp, err := time.Parse(time.RFC3339, eventReq.TimestampStr)
		if err != nil {
			return fmt.Errorf("invalid timestamp format: %w", err)
		}
		eventReq.Timestamp = timestamp
	} else {
		eventReq.Timestamp = time.Now()
	}
	return nil
}

// batchCreateEvents creates multiple events in a batch
func (p *EventsChunkProcessor) batchCreateEvents(ctx context.Context, events []*dto.IngestEventRequest) (int, error) {
	successCount := 0
	for _, eventReq := range events {
		if err := p.eventService.CreateEvent(ctx, eventReq); err != nil {
			p.logger.Error("failed to create event in batch", "error", err)
			continue
		}
		successCount++
	}
	return successCount, nil
}

// CustomersChunkProcessor processes chunks of customer data
type CustomersChunkProcessor struct {
	customerService CustomerService
	logger          *logger.Logger
}

// ProcessChunk processes a chunk of customer records
func (p *CustomersChunkProcessor) ProcessChunk(ctx context.Context, chunk [][]string, headers []string, chunkIndex int) (*ChunkResult, error) {
	processedRecords := 0
	successfulRecords := 0
	failedRecords := 0
	var errors []string

	// Process each record in the chunk
	for i, record := range chunk {
		processedRecords++

		// Create customer request
		customerReq := &dto.CreateCustomerRequest{
			Metadata: make(map[string]string),
		}

		// Map standard fields
		for j, header := range headers {
			if j >= len(record) {
				continue
			}
			value := record[j]
			switch header {
			case "external_id":
				customerReq.ExternalID = value
			case "name":
				customerReq.Name = value
			case "email":
				customerReq.Email = value
			case "address_line1":
				customerReq.AddressLine1 = value
			case "address_line2":
				customerReq.AddressLine2 = value
			case "address_city":
				customerReq.AddressCity = value
			case "address_state":
				customerReq.AddressState = value
			case "address_postal_code":
				customerReq.AddressPostalCode = value
			case "address_country":
				customerReq.AddressCountry = value
			}
		}

		// Parse metadata fields (headers starting with "metadata.")
		for j, header := range headers {
			if j >= len(record) {
				continue
			}
			if strings.HasPrefix(header, "metadata.") {
				metadataKey := strings.TrimPrefix(header, "metadata.")
				customerReq.Metadata[metadataKey] = record[j]
			}
		}

		// Validate the customer request
		if err := customerReq.Validate(); err != nil {
			errors = append(errors, fmt.Sprintf("Record %d: %v", i, err))
			failedRecords++
			continue
		}

		// Process the customer (create or update)
		if err := p.processCustomer(ctx, customerReq); err != nil {
			errors = append(errors, fmt.Sprintf("Record %d: %v", i, err))
			failedRecords++
			continue
		}

		successfulRecords++
	}

	// Create error summary if there are errors
	var errorSummary *string
	if len(errors) > 0 {
		summary := strings.Join(errors, "; ")
		errorSummary = &summary
	}

	return &ChunkResult{
		ProcessedRecords:  processedRecords,
		SuccessfulRecords: successfulRecords,
		FailedRecords:     failedRecords,
		ErrorSummary:      errorSummary,
	}, nil
}

// processCustomer processes a single customer (create or update)
func (p *CustomersChunkProcessor) processCustomer(ctx context.Context, customerReq *dto.CreateCustomerRequest) error {
	// Check if customer with this external ID already exists
	if customerReq.ExternalID != "" {
		customer, err := p.customerService.GetCustomer(ctx, customerReq.ExternalID)
		if err != nil {
			p.logger.Error("failed to search for existing customer", "external_id", customerReq.ExternalID, "error", err)
			return fmt.Errorf("failed to search for existing customer: %w", err)
		}

		// If customer exists, update it
		if customer != nil && customer.Status == types.StatusPublished {
			existingCustomer := customer

			// Create update request from create request
			updateReq := dto.UpdateCustomerRequest{}

			// Only update fields that are different
			if existingCustomer.ExternalID != customerReq.ExternalID {
				updateReq.ExternalID = &customerReq.ExternalID
			}
			if existingCustomer.Email != customerReq.Email {
				updateReq.Email = &customerReq.Email
			}
			if existingCustomer.Name != customerReq.Name {
				updateReq.Name = &customerReq.Name
			}
			if existingCustomer.AddressLine1 != customerReq.AddressLine1 {
				updateReq.AddressLine1 = &customerReq.AddressLine1
			}
			if existingCustomer.AddressLine2 != customerReq.AddressLine2 {
				updateReq.AddressLine2 = &customerReq.AddressLine2
			}
			if existingCustomer.AddressCity != customerReq.AddressCity {
				updateReq.AddressCity = &customerReq.AddressCity
			}
			if existingCustomer.AddressState != customerReq.AddressState {
				updateReq.AddressState = &customerReq.AddressState
			}
			if existingCustomer.AddressPostalCode != customerReq.AddressPostalCode {
				updateReq.AddressPostalCode = &customerReq.AddressPostalCode
			}
			if existingCustomer.AddressCountry != customerReq.AddressCountry {
				updateReq.AddressCountry = &customerReq.AddressCountry
			}

			// Merge metadata
			mergedMetadata := make(map[string]string)
			maps.Copy(mergedMetadata, existingCustomer.Metadata)
			maps.Copy(mergedMetadata, customerReq.Metadata)
			updateReq.Metadata = mergedMetadata

			// Update the customer
			_, err := p.customerService.UpdateCustomer(ctx, existingCustomer.ID, updateReq)
			if err != nil {
				p.logger.Error("failed to update customer", "customer_id", existingCustomer.ID, "error", err)
				return fmt.Errorf("failed to update customer: %w", err)
			}

			p.logger.Info("updated existing customer", "customer_id", existingCustomer.ID, "external_id", customerReq.ExternalID)
			return nil
		}
	}

	// If no existing customer found, create a new one
	_, err := p.customerService.CreateCustomer(ctx, *customerReq)
	if err != nil {
		p.logger.Error("failed to create customer", "error", err)
		return fmt.Errorf("failed to create customer: %w", err)
	}

	p.logger.Info("created new customer", "external_id", customerReq.ExternalID)
	return nil
}
