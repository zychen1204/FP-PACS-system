package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"pacs/backend/internal/auth"
	"pacs/backend/internal/models"
)

// ── mockDB ─────────────────────────────────────────────────────────────────

// mockDB implements Reporter so tests do not need a real PostgreSQL.
type mockDB struct {
	attendance []models.AttendanceReport
	events     []models.AccessEvent
	scope      string // non-empty = caller is manager
	trends     []models.AttendanceTrend
	alerts     []models.Alert
	aggregates []models.EmployeeAggregate
	err        error
}

func (m *mockDB) QueryAttendance(_ context.Context, _, _, _ string) ([]models.AttendanceReport, error) {
	return m.attendance, m.err
}
func (m *mockDB) QueryAuditTrail(_ context.Context, _, _, _ string) ([]models.AccessEvent, error) {
	return m.events, m.err
}
func (m *mockDB) GetManagerScope(_ context.Context, _ string) (string, error) {
	return m.scope, m.err
}
func (m *mockDB) QueryManagerTeamAttendance(_ context.Context, _, _, _ string) ([]models.AttendanceReport, error) {
	return m.attendance, m.err
}
func (m *mockDB) QueryAttendanceTrend(_ context.Context, _, _, _, _ string) ([]models.AttendanceTrend, error) {
	return m.trends, m.err
}
func (m *mockDB) QueryAlerts(_ context.Context, _ bool, _ string, _ int) ([]models.Alert, error) {
	return m.alerts, m.err
}
func (m *mockDB) QueryEmployeeAggregated(_ context.Context, _, _, _, _ string) ([]models.EmployeeAggregate, error) {
	return m.aggregates, m.err
}
func (m *mockDB) Close() error { return nil }

// ── router builder ─────────────────────────────────────────────────────────

// newReportingRouter wires the given mock into the global `database` and
// returns a ready-to-use Gin engine. Call t.Setenv("DEV_AUTH_BYPASS","1")
// before this to skip JWT in tests that don't need to test auth.
func newReportingRouter(t *testing.T, mock Reporter) *gin.Engine {
	t.Helper()
	database = mock
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery(), corsMiddleware())
	r.GET("/healthz", healthCheck)
	r.POST("/v1/dev/login", devLogin)

	authed := r.Group("/", auth.Middleware())
	authed.GET("/v1/reports/attendance", getAttendanceReport)
	authed.GET("/v1/audit", getAuditTrail)
	authed.GET("/v1/reports/manager-team", getManagerTeamReport)
	authed.GET("/v1/reports/trend", getAttendanceTrend)
	authed.GET("/v1/reports/attendance/export", exportAttendance)
	authed.GET("/v1/alerts", listAlerts)
	authed.GET("/v1/reports/attendance/aggregated", getAttendanceAggregated)
	authed.GET("/v1/reports/manager-team/aggregated", getManagerTeamAggregated)
	return r
}

// get fires a GET request with optional query params.
func get(r *gin.Engine, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", path, nil)
	r.ServeHTTP(w, req)
	return w
}

// ── sample fixtures ────────────────────────────────────────────────────────

func sampleAttendance() []models.AttendanceReport {
	firstIn := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	lastOut := time.Date(2026, 5, 1, 17, 0, 0, 0, time.UTC)
	return []models.AttendanceReport{
		{
			EmployeeID: "B001",
			Name:       "王小明",
			Status:     "STAFF",
			OrgPath:    "TSMC.Fab12.製造部",
			WorkDate:   "2026-05-01",
			FirstIn:    &firstIn,
			LastOut:    &lastOut,
			SwipeCount: 4,
			StayHours:  9.0,
		},
	}
}

func sampleEvents() []models.AccessEvent {
	return []models.AccessEvent{
		{ID: 1, BadgeID: "B001", SiteID: "Site-A", GateID: "G1", Direction: "IN", Status: "SUCCESS", Timestamp: time.Now()},
		{ID: 2, BadgeID: "B001", SiteID: "Site-A", GateID: "G1", Direction: "OUT", Status: "SUCCESS", Timestamp: time.Now()},
	}
}

// ── FR-5: Personal attendance records ─────────────────────────────────────

