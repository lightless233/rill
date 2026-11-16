package handler

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/0x2E/fusion/internal/store"
	"github.com/0x2E/fusion/internal/translator"
	"github.com/gin-gonic/gin"
)

const (
	maxArticleBlockTranslationBlocks = 50
	maxArticleBlockTextRunes         = 20000
	maxArticleTitleRunes             = 2000
)

type articleBlockRequest struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
}

type translateArticleBlocksRequest struct {
	Title          string                `json:"title"`
	Blocks         []articleBlockRequest `json:"blocks"`
	TranslateTitle bool                  `json:"translate_title"`
	Force          bool                  `json:"force"`
}

type articleBlockTranslationState struct {
	request     articleBlockRequest
	contentHash string
	translated  string
}

func validateArticleBlocksRequest(req *translateArticleBlocksRequest) error {
	if utf8.RuneCountInString(strings.TrimSpace(req.Title)) > maxArticleTitleRunes {
		return fmt.Errorf("title too long")
	}
	if len(req.Blocks) > maxArticleBlockTranslationBlocks {
		return fmt.Errorf("too many blocks")
	}

	seen := make(map[int]struct{}, len(req.Blocks))
	hasContent := strings.TrimSpace(req.Title) != "" && req.TranslateTitle
	for _, block := range req.Blocks {
		if block.ID < 0 {
			return fmt.Errorf("invalid block id")
		}
		if _, exists := seen[block.ID]; exists {
			return fmt.Errorf("duplicate block id")
		}
		seen[block.ID] = struct{}{}

		if utf8.RuneCountInString(strings.TrimSpace(block.Text)) > maxArticleBlockTextRunes {
			return fmt.Errorf("block too long")
		}
		if strings.TrimSpace(block.Text) != "" {
			hasContent = true
		}
	}
	if !hasContent {
		return fmt.Errorf("empty translation request")
	}
	return nil
}

func (h *Handler) saveArticleBlockTranslation(contentHash, targetLang, sourceText, translatedText, modelName string) {
	if err := h.store.SaveArticleBlockTranslation(store.ArticleBlockTranslationParams{
		ContentHash:    contentHash,
		TargetLang:     targetLang,
		SourceText:     sourceText,
		TranslatedText: translatedText,
		Model:          modelName,
	}); err != nil {
		slog.Warn("failed to cache article block translation", "content_hash", contentHash, "error", err)
	}
}

// translateArticleBlocks translates only the article blocks requested by the
// frontend. Blocks and the title are cached independently, so viewport-driven
// partial progress survives reopening without translating the unseen body.
func (h *Handler) translateArticleBlocks(c *gin.Context) {
	var req translateArticleBlocksRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequestError(c, "invalid request")
		return
	}
	if err := validateArticleBlocksRequest(&req); err != nil {
		badRequestError(c, err.Error())
		return
	}

	cfg, ok := h.loadConfiguredAIConfig(c)
	if !ok {
		return
	}

	title := strings.TrimSpace(req.Title)
	titleHash := translator.ComputeArticleTitleHash(title)
	titleCached := ""
	if req.TranslateTitle && title != "" && !req.Force {
		cached, err := h.store.GetArticleBlockTranslation(titleHash, translator.DefaultTargetLang)
		if err != nil {
			internalError(c, err, "lookup article title translation")
			return
		}
		if cached != nil {
			titleCached = cached.TranslatedText
		}
	}

	states := make([]articleBlockTranslationState, 0, len(req.Blocks))
	cachedBlocks := 0
	missBlocks := 0
	for _, block := range req.Blocks {
		text := strings.TrimSpace(block.Text)
		if text == "" {
			continue
		}
		state := articleBlockTranslationState{
			request:     articleBlockRequest{ID: block.ID, Text: text},
			contentHash: translator.ComputeArticleBlockHash(text),
		}
		if !req.Force {
			cached, err := h.store.GetArticleBlockTranslation(state.contentHash, translator.DefaultTargetLang)
			if err != nil {
				internalError(c, err, "lookup article block translation")
				return
			}
			if cached != nil {
				state.translated = cached.TranslatedText
				cachedBlocks++
			}
		}
		if state.translated == "" {
			missBlocks++
		}
		states = append(states, state)
	}

	sendEvent := startSSEStream(c.Writer)
	total := len(states)
	if req.TranslateTitle && title != "" {
		total++
	}
	if titleCached != "" {
		cachedBlocks++
	}
	if err := sendEvent("meta", gin.H{"model": cfg.Model, "total": total}); err != nil {
		return
	}

	if titleCached != "" {
		if err := sendEvent("segment", gin.H{"key": "title", "text": titleCached}); err != nil {
			return
		}
	}

	blockIndexByID := make(map[int]int, len(states))
	for i := range states {
		blockIndexByID[states[i].request.ID] = i
		if states[i].translated == "" {
			continue
		}
		if err := sendEvent("segment", gin.H{
			"key":  strconv.Itoa(states[i].request.ID),
			"text": states[i].translated,
		}); err != nil {
			return
		}
	}

	translateTitle := req.TranslateTitle && title != ""
	segments := make([]translator.Segment, 0, missBlocks)
	for i := range states {
		if states[i].translated != "" {
			continue
		}
		segments = append(segments, translator.Segment{
			ID:   states[i].request.ID,
			Text: states[i].request.Text,
		})
	}

	if !translateTitle && len(segments) == 0 {
		_ = sendEvent("done", gin.H{"model": cfg.Model, "translated": 0, "cached": cachedBlocks})
		return
	}

	titleForContext := ""
	if translateTitle && titleCached == "" {
		titleForContext = title
	}
	_, err := h.translator.TranslateArticle(
		c.Request.Context(),
		cfg,
		titleForContext,
		segments,
		func(key, text string) error {
			if key == "title" {
				if !translateTitle {
					return nil
				}
				h.saveArticleBlockTranslation(titleHash, translator.DefaultTargetLang, title, text, cfg.Model)
				return sendEvent("segment", gin.H{"key": "title", "text": text})
			}

			blockID, err := strconv.Atoi(key)
			if err != nil {
				return nil
			}
			index, exists := blockIndexByID[blockID]
			if !exists {
				return nil
			}
			state := &states[index]
			state.translated = text
			h.saveArticleBlockTranslation(state.contentHash, translator.DefaultTargetLang, state.request.Text, text, cfg.Model)
			return sendEvent("segment", gin.H{"key": key, "text": text})
		},
	)
	if err != nil {
		if c.Request.Context().Err() != nil {
			return
		}
		slog.Warn("article block translation failed", "error", err)
		_ = sendEvent("error", gin.H{"error": err.Error()})
		return
	}

	for _, state := range states {
		if state.translated == "" {
			_ = sendEvent("error", gin.H{"error": "translation incomplete"})
			return
		}
	}
	if translateTitle && titleCached == "" {
		// TranslateArticle enforces the title segment when it is supplied; if it
		// returned successfully but the callback did not run, fail explicitly.
		if cached, _ := h.store.GetArticleBlockTranslation(titleHash, translator.DefaultTargetLang); cached == nil {
			_ = sendEvent("error", gin.H{"error": "translation incomplete"})
			return
		}
	}

	_ = sendEvent("done", gin.H{
		"model":      cfg.Model,
		"translated": missBlocks,
		"cached":     cachedBlocks,
	})
}
