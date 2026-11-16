package handler

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
)

const eventHeartbeatInterval = 20 * time.Second

// getEvents keeps an authenticated SSE connection open and emits only
// invalidation signals. Feed counts remain authoritative in GET /api/feeds.
func (h *Handler) getEvents(c *gin.Context) {
	if h.eventHub == nil {
		internalError(c, errors.New("event hub not configured"), "get events")
		return
	}

	notifications, unsubscribe := h.eventHub.Subscribe()
	defer unsubscribe()

	sendEvent, sendComment := startSSEStream(c.Writer)
	if err := sendEvent("ready", struct{}{}); err != nil {
		return
	}

	heartbeat := time.NewTicker(eventHeartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case _, ok := <-notifications:
			if !ok {
				return
			}
			if err := sendEvent("feeds_changed", struct{}{}); err != nil {
				return
			}
		case <-heartbeat.C:
			if err := sendComment("heartbeat"); err != nil {
				return
			}
		}
	}
}
