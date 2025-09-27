# Feature-Level Alert System (Sentinel Alert Engine)

## 1. Executive Summary

### 1.1 Overview
The Sentinel Alert Engine introduces a comprehensive feature-level alert system that monitors feature usage against configurable thresholds and triggers alerts based on state transitions. This system extends the existing alert-logging infrastructure to provide granular monitoring of feature consumption across different aggregation types.

### 1.2 Objectives
- **Proactive Monitoring**: Enable real-time monitoring of feature usage against configurable thresholds
- **Flexible Alerting**: Support multiple alert states (OK, Warning, In Alarm) with customizable threshold ranges
- **Seamless Integration**: Leverage existing alert infrastructure and webhook system
- **Scalable Architecture**: Support high-volume feature usage monitoring across multiple tenants and environments

### 1.3 Success Metrics
- Reduce feature overage incidents by 80%
- Enable proactive customer engagement through usage alerts
- Provide real-time visibility into feature consumption patterns
- Support 99.9% alert delivery reliability

## 2. Problem Statement

### 2.1 Current State
- No proactive monitoring of feature usage consumption
- Customers experience unexpected overages without warning
- Manual monitoring required for feature usage tracking

### 2.2 Pain Points
- **Reactive Monitoring**: Alerts only trigger after usage limits are exceeded
- **Limited Granularity**: No feature-specific alert configurations
- **Poor User Experience**: Customers surprised by unexpected charges
- **Operational Overhead**: Manual monitoring of feature consumption

## 3. Solution Overview

### 3.1 System Architecture Overview

```mermaid
graph TB
    subgraph "Feature Usage Tracking"
        A[Feature Usage Events] --> B[Feature Usage Tracking Service]
        B --> C[Usage Calculation]
    end
    
    subgraph "Alert Configuration Layer"
        D[Feature Alert Configurations] --> E[Alert Settings Management]
        E --> F[Threshold Definitions]
    end
    
    subgraph "Alert Evaluation Engine"
        C --> G[Feature Usage Monitor]
        F --> G
        G --> H[State Determination Logic]
        H --> I{State Changed?}
    end
    
    subgraph "Alert Logging System"
        I -->|Yes| J[Create Alert Log Entry]
        I -->|No| K[Skip Alert Generation]
        J --> L[AlertLogs Table]
        L --> M[Webhook Event Trigger]
    end
    
    subgraph "Notification Layer"
        M --> N[Webhook Payload Builder]
        N --> O[Webhook Delivery]
        O --> P[External Systems]
    end
    
    style A fill:#e1f5fe
    style D fill:#f3e5f5
    style G fill:#fff3e0
    style L fill:#e8f5e8
    style P fill:#fce4ec
```

### 3.2 Core Components

#### 3.1.1 Feature Alert Configuration Table
A new table to store alert configurations at the feature level:

```sql
CREATE TABLE feature_alert_configurations (
    id VARCHAR(50) PRIMARY KEY,
    tenant_id VARCHAR(50) NOT NULL,
    environment_id VARCHAR(50) NOT NULL,
    feature_id VARCHAR(50) NOT NULL,
    meter_id VARCHAR(50) NOT NULL,
    entity VARCHAR(50) NOT NULL, -- entitlement_id, customer_id, subscription_id, etc.
    threshold JSONB NOT NULL, -- {upperbound: decimal, lowerbound: decimal}
    threshold_type VARCHAR(50) NOT NULL DEFAULT 'usage_amount',
    status VARCHAR(20) NOT NULL DEFAULT 'published',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    created_by VARCHAR(50),
    updated_by VARCHAR(50),
    
    CONSTRAINT fk_feature_alert_feature FOREIGN KEY (feature_id) REFERENCES features(id),
    CONSTRAINT fk_feature_alert_meter FOREIGN KEY (meter_id) REFERENCES meters(id),
    CONSTRAINT unique_feature_alert_config UNIQUE (tenant_id, environment_id, feature_id, meter_id, entity)
);
```

#### 3.1.2 Alert State Logic
The system supports three distinct alert states based on usage value comparison:

