package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"log/slog"

	"github.com/0x2E/fusion/internal/buildinfo"
	"github.com/0x2E/fusion/internal/extract"
	"github.com/0x2E/fusion/internal/pkg/httpc"
	"github.com/0x2E/fusion/internal/pull"
	"github.com/0x2E/fusion/internal/store"
	"github.com/gin-gonic/gin"
)

type createFeedRequest struct {
	GroupID                int64  `json:"group_id" binding:"required"`
	Name                   string `json:"name" binding:"required"`
	Link                   string `json:"link" binding:"required"`
	SiteURL                string `json:"site_url"`
	Proxy                  string `json:"proxy"`
	RefreshIntervalSeconds *int64 `json:"refresh_interval_seconds"`
}

type updateFeedRequest struct {
	GroupID                *int64  `json:"group_id"`
	Name                   *string `json:"name"`
	Link                   *string `json:"link"`
	SiteURL                *string `json:"site_url"`
	Suspended              *bool   `json:"suspended"`
	Proxy                  *string `json:"proxy"`
	ContentSelector        *string `json:"content_selector"`
	ExcludeSelector        *string `json:"exclude_selector"`
	RefreshIntervalSeconds *int64  `json:"refresh_interval_seconds"`
}

type validateFeedRequest struct {
	URL   string `json:"url" binding:"required"`
	Proxy string `json:"proxy"`
}

type discoveredFeed struct {
	Title string `json:"title"`
	Link  string `json:"link"`
}

type validateFeedResponse struct {
	Feeds []discoveredFeed `json:"feeds"`
}

type appInfoResponse struct {
	Version                 string  `json:"version"`
	Commit                  string  `json:"commit"`
	PullInterval            int     `json:"pull_interval"`
	AllowedRefreshIntervals []int64 `json:"allowed_refresh_intervals"`
	RefreshAllRunning       bool    `json:"refresh_all_running"`
	BookmarkCount           int     `json:"bookmark_count"`
}

type batchCreateFeedsRequest struct {
	Feeds []batchCreateFeedItem `json:"feeds" binding:"required"`
}

type batchCreateFeedItem struct {
	GroupID int64  `json:"group_id" binding:"required"`
	Name    string `json:"name" binding:"required"`
	Link    string `json:"link" binding:"required"`
	SiteURL string `json:"site_url"`
}

// allowedRefreshIntervals is the closed set of per-feed refresh overrides.
// It is exposed through GET /api/app so the frontend renders the same list
// the backend validates against.
var allowedRefreshIntervals = []int64{900, 1800, 3600, 7200, 21600, 43200, 86400}

func isAllowedRefreshInterval(v int64) bool {
	for _, allowed := range allowedRefreshIntervals {
		if v == allowed {
			return true
		}
	}
	return false
}

func invalidRefreshIntervalError() error {
	return fmt.Errorf("invalid refresh_interval_seconds: must be one of %v", allowedRefreshIntervals)
}

// normalizeCreateRefreshInterval maps the request semantics to storage: nil or
// 0 means "use the global interval" and is stored as NULL; any other value
// must be whitelisted.
func normalizeCreateRefreshInterval(v *int64) (*int64, error) {
	if v == nil || *v == 0 {
		return nil, nil
	}
	if !isAllowedRefreshInterval(*v) {
		return nil, invalidRefreshIntervalError()
	}
	return v, nil
}

func (h *Handler) getAppInfo(c *gin.Context) {
	bookmarkCount, err := h.store.CountBookmarks(store.ListBookmarksParams{})
	if err != nil {
		internalError(c, err, "count bookmarks")
		return
	}

	dataResponse(c, appInfoResponse{
		Version:                 buildinfo.Version,
		Commit:                  buildinfo.Commit,
		PullInterval:            h.config.PullInterval,
		AllowedRefreshIntervals: allowedRefreshIntervals,
		RefreshAllRunning:       h.isRefreshAllRunning(),
		BookmarkCount:           bookmarkCount,
	})
}

const refreshAllTimeout = 30 * time.Minute

func (h *Handler) listFeeds(c *gin.Context) {
	feeds, err := h.store.ListFeeds()
	if err != nil {
		internalError(c, err, "list feeds")
		return
	}

	listResponse(c, feeds, len(feeds))
}

func (h *Handler) getFeed(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		badRequestError(c, "invalid id")
		return
	}

	feed, err := h.store.GetFeed(id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFoundError(c, "feed")
			return
		}
		internalError(c, err, "get feed")
		return
	}

	dataResponse(c, feed)
}