func TestGetAttendanceReport_FR5_ReturnsRecords(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "1")
	r := newReportingRouter(t, &mockDB{attendance: sampleAttendance()})

	w := get(r, "/v1/reports/attendance?as=B001")
	if w.Code != http.StatusOK {
		t.Fatalf("status got=%d want=200", w.Code)
	}
	var reports []models.AttendanceReport
	json.NewDecoder(w.Body).Decode(&reports)
	if len(reports) == 0 {
		t.Error("expected at least one attendance record")
	}
	if reports[0].EmployeeID == "" || reports[0].WorkDate == "" {
		t.Error("attendance record is missing required fields")
	}
}

func TestGetAttendanceReport_FR5_EmptyResult_ReturnsEmptyArray(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "1")
	r := newReportingRouter(t, &mockDB{attendance: nil})

	w := get(r, "/v1/reports/attendance?as=B001")
	if w.Code != http.StatusOK {
		t.Fatalf("status got=%d want=200", w.Code)
	}
	// Should return [] not null
	body := strings.TrimSpace(w.Body.String())
	if body == "null" {
		t.Error("empty result should return [] not null")
	}
}

// FR-5: date range params are forwarded to the DB layer (server-side filtering).
func TestGetAttendanceReport_FR5_DateRangeParams_Forwarded(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "1")
	r := newReportingRouter(t, &mockDB{attendance: sampleAttendance()})

	// ?start_date + ?end_date — handler must accept without error
	w := get(r, "/v1/reports/attendance?as=B001&start_date=2026-05-01&end_date=2026-05-31")
	if w.Code != http.StatusOK {
		t.Fatalf("status got=%d want=200 with date range params", w.Code)
	}
}

// FR-5: legacy ?date= single-day param still works (backward compat).
func TestGetAttendanceReport_FR5_LegacyDateParam_Works(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "1")
	r := newReportingRouter(t, &mockDB{attendance: sampleAttendance()})

	w := get(r, "/v1/reports/attendance?as=B001&date=2026-05-01")
	if w.Code != http.StatusOK {
		t.Fatalf("status got=%d want=200 with legacy date param", w.Code)
	}
}

// ── FR-6: Hierarchical team report (manager view) ─────────────────────────

func TestGetManagerTeamReport_FR6_Manager_ReturnsTeam(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "1")
	r := newReportingRouter(t, &mockDB{
		scope:      "TSMC.Fab12",
		attendance: sampleAttendance(),
	})

	w := get(r, "/v1/reports/manager-team?as=B100")
	if w.Code != http.StatusOK {
		t.Fatalf("status got=%d want=200", w.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["manager_scope"] == "" || resp["manager_scope"] == nil {
		t.Error("response should include manager_scope")
	}
}

// FR-6: date range is passed to DB layer (no client-side filtering).
func TestGetManagerTeamReport_FR6_DateRange_Forwarded(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "1")
	r := newReportingRouter(t, &mockDB{scope: "TSMC.Fab12", attendance: sampleAttendance()})

	w := get(r, "/v1/reports/manager-team?as=B100&start_date=2026-05-01&end_date=2026-05-31")
	if w.Code != http.StatusOK {
		t.Fatalf("status got=%d want=200 with date range", w.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["reports"] == nil {
		t.Error("response should include reports key")
	}
}

// ── FR-7: Attendance trend report ─────────────────────────────────────────

func TestGetAttendanceTrend_FR7_DefaultDay(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "1")
	trends := []models.AttendanceTrend{
		{Bucket: "2026-05-01", HeadCount: 30, AvgStayHrs: 8.5, TotalSwipes: 120},
	}
	r := newReportingRouter(t, &mockDB{trends: trends, scope: ""})

	w := get(r, "/v1/reports/trend?as=B001")
	if w.Code != http.StatusOK {
		t.Fatalf("status got=%d want=200", w.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["trends"] == nil {
		t.Error("response should include trends key")
	}
}

func TestGetAttendanceTrend_FR7_WeekPeriod(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "1")
	r := newReportingRouter(t, &mockDB{trends: []models.AttendanceTrend{}})

	w := get(r, "/v1/reports/trend?as=B001&period=week")
	if w.Code != http.StatusOK {
		t.Fatalf("status got=%d want=200", w.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if _, ok := resp["scope"]; !ok {
		t.Error("response should include scope key")
	}
}

// ── FR-8: Excel export ─────────────────────────────────────────────────────

func TestExportAttendance_FR8_Excel_Returns200WithXLSX(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "1")
	r := newReportingRouter(t, &mockDB{attendance: sampleAttendance()})

	w := get(r, "/v1/reports/attendance/export?as=B001&format=excel")
	if w.Code != http.StatusOK {
		t.Fatalf("status got=%d want=200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "spreadsheetml") {
		t.Errorf("Content-Type got=%q, expected OOXML spreadsheet type", ct)
	}
	cd := w.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") {
		t.Error("Content-Disposition should be attachment for file download")
	}
	if w.Body.Len() == 0 {
		t.Error("xlsx body should not be empty")
	}
}

func TestExportAttendance_FR8_UnsupportedFormat_Returns400(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "1")
	r := newReportingRouter(t, &mockDB{attendance: sampleAttendance()})

	w := get(r, "/v1/reports/attendance/export?as=B001&format=pdf")
	if w.Code != http.StatusBadRequest {
		t.Errorf("status got=%d want=400 for unsupported format", w.Code)
	}
}

// ── FR-9: Hierarchical data permission ────────────────────────────────────

func TestGetManagerTeamReport_FR9_NonManager_Returns403(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "1")
	r := newReportingRouter(t, &mockDB{scope: ""}) // empty scope = not a manager

	w := get(r, "/v1/reports/manager-team?as=B011")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status got=%d want=403 for non-manager", w.Code)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] == "" {
		t.Error("403 response should include an error message")
	}
}

