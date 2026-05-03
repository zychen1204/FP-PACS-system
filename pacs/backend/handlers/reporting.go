package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"pacs/backend/models"

	"github.com/gin-gonic/gin"
)

// ReportingHandler handles report queries
type ReportingHandler struct {
	state *SharedState
}

// NewReportingHandler creates a new reporting handler
func NewReportingHandler(state *SharedState) *ReportingHandler {
	return &ReportingHandler{
		state: state,
	}
}

// GetAttendanceReport retrieves attendance report with hierarchical filtering
func (h *ReportingHandler) GetAttendanceReport(c *gin.Context) {
	h.state.Mu.RLock()
	defer h.state.Mu.RUnlock()

	if len(h.state.AccessLog) == 0 {
		c.JSON(http.StatusOK, []models.AttendanceReport{})
		return
	}

	// Group by employee and date
	type reportKey struct {
		EmployeeID string
		WorkDate   string
	}

	reports := make(map[reportKey]*models.AttendanceReport)

	for _, log := range h.state.AccessLog {
		if log.Status != "SUCCESS" {
			continue
		}

		badgeID := log.BadgeID
		workDate := log.Timestamp.Format("2006-01-02")

		key := reportKey{
			EmployeeID: badgeID,
			WorkDate:   workDate,
		}

		if _, exists := reports[key]; !exists {
			orgPath := "TSMC.Fab12"
			if log.SiteID == "Site-B" {
				orgPath = "TSMC.Fab15"
			}
			reports[key] = &models.AttendanceReport{
				EmployeeID: badgeID,
				Name:       fmt.Sprintf("Employee %s", strings.TrimLeft(badgeID, "B")),
				OrgPath:    orgPath,
				WorkDate:   workDate,
				SwipeCount: 0,
			}
		}

		rep := reports[key]
		rep.SwipeCount++

		if log.Direction == "IN" && rep.FirstIn == nil {
			ts := log.Timestamp
			rep.FirstIn = &ts
		} else if log.Direction == "OUT" {
			ts := log.Timestamp
			rep.LastOut = &ts
		}
	}

	// Convert to slice
	var reportList []models.AttendanceReport
	for _, rep := range reports {
		reportList = append(reportList, *rep)
	}

	// Sort by date and name
	sort.Slice(reportList, func(i, j int) bool {
		if reportList[i].WorkDate != reportList[j].WorkDate {
			return reportList[i].WorkDate > reportList[j].WorkDate
		}
		return reportList[i].EmployeeID < reportList[j].EmployeeID
	})

	fmt.Printf("[REPORT] Generated report with %d records\n", len(reportList))
	c.JSON(http.StatusOK, reportList)
}

// HealthCheck returns the health status
func (h *ReportingHandler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "healthy"})
}

// ReadinessCheck returns the readiness status
func (h *ReportingHandler) ReadinessCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ready": true})
}