func (h *Handler) createFeed(c *gin.Context) {
	var req createFeedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequestError(c, "invalid request")
		return
	}
	if err := httpc.ValidateRequestURL(c.Request.Context(), req.Link, h.config.AllowPrivateFeeds); err != nil {
		badRequestError(c, "invalid link")
		return
	}
	if _, err := httpc.ValidateProxyURL(req.Proxy); err != nil {
		badRequestError(c, "invalid proxy")
		return
	}
	refreshInterval, err := normalizeCreateRefreshInterval(req.RefreshIntervalSeconds)
	if err != nil {
		badRequestError(c, err.Error())
		return
	}

	feed, err := h.store.CreateFeed(req.GroupID, req.Name, req.Link, req.SiteURL, strings.TrimSpace(req.Proxy), refreshInterval)
	if err != nil {
		internalError(c, err, "create feed")
		return
	}

	// Trigger initial pull in background; puller binds per-attempt timeout
	// after semaphore admission and shutdown context for lifecycle cancellation.
	go func(feedID int64) {
		if err := h.puller.RefreshFeed(context.Background(), feedID); err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("initial feed pull failed", "feed_id", feedID, "error", err)
		}
	}(feed.ID)

	dataResponse(c, feed)
}

func (h *Handler) updateFeed(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		badRequestError(c, "invalid id")
		return
	}

	var req updateFeedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequestError(c, "invalid request")
		return
	}

	params := store.UpdateFeedParams{}
	if req.GroupID != nil {
		params.GroupID = req.GroupID
	}
	if req.Name != nil {
		params.Name = req.Name
	}
	if req.Link != nil {
		if err := httpc.ValidateRequestURL(c.Request.Context(), *req.Link, h.config.AllowPrivateFeeds); err != nil {
			badRequestError(c, "invalid link")
			return
		}
		params.Link = req.Link
	}
	if req.SiteURL != nil {
		params.SiteURL = req.SiteURL
	}
	if req.Suspended != nil {
		params.Suspended = req.Suspended
	}
	if req.Proxy != nil {
		if _, err := httpc.ValidateProxyURL(*req.Proxy); err != nil {
			badRequestError(c, "invalid proxy")
			return
		}
		trimmedProxy := strings.TrimSpace(*req.Proxy)
		params.Proxy = &trimmedProxy
	}
	if req.ContentSelector != nil {
		trimmed := strings.TrimSpace(*req.ContentSelector)
		if trimmed != "" {
			if err := extract.ValidateSelector(trimmed); err != nil {
				badRequestError(c, "invalid content_selector")
				return
			}
		}
		params.ContentSelector = &trimmed
	}
	if req.ExcludeSelector != nil {
		trimmed := strings.TrimSpace(*req.ExcludeSelector)
		if trimmed != "" {
			if err := extract.ValidateSelector(trimmed); err != nil {
				badRequestError(c, "invalid exclude_selector")
				return
			}
		}
		params.ExcludeSelector = &trimmed
	}
	// refresh_interval_seconds on update: absent = keep, 0 = clear back to the
	// global interval, other values must be whitelisted.
	if req.RefreshIntervalSeconds != nil {
		if *req.RefreshIntervalSeconds == 0 {
			params.ClearRefreshInterval = true
		} else if !isAllowedRefreshInterval(*req.RefreshIntervalSeconds) {
			badRequestError(c, invalidRefreshIntervalError().Error())
			return
		} else {
			params.RefreshIntervalSeconds = req.RefreshIntervalSeconds
		}
	}

	if err := h.store.UpdateFeed(id, params); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFoundError(c, "feed")
			return
		}
		internalError(c, err, "update feed")
		return
	}
	h.puller.Wake()

	feed, err := h.store.GetFeed(id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFoundError(c, "feed")
			return
		}
		internalError(c, err, "get updated feed")
		return
	}

	dataResponse(c, feed)
}

func (h *Handler) deleteFeed(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		badRequestError(c, "invalid id")
		return
	}

	if err := h.store.DeleteFeed(id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFoundError(c, "feed")
			return
		}
		internalError(c, err, "delete feed")
		return
	}
	h.puller.Wake()

	c.Status(http.StatusNoContent)
}

