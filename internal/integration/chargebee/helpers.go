package chargebee

import "time"

// Helper functions shared across Chargebee services

// boolPtr converts a bool to a pointer
func boolPtr(b bool) *bool {
	return &b
}

// int32Ptr converts an int32 to a pointer
func int32Ptr(i int32) *int32 {
	return &i
}

// int64Ptr converts an int64 to a pointer
func int64Ptr(i int64) *int64 {
	return &i
}

// timestampToTime converts a Unix timestamp (int64) to time.Time
func timestampToTime(ts int64) time.Time {
	return time.Unix(ts, 0)
}

// intPtr converts an int32 to a pointer (for itemfamily.go)
func intPtr(i int32) *int32 {
	return &i
}
