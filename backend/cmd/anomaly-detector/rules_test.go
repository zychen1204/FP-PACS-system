package main

import (
	"testing"
	"time"

	"pacs/backend/internal/models"
)

// makeEvent is a helper to build test AccessEvents without boilerplate.
func makeEvent(status, direction, badgeID, siteID, gateID string, ts time.Time) models.AccessEvent {
	return models.AccessEvent{
		BadgeID:   badgeID,
		SiteID:    siteID,
		GateID:    gateID,
		Direction: direction,
		Status:    status,
		Timestamp: ts,
	}
}

// resetApbState clears the sliding-window counter between tests.
func resetApbState() {
	apbStateMu.Lock()
	apbState = make(map[string]*counter)
	apbStateMu.Unlock()
}

// resetTailgateState clears the tailgate counter between tests.
func resetTailgateState() {
	tailgateMu.Lock()
	tailgateState = make(map[string]*counter)
	tailgateMu.Unlock()
}

// ── isOffHoursEntry ────────────────────────────────────────────────────────

// FR-11: 22:00 以後（台北時間）的 IN 應觸發 OFF_HOURS_ENTRY
func TestIsOffHoursEntry_At22h_ReturnsTrue(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Taipei")
	ts := time.Date(2026, 1, 15, 22, 0, 0, 0, loc).UTC()
	e := makeEvent("SUCCESS", "IN", "B001", "S1", "G1", ts)
	if !isOffHoursEntry(e) {
		t.Error("22:00 Taipei SUCCESS IN should be flagged as off-hours")
	}
}

// FR-11: 深夜 03:00 的 IN 應觸發 OFF_HOURS_ENTRY
func TestIsOffHoursEntry_At03h_ReturnsTrue(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Taipei")
	ts := time.Date(2026, 1, 15, 3, 0, 0, 0, loc).UTC()
	e := makeEvent("SUCCESS", "IN", "B001", "S1", "G1", ts)
	if !isOffHoursEntry(e) {
		t.Error("03:00 Taipei SUCCESS IN should be flagged as off-hours")
	}
}

// FR-11: 上班時間（10:00）的 IN 不應觸發
func TestIsOffHoursEntry_At10h_ReturnsFalse(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Taipei")
	ts := time.Date(2026, 1, 15, 10, 0, 0, 0, loc).UTC()
	e := makeEvent("SUCCESS", "IN", "B001", "S1", "G1", ts)
	if isOffHoursEntry(e) {
		t.Error("10:00 Taipei IN should NOT be flagged as off-hours")
	}
}

// FR-11: REJECTED_APB 不應觸發（只有 SUCCESS 才算）
func TestIsOffHoursEntry_RejectedStatus_ReturnsFalse(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Taipei")
	ts := time.Date(2026, 1, 15, 23, 0, 0, 0, loc).UTC()
	e := makeEvent("REJECTED_APB", "IN", "B001", "S1", "G1", ts)
	if isOffHoursEntry(e) {
		t.Error("REJECTED_APB event should not trigger off-hours alert")
	}
}

// FR-11: OUT 方向不應觸發（只有 IN 才算）
func TestIsOffHoursEntry_OutDirection_ReturnsFalse(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Taipei")
	ts := time.Date(2026, 1, 15, 23, 0, 0, 0, loc).UTC()
	e := makeEvent("SUCCESS", "OUT", "B001", "S1", "G1", ts)
	if isOffHoursEntry(e) {
		t.Error("SUCCESS OUT should not trigger off-hours alert")
	}
}

// ── isApbBurst ─────────────────────────────────────────────────────────────

// FR-11: 30 分鐘內連續 3 次 REJECTED_APB 應觸發 APB_BURST
func TestIsApbBurst_ThreeRejections_ReturnsTrue(t *testing.T) {
	resetApbState()
	badge := "APB_BURST_T1"
	e := makeEvent("REJECTED_APB", "IN", badge, "S1", "G1", time.Now())

	isApbBurst(e) // 1st
	isApbBurst(e) // 2nd
	if !isApbBurst(e) { // 3rd → burst
		t.Error("3rd rejection within 30min window should trigger APB_BURST")
	}
}

