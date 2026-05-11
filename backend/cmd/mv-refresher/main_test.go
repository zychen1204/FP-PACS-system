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

// ── envOrDefault ───────────────────────────────────────────────────────────

func TestEnvOrDefault_EnvSet(t *testing.T) {
	t.Setenv("REFRESH_INTERVAL_SECONDS", "60")
	if got := envOrDefault("REFRESH_INTERVAL_SECONDS", "300"); got != "60" {
		t.Errorf("got=%q want=60", got)
	}
}

func TestEnvOrDefault_EnvAbsent_ReturnsFallback(t *testing.T) {
	const absentKey = "MV_ABSENT_KEY_9999"
	if got := envOrDefault(absentKey, "300"); got != "300" {
		t.Errorf("got=%q want=300", got)
	}
}

func TestEnvOrDefault_EmptyEnv_ReturnsFallback(t *testing.T) {
	t.Setenv("REFRESH_INTERVAL_SECONDS", "")
	if got := envOrDefault("REFRESH_INTERVAL_SECONDS", "300"); got != "300" {
		t.Errorf("got=%q want=300 when env is empty", got)
	}
}

// ── Refresh counters ───────────────────────────────────────────────────────

// Verify that atomic counters used by the health endpoint work correctly.
func TestRefreshCount_Increments(t *testing.T) {
	before := atomic.LoadInt64(&refreshCount)
	atomic.AddInt64(&refreshCount, 1)
	after := atomic.LoadInt64(&refreshCount)
	if after != before+1 {
		t.Errorf("refreshCount: before=%d after=%d (expected +1)", before, after)
	}
	atomic.AddInt64(&refreshCount, -1)
}

func TestRefreshErrors_Increments(t *testing.T) {
	before := atomic.LoadInt64(&refreshErrors)
	atomic.AddInt64(&refreshErrors, 1)
	after := atomic.LoadInt64(&refreshErrors)
	if after != before+1 {
		t.Errorf("refreshErrors: before=%d after=%d (expected +1)", before, after)
	}
	atomic.AddInt64(&refreshErrors, -1)
}

// ── Health handler ─────────────────────────────────────────────────────────

func testHealthRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":           "healthy",
			"service":          "mv-refresher",
			"refresh_count":    atomic.LoadInt64(&refreshCount),
			"refresh_errors":   atomic.LoadInt64(&refreshErrors),
			"last_refresh_ns":  atomic.LoadInt64(&lastRefreshDur),
			"uptime":           time.Since(startTime).String(),
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

	for _, key := range []string{"status", "service", "refresh_count", "refresh_errors", "last_refresh_ns", "uptime"} {
		if _, ok := body[key]; !ok {
			t.Errorf("health response missing field %q", key)
		}
	}
	if body["status"] != "healthy" {
		t.Errorf("status got=%v want=healthy", body["status"])
	}
	if body["service"] != "mv-refresher" {
		t.Errorf("service got=%v want=mv-refresher", body["service"])
	}
}
