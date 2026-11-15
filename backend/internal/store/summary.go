package store

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/0x2E/fusion/internal/model"
)

// SaveArticleSummaryParams holds fields persisted for a cached AI summary.
type SaveArticleSummaryParams struct {
	ContentHash string
	TargetLang  string
	SourceTitle string
	Summary     string
	Model       string
}

// GetArticleSummary fetches a cached article summary if present.
// Returns (nil, nil) when no cached entry exists.
func (s *Store) GetArticleSummary(contentHash, targetLang string) (*model.ArticleSummary, error) {
	var result model.ArticleSummary
	err := s.db.QueryRow(`
		SELECT content_hash, target_lang, summary, model, updated_at
		FROM article_summaries
		WHERE content_hash = :content_hash AND target_lang = :target_lang
	`, sql.Named("content_hash", contentHash), sql.Named("target_lang", targetLang)).Scan(
		&result.ContentHash,
		&result.TargetLang,
		&result.Summary,
		&result.Model,
		&result.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get article summary: %w", err)
	}
	return &result, nil
}

// SaveArticleSummary persists or replaces a cached article summary.
func (s *Store) SaveArticleSummary(params SaveArticleSummaryParams) error {
	_, err := s.db.Exec(`
		INSERT INTO article_summaries (
			content_hash,
			target_lang,
			source_title,
			summary,
			model
		)
		VALUES (
			:content_hash,
			:target_lang,
			:source_title,
			:summary,
			:model
		)
		ON CONFLICT (content_hash, target_lang) DO UPDATE SET
			source_title = excluded.source_title,
			summary = excluded.summary,
			model = excluded.model,
			updated_at = unixepoch()
	`,
		sql.Named("content_hash", params.ContentHash),
		sql.Named("target_lang", params.TargetLang),
		sql.Named("source_title", params.SourceTitle),
		sql.Named("summary", params.Summary),
		sql.Named("model", params.Model),
	)
	if err != nil {
		return fmt.Errorf("save article summary: %w", err)
	}
	return nil
}
