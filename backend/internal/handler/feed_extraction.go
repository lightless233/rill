package handler

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/0x2E/fusion/internal/extract"
	"github.com/0x2E/fusion/internal/pkg/httpc"
	"github.com/0x2E/fusion/internal/store"
	"github.com/gin-gonic/gin"
)

const maxPreviewWorkTimeout = 20 * time.Second

type previewFeedExtractionRequest struct {
	URL             string  `json:"url" binding:"required"`
	ContentSelector string  `json:"content_selector" binding:"required"`
	ExcludeSelector string  `json:"exclude_selector"`
	Proxy           *string `json:"proxy"`
}

type previewFeedExtractionResponse struct {
	Content string `json:"content"`
	URL     string `json:"url"`
}

func (h *Handler) previewFeedExtraction(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.previewWorkTimeout())
	defer cancel()

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		badRequestError(c, "invalid id")
		return
	}

	var req previewFeedExtractionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		if ctx.Err() != nil {
			h.writePreviewError(c, ctx, err)
			return
		}
		badRequestError(c, "invalid request")
		return
	}
	if ctx.Err() != nil {
		h.writePreviewError(c, ctx, ctx.Err())
		return
	}

	feed, err := h.store.GetFeed(id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFoundError(c, "feed")
			return
		}
		internalError(c, err, "get feed for extraction preview")
		return
	}

	contentSelector := strings.TrimSpace(req.ContentSelector)
	if contentSelector == "" {
		badRequestError(c, "content_selector is required")
		return
	}
	if err := extract.ValidateSelector(contentSelector); err != nil {
		badRequestError(c, "invalid content_selector")
		return
	}
	excludeSelector := strings.TrimSpace(req.ExcludeSelector)
	if excludeSelector != "" {
		if err := extract.ValidateSelector(excludeSelector); err != nil {
			badRequestError(c, "invalid exclude_selector")
			return
		}
	}

	proxy := feed.Proxy
	if req.Proxy != nil {
		trimmed := strings.TrimSpace(*req.Proxy)
		if _, err := httpc.ValidateProxyURL(trimmed); err != nil {
			badRequestError(c, "invalid proxy")
			return
		}
		proxy = trimmed
	}

	articleURL := strings.TrimSpace(req.URL)
	if articleURL == "" {
		badRequestError(c, "invalid url")
		return
	}

	result, err := extract.Article(ctx, extract.Options{
		URL:             articleURL,
		ContentSelector: contentSelector,
		ExcludeSelector: excludeSelector,
		Proxy:           proxy,
		AllowPrivate:    h.config != nil && h.config.AllowPrivateFeeds,
		Timeout:         h.extractionClientTimeout(),
	})
	if err != nil {
		h.writePreviewError(c, ctx, err)
		return
	}
	if err := ctx.Err(); err != nil {
		h.writePreviewError(c, ctx, err)
		return
	}

	dataResponse(c, previewFeedExtractionResponse{
		Content: result.Content,
		URL:     result.FinalURL,
	})
}

func (h *Handler) previewWorkTimeout() time.Duration {
	if h.previewTimeout > 0 {
		return h.previewTimeout
	}
	if h.config != nil && h.config.PullTimeout > 0 {
		configured := time.Duration(h.config.PullTimeout) * time.Second
		if configured < maxPreviewWorkTimeout {
			return configured
		}
	}
	return maxPreviewWorkTimeout
}

func (h *Handler) extractionClientTimeout() time.Duration {
	if h.config != nil && h.config.PullTimeout > 0 {
		return time.Duration(h.config.PullTimeout) * time.Second
	}
	return maxPreviewWorkTimeout
}

func (h *Handler) writePreviewError(c *gin.Context, ctx context.Context, err error) {
	if c.Request.Context().Err() != nil {
		return
	}
	if ctx.Err() != nil {
		badGatewayError(c, "preview timed out")
		return
	}

	switch {
	case errors.Is(err, httpc.ErrInvalidURL),
		errors.Is(err, httpc.ErrPrivateTarget),
		errors.Is(err, httpc.ErrInvalidProxy):
		badRequestError(c, err.Error())
	case errors.Is(err, extract.ErrContentSelectorRequired),
		errors.Is(err, extract.ErrInvalidSelector):
		badRequestError(c, "invalid content_selector")
	case errors.Is(err, extract.ErrNoMatch),
		errors.Is(err, extract.ErrEmptyContent):
		badRequestError(c, "no matching content")
	case errors.Is(err, extract.ErrTooLarge):
		badGatewayError(c, "article page too large")
	case errors.Is(err, extract.ErrUnsupportedContentType):
		badGatewayError(c, "unsupported article content type")
	case errors.Is(err, httpc.ErrResolveHost),
		errors.Is(err, extract.ErrUpstream),
		errors.Is(err, context.DeadlineExceeded):
		badGatewayError(c, "failed to fetch article")
	default:
		internalError(c, err, "preview feed extraction")
	}
}
