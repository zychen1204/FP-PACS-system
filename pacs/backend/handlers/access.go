package handlers

import (
	"fmt"
	"net/http"
	"time"

	"pacs/backend/models"

	"github.com/gin-gonic/gin"
)

// AccessHandler handles badge swipe requests
type AccessHandler struct {
	state *SharedState
}

// NewAccessHandler creates a new access handler
func NewAccessHandler(state *SharedState) *AccessHandler {
	return &AccessHandler{state: state}
}

// HandleSwipe processes a badge swipe request
func (h *AccessHandler) HandleSwipe(c *gin.Context) {
	var req models.SwipeRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.SwipeResponse{
			Status:  "ERROR",
			Message: "Invalid request",
		})
		return
	}

	fmt.Printf("[SWIPE] Badge: %s, Site: %s, Gate: %s, Direction: %s\n",
		req.BadgeID, req.SiteID, req.GateID, req.Direction)

	h.state.Mu.Lock()
	defer h.state.Mu.Unlock()

	// Check anti-passback
	apbKey := fmt.Sprintf("%s:%s", req.SiteID, req.BadgeID)

	if lastDir, exists := h.state.AntiPassback[apbKey]; exists && lastDir == req.Direction {
		// Anti-passback violation
		log := models.AccessLog{
			BadgeID:   req.BadgeID,
			SiteID:    req.SiteID,
			GateID:    req.GateID,
			Direction: req.Direction,
			Status:    "REJECTED_APB",
			Timestamp: time.Now().UTC(),
			Reason:    "Anti-Passback Violation",
		}
		h.state.AccessLog = append(h.state.AccessLog, log)

		fmt.Printf("[REJECTED] Anti-Passback violation for %s at %s\n", req.BadgeID, req.SiteID)
		c.JSON(http.StatusForbidden, models.SwipeResponse{
			Status:  "REJECTED_APB",
			Message: "Anti-Passback Violation",
		})
		return
	}

	// Update anti-passback state
	h.state.AntiPassback[apbKey] = req.Direction

	// Log successful access
	log := models.AccessLog{
		BadgeID:   req.BadgeID,
		SiteID:    req.SiteID,
		GateID:    req.GateID,
		Direction: req.Direction,
		Status:    "SUCCESS",
		Timestamp: time.Now().UTC(),
	}
	h.state.AccessLog = append(h.state.AccessLog, log)

	fmt.Printf("[SUCCESS] Access granted for %s\n", req.BadgeID)
	c.JSON(http.StatusOK, models.SwipeResponse{
		Status:  "SUCCESS",
		Message: "Access granted",
	})
}

// GetMetrics returns Prometheus metrics
func (h *AccessHandler) GetMetrics(c *gin.Context) {
	h.state.Mu.RLock()
	defer h.state.Mu.RUnlock()

	successCount := 0
	rejectedCount := 0

	for _, log := range h.state.AccessLog {
		if log.Status == "SUCCESS" {
			successCount++
		} else {
			rejectedCount++
		}
	}

	metricsText := fmt.Sprintf(`# HELP pacs_swipe_total Total badge swipes
# TYPE pacs_swipe_total counter
pacs_swipe_total{status="success"} %d
pacs_swipe_total{status="rejected"} %d
`, successCount, rejectedCount)

	c.Data(http.StatusOK, "text/plain", []byte(metricsText))
}

// HealthCheck returns the health status
func (h *AccessHandler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "healthy"})
}