- **ok State**: `value > upperbound` - Usage is above the upper threshold (healthy)
- **warning State**: `lowerbound <= value <= upperbound` - Usage is within the warning range
- **in_alarm State**: `value < lowerbound` - Usage is below the lower threshold (critical)

#### 3.1.3 Threshold Configuration
```json
{
  "upperbound": "1000.00",
  "lowerbound": "100.00"
}
```

### 3.2 Architecture Integration

#### 3.2.1 Existing Infrastructure Reuse
- **AlertLogs Table**: Extend to support feature entity type
- **Webhook System**: Leverage existing webhook infrastructure
- **Alert Service**: Extend current alert service for feature monitoring

#### 3.2.2 New Components
- **FeatureAlertConfiguration Entity**: New Ent schema and domain model
- **Feature Alert Service**: Service layer for alert configuration management
- **Feature Usage Monitor**: Component to evaluate feature usage against thresholds
- **Feature Alert Webhook Payloads**: New webhook payload builders

## 4. Technical Specification

### 4.1 Data Models

#### 4.1.1 Feature Alert Configuration Schema
```go
// ent/schema/featurealertconfiguration.go
type FeatureAlertConfiguration struct {
    ent.Schema
}

func (FeatureAlertConfiguration) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Unique().Immutable(),
        field.String("feature_id").NotEmpty(),
        field.String("meter_id").NotEmpty(),
        field.String("entity").NotEmpty(),
        field.JSON("threshold", FeatureAlertThreshold{}),
        field.String("threshold_type").Default("usage_amount"),
    }
}

type FeatureAlertThreshold struct {
    Upperbound decimal.Decimal `json:"upperbound"`
    Lowerbound decimal.Decimal `json:"lowerbound"`
}
```

#### 4.1.2 Extended Alert Types
```go
// internal/types/alertlogs.go
const (
    // Existing wallet alerts
    AlertTypeLowOngoingBalance AlertType = "low_ongoing_balance"
    AlertTypeLowCreditBalance  AlertType = "low_credit_balance"
    
    // New feature alerts
    AlertTypeFeatureUsageThreshold AlertType = "feature_usage_threshold"
)

const (
    // Existing entity types
    AlertEntityTypeWallet AlertEntityType = "wallet"
    
    // New entity types for feature alerts
    AlertEntityTypeEntitlement AlertEntityType = "entitlement"
    AlertEntityTypeSubscription AlertEntityType = "subscription"
)

// Extended alert states
const (
    AlertStateOk      AlertState = "ok"
    AlertStateWarning AlertState = "warning"  // New state
    AlertStateInAlarm AlertState = "in_alarm"
)
```

#### 4.1.3 Feature Alert Info Structure
```go
type FeatureAlertInfo struct {
    FeatureID      string                `json:"feature_id"`
    MeterID        string                `json:"meter_id"`
    Entity         string                `json:"entity"` // entitlement_id, customer_id, etc.
    EntityType     string                `json:"entity_type"` // entitlement, customer, subscription, etc.
    Threshold      FeatureAlertThreshold `json:"threshold"`
    CurrentUsage   decimal.Decimal       `json:"current_usage"`
    AggregationType types.AggregationType `json:"aggregation_type"`
    Period         string                `json:"period"`
    Timestamp      time.Time             `json:"timestamp"`
}
```

### 4.2 Service Layer

#### 4.2.1 Feature Alert Configuration Service
```go
type FeatureAlertConfigurationService interface {
    CreateSetting(ctx context.Context, req *CreateFeatureAlertConfigRequest) (*FeatureAlertConfiguration, error)
    DeleteSetting(ctx context.Context, id string) error
    GetSetting(ctx context.Context, id string) (*FeatureAlertConfiguration, error)
    ListSettings(ctx context.Context, filter *FeatureAlertConfigFilter) (*ListFeatureAlertConfigResponse, error)
    GetSettingsByFeature(ctx context.Context, featureID string) ([]*FeatureAlertConfiguration, error)
    // No UpdateSetting method - mutations not allowed at feature level
}
```

