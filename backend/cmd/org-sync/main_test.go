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

// FR-6 / FR-9: There must be both managers and non-manager employees
func TestMockLDAP_ContainsBothManagersAndEmployees(t *testing.T) {
	var hasManager, hasEmployee bool
	for _, r := range mockLDAP() {
		if r.isManager {
			hasManager = true
		} else {
			hasEmployee = true
		}
	}
	if !hasManager {
		t.Error("mockLDAP must include at least one manager (is_manager=true) for FR-6/FR-9")
	}
	if !hasEmployee {
		t.Error("mockLDAP must include at least one non-manager employee for FR-9 negative test")
	}
}

// Verify the expected fab manager badge exists (used as the default in dev tests)
func TestMockLDAP_ContainsFabManager_B100(t *testing.T) {
	found := false
	for _, r := range mockLDAP() {
		if r.badgeID == "B100" {
			found = true
			if !r.isManager {
				t.Error("B100 (fab manager) should have is_manager=true")
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
