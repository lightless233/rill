package store

import (
	"testing"

	"github.com/0x2E/fusion/internal/model"
)

func TestArticleTranslationCacheRoundTrip(t *testing.T) {
	store, _ := setupTestDB(t)
	defer closeStore(t, store)

	got, err := store.GetArticleTranslation("missing-hash", "zh-CN")
	if err != nil {
		t.Fatalf("GetArticleTranslation(missing) failed: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil on cache miss, got %+v", got)
	}

	err = store.SaveArticleTranslation(SaveArticleTranslationParams{
		ContentHash:     "hash-1",
		TargetLang:      "zh-CN",
		SourceTitle:     "Hello World",
		TranslatedTitle: "你好世界",
		Blocks: []model.TranslatedSegment{
			{ID: 0, Text: "第一段"},
			{ID: 1, Text: "第二段"},
		},
		Model: "deepseek-chat",
	})
	if err != nil {
		t.Fatalf("SaveArticleTranslation failed: %v", err)
	}

	got, err = store.GetArticleTranslation("hash-1", "zh-CN")
	if err != nil || got == nil {
		t.Fatalf("GetArticleTranslation failed: err=%v got=%v", err, got)
	}
	if got.TranslatedTitle != "你好世界" || got.Model != "deepseek-chat" || len(got.Blocks) != 2 {
		t.Fatalf("unexpected cached translation: %+v", got)
	}

	// Overwrite with updated model & translation.
	err = store.SaveArticleTranslation(SaveArticleTranslationParams{
		ContentHash:     "hash-1",
		TargetLang:      "zh-CN",
		SourceTitle:     "Hello World",
		TranslatedTitle: "新版标题",
		Blocks:          []model.TranslatedSegment{{ID: 0, Text: "新版第一段"}},
		Model:           "deepseek-reasoner",
	})
	if err != nil {
		t.Fatalf("SaveArticleTranslation overwrite failed: %v", err)
	}
	got, err = store.GetArticleTranslation("hash-1", "zh-CN")
	if err != nil || got == nil {
		t.Fatalf("GetArticleTranslation after overwrite failed: err=%v got=%v", err, got)
	}
	if got.TranslatedTitle != "新版标题" || got.Model != "deepseek-reasoner" || len(got.Blocks) != 1 {
		t.Fatalf("unexpected overwritten translation: %+v", got)
	}
}

func TestArticleBlockTranslationCacheRoundTrip(t *testing.T) {
	store, _ := setupTestDB(t)
	defer closeStore(t, store)

	got, err := store.GetArticleBlockTranslation("missing-block", "zh-CN")
	if err != nil {
		t.Fatalf("GetArticleBlockTranslation(missing) failed: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil on cache miss, got %+v", got)
	}

	err = store.SaveArticleBlockTranslation(ArticleBlockTranslationParams{
		ContentHash:    "block-hash-1",
		TargetLang:     "zh-CN",
		SourceText:     "First paragraph.",
		TranslatedText: "第一段。",
		Model:          "deepseek-chat",
	})
	if err != nil {
		t.Fatalf("SaveArticleBlockTranslation failed: %v", err)
	}

	got, err = store.GetArticleBlockTranslation("block-hash-1", "zh-CN")
	if err != nil || got == nil {
		t.Fatalf("GetArticleBlockTranslation failed: err=%v got=%v", err, got)
	}
	if got.SourceText != "First paragraph." || got.TranslatedText != "第一段。" || got.Model != "deepseek-chat" {
		t.Fatalf("unexpected cached block: %+v", got)
	}

	err = store.SaveArticleBlockTranslation(ArticleBlockTranslationParams{
		ContentHash:    "block-hash-1",
		TargetLang:     "zh-CN",
		SourceText:     "First paragraph.",
		TranslatedText: "更新后的第一段。",
		Model:          "deepseek-reasoner",
	})
	if err != nil {
		t.Fatalf("SaveArticleBlockTranslation overwrite failed: %v", err)
	}
	got, err = store.GetArticleBlockTranslation("block-hash-1", "zh-CN")
	if err != nil || got == nil {
		t.Fatalf("GetArticleBlockTranslation after overwrite failed: err=%v got=%v", err, got)
	}
	if got.TranslatedText != "更新后的第一段。" || got.Model != "deepseek-reasoner" {
		t.Fatalf("unexpected overwritten block: %+v", got)
	}
}

func TestArticleSummaryCacheRoundTrip(t *testing.T) {
	store, _ := setupTestDB(t)
	defer closeStore(t, store)

	got, err := store.GetArticleSummary("missing-summary-hash", "zh-CN")
	if err != nil {
		t.Fatalf("GetArticleSummary(missing) failed: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil on cache miss, got %+v", got)
	}

	err = store.SaveArticleSummary(SaveArticleSummaryParams{
		ContentHash: "sum-hash-1",
		TargetLang:  "zh-CN",
		SourceTitle: "Hello World",
		Summary:     "核心要点总结。\n\n- 要点一\n- 要点二",
		Model:       "deepseek-chat",
	})
	if err != nil {
		t.Fatalf("SaveArticleSummary failed: %v", err)
	}

	got, err = store.GetArticleSummary("sum-hash-1", "zh-CN")
	if err != nil || got == nil {
		t.Fatalf("GetArticleSummary failed: err=%v got=%v", err, got)
	}
	if got.UpdatedAt <= 0 {
		t.Fatalf("expected positive updated_at, got %d", got.UpdatedAt)
	}
	if got.Summary != "核心要点总结。\n\n- 要点一\n- 要点二" || got.Model != "deepseek-chat" {
		t.Fatalf("unexpected cached summary: %+v", got)
	}

	err = store.SaveArticleSummary(SaveArticleSummaryParams{
		ContentHash: "sum-hash-1",
		TargetLang:  "zh-CN",
		SourceTitle: "Hello World",
		Summary:     "更新后的总结。",
		Model:       "deepseek-reasoner",
	})
	if err != nil {
		t.Fatalf("SaveArticleSummary overwrite failed: %v", err)
	}
	got, err = store.GetArticleSummary("sum-hash-1", "zh-CN")
	if err != nil || got == nil {
		t.Fatalf("GetArticleSummary overwrite read failed: err=%v got=%v", err, got)
	}
	if got.Summary != "更新后的总结。" || got.Model != "deepseek-reasoner" {
		t.Fatalf("unexpected overwritten summary: %+v", got)
	}
}