#### 4.2.2 Feature Usage Monitor Service
```go
type FeatureUsageMonitorService interface {
    EvaluateFeatureUsage(ctx context.Context, featureID, entity, entityType string, currentUsage decimal.Decimal) error
    CheckAllFeatureAlerts(ctx context.Context) error // For cron jobs
    GetFeatureUsageForAlert(ctx context.Context, featureID, meterID, entity, entityType string) (decimal.Decimal, error)
}
```

### 4.3 Alert Evaluation Logic

#### 4.3.1 Usage Calculation
The system calculates current usage based on the meter's aggregation type:
- **SUM/COUNT**: Total accumulated usage in current period
- **MAX**: Maximum recorded value in current period  
- **LATEST**: Most recent recorded value
- **COUNT_UNIQUE**: Count of unique values in current period
- **AVERAGE**: Average value in current period

#### 4.3.2 State Determination Algorithm
```go
func DetermineAlertState(currentUsage decimal.Decimal, threshold FeatureAlertThreshold) AlertState {
    if currentUsage.GreaterThan(threshold.Upperbound) {
        return AlertStateOk
    } else if currentUsage.GreaterThanOrEqual(threshold.Lowerbound) && 
              currentUsage.LessThanOrEqual(threshold.Upperbound) {
        return AlertStateWarning
    } else {
        return AlertStateInAlarm
    }
}
```

#### 4.3.3 State Transition Logic
The system follows the existing alert engine pattern:
1. Calculate current feature usage for the specific entity (entitlement, customer, etc.)
2. Determine new alert state based on thresholds
3. Query latest alert log for this feature/entity combination
4. If state has changed or no previous alert exists:
   - Create new alert log entry with entity_type as the actual entity being monitored
   - Trigger appropriate webhook event
5. If state unchanged, skip alert generation

### 4.4 Alert Logging Integration Details

#### 4.4.1 AlertLogs Table Integration

```mermaid
erDiagram
    FEATURE_ALERT_CONFIGURATIONS {
        string id PK
        string tenant_id
        string environment_id
        string feature_id FK
        string meter_id FK
        string entity
        jsonb threshold
        string threshold_type
        string status
        timestamp created_at
        timestamp updated_at
    }
    
    ALERT_LOGS {
        string id PK
        string tenant_id
        string environment_id
        string entity_type
        string entity_id
        string alert_type
        string alert_status
        jsonb alert_info
        timestamp created_at
    }
    
    FEATURES {
        string id PK
        string name
        string meter_id FK
    }
    
    METERS {
        string id PK
        string event_name
        jsonb aggregation
    }
    
    FEATURE_ALERT_CONFIGURATIONS ||--|| FEATURES : "feature_id"
    FEATURE_ALERT_CONFIGURATIONS ||--|| METERS : "meter_id"
    ALERT_LOGS ||--o{ FEATURE_ALERT_CONFIGURATIONS : "triggers alerts for"
```

#### 4.4.2 Alert Logging Workflow

```mermaid
sequenceDiagram
    participant FU as Feature Usage Service
    participant FAM as Feature Alert Monitor
    participant AL as AlertLogs Service
    participant WH as Webhook Handler
    
    FU->>FAM: Feature usage event
    FAM->>FAM: Get alert configurations
    FAM->>FAM: Calculate current usage
    FAM->>FAM: Determine alert state
    
    alt State Changed
        FAM->>AL: Create alert log entry
        Note over AL: entity_type = "entitlement"<br/>entity_id = "ent_123"<br/>alert_type = "feature_usage_threshold"<br/>alert_status = "warning"
        AL->>AL: Save to AlertLogs table
        AL->>WH: Trigger webhook event
        WH->>WH: Build webhook payload
        WH->>WH: Send webhook
    else No State Change
        FAM->>FAM: Skip alert generation
    end
```

### 4.5 Webhook Integration

#### 4.5.1 New Webhook Events
```go
// internal/types/webhook.go
const (
    // Feature alert events
    WebhookEventFeatureUsageThresholdOk      = "feature.usage.threshold.ok"
    WebhookEventFeatureUsageThresholdWarning = "feature.usage.threshold.warning"
    WebhookEventFeatureUsageThresholdAlarm   = "feature.usage.threshold.alarm"
)
```

