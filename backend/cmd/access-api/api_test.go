package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"pacs/backend/internal/cache"
	"pacs/backend/internal/queue"
)

// testRedisAddr exposes the miniredis address so individual tests can inspect
// the Redis state (e.g. stream length for FR-4 persistence check).
var testRedisAddr string

// TestMain wires the package-level globals (redisCache, eventStream) to a
// fresh in-process miniredis before the test suite runs, then tears it down.
func TestMain(m *testing.M) {
	mr, err := miniredis.Run()
	if err != nil {
		panic("miniredis: " + err.Error())
	}
	defer mr.Close()

	testRedisAddr = mr.Addr()
	os.Setenv("REDIS_HOST", mr.Host())
	os.Setenv("REDIS_PORT", mr.Port())

	redisCache, err = cache.NewRedisCache()
	if err != nil {
		panic("cache init: " + err.Error())
	}
	defer func() { _ = redisCache.Close() }()

	eventStream, err = queue.NewRedisStream()
	if err != nil {
		panic("stream init: " + err.Error())
	}
	defer func() { _ = eventStream.Close() }()
	_ = eventStream.CreateConsumerGroup(context.Background())

	os.Exit(m.Run())
}

// newTestRouter builds a Gin router equivalent to the production setup.
func newTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery(), corsMiddleware())
	r.GET("/healthz", healthCheck)
	r.GET("/readyz", readinessCheck)
	r.GET("/metrics", getMetrics)
	r.POST("/v1/swipe", handleSwipe)
	return r
}

// swipeJSON returns a JSON body for /v1/swipe.
func swipeJSON(badgeID, siteID, gateID, direction string) *bytes.Buffer {
	b, _ := json.Marshal(map[string]string{
		"badge_id":  badgeID,
		"site_id":   siteID,
		"gate_id":   gateID,
		"direction": direction,
	})
	return bytes.NewBuffer(b)
}

// doSwipe fires one POST /v1/swipe using site "Site-A" by default.
func doSwipe(r *gin.Engine, badgeID, gateID, direction string) *httptest.ResponseRecorder {
	return doSwipeAt(r, badgeID, "Site-A", gateID, direction)
}

