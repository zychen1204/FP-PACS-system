package models

import "time"

// SwipeRequest represents an incoming badge swipe request
type SwipeRequest struct {
	BadgeID   string `json:"badge_id" binding:"required"`
	SiteID    string `json:"site_id" binding:"required"`
	GateID    string `json:"gate_id" binding:"required"`
	Direction string `json:"direction" binding:"required,oneof=IN OUT"`
}

// SwipeResponse represents the response to a swipe request
type SwipeResponse struct {
	Status    string `json:"status"`
	Message   string `json:"message"`
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
	OrgPath    string     `json:"org_path"`
	WorkDate   string     `json:"work_date"`
	FirstIn    *time.Time `json:"first_in,omitempty"`
	LastOut    *time.Time `json:"last_out,omitempty"`
	SwipeCount int        `json:"swipe_count"`
	StayHours  float64    `json:"stay_hours"`
}