#### 4.4.2 Feature Alert Webhook Payload
```go
type FeatureAlertWebhookPayload struct {
    EventType       string                `json:"event_type"`
    FeatureID       string                `json:"feature_id"`
    FeatureName     string                `json:"feature_name"`
    MeterID         string                `json:"meter_id"`
    Entity          string                `json:"entity"` // entitlement_id, customer_id, etc.
    EntityType      string                `json:"entity_type"` // entitlement, customer, subscription, etc.
    AlertState      types.AlertState      `json:"alert_state"`
    CurrentUsage    decimal.Decimal       `json:"current_usage"`
    Threshold       FeatureAlertThreshold `json:"threshold"`
    AggregationType types.AggregationType `json:"aggregation_type"`
    Period          string                `json:"period"`
    Timestamp       time.Time             `json:"timestamp"`
    TenantID        string                `json:"tenant_id"`
    EnvironmentID   string                `json:"environment_id"`
}
```

### 4.5 API Endpoints

#### 4.5.1 Feature Alert Configuration Management
```go
// POST /api/v1/feature-alert-setting
type CreateFeatureAlertConfigRequest struct {
    FeatureID     string                `json:"feature_id" validate:"required"`
    MeterID       string                `json:"meter_id" validate:"required"`
    Entity        string                `json:"entity" validate:"required"` // entitlement_id, subscription_id, etc.
    EntityType    string                `json:"entity_type" validate:"required"` // entitlement, subscription, etc.
    Threshold     FeatureAlertThreshold `json:"threshold" validate:"required"`
    ThresholdType string                `json:"threshold_type,omitempty"`
}

// DELETE /api/v1/feature-alert-setting/{id}
// No update endpoint - mutations not allowed at feature level

// GET /api/v1/feature-alert-setting
type FeatureAlertConfigFilter struct {
    *types.QueryFilter
    FeatureID  string `json:"feature_id,omitempty" form:"feature_id"`
    MeterID    string `json:"meter_id,omitempty" form:"meter_id"`
    Entity     string `json:"entity,omitempty" form:"entity"`
    EntityType string `json:"entity_type,omitempty" form:"entity_type"`
}
```

## 5. Implementation Plan

### 5.1 Implementation Workflow with Alert Logging Integration

```mermaid
gantt
    title Sentinel Alert Engine Implementation Timeline
    dateFormat  YYYY-MM-DD
    section Phase 1: Core Infrastructure
    Create FeatureAlertConfiguration Schema    :p1-1, 2024-01-01, 3d
    Implement Domain Models                   :p1-2, after p1-1, 2d
    Extend AlertLogs for Feature Alerts       :p1-3, after p1-2, 3d
    Update Alert Types & States               :p1-4, after p1-3, 2d
    
    section Phase 2: Service Layer
    FeatureAlertConfigurationService          :p2-1, after p1-4, 4d
    FeatureUsageMonitorService               :p2-2, after p2-1, 3d
    AlertLogsService Extension               :p2-3, after p2-2, 3d
    Alert Evaluation Logic                   :p2-4, after p2-3, 3d
    
    section Phase 3: API Layer
    Alert Configuration APIs                 :p3-1, after p2-4, 3d
    Request/Response DTOs                   :p3-2, after p3-1, 2d
    Validation & Error Handling             :p3-3, after p3-2, 2d
    
    section Phase 4: Webhook Integration
    Webhook Payload Builders                :p4-1, after p3-3, 3d
    Webhook Event Types                     :p4-2, after p4-1, 2d
    Webhook Handlers                        :p4-3, after p4-2, 3d
```

### 5.2 Phase 1: Core Infrastructure (Week 1-2)
- [ ] Create FeatureAlertConfiguration Ent schema
- [ ] Implement domain models and repositories
- [ ] Extend AlertLogs to support feature entity type
- [ ] Update alert types and states in types package

