package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/0x2E/fusion/internal/model"
	"github.com/0x2E/fusion/internal/store"
	"github.com/0x2E/fusion/internal/translator"
	"github.com/gin-gonic/gin"
)

const maxTranslationBlocks = 2000

type translationBlockRequest struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
}

type translateArticleRequest struct {
	Title  string                    `json:"title"`
	Blocks []translationBlockRequest `json:"blocks"`
	Force  bool                      `json:"force"`
}

type testTranslationRequest struct {
	APIURL *string `json:"api_url"`
	APIKey *string `json:"api_key"`
	Model  *string `json:"model"`
}

func validateTranslateRequest(req *translateArticleRequest) ([]translator.Segment, error) {
	if len(req.Blocks) > maxTranslationBlocks {
		return nil, fmt.Errorf("too many blocks")
	}

	segments := make([]translator.Segment, 0, len(req.Blocks))
	for _, b := range req.Blocks {
		if b.ID < 0 {
			return nil, fmt.Errorf("invalid block id")
		}
		if strings.TrimSpace(b.Text) == "" {
			continue
		}
		segments = append(segments, translator.Segment{
			ID:   b.ID,
			Text: b.Text,
		})
	}

	if strings.TrimSpace(req.Title) == "" && len(segments) == 0 {
		return nil, fmt.Errorf("empty translation content")
	}
	return segments, nil
}

func (h *Handler) lookupTranslation(c *gin.Context) {
	var req translateArticleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequestError(c, "invalid request")
		return
	}

	segments, err := validateTranslateRequest(&req)
	if err != nil {
		badRequestError(c, err.Error())
		return
	}

	contentHash := translator.ComputeContentHash(req.Title, segments)
	cached, err := h.store.GetArticleTranslation(contentHash, translator.DefaultTargetLang)
	if err != nil {
		internalError(c, err, "lookup translation")
		return
	}
	if cached == nil {
		dataResponse(c, gin.H{"found": false})
		return
	}

	dataResponse(c, gin.H{
		"found":            true,
		"translated_title": cached.TranslatedTitle,
		"blocks":           cached.Blocks,
		"model":            cached.Model,
	})
}

func (h *Handler) testTranslation(c *gin.Context) {
	var req testTranslationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequestError(c, "invalid request")
		return
	}

	saved, err := h.store.GetTranslationConfig()
	if err != nil {
		internalError(c, err, "get translation config")
		return
	}

	apiURL := saved.APIURL
	if req.APIURL != nil {
		apiURL = strings.TrimSpace(*req.APIURL)
	}
	if err := translator.ValidateAPIURL(apiURL); err != nil {
		badRequestError(c, "invalid translation_api_url")
		return
	}

	apiKey := saved.APIKey
	if req.APIKey != nil && strings.TrimSpace(*req.APIKey) != "" {
		apiKey = strings.TrimSpace(*req.APIKey)
	}
	if apiKey == "" {
		badRequestError(c, "translation api key is not configured")
		return
	}

	modelName := saved.Model
	if req.Model != nil {
		modelName = strings.TrimSpace(*req.Model)
	}
	effectiveModel := translator.EffectiveModel(modelName)

	if err := h.translator.TestConnection(c.Request.Context(), translator.Config{
		APIURL: apiURL,
		APIKey: apiKey,
		Model:  effectiveModel,
	}); err != nil {
		if errors.Is(err, translator.ErrInvalidAPIURL) || errors.Is(err, translator.ErrMissingAPIKey) {
			badRequestError(c, err.Error())
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	dataResponse(c, gin.H{
		"ok":    true,
		"model": effectiveModel,
	})
}

func (h *Handler) loadConfiguredAIConfig(c *gin.Context) (translator.Config, bool) {
	storedCfg, err := h.store.GetTranslationConfig()
	if err != nil {
		internalError(c, err, "load ai config")
		return translator.Config{}, false
	}
	if strings.TrimSpace(storedCfg.APIKey) == "" {
		badRequestError(c, translator.ErrMissingAPIKey.Error())
		return translator.Config{}, false
	}
	if err := translator.ValidateAPIURL(storedCfg.APIURL); err != nil {
		badRequestError(c, "invalid translation_api_url")
		return translator.Config{}, false
	}
	return translator.Config{
		APIURL: storedCfg.APIURL,
		APIKey: storedCfg.APIKey,
		Model:  translator.EffectiveModel(storedCfg.Model),
	}, true
}

func startSSEStream(w http.ResponseWriter) func(eventName string, payload any) error {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	rc := http.NewResponseController(w)
	return func(eventName string, payload any) error {
		// Server-wide WriteTimeout (30s in cmd/fusion) applies from header read;
		// extend per chunk so long article streams do not cut off mid-flight.
		_ = rc.SetWriteDeadline(time.Now().Add(3 * time.Minute))
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventName, data); err != nil {
			return err
		}
		return rc.Flush()
	}
}

