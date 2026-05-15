package models

import "time"

// SwipeRequest represents an incoming badge swipe request
type SwipeRequest struct {
	BadgeID   string `json:"badge_id" binding:"required"`
	SiteID    string `json:"site_id"`  // optional, forwarded from frontend for DB storage
	GateID    string `json:"gate_id" binding:"required"`
	Direction string `json:"direction" binding:"required,oneof=IN OUT"`
}

// SwipeResponse represents the response to a swipe request
type SwipeResponse struct {
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
	Reason    string `json:"reason,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
}

// AccessEvent represents a persisted access event (append-only)
type AccessEvent struct {
	ID        int64     `json:"id"`
	BadgeID   string    `json:"badge_id"`
	SiteID    string    `json:"site_id"`
	GateID    string    `json:"gate_id"`
	Direction string    `json:"direction"`
	Status    string    `json:"status"`
	Reason    string    `json:"reason,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// AttendanceReport represents an employee's daily attendance summary
type AttendanceReport struct {
	EmployeeID string     `json:"employee_id"`
	Name       string     `json:"name"`
	IsManager  bool       `json:"is_manager"`
	OrgPath    string     `json:"org_path"`
	WorkDate   string     `json:"work_date"`
	FirstIn    *time.Time `json:"first_in,omitempty"`
	LastOut    *time.Time `json:"last_out,omitempty"`
	SwipeCount int        `json:"swipe_count"`
	StayHours  float64    `json:"stay_hours"`
}

// AttendanceTrend is the per-bucket aggregate result for FR-7 trend reports.
// Bucket 由 reporting-api 在 MV 上做 date_trunc 切（day/week/month/quarter）。
type AttendanceTrend struct {
	Bucket      string  `json:"bucket"`        // ISO date string of bucket start
	HeadCount   int     `json:"head_count"`    // distinct badges in bucket
	AvgStayHrs  float64 `json:"avg_stay_hrs"`  // average stay_hours across badges
	TotalSwipes int     `json:"total_swipes"`  // sum of swipe_count
}

// Alert is FR-11 anomaly record.
type Alert struct {
	ID         int64     `json:"id"`
	AlertType  string    `json:"alert_type"`
	Severity   string    `json:"severity"`
	BadgeID    *string   `json:"badge_id,omitempty"`
	SiteID     *string   `json:"site_id,omitempty"`
	GateID     *string   `json:"gate_id,omitempty"`
	Details    string    `json:"details"` // JSON as raw string for forward-compat
	OccurredAt time.Time `json:"occurred_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}