### 5.2 Phase 2: Service Layer (Week 3-4)
- [ ] Implement FeatureAlertConfigurationService
- [ ] Create FeatureUsageMonitorService
- [ ] Extend AlertLogsService for feature alerts
- [ ] Implement alert evaluation logic

### 5.3 Phase 3: API Layer (Week 5)
- [ ] Create feature alert configuration API endpoints
- [ ] Implement request/response DTOs
- [ ] Add validation and error handling
- [ ] Create API documentation

### 5.4 Phase 4: Webhook Integration (Week 6)
- [ ] Create feature alert webhook payload builders
- [ ] Extend webhook event types
- [ ] Implement feature alert webhook handlers
- [ ] Test webhook delivery

### 5.5 Phase 5: Monitoring Integration (Week 7-8)
- [ ] Integrate with feature usage tracking service
- [ ] Implement real-time alert evaluation
- [ ] Create cron job for periodic alert checks
- [ ] Add monitoring and logging

### 5.6 Phase 6: Testing & Documentation (Week 9-10)
- [ ] Comprehensive unit and integration tests
- [ ] Performance testing with high-volume scenarios
- [ ] API documentation and examples
- [ ] User guide and troubleshooting documentation

## 6. Integration Points

### 6.1 Feature Usage Tracking Integration
- Hook into existing feature usage calculation pipeline
- Trigger alert evaluation on usage updates
- Support all aggregation types (SUM, MAX, COUNT, etc.)

### 6.2 Existing Alert Infrastructure
- Reuse AlertLogs table with new entity type
- Leverage existing webhook delivery system
- Maintain consistent alert state transition logic

### 6.3 Cron Job Integration
- Extend existing cron infrastructure
- Periodic evaluation of all feature alert configurations
- Catch any missed real-time evaluations

### 6.4 Complete Data Flow Integration

```mermaid
graph LR
    subgraph "Event Sources"
        A[Usage Events] --> B[Feature Usage Tracking]
        C[Periodic Cron] --> D[Batch Evaluation]
    end
    
    subgraph "Alert Configuration Layer"
        E[Feature Alert Configs] --> F[Alert Settings]
        F --> G[Threshold Definitions]
    end
    
    subgraph "Alert Evaluation Engine"
        B --> H[Real-time Monitor]
        D --> I[Batch Monitor]
        G --> H
        G --> I
        H --> J[State Determination]
        I --> J
    end
    
    subgraph "Alert Logging System"
        J --> K{State Changed?}
        K -->|Yes| L[Create AlertLog Entry]
        K -->|No| M[Skip Alert]
        L --> N[AlertLogs Table]
        N --> O[Webhook Trigger]
    end
    
    subgraph "Notification & Monitoring"
        O --> P[Webhook Delivery]
        P --> Q[External Systems]
        N --> R[Alert Analytics]
        R --> S[Dashboard & Reports]
    end
    
    style A fill:#e1f5fe
    style E fill:#f3e5f5
    style J fill:#fff3e0
    style N fill:#e8f5e8
    style Q fill:#fce4ec
```

### 6.5 AlertLogs Table Schema Integration

```mermaid
graph TB
    subgraph "AlertLogs Table Structure"
        A[AlertLogs Table] --> B[entity_type: entitlement]
        A --> C[entity_id: entitlement_123]
        A --> D[alert_type: feature_usage_threshold]
        A --> E[alert_status: warning/ok/in_alarm]
        A --> F[alert_info: JSON with feature details]
    end
    
    subgraph "Feature Alert Integration"
        G[Feature Alert Config] --> H[feature_id: feature_456]
        G --> I[meter_id: meter_789]
        G --> J[entity: entitlement_123]
        G --> K[threshold: upper/lower bounds]
    end
    
    subgraph "Alert Log Creation"
        L[Usage Event] --> M[Calculate Usage]
        M --> N[Compare with Threshold]
        N --> O[Determine State]
        O --> P[Create AlertLog Entry]
        P --> Q[Set entity_type = entitlement]
        P --> R[Set entity_id = entitlement_123]
        P --> S[Set alert_type = feature_usage_threshold]
        P --> T[Set alert_status = determined_state]
    end
    
    style A fill:#e8f5e8
    style G fill:#f3e5f5
    style P fill:#fff3e0
```