// doSwipeAt fires one POST /v1/swipe with an explicit site_id.
func doSwipeAt(r *gin.Engine, badgeID, siteID, gateID, direction string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/swipe", swipeJSON(badgeID, siteID, gateID, direction))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

// ── FR-1: sub-50ms write path / access granted ─────────────────────────────

func TestHandleSwipe_IN_Returns200Success(t *testing.T) {
	r := newTestRouter()
	w := doSwipe(r, "FR1_IN", "1-A", "IN")

	if w.Code != http.StatusOK {
		t.Fatalf("status got=%d want=200", w.Code)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "SUCCESS" {
		t.Errorf("response status got=%q want=SUCCESS", resp["status"])
	}
}

func TestHandleSwipe_OUT_AfterIN_Returns200(t *testing.T) {
	r := newTestRouter()
	doSwipe(r, "FR1_OUT", "1-A", "IN") // establish state
	w := doSwipe(r, "FR1_OUT", "1-A", "OUT")

	if w.Code != http.StatusOK {
		t.Fatalf("status got=%d want=200", w.Code)
	}
}

// ── FR-2: Anti-Passback ────────────────────────────────────────────────────

func TestHandleSwipe_APB_SameDirection_Returns403(t *testing.T) {
	r := newTestRouter()
	doSwipe(r, "FR2_APB", "1-A", "IN") // first IN → sets APB state

	w := doSwipe(r, "FR2_APB", "1-A", "IN") // second IN → APB
	if w.Code != http.StatusForbidden {
		t.Fatalf("status got=%d want=403 (APB)", w.Code)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "REJECTED_APB" {
		t.Errorf("response status got=%q want=REJECTED_APB", resp["status"])
	}
}

// ── FR-3: Rejection reason returned to caller ──────────────────────────────

func TestHandleSwipe_APB_IncludesErrorCode(t *testing.T) {
	r := newTestRouter()
	doSwipe(r, "FR3_APB", "1-A", "IN")

	w := doSwipe(r, "FR3_APB", "1-A", "IN") // APB rejection
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)

	if resp["error_code"] != "ERR_ANTI_PASSBACK" {
		t.Errorf("error_code got=%q want=ERR_ANTI_PASSBACK", resp["error_code"])
	}
	if resp["message"] == "" {
		t.Error("rejection response should include a human-readable message")
	}
}

// ── FR-4: Async event persistence via Redis Stream ─────────────────────────

func TestHandleSwipe_PublishesToRedisStream(t *testing.T) {
	r := newTestRouter()

	// Record stream length before swipe
	rc := redis.NewClient(&redis.Options{Addr: testRedisAddr})
	defer func() { _ = rc.Close() }()
	ctx := context.Background()

	before, _ := rc.XLen(ctx, queue.StreamName).Result()

	doSwipe(r, "FR4_STREAM", "1-A", "IN")

	// The publish is a goroutine; retry briefly to let it complete.
	var after int64
	for i := 0; i < 20; i++ {
		after, _ = rc.XLen(ctx, queue.StreamName).Result()
		if after > before {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if after <= before {
		t.Errorf("event should be published to stream: before=%d after=%d", before, after)
	}
}

// FR-4: Rejected events are also persisted to the stream (for audit)
func TestHandleSwipe_APBRejection_AlsoPublishesToStream(t *testing.T) {
	r := newTestRouter()
	rc := redis.NewClient(&redis.Options{Addr: testRedisAddr})
	defer func() { _ = rc.Close() }()
	ctx := context.Background()

	doSwipe(r, "FR4_APB_STREAM", "1-B", "IN")

	before, _ := rc.XLen(ctx, queue.StreamName).Result()
	doSwipe(r, "FR4_APB_STREAM", "1-B", "IN") // APB

	var after int64
	for i := 0; i < 20; i++ {
		after, _ = rc.XLen(ctx, queue.StreamName).Result()
		if after > before {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if after <= before {
		t.Errorf("APB rejected event should also be published to stream: before=%d after=%d", before, after)
	}
}

// ── Input validation ───────────────────────────────────────────────────────

func TestHandleSwipe_MissingBadgeID_Returns400(t *testing.T) {
	r := newTestRouter()
	w := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"site_id":"Site-A","gate_id":"1-A","direction":"IN"}`)
	req, _ := http.NewRequest("POST", "/v1/swipe", body)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status got=%d want=400", w.Code)
	}
}

func TestHandleSwipe_InvalidDirection_Returns400(t *testing.T) {
	r := newTestRouter()
	w := doSwipe(r, "B001", "1-A", "SIDEWAYS")
	if w.Code != http.StatusBadRequest {
		t.Errorf("status got=%d want=400 for invalid direction", w.Code)
	}
}

// ── Health / metrics ───────────────────────────────────────────────────────

func TestHealthCheck_Returns200(t *testing.T) {
	r := newTestRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/healthz", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status got=%d want=200", w.Code)
	}
}

// NFR-7: Prometheus-compatible metrics endpoint
func TestMetrics_ContainsSwipeCounters(t *testing.T) {
	r := newTestRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/metrics", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status got=%d want=200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "pacs_swipe_total") {
		t.Error("metrics body should contain pacs_swipe_total counter")
	}
}

// ── Tier hierarchy validation ──────────────────────────────────────────────

// Tier-2 IN without prior Tier-1 IN must be rejected.
func TestHandleSwipe_Tier2IN_WithoutTier1_Returns403(t *testing.T) {
	r := newTestRouter()
	w := doSwipe(r, "TIER_BADGE", "2-A", "IN") // no tier-1 IN first

	if w.Code != http.StatusForbidden {
		t.Fatalf("status got=%d want=403 for tier-2 IN without tier-1", w.Code)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "REJECTED_APB" {
		t.Errorf("status got=%q want=REJECTED_APB", resp["status"])
	}
	if resp["reason"] == "" {
		t.Error("tier violation response should include reason")
	}
}

// Tier-2 IN after Tier-1 IN must succeed.
func TestHandleSwipe_Tier2IN_AfterTier1IN_Returns200(t *testing.T) {
	r := newTestRouter()
	doSwipe(r, "TIER_BADGE2", "1-A", "IN") // enter tier-1 first
	w := doSwipe(r, "TIER_BADGE2", "2-A", "IN")

	if w.Code != http.StatusOK {
		t.Fatalf("status got=%d want=200 for tier-2 IN after tier-1 IN", w.Code)
	}
}

// Strict exit: Tier-1 OUT is rejected while Tier-2 is still IN.
func TestHandleSwipe_Tier1OUT_BlockedIfTier2StillIN(t *testing.T) {
	r := newTestRouter()
	doSwipe(r, "STRICT_B", "1-A", "IN") // enter outer
	doSwipe(r, "STRICT_B", "2-A", "IN") // enter inner
	w := doSwipe(r, "STRICT_B", "1-A", "OUT")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status got=%d want=403: tier-1 OUT should fail if tier-2 still IN", w.Code)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error_code"] != "ERR_TIER_VIOLATION" {
		t.Errorf("error_code got=%q want=ERR_TIER_VIOLATION", resp["error_code"])
	}
}

// Correct exit order: OUT inner first, then outer.
func TestHandleSwipe_CorrectExitOrder_Returns200(t *testing.T) {
	r := newTestRouter()
	doSwipe(r, "ORDER_B", "1-A", "IN")  // enter outer
	doSwipe(r, "ORDER_B", "2-A", "IN")  // enter inner
	doSwipe(r, "ORDER_B", "2-A", "OUT") // exit inner first
	w := doSwipe(r, "ORDER_B", "1-A", "OUT")
	if w.Code != http.StatusOK {
		t.Fatalf("status got=%d want=200: tier-1 OUT after tier-2 OUT should succeed", w.Code)
	}
}

// Site isolation: APB state is independent per site.
func TestHandleSwipe_SiteSeparation_IndependentAPB(t *testing.T) {
	r := newTestRouter()
	doSwipeAt(r, "SITE_B", "Site-A", "1-A", "IN") // IN at Site-A

	// Same badge, same gate, same direction but DIFFERENT site → should be allowed
	w := doSwipeAt(r, "SITE_B", "Site-B", "1-A", "IN")
	if w.Code != http.StatusOK {
		t.Fatalf("status got=%d want=200: different site should have independent APB", w.Code)
	}
}

// Within the same tier, any gate letter is valid for exit (1-A in, 1-B out).
func TestHandleSwipe_SameTier_CrossGate_AllowedOut(t *testing.T) {
	r := newTestRouter()
	doSwipe(r, "CROSS_B", "1-A", "IN") // enter via gate A
	w := doSwipe(r, "CROSS_B", "1-B", "OUT")
	if w.Code != http.StatusOK {
		t.Fatalf("status got=%d want=200: exit through same-tier different gate should be allowed", w.Code)
	}
}

// gateTier must parse both HTML (Gate-2B) and spec (2-B) formats for all lanes.
func TestGateTier_AllFormats(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"Gate-1A", "1"}, {"Gate-1B", "1"}, {"Gate-1C", "1"},
		{"Gate-2A", "2"}, {"Gate-2B", "2"}, {"Gate-2C", "2"},
		{"1-A", "1"}, {"1-B", "1"}, {"1-C", "1"},
		{"2-A", "2"}, {"2-B", "2"}, {"2-C", "2"},
	}
	for _, tc := range cases {
		if got := gateTier(tc.input); got != tc.want {
			t.Errorf("gateTier(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// CORS preflight should return 204
func TestCORSPreflight_Returns204(t *testing.T) {
	r := newTestRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("OPTIONS", "/v1/swipe", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("OPTIONS preflight got=%d want=204", w.Code)
	}
}