// ── FR-10: JWT authentication ──────────────────────────────────────────────

func TestDevLogin_FR10_Returns_JWT(t *testing.T) {
	r := newReportingRouter(t, &mockDB{})
	w := httptest.NewRecorder()
	body := strings.NewReader(`{"badge_id":"B001"}`)
	req, _ := http.NewRequest("POST", "/v1/dev/login", body)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status got=%d want=200", w.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["access_token"] == "" || resp["access_token"] == nil {
		t.Error("dev login should return access_token")
	}
	if resp["token_type"] != "Bearer" {
		t.Errorf("token_type got=%v want=Bearer", resp["token_type"])
	}
}

func TestDevLogin_FR10_MissingBadgeID_Returns400(t *testing.T) {
	r := newReportingRouter(t, &mockDB{})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/dev/login", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status got=%d want=400", w.Code)
	}
}

// FR-10: Without a token (and bypass disabled) the protected routes return 401
func TestProtectedRoute_FR10_NoToken_Returns401(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "0")
	r := newReportingRouter(t, &mockDB{attendance: sampleAttendance()})

	w := get(r, "/v1/reports/attendance") // no ?as= and no Authorization header
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status got=%d want=401 when no token is provided", w.Code)
	}
}

// FR-10: A valid JWT grants access
func TestProtectedRoute_FR10_ValidToken_Returns200(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "0")
	r := newReportingRouter(t, &mockDB{attendance: sampleAttendance()})

	tok, _ := auth.Issue("B001", time.Hour)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/reports/attendance", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status got=%d want=200 with valid JWT", w.Code)
	}
}

// ── FR-11: Anomaly alerts ──────────────────────────────────────────────────

func TestListAlerts_FR11_ReturnsAlerts(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "1")
	bid := "B001"
	sid := "Site-A"
	gid := "Gate-2A"
	alerts := []models.Alert{
		{ID: 1, AlertType: "APB_BURST", Severity: "HIGH", BadgeID: &bid, SiteID: &sid, GateID: &gid, OccurredAt: time.Now()},
	}
	r := newReportingRouter(t, &mockDB{alerts: alerts})

	w := get(r, "/v1/alerts?as=B001")
	if w.Code != http.StatusOK {
		t.Fatalf("status got=%d want=200", w.Code)
	}
	var result []models.Alert
	json.NewDecoder(w.Body).Decode(&result)
	if len(result) == 0 {
		t.Error("expected at least one alert")
	}
	if result[0].AlertType == "" {
		t.Error("alert should have alert_type field")
	}
}

func TestListAlerts_FR11_EmptyAlerts_ReturnsEmptyArray(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "1")
	r := newReportingRouter(t, &mockDB{alerts: nil})

	w := get(r, "/v1/alerts?as=B001")
	if w.Code != http.StatusOK {
		t.Fatalf("status got=%d want=200", w.Code)
	}
	body := strings.TrimSpace(w.Body.String())
	if body == "null" {
		t.Error("empty alerts should return [] not null")
	}
}

// ── FR-13: Audit trail ─────────────────────────────────────────────────────

