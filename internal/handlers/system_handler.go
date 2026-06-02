package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Health is a liveness probe (process is up).
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Ready is a readiness probe: the DB must be reachable.
func (h *Handler) Ready(c *gin.Context) {
	sqlDB, err := h.DB.DB()
	if err == nil {
		err = sqlDB.Ping()
	}
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "db": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}