## 7. Security & Performance Considerations

### 7.1 Security
- **Tenant Isolation**: All alert configurations scoped to tenant/environment
- **Access Control**: Feature alert configuration requires appropriate permissions
- **Data Protection**: No sensitive data in webhook payloads
- **Audit Trail**: Complete audit log of configuration changes

### 7.2 Performance
- **Efficient Queries**: Optimized database queries with proper indexing
- **Caching Strategy**: Cache frequently accessed configurations
- **Batch Processing**: Efficient bulk alert evaluation for cron jobs
- **Rate Limiting**: Prevent webhook spam with intelligent rate limiting

### 7.3 Scalability
- **Horizontal Scaling**: Stateless service design for easy scaling
- **Database Optimization**: Proper indexing and query optimization
- **Async Processing**: Non-blocking alert evaluation and webhook delivery
- **Resource Management**: Configurable limits and throttling

## 8. Monitoring & Observability

### 8.1 Metrics
- Alert evaluation latency
- Webhook delivery success rate
- Configuration creation/update rates
- Alert state transition frequencies

### 8.2 Logging
- Structured logging for all alert evaluations
- Detailed webhook delivery logs
- Configuration change audit logs
- Error tracking and alerting

### 8.3 Health Checks
- Service health endpoints
- Database connectivity checks
- Webhook delivery system health
- Alert evaluation pipeline status

## 9. Testing Strategy

### 9.1 Unit Tests
- Alert state determination logic
- Threshold validation
- Usage calculation accuracy
- Service layer functionality

### 9.2 Integration Tests
- End-to-end alert flow
- Webhook delivery verification
- Database operations
- API endpoint functionality

### 9.3 Performance Tests
- High-volume alert evaluation
- Concurrent configuration management
- Webhook delivery under load
- Database performance with large datasets

## 10. Migration & Rollout

### 10.1 Database Migration
- Create new tables with proper constraints
- Add indexes for optimal performance
- Ensure backward compatibility

### 10.2 Feature Flags
- Gradual rollout with feature flags
- Tenant-by-tenant enablement
- Easy rollback capability

### 10.3 Monitoring
- Real-time monitoring during rollout
- Performance impact assessment
- User feedback collection

## 11. Future Enhancements

### 11.1 Advanced Alerting
- Multi-condition alerts (AND/OR logic)
- Time-based alert suppression
- Alert escalation policies
- Custom alert templates

### 11.2 Analytics Integration
- Alert effectiveness analytics
- Usage pattern analysis
- Predictive alerting capabilities
- Custom dashboard integration

### 11.3 External Integrations
- Slack/Teams notifications
- PagerDuty integration
- Email alert delivery
- SMS notifications

## 12. Success Criteria

### 12.1 Functional Requirements
- ✅ Support all three alert states (OK, Warning, In Alarm)
- ✅ Real-time alert evaluation on usage updates
- ✅ Configurable thresholds per feature/entity combination
- ✅ Seamless webhook integration
- ✅ Comprehensive API for configuration management

### 12.2 Non-Functional Requirements
- ✅ 99.9% alert delivery reliability
- ✅ Sub-second alert evaluation latency
- ✅ Support for 10,000+ concurrent alert configurations
- ✅ Zero-downtime deployments
- ✅ Complete audit trail and observability

### 12.3 Business Impact
- ✅ 80% reduction in feature overage incidents
- ✅ Improved customer satisfaction through proactive alerts
- ✅ Enhanced operational visibility into feature usage
- ✅ Reduced support tickets related to unexpected charges

---

## Appendix A: Database Schema

