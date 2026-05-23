package models

import "time"

// SwipeRequest represents an incoming badge swipe request
type SwipeRequest struct {
	BadgeID   string `json:"badge_id" binding:"required"`
	SiteID    string `json:"site_id"` // optional, forwarded from frontend for DB storage
	GateID    string `json:"gate_id" binding:"required"`
	Direction string `json:"direction" binding:"required,oneof=IN OUT"`
	// EventTime: 可選 RFC3339 時間戳，留空則 handler 用 server time。
	// 用於 0104 大規模壓測 / 批次回放生成歷史事件；以 string 保留，handler 解析失敗
	// 回 400，避免 binding 把畸形 payload 默默灌成 time.Time 零值寫進 DB。
	EventTime string `json:"event_time,omitempty"`
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
	Status     string     `json:"status"` // "MANAGER_L1", "MANAGER_L2", or "STAFF"
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
	Bucket      string  `json:"bucket"`       // ISO date string of bucket start
	HeadCount   int     `json:"head_count"`   // distinct badges in bucket
	AvgStayHrs  float64 `json:"avg_stay_hrs"` // average stay_hours across badges
	TotalSwipes int     `json:"total_swipes"` // sum of swipe_count
}

// EmployeeAggregate is per-employee aggregated attendance over a date range (month/quarter).
// Returned by the /v1/reports/attendance/aggregated and /v1/reports/manager-team/aggregated endpoints.
type EmployeeAggregate struct {
	EmployeeID     string  `json:"employee_id"`
	Name           string  `json:"name"`
	Status         string  `json:"status"`
	OrgPath        string  `json:"org_path"`
	TotalSwipes    int     `json:"total_swipes"`
	TotalStayHours float64 `json:"total_stay_hours"`
	DayCount       int     `json:"day_count"`
	AvgSwipes      float64 `json:"avg_swipes"`
	AvgStayHours   float64 `json:"avg_stay_hours"`
}

// TrendSummary holds period-level averages derived from AttendanceTrend buckets.
// Returned alongside trends so the frontend does not need to re-aggregate.
type TrendSummary struct {
	AvgSwipesPerPerson float64 `json:"avg_swipes_per_person"`
	AvgHeadCount       float64 `json:"avg_head_count"`
	AvgStayHrs         float64 `json:"avg_stay_hrs"`
}

// Alert is FR-11 anomaly record.
type Alert struct {
	ID         int64      `json:"id"`
	AlertType  string     `json:"alert_type"`
	Severity   string     `json:"severity"`
	BadgeID    *string    `json:"badge_id,omitempty"`
	SiteID     *string    `json:"site_id,omitempty"`
	GateID     *string    `json:"gate_id,omitempty"`
	Details    string     `json:"details"` // JSON as raw string for forward-compat
	OccurredAt time.Time  `json:"occurred_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}
