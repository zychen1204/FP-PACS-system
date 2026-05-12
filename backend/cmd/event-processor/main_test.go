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
	t.Setenv("HEALTH_PORT", "9100")
	if got := envOrDefault("HEALTH_PORT", "8082"); got != "9100" {
		t.Errorf("got=%q want=9100", got)
	}
}

func TestEnvOrDefault_EnvAbsent_ReturnsFallback(t *testing.T) {
	const absentKey = "EP_ABSENT_KEY_9999"
	if got := envOrDefault(absentKey, "8082"); got != "8082" {
		t.Errorf("got=%q want=8082", got)
	}
}

func TestEnvOrDefault_EmptyEnv_ReturnsFallback(t *testing.T) {
	t.Setenv("HEALTH_PORT", "")
	if got := envOrDefault("HEALTH_PORT", "8082"); got != "8082" {
		t.Errorf("got=%q want=8082 when env is empty", got)
	}
}

// ── Atomic counters ────────────────────────────────────────────────────────

// Verify that the processed / errCount atomics used by the health endpoint
// increment correctly — this is the operational metric tracked for NFR-7.
func TestProcessedCounter_Increments(t *testing.T) {
	before := atomic.LoadInt64(&processed)
	atomic.AddInt64(&processed, 1)
	after := atomic.LoadInt64(&processed)
	if after != before+1 {
		t.Errorf("processed counter: before=%d after=%d (expected +1)", before, after)
	}
	atomic.AddInt64(&processed, -1) // restore
}

func TestErrCounter_Increments(t *testing.T) {
	before := atomic.LoadInt64(&errCount)
	atomic.AddInt64(&errCount, 1)
	after := atomic.LoadInt64(&errCount)
	if after != before+1 {
		t.Errorf("errCount counter: before=%d after=%d (expected +1)", before, after)
	}
	atomic.AddInt64(&errCount, -1)
}

// ── Health handler ─────────────────────────────────────────────────────────

// Build the same handler that runHealthServer uses and verify its JSON shape.
func testHealthRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"service":   "event-processor",
			"processed": atomic.LoadInt64(&processed),
			"errors":    atomic.LoadInt64(&errCount),
			"uptime":    time.Since(startTime).String(),
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

	for _, key := range []string{"status", "service", "processed", "errors", "uptime"} {
		if _, ok := body[key]; !ok {
			t.Errorf("health response missing field %q", key)
		}
	}
	if body["status"] != "healthy" {
		t.Errorf("status got=%v want=healthy", body["status"])
	}
	if body["service"] != "event-processor" {
		t.Errorf("service got=%v want=event-processor", body["service"])
	}
}
