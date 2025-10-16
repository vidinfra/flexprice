# Sync Logic Product Requirements Document (PRD)

## Overview
This document outlines the data synchronization logic for scheduled export tasks, covering three distinct execution flows: Force Runs, First Runs, and Incremental Runs.

## Core Concepts

### Time Intervals
The system supports multiple sync intervals with a 15-minute buffer for data ingestion:

| Interval | Cron Schedule | Buffer | Description |
|----------|---------------|--------|-------------|
| **Testing** | `*/10 * * * *` | None | Every 10 minutes (no buffer for testing) |
| **Hourly** | `15 * * * *` | 15 min | Every hour at 15 minutes past |
| **Daily** | `15 0 * * *` | 15 min | Every day at 00:15 AM |
| **Weekly** | `15 0 * * 1` | 15 min | Every Monday at 00:15 AM |
| **Monthly** | `15 0 1 * *` | 15 min | First day of every month at 00:15 AM |
| **Yearly** | `15 0 1 1 *` | 15 min | First day of every year at 00:15 AM |

### Interval Calculation Logic
The `CalculateIntervalBoundaries` function returns the **current interval** based on the current time:

| Current Time | Interval | Returns |
|--------------|----------|---------|
| 10:30 AM | Hourly | 10:00 AM → 11:00 AM |
| 2:07 PM | Testing | 2:00 PM → 2:10 PM |
| Oct 16, 3:45 PM | Daily | Oct 16 00:00 → Oct 17 00:00 |
| Thursday, Oct 16 | Weekly | Mon Oct 13 00:00 → Mon Oct 20 00:00 |

## Three Execution Flows

### 1. Force Run (Manual Trigger)
**Triggered by:** User action via API  
**ScheduledTaskID:** Empty (isolated from scheduled tasks)  
**Purpose:** Immediate data export for specific time range

#### Behavior:
- **Uses current interval** from `CalculateIntervalBoundaries`
- **Exports immediately** without waiting for cron schedule
- **Not tracked** by incremental sync logic
- **User context preserved** in audit fields

#### Example:
```
User triggers force run at 10:30 AM for hourly sync
→ Exports 10:00 AM - 11:00 AM (current interval)
→ Creates task with ScheduledTaskID = ""
→ User ID stored in created_by/updated_by fields
```

### 2. First Run (Scheduled Task Creation)
**Triggered by:** Cron schedule (first execution)  
**ScheduledTaskID:** Actual scheduled task ID  
**Purpose:** Initial data export for new scheduled task

#### Behavior:
- **Cron runs 15 minutes after interval boundary**
- **Exports previous completed interval** (not current)
- **No previous export found** in database
- **System context** (empty user fields)

#### Example:
```
Scheduled task created for hourly sync
Cron runs at 11:15 AM (15 min after 11:00 AM boundary)
→ Current interval: 11:00 AM - 12:00 PM
→ Exports: 10:00 AM - 11:00 AM (previous completed)
→ Creates task with ScheduledTaskID = "scheduled_task_123"
```

### 3. Incremental Run (Subsequent Executions)
**Triggered by:** Cron schedule (subsequent executions)  
**ScheduledTaskID:** Actual scheduled task ID  
**Purpose:** Export data since last successful export

#### Behavior:
- **Finds last successful export** in database
- **Uses last export's end time** as new start time
- **Uses current interval start** as new end time
- **Creates continuous data chain**

#### Example:
```
Last export: 10:00 AM - 11:00 AM (completed at 11:15 AM)
Current time: 12:15 PM (cron execution)
→ Current interval: 12:00 PM - 1:00 PM
→ Exports: 11:00 AM - 12:00 PM (incremental)
→ Creates continuous chain: 10:00 AM → 11:00 AM → 12:00 PM
```

## Detailed Flow Examples

### Daily Sync Example

| Time | Event | Interval | Export Range | Reason |
|------|-------|----------|--------------|--------|
| **Oct 15, 11:30 PM** | Force Run | Current | Oct 15 00:00 → Oct 16 00:00 | User wants current day data |
| **Oct 16, 00:15 AM** | First Run | Previous | Oct 15 00:00 → Oct 16 00:00 | First scheduled run, previous completed day |
| **Oct 17, 00:15 AM** | Incremental | Last to Current | Oct 16 00:00 → Oct 17 00:00 | From last export to current interval start |

### Hourly Sync Example

| Time | Event | Interval | Export Range | Reason |
|------|-------|----------|--------------|--------|
| **10:30 AM** | Force Run | Current | 10:00 AM → 11:00 AM | User wants current hour data |
| **11:15 AM** | First Run | Previous | 10:00 AM → 11:00 AM | First scheduled run, previous completed hour |
| **12:15 PM** | Incremental | Last to Current | 11:00 AM → 12:00 PM | From last export to current interval start |

## Force Run with Custom Time Range

### API Endpoint
```http
POST /api/v1/scheduled-tasks/{id}/force-run
{
  "start_time": "2025-10-16T08:00:00Z",
  "end_time": "2025-10-16T14:00:00Z"
}
```

### Behavior:
- **Ignores interval boundaries** when custom times provided
- **Uses exact start/end times** from request
- **Still creates isolated task** (ScheduledTaskID = "")
- **Preserves user context** in audit fields

### Example:
```
User requests: 8:00 AM - 2:00 PM (6-hour custom range)
→ Exports exactly 8:00 AM - 2:00 PM
→ Ignores hourly/daily boundaries
→ Creates task with ScheduledTaskID = ""
→ User ID in created_by/updated_by
```

## Data Isolation Strategy

### Task Identification
| Task Type | ScheduledTaskID | User Context | Incremental Tracking |
|-----------|-----------------|--------------|---------------------|
| **Force Run** | `""` (empty) | ✅ Preserved | ❌ Not tracked |
| **First Run** | `scheduled_task_123` | ❌ System | ✅ Tracked |
| **Incremental** | `scheduled_task_123` | ❌ System | ✅ Tracked |

### Benefits:
- **No data duplication** between force runs and scheduled runs
- **Clean separation** of manual vs automated exports
- **Reliable incremental sync** chain
- **Audit trail** for user-initiated actions

## Error Handling

### Force Run Failures
- **Retry logic** can be implemented
- **User notification** on failure
- **No impact** on scheduled sync chain

### Scheduled Run Failures
- **Retry mechanism** via Temporal
- **Next run** continues from last successful export
- **No data gaps** in incremental chain

## Implementation Notes

### Key Functions:
1. **`CalculateIntervalBoundaries`** - Returns current interval
2. **`calculateTimeRange`** - Determines export range based on flow type
3. **`GetLastSuccessfulExportTask`** - Finds previous export for incremental sync

### Database Schema:
- **`scheduled_task_id`** - Empty for force runs, actual ID for scheduled runs
- **`created_by`** - User ID for force runs, empty for scheduled runs
- **`updated_by`** - User ID for force runs, empty for scheduled runs

## Summary

This sync logic provides:
- **Flexible force runs** with custom time ranges
- **Reliable incremental sync** for scheduled tasks
- **Clean data isolation** between manual and automated exports
- **Comprehensive audit trail** for all operations
- **No data duplication** or gaps in sync chains

The 15-minute buffer ensures data ingestion is complete before export, while the three-flow system handles all use cases efficiently and reliably.
