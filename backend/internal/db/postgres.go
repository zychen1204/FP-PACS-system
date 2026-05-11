package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"pacs/backend/internal/models"

	_ "github.com/lib/pq"
)

// PostgresDB handles database operations
type PostgresDB struct {
	db *sql.DB
}

// NewPostgresDB creates a new PostgreSQL connection with retry logic
func NewPostgresDB() (*PostgresDB, error) {
	host := envOrDefault("DB_HOST", "localhost")
	port := envOrDefault("DB_PORT", "5432")
	user := envOrDefault("DB_USER", "pacs_user")
	password := envOrDefault("DB_PASSWORD", "pacs_password")
	dbName := envOrDefault("DB_NAME", "pacs_db")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbName)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Retry connection up to 30 times (60 seconds)
	for i := 0; i < 30; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err = db.PingContext(ctx)
		cancel()
		if err == nil {
			break
		}
		fmt.Printf("[DB] Waiting for PostgreSQL... (%d/30)\n", i+1)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		return nil, fmt.Errorf("postgres connection failed after retries: %w", err)
	}

	return &PostgresDB{db: db}, nil
}

// InsertEvent inserts an access event (append-only, FR12 compliant).
// 0005 partition migration 後 access_events 為 RANGE (event_date) partition；
// partition key 不能由 trigger 自動填，故 INSERT 顯式計算
// (event_time AT TIME ZONE 'Asia/Taipei')::date。
func (p *PostgresDB) InsertEvent(ctx context.Context, event models.AccessEvent) error {
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO access_events
		   (badge_id, site_id, gate_id, direction, status, reason, event_time, event_date)
		 VALUES
		   ($1, $2, $3, $4, $5, $6, $7, ($7 AT TIME ZONE 'Asia/Taipei')::date)`,
		event.BadgeID, event.SiteID, event.GateID, event.Direction,
		event.Status, event.Reason, event.Timestamp,
	)
	return err
}

// QueryAttendance returns attendance reports, optionally filtered by date.
// 改 event_time::date → event_date 後直接命中 idx_events_status_date partial index
// (event_date, badge_id) WHERE status='SUCCESS'，並修掉原 GROUP BY 與 SELECT 對齊。
func (p *PostgresDB) QueryAttendance(ctx context.Context, date string) ([]models.AttendanceReport, error) {
	query := `
		SELECT
			e.badge_id,
			COALESCE(emp.name, 'Employee ' || e.badge_id) AS name,
			COALESCE(emp.org_path, 'Unknown')             AS org_path,
			e.event_date::text                            AS work_date,
			MIN(CASE WHEN e.direction = 'IN'  THEN e.event_time END) AS first_in,
			MAX(CASE WHEN e.direction = 'OUT' THEN e.event_time END) AS last_out,
			COUNT(*)                                      AS swipe_count
		FROM access_events e
		LEFT JOIN employees emp ON e.badge_id = emp.badge_id
		WHERE e.status = 'SUCCESS'
	`
	args := []interface{}{}

	if date != "" {
		query += " AND e.event_date = $1"
		args = append(args, date)
	}

	query += `
		GROUP BY e.badge_id, emp.name, emp.org_path, e.event_date
		ORDER BY e.event_date DESC, e.badge_id
	`

	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []models.AttendanceReport
	for rows.Next() {
		var r models.AttendanceReport
		if err := rows.Scan(&r.EmployeeID, &r.Name, &r.OrgPath, &r.WorkDate, &r.FirstIn, &r.LastOut, &r.SwipeCount); err != nil {
			return nil, err
		}
		if r.FirstIn != nil && r.LastOut != nil {
			r.StayHours = r.LastOut.Sub(*r.FirstIn).Hours()
		}
		reports = append(reports, r)
	}
	return reports, nil
}

// QueryAuditTrail returns the full audit trail for a badge within a date range (FR13)
// 改 event_time::date → event_date 後可命中 idx_events_badge_eventdate。
func (p *PostgresDB) QueryAuditTrail(ctx context.Context, badgeID, startDate, endDate string) ([]models.AccessEvent, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, badge_id, site_id, gate_id, direction, status, reason, event_time
		 FROM access_events
		 WHERE badge_id = $1 AND event_date BETWEEN $2 AND $3
		 ORDER BY event_time DESC`,
		badgeID, startDate, endDate,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []models.AccessEvent
	for rows.Next() {
		var e models.AccessEvent
		if err := rows.Scan(&e.ID, &e.BadgeID, &e.SiteID, &e.GateID, &e.Direction, &e.Status, &e.Reason, &e.Timestamp); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, nil
}

// GetManagerScope returns the manager's org_path_ltree if caller is an active manager.
// FR-9 pattern a step 1：空結果由 caller 翻成 403。
func (p *PostgresDB) GetManagerScope(ctx context.Context, badgeID string) (string, error) {
	var scope string
	err := p.db.QueryRowContext(ctx,
		`SELECT org_path_ltree::text FROM employees
		 WHERE badge_id = $1 AND is_manager = TRUE AND is_active = TRUE`,
		badgeID,
	).Scan(&scope)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return scope, err
}

// QueryManagerTeamAttendance returns attendance for all employees under the given ltree scope.
// FR-6 / FR-9 pattern a step 2：用 ltree `<@` (descendant of) operator + GiST index 查子樹。
// 若 date 為空，回 mv_daily_attendance 上所有日期；否則 filter event_date。
func (p *PostgresDB) QueryManagerTeamAttendance(ctx context.Context, scopeLtree, date string) ([]models.AttendanceReport, error) {
	query := `
		SELECT mv.badge_id, mv.name, mv.org_path,
		       mv.event_date::text AS work_date,
		       mv.first_in, mv.last_out, mv.swipe_count,
		       COALESCE(mv.stay_hours, 0)::float8 AS stay_hours
		FROM mv_daily_attendance mv
		WHERE mv.org_path_ltree <@ $1::ltree
	`
	args := []interface{}{scopeLtree}
	if date != "" {
		query += " AND mv.event_date = $2"
		args = append(args, date)
	}
	query += " ORDER BY mv.event_date DESC, mv.badge_id"

	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []models.AttendanceReport
	for rows.Next() {
		var r models.AttendanceReport
		if err := rows.Scan(&r.EmployeeID, &r.Name, &r.OrgPath, &r.WorkDate, &r.FirstIn, &r.LastOut, &r.SwipeCount, &r.StayHours); err != nil {
			return nil, err
		}
		reports = append(reports, r)
	}
	return reports, nil
}

// QueryAttendanceTrend aggregates mv_daily_attendance into time buckets (FR-7).
// period: 'day' | 'week' | 'month' | 'quarter'。scope 為空時不過濾組織子樹。
func (p *PostgresDB) QueryAttendanceTrend(ctx context.Context, period, scope, startDate, endDate string) ([]models.AttendanceTrend, error) {
	bucketExpr := "date_trunc('day', event_date::timestamp)::date"
	switch period {
	case "week":
		bucketExpr = "date_trunc('week', event_date::timestamp)::date"
	case "month":
		bucketExpr = "date_trunc('month', event_date::timestamp)::date"
	case "quarter":
		bucketExpr = "date_trunc('quarter', event_date::timestamp)::date"
	}

	query := `
		SELECT ` + bucketExpr + ` AS bucket,
		       COUNT(DISTINCT badge_id)        AS head_count,
		       COALESCE(AVG(stay_hours), 0)::float8 AS avg_stay_hrs,
		       COALESCE(SUM(swipe_count), 0)::int   AS total_swipes
		FROM mv_daily_attendance
		WHERE 1 = 1
	`
	args := []interface{}{}
	idx := 1
	if scope != "" {
		query += fmt.Sprintf(" AND org_path_ltree <@ $%d::ltree", idx)
		args = append(args, scope)
		idx++
	}
	if startDate != "" {
		query += fmt.Sprintf(" AND event_date >= $%d", idx)
		args = append(args, startDate)
		idx++
	}
	if endDate != "" {
		query += fmt.Sprintf(" AND event_date <= $%d", idx)
		args = append(args, endDate)
		idx++
	}
	query += " GROUP BY 1 ORDER BY 1 DESC"

	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trends []models.AttendanceTrend
	for rows.Next() {
		var t models.AttendanceTrend
		var bucket time.Time
		if err := rows.Scan(&bucket, &t.HeadCount, &t.AvgStayHrs, &t.TotalSwipes); err != nil {
			return nil, err
		}
		t.Bucket = bucket.Format("2006-01-02")
		trends = append(trends, t)
	}
	return trends, nil
}

// QueryAlerts returns alerts; if openOnly is true filter resolved_at IS NULL.
// FR-11 read side。
func (p *PostgresDB) QueryAlerts(ctx context.Context, openOnly bool, limit int) ([]models.Alert, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `
		SELECT id, alert_type, severity, badge_id, site_id, gate_id,
		       details::text, occurred_at, resolved_at
		FROM alerts
	`
	if openOnly {
		query += " WHERE resolved_at IS NULL"
	}
	query += " ORDER BY occurred_at DESC LIMIT $1"

	rows, err := p.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []models.Alert
	for rows.Next() {
		var a models.Alert
		if err := rows.Scan(&a.ID, &a.AlertType, &a.Severity, &a.BadgeID, &a.SiteID, &a.GateID,
			&a.Details, &a.OccurredAt, &a.ResolvedAt); err != nil {
			return nil, err
		}
		alerts = append(alerts, a)
	}
	return alerts, nil
}

// InsertAlert is used by anomaly-detector to record an alert.
func (p *PostgresDB) InsertAlert(ctx context.Context, alertType, severity string, badgeID, siteID, gateID *string, detailsJSON string) error {
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO alerts (alert_type, severity, badge_id, site_id, gate_id, details)
		 VALUES ($1, $2, $3, $4, $5, $6::jsonb)`,
		alertType, severity, badgeID, siteID, gateID, detailsJSON)
	return err
}

// Close closes the database connection
func (p *PostgresDB) Close() error {
	return p.db.Close()
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
