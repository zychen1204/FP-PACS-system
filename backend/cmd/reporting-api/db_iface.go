package main

import (
	"context"

	"pacs/backend/internal/models"
)

// Reporter is the DB interface used by all reporting-api handlers.
// The real implementation is *db.PostgresDB; tests inject a mockDB.
type Reporter interface {
	QueryAttendance(ctx context.Context, date string) ([]models.AttendanceReport, error)
	QueryAuditTrail(ctx context.Context, badgeID, startDate, endDate string) ([]models.AccessEvent, error)
	GetManagerScope(ctx context.Context, badgeID string) (string, error)
	QueryManagerTeamAttendance(ctx context.Context, scopeLtree, date string) ([]models.AttendanceReport, error)
	QueryAttendanceTrend(ctx context.Context, period, scope, startDate, endDate string) ([]models.AttendanceTrend, error)
	QueryAlerts(ctx context.Context, openOnly bool, limit int) ([]models.Alert, error)
	Close() error
}