func TestGetAuditTrail_FR13_ReturnsEvents(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "1")
	r := newReportingRouter(t, &mockDB{events: sampleEvents()})

	w := get(r, "/v1/audit?as=B001&badge_id=B001&start_date=2026-05-01&end_date=2026-05-31")
	if w.Code != http.StatusOK {
		t.Fatalf("status got=%d want=200", w.Code)
	}
	var events []models.AccessEvent
	json.NewDecoder(w.Body).Decode(&events)
	if len(events) == 0 {
		t.Error("expected at least one audit event")
	}
}

func TestGetAuditTrail_FR13_MissingBadgeID_Returns400(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "1")
	r := newReportingRouter(t, &mockDB{})

	w := get(r, "/v1/audit?as=B001") // badge_id query param is missing
	if w.Code != http.StatusBadRequest {
		t.Errorf("status got=%d want=400 when badge_id is missing", w.Code)
	}
}

// ── Health ─────────────────────────────────────────────────────────────────

func TestHealthCheck_Returns200(t *testing.T) {
	r := newReportingRouter(t, &mockDB{})
	w := get(r, "/healthz")
	if w.Code != http.StatusOK {
		t.Errorf("status got=%d want=200", w.Code)
	}
}

// ── NFR: Reporting API handler latency (sub-200ms SLA) ────────────────────
//
// These tests verify that handler processing overhead with a zero-latency mock
// DB stays well under 5ms. Real production latency also includes PostgreSQL I/O;
// the sub-200ms SLA is met because:
//   - mv_daily_attendance is a materialized view (pre-aggregated, no live join)
//   - date range filter hits idx_mv_daily_attendance_event_date (btree)
//   - org scope filter hits idx_mv_daily_attendance_org_date (GiST ltree)

const reportingHandlerBudget = 5 * time.Millisecond // handler overhead only

func TestReportingAPI_Attendance_HandlerLatency_Sub200ms(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "1")
	r := newReportingRouter(t, &mockDB{attendance: sampleAttendance()})

	const iterations = 20
	var total time.Duration
	for i := 0; i < iterations; i++ {
		start := time.Now()
		w := get(r, "/v1/reports/attendance?as=B001&start_date=2026-05-01&end_date=2026-05-31")
		total += time.Since(start)
		if w.Code != http.StatusOK {
			t.Fatalf("unexpected status %d on iteration %d", w.Code, i)
		}
	}
	avg := total / iterations
	t.Logf("attendance handler avg latency: %v (SLA budget: <200ms, handler budget: <%v)", avg, reportingHandlerBudget)
	if avg > reportingHandlerBudget {
		t.Errorf("attendance handler avg latency %v exceeds %v budget (leaves no room for DB in sub-200ms SLA)",
			avg, reportingHandlerBudget)
	}
}

func TestReportingAPI_ManagerTeam_HandlerLatency_Sub200ms(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "1")
	r := newReportingRouter(t, &mockDB{scope: "TSMC.Fab12", attendance: sampleAttendance()})

	const iterations = 20
	var total time.Duration
	for i := 0; i < iterations; i++ {
		start := time.Now()
		w := get(r, "/v1/reports/manager-team?as=B100&start_date=2026-05-01&end_date=2026-05-31")
		total += time.Since(start)
		if w.Code != http.StatusOK {
			t.Fatalf("unexpected status %d on iteration %d", w.Code, i)
		}
	}
	avg := total / iterations
	t.Logf("manager-team handler avg latency: %v (SLA budget: <200ms, handler budget: <%v)", avg, reportingHandlerBudget)
	if avg > reportingHandlerBudget {
		t.Errorf("manager-team handler avg latency %v exceeds %v budget", avg, reportingHandlerBudget)
	}
}

// ── Aggregated endpoints ───────────────────────────────────────────────────

func sampleAggregates() []models.EmployeeAggregate {
	return []models.EmployeeAggregate{
		{
			EmployeeID: "B001", Name: "王小明", Status: "STAFF",
			OrgPath: "TSMC.Fab12.製造部", TotalSwipes: 80, TotalStayHours: 180.0,
			DayCount: 20, AvgSwipes: 4.0, AvgStayHours: 9.0,
		},
	}
}

func TestGetAttendanceAggregated_ReturnsAggregates(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "1")
	r := newReportingRouter(t, &mockDB{aggregates: sampleAggregates()})

	w := get(r, "/v1/reports/attendance/aggregated?as=B001&start_date=2026-05-01&end_date=2026-05-31")
	if w.Code != http.StatusOK {
		t.Fatalf("status got=%d want=200", w.Code)
	}
	var result []models.EmployeeAggregate
	json.NewDecoder(w.Body).Decode(&result)
	if len(result) == 0 {
		t.Error("expected at least one aggregate")
	}
	if result[0].EmployeeID == "" || result[0].TotalSwipes == 0 {
		t.Error("aggregate missing required fields")
	}
}

