package model

// Group represents a feed group.
type Group struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	SortOrder int64  `json:"sort_order"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// Feed represents an RSS/Atom feed.
type Feed struct {
	ID                     int64  `json:"id"`
	GroupID                int64  `json:"group_id"`
	Name                   string `json:"name"`
	Link                   string `json:"link"`
	SiteURL                string `json:"site_url,omitempty"`
	Suspended              bool   `json:"suspended"`
	Proxy                  string `json:"proxy,omitempty"`
	RefreshIntervalSeconds *int64 `json:"refresh_interval_seconds,omitempty"`
	CreatedAt              int64  `json:"created_at"`
	UpdatedAt              int64  `json:"updated_at"`

	FetchState FeedFetchState `json:"fetch_state"`

	UnreadCount int64 `json:"unread_count"`
	ItemCount   int64 `json:"item_count"`
}

// FeedFetchState stores runtime pull metadata for a feed.
// Time fields are Unix seconds; 0 means unknown/unset.
type FeedFetchState struct {
	// ETag is used for If-None-Match conditional requests.
	ETag string `json:"etag,omitempty"`
	// LastModified stores raw HTTP Last-Modified for If-Modified-Since.
	LastModified string `json:"last_modified,omitempty"`
	// CacheControl stores raw HTTP Cache-Control value.
	CacheControl string `json:"cache_control,omitempty"`
	// ExpiresAt is parsed from the HTTP Expires header.
	ExpiresAt int64 `json:"expires_at"`
	// LastCheckedAt is the last fetch attempt time (success or failure).
	LastCheckedAt int64 `json:"last_checked_at"`
	// NextCheckAt is the earliest next fetch time decided by pull policy.
	NextCheckAt int64 `json:"next_check_at"`
	// LastHTTPStatus is the last HTTP status code observed during fetch.
	LastHTTPStatus int `json:"last_http_status"`
	// RetryAfterUntil is derived from Retry-After and blocks fetch before this time.
	RetryAfterUntil int64 `json:"retry_after_until"`
	// LastSuccessAt is the last successful check time (includes 200 and 304).
	LastSuccessAt int64 `json:"last_success_at"`
	// LastErrorAt is the most recent fetch failure time.
	LastErrorAt int64 `json:"last_error_at"`
	// LastError keeps the latest fetch error message.
	LastError string `json:"last_error,omitempty"`
	// ConsecutiveFailures is reset on success and incremented on each failure.
	ConsecutiveFailures int64 `json:"consecutive_failures"`
}

// Item represents a feed item.
type Item struct {
	ID         int64  `json:"id"`
	FeedID     int64  `json:"feed_id"`
	GUID       string `json:"guid"`
	Title      string `json:"title"`
	Link       string `json:"link"`
	Content    string `json:"content"`
	PubDate    int64  `json:"pub_date"`
	Unread     bool   `json:"unread"`
	CreatedAt  int64  `json:"created_at"`
	BookmarkID *int64 `json:"bookmark_id"`
}

// Settings holds the user's synced preferences. A nil field means the user
// has never explicitly chosen a value (clients fall back to their own default).
type Settings struct {
	Locale                *string `json:"locale"`
	ArticlePageSize       *int64  `json:"article_page_size"`
	Theme                 *string `json:"theme"`
	AutoMarkRead          *string `json:"auto_mark_read"`
	LatinFont             *string `json:"latin_font"`
	CJKFont               *string `json:"cjk_font"`
	MonoFont              *string `json:"mono_font"`
	TranslationAPIURL     *string `json:"translation_api_url"`
	TranslationAPIKeySet  bool    `json:"translation_api_key_set"`
	TranslationAPIKeyMask string  `json:"translation_api_key_mask"`
	TranslationModel      *string `json:"translation_model"`
	TranslationMode       *string `json:"translation_mode"`
	ListTranslationMode   *string `json:"list_translation_mode"`
	UpdatedAt             int64   `json:"updated_at"`
}

// TranslatedSegment holds one translated text segment identified by block key.
type TranslatedSegment struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
}

// ArticleTranslation stores a cached translation of an article snapshot.
type ArticleTranslation struct {
	ContentHash     string              `json:"content_hash"`
	TargetLang      string              `json:"target_lang"`
	TranslatedTitle string              `json:"translated_title"`
	Blocks          []TranslatedSegment `json:"blocks"`
	Model           string              `json:"model"`
	UpdatedAt       int64               `json:"updated_at"`
}

// ArticleBlockTranslation caches one independently translated article block
// (or title) so lazy viewport translation can persist partial progress.
type ArticleBlockTranslation struct {
	ContentHash    string `json:"content_hash"`
	TargetLang     string `json:"target_lang"`
	SourceText     string `json:"source_text"`
	TranslatedText string `json:"translated_text"`
	Model          string `json:"model"`
	UpdatedAt      int64  `json:"updated_at"`
}

// ArticleSummary stores a cached AI summary of an article snapshot.
type ArticleSummary struct {
	ContentHash string `json:"content_hash"`
	TargetLang  string `json:"target_lang"`
	Summary     string `json:"summary"`
	Model       string `json:"model"`
	UpdatedAt   int64  `json:"updated_at"`
}

// Bookmark represents a saved item snapshot.
type Bookmark struct {
	ID       int64  `json:"id"`
	ItemID   *int64 `json:"item_id"` // nullable
	Link     string `json:"link"`
	Title    string `json:"title"`
	Content  string `json:"content"`
	PubDate  int64  `json:"pub_date"`
	FeedName string `json:"feed_name"`
	// FeedID is a soft association to the source feed used for filtering. It is
	// nil for orphaned bookmarks whose feed/item no longer exists; the bookmark
	// itself always survives such deletions (it is a content snapshot).
	FeedID *int64 `json:"feed_id"`
	// Unread mirrors the linked item's unread state (false for orphans).
	Unread    bool  `json:"unread"`
	CreatedAt int64 `json:"created_at"`
}