func (h *Handler) translateArticle(c *gin.Context) {
	var req translateArticleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequestError(c, "invalid request")
		return
	}

	segments, err := validateTranslateRequest(&req)
	if err != nil {
		badRequestError(c, err.Error())
		return
	}

	contentHash := translator.ComputeContentHash(req.Title, segments)
	var cached *model.ArticleTranslation
	if !req.Force {
		cached, err = h.store.GetArticleTranslation(contentHash, translator.DefaultTargetLang)
		if err != nil {
			internalError(c, err, "check cached translation")
			return
		}
	}

	var cfg translator.Config
	if cached == nil {
		var ok bool
		cfg, ok = h.loadConfiguredAIConfig(c)
		if !ok {
			return
		}
	}

	sendEvent := startSSEStream(c.Writer)

	totalCount := len(segments)
	if strings.TrimSpace(req.Title) != "" {
		totalCount++
	}

	if cached != nil {
		_ = sendEvent("meta", map[string]any{
			"total":  totalCount,
			"model":  cached.Model,
			"cached": true,
		})
		if strings.TrimSpace(cached.TranslatedTitle) != "" {
			_ = sendEvent("segment", map[string]any{
				"key":  "title",
				"text": cached.TranslatedTitle,
			})
		}
		for _, seg := range cached.Blocks {
			_ = sendEvent("segment", map[string]any{
				"key":  strconv.Itoa(seg.ID),
				"text": seg.Text,
			})
		}
		_ = sendEvent("done", map[string]any{
			"model":  cached.Model,
			"cached": true,
		})
		return
	}

	if err := sendEvent("meta", map[string]any{
		"total":  totalCount,
		"model":  cfg.Model,
		"cached": false,
	}); err != nil {
		return
	}

	result, err := h.translator.TranslateArticle(
		c.Request.Context(),
		cfg,
		req.Title,
		segments,
		func(key, text string) error {
			return sendEvent("segment", map[string]any{
				"key":  key,
				"text": text,
			})
		},
	)
	if err != nil {
		if c.Request.Context().Err() != nil {
			return
		}
		slog.Warn("article translation failed", "error", err)
		_ = sendEvent("error", map[string]any{
			"error": err.Error(),
		})
		return
	}

	modelSegments := make([]model.TranslatedSegment, len(result.Blocks))
	for i, b := range result.Blocks {
		modelSegments[i] = model.TranslatedSegment{
			ID:   b.ID,
			Text: b.Text,
		}
	}

	if err := h.store.SaveArticleTranslation(store.SaveArticleTranslationParams{
		ContentHash:     contentHash,
		TargetLang:      translator.DefaultTargetLang,
		SourceTitle:     req.Title,
		TranslatedTitle: result.TranslatedTitle,
		Blocks:          modelSegments,
		Model:           result.Model,
	}); err != nil {
		slog.Error("failed to persist cached translation", "error", err)
	}

	_ = sendEvent("done", map[string]any{
		"model":  result.Model,
		"cached": false,
	})
}

type summarizeArticleRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Force   bool   `json:"force"`
}

func (h *Handler) lookupSummary(c *gin.Context) {
	var req summarizeArticleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequestError(c, "invalid request")
		return
	}
	if strings.TrimSpace(req.Title) == "" && strings.TrimSpace(req.Content) == "" {
		badRequestError(c, "empty summary content")
		return
	}

	contentHash := translator.ComputeSummaryHash(req.Title, req.Content)
	cached, err := h.store.GetArticleSummary(contentHash, translator.DefaultTargetLang)
	if err != nil {
		internalError(c, err, "lookup summary")
		return
	}
	if cached == nil {
		dataResponse(c, gin.H{"found": false})
		return
	}

	dataResponse(c, gin.H{
		"found":   true,
		"summary": cached.Summary,
		"model":   cached.Model,
	})
}

func (h *Handler) summarizeArticle(c *gin.Context) {
	var req summarizeArticleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequestError(c, "invalid request")
		return
	}
	if strings.TrimSpace(req.Title) == "" && strings.TrimSpace(req.Content) == "" {
		badRequestError(c, "empty summary content")
		return
	}

	contentHash := translator.ComputeSummaryHash(req.Title, req.Content)
	var cached *model.ArticleSummary
	if !req.Force {
		var err error
		cached, err = h.store.GetArticleSummary(contentHash, translator.DefaultTargetLang)
		if err != nil {
			internalError(c, err, "check cached summary")
			return
		}
	}

	var cfg translator.Config
	if cached == nil {
		var ok bool
		cfg, ok = h.loadConfiguredAIConfig(c)
		if !ok {
			return
		}
	}

	sendEvent := startSSEStream(c.Writer)

	if cached != nil {
		_ = sendEvent("meta", map[string]any{
			"model":  cached.Model,
			"cached": true,
		})
		_ = sendEvent("delta", map[string]any{
			"text": cached.Summary,
		})
		_ = sendEvent("done", map[string]any{
			"summary": cached.Summary,
			"model":   cached.Model,
			"cached":  true,
		})
		return
	}

	if err := sendEvent("meta", map[string]any{
		"model":  cfg.Model,
		"cached": false,
	}); err != nil {
		return
	}

	summaryText, modelName, err := h.translator.SummarizeArticle(
		c.Request.Context(),
		cfg,
		req.Title,
		req.Content,
		func(delta string) error {
			return sendEvent("delta", map[string]any{
				"text": delta,
			})
		},
	)
	if err != nil {
		if c.Request.Context().Err() != nil {
			return
		}
		slog.Warn("article summary failed", "error", err)
		_ = sendEvent("error", map[string]any{
			"error": err.Error(),
		})
		return
	}

	if err := h.store.SaveArticleSummary(store.SaveArticleSummaryParams{
		ContentHash: contentHash,
		TargetLang:  translator.DefaultTargetLang,
		SourceTitle: req.Title,
		Summary:     summaryText,
		Model:       modelName,
	}); err != nil {
		slog.Error("failed to persist cached summary", "error", err)
	}

	_ = sendEvent("done", map[string]any{
		"summary": summaryText,
		"model":   modelName,
		"cached":  false,
	})
}
