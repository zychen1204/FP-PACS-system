package handlers

import (
	"sync"

	"pacs/backend/models"
)

// SharedState holds all shared data with a single protecting mutex
type SharedState struct {
	Mu           sync.RWMutex
	AccessLog    []models.AccessLog
	AntiPassback map[string]string
}