### Feature Alert Configuration Table
```sql
CREATE TABLE feature_alert_configurations (
    id VARCHAR(50) PRIMARY KEY,
    tenant_id VARCHAR(50) NOT NULL,
    environment_id VARCHAR(50) NOT NULL,
    feature_id VARCHAR(50) NOT NULL,
    meter_id VARCHAR(50) NOT NULL,
    entity VARCHAR(50) NOT NULL,
    threshold JSONB NOT NULL,
    threshold_type VARCHAR(50) NOT NULL DEFAULT 'usage_amount',
    status VARCHAR(20) NOT NULL DEFAULT 'published',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    created_by VARCHAR(50),
    updated_by VARCHAR(50),
    
    CONSTRAINT fk_feature_alert_feature FOREIGN KEY (feature_id) REFERENCES features(id),
    CONSTRAINT fk_feature_alert_meter FOREIGN KEY (meter_id) REFERENCES meters(id),
    CONSTRAINT unique_feature_alert_config UNIQUE (tenant_id, environment_id, feature_id, meter_id, entity)
);

CREATE INDEX idx_feature_alert_config_tenant_env ON feature_alert_configurations(tenant_id, environment_id);
CREATE INDEX idx_feature_alert_config_feature ON feature_alert_configurations(feature_id);
CREATE INDEX idx_feature_alert_config_meter ON feature_alert_configurations(meter_id);
CREATE INDEX idx_feature_alert_config_entity ON feature_alert_configurations(entity);
```

## Appendix B: API Examples

### Create Feature Alert Configuration
```bash
curl -X POST /api/v1/feature-alert-setting \
  -H "Content-Type: application/json" \
  -d '{
    "feature_id": "feature_123",
    "meter_id": "meter_456",
    "entity": "entitlement_789",
    "entity_type": "entitlement",
    "threshold": {
      "upperbound": "1000.00",
      "lowerbound": "100.00"
    },
    "threshold_type": "usage_amount"
  }'
```

### List Feature Alert Configurations
```bash
curl -X GET /api/v1/feature-alert-setting?feature_id=feature_123&entity_type=entitlement
```

### Webhook Payload Example
```json
{
  "event_type": "feature.usage.threshold.warning",
  "feature_id": "feature_123",
  "feature_name": "API Calls",
  "meter_id": "meter_456",
  "entity": "entitlement_789",
  "entity_type": "entitlement",
  "alert_state": "warning",
  "current_usage": "750.00",
  "threshold": {
    "upperbound": "1000.00",
    "lowerbound": "100.00"
  },
  "aggregation_type": "sum",
  "period": "2024-01",
  "timestamp": "2024-01-15T10:30:00Z",
  "tenant_id": "tenant_123",
  "environment_id": "env_456"
}
```

## Appendix C: Alert Logging Integration Examples

### AlertLogs Table Entry Example
```sql
INSERT INTO alert_logs (
    id,
    tenant_id,
    environment_id,
    entity_type,
    entity_id,
    alert_type,
    alert_status,
    alert_info,
    created_at
) VALUES (
    'alert_12345',
    'tenant_123',
    'env_456',
    'entitlement',
    'entitlement_789',
    'feature_usage_threshold',
    'warning',
    '{
        "feature_id": "feature_123",
        "meter_id": "meter_456",
        "entity": "entitlement_789",
        "entity_type": "entitlement",
        "threshold": {
            "upperbound": "1000.00",
            "lowerbound": "100.00"
        },
        "current_usage": "750.00",
        "aggregation_type": "sum",
        "period": "2024-01",
        "timestamp": "2024-01-15T10:30:00Z"
    }',
    NOW()
);
```

### Alert State Transition Example
```mermaid
sequenceDiagram
    participant U as Usage Event
    participant M as Monitor
    participant A as AlertLogs
    participant W as Webhook
    
    Note over U,W: Feature Usage: 750 (Warning State)
    
    U->>M: Usage event (750 units)
    M->>M: Get alert configs for feature_123
    M->>M: Calculate current usage: 750
    M->>M: Determine state: WARNING
    M->>A: Query latest alert for feature_123 + entitlement_789
    A-->>M: Previous state: OK
    
    Note over M: State changed: OK → WARNING
    
    M->>A: Create alert log entry
    Note over A: entity_type = "entitlement"<br/>entity_id = "entitlement_789"<br/>alert_type = "feature_usage_threshold"<br/>alert_status = "warning"
    
    A->>W: Trigger webhook event
    W->>W: Build webhook payload
    W->>W: Send webhook notification
```
