package models

import "time"

// AccessLog represents a badge swipe event
type AccessLog struct {
	BadgeID   string    `json:"badge_id"`
	SiteID    string    `json:"site_id"`
	GateID    string    `json:"gate_id"`
	Direction string    `json:"direction"` // IN or OUT
	Status    string    `json:"status"`    // SUCCESS or REJECTED_APB
	Timestamp time.Time `json:"timestamp"`
	Reason    string    `json:"reason,omitempty"`
}

// SwipeRequest represents an incoming swipe request
type SwipeRequest struct {
	BadgeID   string `json:"badge_id" binding:"required"`
	SiteID    string `json:"site_id" binding:"required"`
	GateID    string `json:"gate_id" binding:"required"`
	Direction string `json:"direction" binding:"required,oneof=IN OUT"`
}

// SwipeResponse represents the response to a swipe request
type SwipeResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// AttendanceReport represents an employee's attendance record
type AttendanceReport struct {
	EmployeeID string    `json:"employee_id"`
	Name       string    `json:"name"`
	OrgPath    string    `json:"org_path"`
	WorkDate   string    `json:"work_date"`
	FirstIn    *time.Time `json:"first_in,omitempty"`
	LastOut    *time.Time `json:"last_out,omitempty"`
	SwipeCount int       `json:"swipe_count"`
}