// FR-11: 只有 2 次拒絕不應觸發
func TestIsApbBurst_TwoRejections_ReturnsFalse(t *testing.T) {
	resetApbState()
	badge := "APB_BURST_T2"
	e := makeEvent("REJECTED_APB", "IN", badge, "S1", "G1", time.Now())

	isApbBurst(e) // 1st
	if isApbBurst(e) { // 2nd — not yet 3
		t.Error("2nd rejection should NOT trigger APB_BURST")
	}
}

// FR-11: SUCCESS 事件不應計入 APB_BURST 計數
func TestIsApbBurst_SuccessEvent_NotCounted(t *testing.T) {
	resetApbState()
	badge := "APB_BURST_T3"
	e := makeEvent("SUCCESS", "IN", badge, "S1", "G1", time.Now())

	for i := 0; i < 5; i++ {
		if isApbBurst(e) {
			t.Errorf("SUCCESS event should never trigger APB_BURST (iteration %d)", i)
		}
	}
}

// FR-11: 視窗外的拒絕應重置計數器
func TestIsApbBurst_OutsideWindow_ResetsCount(t *testing.T) {
	resetApbState()
	badge := "APB_BURST_T4"
	now := time.Now()

	// 手動插入一個已過期的計數 (windowAt 在 31 分鐘前)
	apbStateMu.Lock()
	apbState[badge] = &counter{count: 2, windowAt: now.Add(-31 * time.Minute)}
	apbStateMu.Unlock()

	e := makeEvent("REJECTED_APB", "IN", badge, "S1", "G1", now)
	// 視窗過期 → 應重置為 count=1，不觸發
	if isApbBurst(e) {
		t.Error("expired window should reset counter; should not trigger on first rejection after reset")
	}
}

// ── isTailgating ───────────────────────────────────────────────────────────

// FR-11: 5 秒內同一閘門 3 次 IN 應觸發 TAILGATING
func TestIsTailgating_ThreeInFiveSeconds_ReturnsTrue(t *testing.T) {
	resetTailgateState()
	e := makeEvent("SUCCESS", "IN", "B001", "TG_SITE_1", "TG_GATE_1", time.Now())

	isTailgating(e) // 1st
	isTailgating(e) // 2nd
	if !isTailgating(e) { // 3rd → tailgate
		t.Error("3rd IN at same gate within 5s should trigger TAILGATING")
	}
}

// FR-11: 只有 2 次 IN 不應觸發
func TestIsTailgating_TwoIn_ReturnsFalse(t *testing.T) {
	resetTailgateState()
	e := makeEvent("SUCCESS", "IN", "B001", "TG_SITE_2", "TG_GATE_2", time.Now())

	isTailgating(e) // 1st
	if isTailgating(e) { // 2nd — not yet 3
		t.Error("2nd IN should NOT trigger TAILGATING")
	}
}

// FR-11: REJECTED 事件不應計入尾隨計數
func TestIsTailgating_RejectedEvent_NotCounted(t *testing.T) {
	resetTailgateState()
	e := makeEvent("REJECTED_APB", "IN", "B001", "TG_SITE_3", "TG_GATE_3", time.Now())

	for i := 0; i < 5; i++ {
		if isTailgating(e) {
			t.Errorf("REJECTED event should not count toward tailgating (iteration %d)", i)
		}
	}
}

// FR-11: OUT 方向不應計入尾隨計數
func TestIsTailgating_OutDirection_NotCounted(t *testing.T) {
	resetTailgateState()
	e := makeEvent("SUCCESS", "OUT", "B001", "TG_SITE_4", "TG_GATE_4", time.Now())

	for i := 0; i < 5; i++ {
		if isTailgating(e) {
			t.Errorf("OUT direction should not count toward tailgating (iteration %d)", i)
		}
	}
}

// FR-11: 不同閘門的計數器互相獨立
func TestIsTailgating_DifferentGates_Independent(t *testing.T) {
	resetTailgateState()
	e1 := makeEvent("SUCCESS", "IN", "B001", "SITE_X", "GATE_A", time.Now())
	e2 := makeEvent("SUCCESS", "IN", "B001", "SITE_X", "GATE_B", time.Now())

	// Saturate gate A
	isTailgating(e1)
	isTailgating(e1)

	// Gate B should still be at count=1, not triggered
	if isTailgating(e2) {
		t.Error("gate B counter should be independent from gate A")
	}
}
