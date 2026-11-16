package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/0x2E/fusion/internal/model"
)

// SaveArticleTranslationParams holds fields persisted for a cached translation.
type SaveArticleTranslationParams struct {
	ContentHash     string
	TargetLang      string
	SourceTitle     string
	TranslatedTitle string
	Blocks          []model.TranslatedSegment
	Model           string
}

// GetArticleTranslation fetches a cached article translation if present.
// Returns (nil, nil) when no cached entry exists.
func (s *Store) GetArticleTranslation(contentHash, targetLang string) (*model.ArticleTranslation, error) {
	var (
		blocksJSON string
		result     model.ArticleTranslation
	)
	err := s.db.QueryRow(`
		SELECT content_hash, target_lang, translated_title, blocks_json, model, updated_at
		FROM article_translations
		WHERE content_hash = :content_hash AND target_lang = :target_lang
	`, sql.Named("content_hash", contentHash), sql.Named("target_lang", targetLang)).Scan(
		&result.ContentHash,
		&result.TargetLang,
		&result.TranslatedTitle,
		&blocksJSON,
		&result.Model,
		&result.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get article translation: %w", err)
	}

	if err := json.Unmarshal([]byte(blocksJSON), &result.Blocks); err != nil {
		return nil, fmt.Errorf("decode cached translation blocks: %w", err)
	}
	return &result, nil
}

// GetArticleBlockTranslation fetches a cached block/title translation.
// Returns (nil, nil) when no cached entry exists.
func (s *Store) GetArticleBlockTranslation(contentHash, targetLang string) (*model.ArticleBlockTranslation, error) {
	result := &model.ArticleBlockTranslation{}
	err := s.db.QueryRow(`
		SELECT content_hash, target_lang, source_text, translated_text, model, updated_at
		FROM article_block_translations
		WHERE content_hash = :content_hash AND target_lang = :target_lang
	`, sql.Named("content_hash", contentHash), sql.Named("target_lang", targetLang)).Scan(
		&result.ContentHash,
		&result.TargetLang,
		&result.SourceText,
		&result.TranslatedText,
		&result.Model,
		&result.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get article block translation: %w", err)
	}
	return result, nil
}

// SaveArticleBlockTranslation persists or replaces one cached block/title
// translation.
func (s *Store) SaveArticleBlockTranslation(params ArticleBlockTranslationParams) error {
	_, err := s.db.Exec(`
		INSERT INTO article_block_translations (
			content_hash,
			target_lang,
			source_text,
			translated_text,
			model
		)
		VALUES (
			:content_hash,
			:target_lang,
			:source_text,
			:translated_text,
			:model
		)
		ON CONFLICT (content_hash, target_lang) DO UPDATE SET
			source_text = excluded.source_text,
			translated_text = excluded.translated_text,
			model = excluded.model,
			updated_at = unixepoch()
	`,
		sql.Named("content_hash", params.ContentHash),
		sql.Named("target_lang", params.TargetLang),
		sql.Named("source_text", params.SourceText),
		sql.Named("translated_text", params.TranslatedText),
		sql.Named("model", params.Model),
	)
	if err != nil {
		return fmt.Errorf("save article block translation: %w", err)
	}
	return nil
}

type ArticleBlockTranslationParams struct {
	ContentHash    string
	TargetLang     string
	SourceText     string
	TranslatedText string
	Model          string
}

// SaveArticleTranslation persists or replaces a cached article translation.
func (s *Store) SaveArticleTranslation(params SaveArticleTranslationParams) error {
	blocksJSON, err := json.Marshal(params.Blocks)
	if err != nil {
		return fmt.Errorf("encode translation blocks: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT INTO article_translations (
			content_hash,
			target_lang,
			source_title,
			translated_title,
			blocks_json,
			model
		)
		VALUES (
			:content_hash,
			:target_lang,
			:source_title,
			:translated_title,
			:blocks_json,
			:model
		)
		ON CONFLICT (content_hash, target_lang) DO UPDATE SET
			source_title = excluded.source_title,
			translated_title = excluded.translated_title,
			blocks_json = excluded.blocks_json,
			model = excluded.model,
			updated_at = unixepoch()
	`,
		sql.Named("content_hash", params.ContentHash),
		sql.Named("target_lang", params.TargetLang),
		sql.Named("source_title", params.SourceTitle),
		sql.Named("translated_title", params.TranslatedTitle),
		sql.Named("blocks_json", string(blocksJSON)),
		sql.Named("model", params.Model),
	)
	if err != nil {
		return fmt.Errorf("save article translation: %w", err)
	}
	return nil
}