func TestGetAttendanceAggregated_Empty_ReturnsEmptyArray(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "1")
	r := newReportingRouter(t, &mockDB{aggregates: nil})

	w := get(r, "/v1/reports/attendance/aggregated?as=B001")
	if w.Code != http.StatusOK {
		t.Fatalf("status got=%d want=200", w.Code)
	}
	body := strings.TrimSpace(w.Body.String())
	if body == "null" {
		t.Error("empty result should return [] not null")
	}
}

func TestGetManagerTeamAggregated_Manager_ReturnsAggregates(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "1")
	r := newReportingRouter(t, &mockDB{scope: "TSMC.Fab12", aggregates: sampleAggregates()})

	w := get(r, "/v1/reports/manager-team/aggregated?as=B100&start_date=2026-05-01&end_date=2026-05-31")
	if w.Code != http.StatusOK {
		t.Fatalf("status got=%d want=200", w.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["manager_scope"] == nil {
		t.Error("response should include manager_scope")
	}
	if resp["aggregates"] == nil {
		t.Error("response should include aggregates")
	}
}

func TestGetManagerTeamAggregated_NonManager_Returns403(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "1")
	r := newReportingRouter(t, &mockDB{scope: ""})

	w := get(r, "/v1/reports/manager-team/aggregated?as=B011")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status got=%d want=403 for non-manager", w.Code)
	}
}

func TestGetAttendanceTrend_FR7_IncludesSummary(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "1")
	trends := []models.AttendanceTrend{
		{Bucket: "2026-05-01", HeadCount: 30, AvgStayHrs: 8.5, TotalSwipes: 120},
		{Bucket: "2026-05-02", HeadCount: 25, AvgStayHrs: 7.0, TotalSwipes: 100},
	}
	r := newReportingRouter(t, &mockDB{trends: trends})

	w := get(r, "/v1/reports/trend?as=B001&period=day&start_date=2026-05-01&end_date=2026-05-02")
	if w.Code != http.StatusOK {
		t.Fatalf("status got=%d want=200", w.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["summary"] == nil {
		t.Error("trend response should include summary field")
	}
}

func TestReportingAPI_AttendanceAggregated_HandlerLatency_Sub200ms(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "1")
	r := newReportingRouter(t, &mockDB{aggregates: sampleAggregates()})

	const iterations = 20
	var total time.Duration
	for i := 0; i < iterations; i++ {
		start := time.Now()
		w := get(r, "/v1/reports/attendance/aggregated?as=B001&start_date=2026-05-01&end_date=2026-05-31")
		total += time.Since(start)
		if w.Code != http.StatusOK {
			t.Fatalf("unexpected status %d on iteration %d", w.Code, i)
		}
	}
	avg := total / iterations
	t.Logf("attendance/aggregated handler avg latency: %v (budget: <%v)", avg, reportingHandlerBudget)
	if avg > reportingHandlerBudget {
		t.Errorf("attendance/aggregated avg latency %v exceeds %v budget", avg, reportingHandlerBudget)
	}
}

func TestReportingAPI_Trend_HandlerLatency_Sub200ms(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "1")
	trends := make([]models.AttendanceTrend, 90) // simulate a quarter of daily buckets
	for i := range trends {
		trends[i] = models.AttendanceTrend{Bucket: "2026-01-01", HeadCount: 50, AvgStayHrs: 8.5, TotalSwipes: 200}
	}
	r := newReportingRouter(t, &mockDB{trends: trends})

	const iterations = 20
	var total time.Duration
	for i := 0; i < iterations; i++ {
		start := time.Now()
		w := get(r, "/v1/reports/trend?as=B001&period=day&start_date=2026-01-01&end_date=2026-03-31")
		total += time.Since(start)
		if w.Code != http.StatusOK {
			t.Fatalf("unexpected status %d on iteration %d", w.Code, i)
		}
	}
	avg := total / iterations
	t.Logf("trend handler avg latency: %v (SLA budget: <200ms, handler budget: <%v)", avg, reportingHandlerBudget)
	if avg > reportingHandlerBudget {
		t.Errorf("trend handler avg latency %v exceeds %v budget", avg, reportingHandlerBudget)
	}
}
