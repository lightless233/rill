package handler

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/0x2E/fusion/internal/model"
	"github.com/0x2E/fusion/internal/store"
	"github.com/0x2E/fusion/internal/translator"
	"github.com/gin-gonic/gin"
)

const (
	maxTranslationBlocks           = 2000
	maxListTranslationItems        = 50
	maxListTranslationTitleRunes   = 500
	maxListTranslationSummaryRunes = 2000
	listCacheSummaryBlockID        = 0
)

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
	seenIDs := make(map[int]struct{}, len(req.Blocks))
	for _, b := range req.Blocks {
		if b.ID < 0 {
			return nil, fmt.Errorf("invalid block id")
		}
		if _, dup := seenIDs[b.ID]; dup {
			return nil, fmt.Errorf("duplicate block id")
		}
		seenIDs[b.ID] = struct{}{}
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

func isCompleteCachedTranslation(title string, segments []translator.Segment, cached *model.ArticleTranslation) bool {
	if cached == nil {
		return false
	}
	if strings.TrimSpace(title) != "" && strings.TrimSpace(cached.TranslatedTitle) == "" {
		return false
	}
	if len(segments) == 0 {
		return true
	}
	byID := make(map[int]string, len(cached.Blocks))
	for _, b := range cached.Blocks {
		if strings.TrimSpace(b.Text) != "" {
			byID[b.ID] = b.Text
		}
	}
	for _, seg := range segments {
		if _, ok := byID[seg.ID]; !ok {
			return false
		}
	}
	return true
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
	if !isCompleteCachedTranslation(req.Title, segments, cached) {
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

type listTranslationItemRequest struct {
	ItemID  int64  `json:"item_id"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

type listTranslationRequest struct {
	Items []listTranslationItemRequest `json:"items"`
	Force bool                         `json:"force"`
}

type listTranslationItemEvent struct {
	ItemID            int64  `json:"item_id"`
	TranslatedTitle   string `json:"translated_title,omitempty"`
	TranslatedSummary string `json:"translated_summary,omitempty"`
	Cached            bool   `json:"cached"`
}

type listTranslationState struct {
	request           listTranslationItemRequest
	contentHash       string
	titleSegmentID    int
	summarySegmentID  int
	translatedTitle   string
	translatedSummary string
	receivedFields    int
	requiredFields    int
	cached            bool
	emitted           bool
}

type listTranslationSegmentTarget struct {
	stateIndex int
	isTitle    bool
}

func validateListTranslationRequest(req *listTranslationRequest) error {
	if len(req.Items) == 0 {
		return fmt.Errorf("items is required")
	}
	if len(req.Items) > maxListTranslationItems {
		return fmt.Errorf("too many items")
	}

	seen := make(map[int64]struct{}, len(req.Items))
	for _, item := range req.Items {
		if _, exists := seen[item.ItemID]; exists {
			return fmt.Errorf("duplicate item_id")
		}
		seen[item.ItemID] = struct{}{}

		if utf8.RuneCountInString(strings.TrimSpace(item.Title)) > maxListTranslationTitleRunes {
			return fmt.Errorf("title too long")
		}
		if utf8.RuneCountInString(strings.TrimSpace(item.Summary)) > maxListTranslationSummaryRunes {
			return fmt.Errorf("summary too long")
		}
		if strings.TrimSpace(item.Title) == "" && strings.TrimSpace(item.Summary) == "" {
			return fmt.Errorf("empty item")
		}
	}
	return nil
}

func cachedListTranslation(cached *model.ArticleTranslation, title, summary string) (string, string, bool) {
	if cached == nil {
		return "", "", false
	}

	translatedTitle := strings.TrimSpace(cached.TranslatedTitle)
	translatedSummary := ""
	for _, block := range cached.Blocks {
		if block.ID == listCacheSummaryBlockID {
			translatedSummary = strings.TrimSpace(block.Text)
			break
		}
	}

	if strings.TrimSpace(title) != "" && translatedTitle == "" {
		return "", "", false
	}
	if strings.TrimSpace(summary) != "" && translatedSummary == "" {
		return "", "", false
	}
	return translatedTitle, translatedSummary, true
}

func emitListItem(sendEvent func(string, any) error, state *listTranslationState, cached bool) error {
	if state.emitted {
		return nil
	}

	event := listTranslationItemEvent{
		ItemID: state.request.ItemID,
		Cached: cached,
	}
	if state.titleSegmentID >= 0 {
		event.TranslatedTitle = state.translatedTitle
	}
	if state.summarySegmentID >= 0 {
		event.TranslatedSummary = state.translatedSummary
	}
	if err := sendEvent("item", event); err != nil {
		return err
	}
	state.emitted = true
	return nil
}

func (h *Handler) saveListTranslation(state *listTranslationState, modelName string) error {
	blocks := make([]model.TranslatedSegment, 0, 1)
	if state.summarySegmentID >= 0 {
		blocks = append(blocks, model.TranslatedSegment{
			ID:   listCacheSummaryBlockID,
			Text: state.translatedSummary,
		})
	}

	return h.store.SaveArticleTranslation(store.SaveArticleTranslationParams{
		ContentHash:     state.contentHash,
		TargetLang:      translator.DefaultTargetLang,
		SourceTitle:     strings.TrimSpace(state.request.Title),
		TranslatedTitle: state.translatedTitle,
		Blocks:          blocks,
		Model:           modelName,
	})
}

func (h *Handler) translateItems(c *gin.Context) {
	var req listTranslationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequestError(c, "invalid request")
		return
	}
	if err := validateListTranslationRequest(&req); err != nil {
		badRequestError(c, err.Error())
		return
	}

	cfg, ok := h.loadConfiguredAIConfig(c)
	if !ok {
		return
	}

	states := make([]listTranslationState, len(req.Items))
	cachedCount := 0
	missCount := 0

	for i, item := range req.Items {
		title := strings.TrimSpace(item.Title)
		summary := strings.TrimSpace(item.Summary)
		state := listTranslationState{
			request:          item,
			contentHash:      translator.ComputeListContentHash(title, summary),
			titleSegmentID:   -1,
			summarySegmentID: -1,
		}
		if title != "" {
			state.titleSegmentID = i * 2
		}
		if summary != "" {
			state.summarySegmentID = i*2 + 1
		}
		states[i] = state

		if !req.Force {
			cached, err := h.store.GetArticleTranslation(state.contentHash, translator.DefaultTargetLang)
			if err != nil {
				internalError(c, err, "lookup list translation")
				return
			}
			translatedTitle, translatedSummary, complete := cachedListTranslation(cached, title, summary)
			if complete {
				states[i].translatedTitle = translatedTitle
				states[i].translatedSummary = translatedSummary
				states[i].cached = true
				cachedCount++
				continue
			}
		}

		if title != "" {
			states[i].requiredFields++
		}
		if summary != "" {
			states[i].requiredFields++
		}
		missCount++
	}

	sendEvent, _ := startSSEStream(c.Writer)
	modelName := cfg.Model
	if err := sendEvent("meta", gin.H{"model": modelName, "total": len(req.Items)}); err != nil {
		return
	}

	for i := range states {
		if !states[i].cached {
			continue
		}
		if err := emitListItem(sendEvent, &states[i], true); err != nil {
			return
		}
	}

	if missCount == 0 {
		_ = sendEvent("done", gin.H{"model": modelName, "translated": 0, "cached": cachedCount})
		return
	}

	segments := make([]translator.Segment, 0, len(req.Items)*2)
	targets := make(map[int]listTranslationSegmentTarget, len(req.Items)*2)
	for i := range states {
		if states[i].cached {
			continue
		}
		if states[i].titleSegmentID >= 0 {
			segments = append(segments, translator.Segment{
				ID:   states[i].titleSegmentID,
				Text: strings.TrimSpace(states[i].request.Title),
			})
			targets[states[i].titleSegmentID] = listTranslationSegmentTarget{stateIndex: i, isTitle: true}
		}
		if states[i].summarySegmentID >= 0 {
			segments = append(segments, translator.Segment{
				ID:   states[i].summarySegmentID,
				Text: strings.TrimSpace(states[i].request.Summary),
			})
			targets[states[i].summarySegmentID] = listTranslationSegmentTarget{stateIndex: i}
		}
	}

	_, err := h.translator.TranslateArticle(
		c.Request.Context(),
		cfg,
		"",
		segments,
		func(key, text string) error {
			segmentID, err := strconv.Atoi(key)
			if err != nil {
				return nil
			}
			target, exists := targets[segmentID]
			if !exists {
				return nil
			}

			state := &states[target.stateIndex]
			if target.isTitle {
				state.translatedTitle = text
			} else {
				state.translatedSummary = text
			}
			state.receivedFields++
			if state.receivedFields < state.requiredFields {
				return nil
			}

			if err := h.saveListTranslation(state, modelName); err != nil {
				slog.Warn("failed to cache list translation", "item_id", state.request.ItemID, "error", err)
			}
			return emitListItem(sendEvent, state, false)
		},
	)
	if err != nil {
		if c.Request.Context().Err() != nil {
			return
		}
		slog.Warn("list translation failed", "error", err)
		_ = sendEvent("error", gin.H{"error": err.Error()})
		return
	}

	for i := range states {
		if states[i].cached || states[i].emitted {
			continue
		}
		_ = sendEvent("error", gin.H{"error": "translation incomplete"})
		return
	}

	_ = sendEvent("done", gin.H{"model": modelName, "translated": missCount, "cached": cachedCount})
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
		if !isCompleteCachedTranslation(req.Title, segments, cached) {
			cached = nil
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

	sendEvent, _ := startSSEStream(c.Writer)

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

	sendEvent, _ := startSSEStream(c.Writer)

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
