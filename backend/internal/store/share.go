package store

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/0x2E/fusion/internal/model"
)

type CreateArticleShareParams struct {
	Token        string
	ItemID       *int64
	Title        string
	Content      string
	Link         string
	FeedName     string
	PubDate      int64
	PasscodeHash string
	ExpiresAt    int64
}

func (s *Store) CreateArticleShare(params CreateArticleShareParams) (*model.ArticleShare, error) {
	_, err := s.db.Exec(`
		INSERT INTO article_shares (
			token,
			item_id,
			title,
			content,
			link,
			feed_name,
			pub_date,
			passcode_hash,
			expires_at
		)
		VALUES (
			:token,
			:item_id,
			:title,
			:content,
			:link,
			:feed_name,
			:pub_date,
			:passcode_hash,
			:expires_at
		)
	`, sql.Named("token", params.Token),
		sql.Named("item_id", params.ItemID),
		sql.Named("title", params.Title),
		sql.Named("content", params.Content),
		sql.Named("link", params.Link),
		sql.Named("feed_name", params.FeedName),
		sql.Named("pub_date", params.PubDate),
		sql.Named("passcode_hash", nullIfEmpty(params.PasscodeHash)),
		sql.Named("expires_at", params.ExpiresAt),
	)
	if err != nil {
		return nil, fmt.Errorf("create article share: %w", err)
	}
	return s.GetArticleShareByToken(params.Token)
}

func (s *Store) GetArticleShareByToken(token string) (*model.ArticleShare, error) {
	share := &model.ArticleShare{}
	var (
		itemID   sql.NullInt64
		passcode sql.NullString
	)
	err := s.db.QueryRow(`
		SELECT
			id,
			token,
			item_id,
			title,
			content,
			link,
			feed_name,
			pub_date,
			passcode_hash,
			expires_at,
			created_at
		FROM article_shares
		WHERE token = :token
	`, sql.Named("token", token)).Scan(
		&share.ID,
		&share.Token,
		&itemID,
		&share.Title,
		&share.Content,
		&share.Link,
		&share.FeedName,
		&share.PubDate,
		&passcode,
		&share.ExpiresAt,
		&share.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: share", ErrNotFound)
		}
		return nil, fmt.Errorf("get article share: %w", err)
	}
	if itemID.Valid {
		id := itemID.Int64
		share.ItemID = &id
	}
	if passcode.Valid {
		share.PasscodeHash = passcode.String
	}
	return share, nil
}

// ScrubExpiredArticleShares removes article content and passcode hashes from
// expired shares while retaining the token/expiry tombstone for a precise 410.
func (s *Store) ScrubExpiredArticleShares(now int64) error {
	_, err := s.db.Exec(`
		UPDATE article_shares
		SET title = '',
			content = '',
			link = '',
			feed_name = '',
			passcode_hash = NULL
		WHERE expires_at <= :now
		  AND (title <> '' OR content <> '' OR link <> '' OR feed_name <> '' OR passcode_hash IS NOT NULL)
	`, sql.Named("now", now))
	if err != nil {
		return fmt.Errorf("scrub expired article shares: %w", err)
	}
	return nil
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}
