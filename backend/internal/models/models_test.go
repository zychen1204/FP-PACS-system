package models

import (
	"encoding/json"
	"testing"
	"time"
)

// ── SwipeRequest ───────────────────────────────────────────────────────────

func TestSwipeRequest_JSONRoundtrip(t *testing.T) {
	want := SwipeRequest{BadgeID: "B001", SiteID: "Site-A", GateID: "G1", Direction: "IN"}
	b, _ := json.Marshal(want)
	var got SwipeRequest
	json.Unmarshal(b, &got)
	if got != want {
		t.Errorf("roundtrip mismatch: got=%+v want=%+v", got, want)
	}
}

func TestSwipeRequest_JSONFieldNames(t *testing.T) {
	req := SwipeRequest{BadgeID: "B001", SiteID: "Site-A", GateID: "G1", Direction: "IN"}
	b, _ := json.Marshal(req)
	var m map[string]interface{}
	json.Unmarshal(b, &m)

	for _, key := range []string{"badge_id", "site_id", "gate_id", "direction"} {
		if _, ok := m[key]; !ok {
			t.Errorf("JSON field %q should be present in SwipeRequest", key)
		}
	}
}

// ── SwipeResponse ──────────────────────────────────────────────────────────

// error_code is omitempty; should be absent when empty
func TestSwipeResponse_ErrorCodeOmittedWhenEmpty(t *testing.T) {
	resp := SwipeResponse{Status: "SUCCESS", Message: "Access granted"}
	b, _ := json.Marshal(resp)
	var m map[string]interface{}
	json.Unmarshal(b, &m)
	if _, ok := m["error_code"]; ok {
		t.Error("error_code should be omitted from JSON when empty (omitempty)")
	}
}

func TestSwipeResponse_ErrorCodePresentWhenSet(t *testing.T) {
	resp := SwipeResponse{Status: "REJECTED_APB", ErrorCode: "ERR_ANTI_PASSBACK"}
	b, _ := json.Marshal(resp)
	var m map[string]interface{}
	json.Unmarshal(b, &m)
	if m["error_code"] != "ERR_ANTI_PASSBACK" {
		t.Errorf("error_code got=%v want=ERR_ANTI_PASSBACK", m["error_code"])
	}
}

// ── AccessEvent ────────────────────────────────────────────────────────────

func TestAccessEvent_JSONFieldNames(t *testing.T) {
	ts := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	e := AccessEvent{
		ID: 1, BadgeID: "B001", SiteID: "S1",
		GateID: "G1", Direction: "IN", Status: "SUCCESS",
		Reason: "test", Timestamp: ts,
	}
	b, _ := json.Marshal(e)
	var m map[string]interface{}
	json.Unmarshal(b, &m)

	for _, key := range []string{"id", "badge_id", "site_id", "gate_id", "direction", "status", "timestamp"} {
		if _, ok := m[key]; !ok {
			t.Errorf("JSON field %q should be present in AccessEvent", key)
		}
	}
}

// reason is omitempty — absent when empty
func TestAccessEvent_ReasonOmittedWhenEmpty(t *testing.T) {
	e := AccessEvent{BadgeID: "B001", Status: "SUCCESS", Timestamp: time.Now()}
	b, _ := json.Marshal(e)
	var m map[string]interface{}
	json.Unmarshal(b, &m)
	if v, ok := m["reason"]; ok && v != "" {
		t.Error("reason should be omitted from JSON when empty (omitempty)")
	}
}

// ── AttendanceReport ───────────────────────────────────────────────────────

// first_in and last_out are *time.Time; should be omitted when nil
func TestAttendanceReport_NilTimesOmitted(t *testing.T) {
	r := AttendanceReport{EmployeeID: "B001", WorkDate: "2026-05-01", SwipeCount: 2}
	b, _ := json.Marshal(r)
	var m map[string]interface{}
	json.Unmarshal(b, &m)
	if _, ok := m["first_in"]; ok {
		t.Error("nil first_in should be omitted from JSON (omitempty)")
	}
	if _, ok := m["last_out"]; ok {
		t.Error("nil last_out should be omitted from JSON (omitempty)")
	}
}

func TestAttendanceReport_TimesIncludedWhenSet(t *testing.T) {
	firstIn := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	lastOut := time.Date(2026, 5, 1, 17, 0, 0, 0, time.UTC)
	r := AttendanceReport{
		EmployeeID: "B001", WorkDate: "2026-05-01",
		FirstIn: &firstIn, LastOut: &lastOut,
	}
	b, _ := json.Marshal(r)
	var m map[string]interface{}
	json.Unmarshal(b, &m)
	if _, ok := m["first_in"]; !ok {
		t.Error("first_in should appear in JSON when set")
	}
	if _, ok := m["last_out"]; !ok {
		t.Error("last_out should appear in JSON when set")
	}
}

// ── AttendanceTrend ────────────────────────────────────────────────────────

func TestAttendanceTrend_JSONFieldNames(t *testing.T) {
	tr := AttendanceTrend{Bucket: "2026-05-01", HeadCount: 30, AvgStayHrs: 8.5, TotalSwipes: 120}
	b, _ := json.Marshal(tr)
	var m map[string]interface{}
	json.Unmarshal(b, &m)

	for _, key := range []string{"bucket", "head_count", "avg_stay_hrs", "total_swipes"} {
		if _, ok := m[key]; !ok {
			t.Errorf("JSON field %q should be present in AttendanceTrend", key)
		}
	}
}

// ── Alert ──────────────────────────────────────────────────────────────────

// Optional pointer fields should be omitted when nil
func TestAlert_NilOptionalFieldsOmitted(t *testing.T) {
	a := Alert{
		ID:         1,
		AlertType:  "OFF_HOURS_ENTRY",
		Severity:   "MEDIUM",
		OccurredAt: time.Now(),
		// BadgeID, SiteID, GateID, ResolvedAt are nil
	}
	b, _ := json.Marshal(a)
	var m map[string]interface{}
	json.Unmarshal(b, &m)

	for _, key := range []string{"badge_id", "site_id", "gate_id", "resolved_at"} {
		if _, ok := m[key]; ok {
			t.Errorf("nil field %q should be omitted from JSON (omitempty)", key)
		}
	}
}

func TestAlert_OptionalFieldsIncludedWhenSet(t *testing.T) {
	bid, sid, gid := "B001", "Site-A", "G1"
	a := Alert{
		ID: 1, AlertType: "APB_BURST", Severity: "HIGH",
		BadgeID: &bid, SiteID: &sid, GateID: &gid,
		OccurredAt: time.Now(),
	}
	b, _ := json.Marshal(a)
	var m map[string]interface{}
	json.Unmarshal(b, &m)

	for _, key := range []string{"badge_id", "site_id", "gate_id"} {
		if _, ok := m[key]; !ok {
			t.Errorf("field %q should appear in JSON when set", key)
		}
	}
}
