### Analytics API revamp
We need to modify analytics api to make user external_customer_id is an optional field

- analytic api should be work as it is
- `func (s *featureUsageTrackingService) GetDetailedUsageAnalytics` modify this function to make sure external_customer_id should be optional