func (h *Handler) validateFeed(c *gin.Context) {
	var req validateFeedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequestError(c, "invalid request")
		return
	}

	target := strings.TrimSpace(req.URL)
	proxy := strings.TrimSpace(req.Proxy)
	allowPrivateFeeds := h.config != nil && h.config.AllowPrivateFeeds

	ctx, cancel := context.WithTimeout(c.Request.Context(), discoveryRequestTimeout)
	defer cancel()

	feeds, err := discoverFeeds(ctx, target, proxy, allowPrivateFeeds)
	if err != nil {
		var upstreamErr *upstreamFetchError
		switch {
		case errors.Is(err, httpc.ErrInvalidURL), errors.Is(err, httpc.ErrPrivateTarget):
			badRequestError(c, "invalid url")
		case errors.Is(err, httpc.ErrInvalidProxy):
			badRequestError(c, "invalid proxy")
		case errors.Is(err, httpc.ErrResolveHost), errors.As(err, &upstreamErr):
			slog.Warn("feed discovery upstream failure", "url", target, "error", err)
			badGatewayError(c, "failed to fetch feed url")
		default:
			internalError(c, err, "validate feed")
		}
		return
	}

	dataResponse(c, validateFeedResponse{Feeds: feeds})
}

func normalizeDiscoveredFeeds(found []discoveredFeed) []discoveredFeed {
	result := make([]discoveredFeed, 0, len(found))
	seen := make(map[string]struct{}, len(found))

	for _, feed := range found {
		link := strings.TrimSpace(feed.Link)
		if link == "" {
			continue
		}
		if _, exists := seen[link]; exists {
			continue
		}

		seen[link] = struct{}{}
		result = append(result, discoveredFeed{
			Title: pull.NormalizeTitle(feed.Title),
			Link:  link,
		})
	}

	return result
}

func (h *Handler) refreshFeed(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		badRequestError(c, "invalid id")
		return
	}

	if _, err := h.store.GetFeed(id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFoundError(c, "feed")
			return
		}
		internalError(c, err, "get feed for refresh")
		return
	}

	// Trigger refresh in background.
	// Do not use the request context here: once the handler returns, it may be cancelled.
	go func(feedID int64) {
		if err := h.puller.RefreshFeed(context.Background(), feedID); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			slog.Warn("refresh feed failed", "feed_id", feedID, "error", err)
		}
	}(id)

	c.Status(http.StatusAccepted)
}

func (h *Handler) refreshAllFeeds(c *gin.Context) {
	if !h.tryStartRefreshAll() {
		c.Status(http.StatusAccepted)
		return
	}

	// Run in background so the HTTP response returns immediately.
	go func() {
		defer h.finishRefreshAll()

		ctx, cancel := context.WithTimeout(context.Background(), refreshAllTimeout)
		defer cancel()

		if count, err := h.puller.RefreshAll(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			slog.Warn("refresh all feeds failed", "refreshed", count, "error", err)
		}
	}()

	c.Status(http.StatusAccepted)
}

func (h *Handler) tryStartRefreshAll() bool {
	h.refreshAllMu.Lock()
	defer h.refreshAllMu.Unlock()

	if h.refreshAllRunning {
		return false
	}

	h.refreshAllRunning = true
	return true
}

func (h *Handler) finishRefreshAll() {
	h.refreshAllMu.Lock()
	h.refreshAllRunning = false
	h.refreshAllMu.Unlock()
}

func (h *Handler) isRefreshAllRunning() bool {
	h.refreshAllMu.Lock()
	running := h.refreshAllRunning
	h.refreshAllMu.Unlock()
	return running
}

func (h *Handler) batchCreateFeeds(c *gin.Context) {
	var req batchCreateFeedsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequestError(c, "invalid request")
		return
	}

	inputs := make([]store.BatchCreateFeedsInput, len(req.Feeds))
	for i, f := range req.Feeds {
		if err := httpc.ValidateRequestURL(c.Request.Context(), f.Link, h.config.AllowPrivateFeeds); err != nil {
			badRequestError(c, "invalid link")
			return
		}
		inputs[i] = store.BatchCreateFeedsInput{
			GroupID: f.GroupID,
			Name:    f.Name,
			Link:    f.Link,
			SiteURL: f.SiteURL,
		}
	}

	result, err := h.store.BatchCreateFeeds(inputs)
	if err != nil {
		internalError(c, err, "batch create feeds")
		return
	}

	// Trigger initial pull for each new feed in background.
	for _, id := range result.CreatedIDs {
		go func(feedID int64) {
			if err := h.puller.RefreshFeed(context.Background(), feedID); err != nil && !errors.Is(err, context.Canceled) {
				slog.Warn("initial feed pull failed", "feed_id", feedID, "error", err)
			}
		}(id)
	}

	dataResponse(c, gin.H{
		"created": result.Created,
		"failed":  len(result.Errors),
		"errors":  result.Errors,
	})
}
