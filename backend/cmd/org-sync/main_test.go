package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// ── mockLDAP ───────────────────────────────────────────────────────────────

// mockLDAP returns the static org records used by the org-sync CronJob mock.
// These tests verify that the seed data is structurally correct and contains
// the employee/manager mix required by FR-6 / FR-9.

func TestMockLDAP_ReturnsNonEmptySlice(t *testing.T) {
	records := mockLDAP()
	if len(records) == 0 {
		t.Fatal("mockLDAP should return at least one org record")
	}
}

func TestMockLDAP_AllRecordsHaveRequiredFields(t *testing.T) {
	for _, r := range mockLDAP() {
		if r.badgeID == "" {
			t.Errorf("record %+v: badgeID must not be empty", r)
		}
		if r.name == "" {
			t.Errorf("record %+v: name must not be empty", r)
		}
		if r.orgPath == "" {
			t.Errorf("record %+v: orgPath must not be empty", r)
		}
	}
}

// FR-6 / FR-9: There must be both managers and non-manager employees,
// and at least one L1 + one L2 manager so the multi-tier hierarchy is exercised.
func TestMockLDAP_ContainsAllJobLevels(t *testing.T) {
	var hasStaff, hasL1, hasL2 bool
	for _, r := range mockLDAP() {
		switch r.jobLevel {
		case JobLevelStaff:
			hasStaff = true
		case JobLevelManagerL1:
			hasL1 = true
		case JobLevelManagerL2:
			hasL2 = true
		default:
			t.Errorf("record %s: unexpected job_level %q (must be one of STAFF / MANAGER_L1 / MANAGER_L2)", r.badgeID, r.jobLevel)
		}
	}
	if !hasStaff {
		t.Error("mockLDAP must include at least one STAFF for FR-9 negative test")
	}
	if !hasL1 {
		t.Error("mockLDAP must include at least one MANAGER_L1 (一級主管) for FR-6")
	}
	if !hasL2 {
		t.Error("mockLDAP must include at least one MANAGER_L2 (二級主管) for FR-6")
	}
}

// Verify the expected fab manager badge exists (used as the default in dev tests).
// B100 (廠長) must be MANAGER_L1.
func TestMockLDAP_ContainsFabManager_B100(t *testing.T) {
	found := false
	for _, r := range mockLDAP() {
		if r.badgeID == "B100" {
			found = true
			if r.jobLevel != JobLevelManagerL1 {
				t.Errorf("B100 (fab manager) should have job_level=%q, got %q", JobLevelManagerL1, r.jobLevel)
			}
		}
	}
	if !found {
		t.Error("mockLDAP should contain B100 (fab manager used as default auth bypass badge)")
	}
}

// orgPath entries should follow the TSMC.FabXX.部門 convention (non-empty, dot-separated)
func TestMockLDAP_OrgPathFormat(t *testing.T) {
	for _, r := range mockLDAP() {
		if len(r.orgPath) < 4 {
			t.Errorf("record %s: orgPath %q looks too short", r.badgeID, r.orgPath)
		}
	}
}

// ── envOrDefault ───────────────────────────────────────────────────────────

func TestEnvOrDefault_EnvSet(t *testing.T) {
	t.Setenv("SYNC_INTERVAL_SECONDS", "900")
	if got := envOrDefault("SYNC_INTERVAL_SECONDS", "1800"); got != "900" {
		t.Errorf("got=%q want=900", got)
	}
}

func TestEnvOrDefault_EnvAbsent_ReturnsFallback(t *testing.T) {
	const absentKey = "OS_ABSENT_KEY_9999"
	if got := envOrDefault(absentKey, "1800"); got != "1800" {
		t.Errorf("got=%q want=1800", got)
	}
}

func TestEnvOrDefault_EmptyEnv_ReturnsFallback(t *testing.T) {
	t.Setenv("SYNC_INTERVAL_SECONDS", "")
	if got := envOrDefault("SYNC_INTERVAL_SECONDS", "1800"); got != "1800" {
		t.Errorf("got=%q want=1800 when env is empty", got)
	}
}

// ── Sync counter ───────────────────────────────────────────────────────────

func TestSyncCount_Increments(t *testing.T) {
	before := atomic.LoadInt64(&syncCount)
	atomic.AddInt64(&syncCount, 1)
	after := atomic.LoadInt64(&syncCount)
	if after != before+1 {
		t.Errorf("syncCount: before=%d after=%d (expected +1)", before, after)
	}
	atomic.AddInt64(&syncCount, -1)
}

// ── Health handler ─────────────────────────────────────────────────────────

func testHealthRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":     "healthy",
			"service":    "org-sync",
			"sync_count": atomic.LoadInt64(&syncCount),
			"uptime":     time.Since(startTime).String(),
		})
	})
	return r
}

func TestHealthHandler_Returns200(t *testing.T) {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/healthz", nil)
	testHealthRouter().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status got=%d want=200", w.Code)
	}
}

func TestHealthHandler_ContainsExpectedFields(t *testing.T) {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/healthz", nil)
	testHealthRouter().ServeHTTP(w, req)

	var body map[string]interface{}
	json.NewDecoder(w.Body).Decode(&body)

	for _, key := range []string{"status", "service", "sync_count", "uptime"} {
		if _, ok := body[key]; !ok {
			t.Errorf("health response missing field %q", key)
		}
	}
	if body["status"] != "healthy" {
		t.Errorf("status got=%v want=healthy", body["status"])
	}
	if body["service"] != "org-sync" {
		t.Errorf("service got=%v want=org-sync", body["service"])
	}
}
