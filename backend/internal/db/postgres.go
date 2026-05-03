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

// InsertEvent inserts an access event (append-only, FR12 compliant)
func (p *PostgresDB) InsertEvent(ctx context.Context, event models.AccessEvent) error {
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO access_events (badge_id, site_id, gate_id, direction, status, reason, event_time)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		event.BadgeID, event.SiteID, event.GateID, event.Direction,
		event.Status, event.Reason, event.Timestamp,
	)
	return err
}

// QueryAttendance returns attendance reports, optionally filtered by date
func (p *PostgresDB) QueryAttendance(ctx context.Context, date string) ([]models.AttendanceReport, error) {
	query := `
		SELECT
			e.badge_id,
			COALESCE(emp.name, 'Employee ' || e.badge_id) AS name,
			COALESCE(emp.org_path, 'Unknown') AS org_path,
			e.event_time::date::text AS work_date,
			MIN(CASE WHEN e.direction = 'IN' THEN e.event_time END) AS first_in,
			MAX(CASE WHEN e.direction = 'OUT' THEN e.event_time END) AS last_out,
			COUNT(*) AS swipe_count
		FROM access_events e
		LEFT JOIN employees emp ON e.badge_id = emp.badge_id
		WHERE e.status = 'SUCCESS'
	`
	args := []interface{}{}

	if date != "" {
		query += " AND e.event_time::date = $1"
		args = append(args, date)
	}

	query += `
		GROUP BY e.badge_id, emp.name, emp.org_path, e.event_time::date
		ORDER BY work_date DESC, e.badge_id
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
func (p *PostgresDB) QueryAuditTrail(ctx context.Context, badgeID, startDate, endDate string) ([]models.AccessEvent, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, badge_id, site_id, gate_id, direction, status, reason, event_time
		 FROM access_events
		 WHERE badge_id = $1 AND event_time::date BETWEEN $2 AND $3
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
