package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/0x2E/fusion/internal/model"
)

// TranslationConfig holds the raw server-side translation upstream settings.
type TranslationConfig struct {
	APIURL string
	APIKey string
	Model  string
}

func normalizeOptionalSettingString(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	trimmed := strings.TrimSpace(v.String)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func normalizeTrimmedParam(v *string) *string {
	if v == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*v)
	return &trimmed
}

func maskAPIKey(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) <= 8 {
		return "••••"
	}
	prefixLen := 3
	if len(runes) >= 12 {
		prefixLen = 4
	}
	return string(runes[:prefixLen]) + "••••" + string(runes[len(runes)-4:])
}

// GetSettings returns the singleton settings row. A missing row is not an
// error: it means no preference was ever saved, so all fields stay nil.
func (s *Store) GetSettings() (*model.Settings, error) {
	settings := &model.Settings{}
	var apiURL, apiKey, modelName sql.NullString
	err := s.db.QueryRow(`
		SELECT
			locale,
			article_page_size,
			theme,
			auto_mark_read,
			latin_font,
			cjk_font,
			mono_font,
			translation_api_url,
			translation_api_key,
			translation_model,
			translation_mode,
			list_translation_mode,
			updated_at
		FROM settings
		WHERE id = 1
	`).Scan(
		&settings.Locale,
		&settings.ArticlePageSize,
		&settings.Theme,
		&settings.AutoMarkRead,
		&settings.LatinFont,
		&settings.CJKFont,
		&settings.MonoFont,
		&apiURL,
		&apiKey,
		&modelName,
		&settings.TranslationMode,
		&settings.ListTranslationMode,
		&settings.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return settings, nil
		}
		return nil, fmt.Errorf("get settings: %w", err)
	}

	settings.TranslationAPIURL = normalizeOptionalSettingString(apiURL)
	settings.TranslationModel = normalizeOptionalSettingString(modelName)
	if rawKey := strings.TrimSpace(apiKey.String); apiKey.Valid && rawKey != "" {
		settings.TranslationAPIKeySet = true
		settings.TranslationAPIKeyMask = maskAPIKey(rawKey)
	}
	return settings, nil
}

// GetTranslationConfig returns the unmasked server-side translation endpoint config.
func (s *Store) GetTranslationConfig() (*TranslationConfig, error) {
	var apiURL, apiKey, modelName sql.NullString
	err := s.db.QueryRow(`
		SELECT translation_api_url, translation_api_key, translation_model
		FROM settings
		WHERE id = 1
	`).Scan(&apiURL, &apiKey, &modelName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &TranslationConfig{}, nil
		}
		return nil, fmt.Errorf("get translation config: %w", err)
	}
	return &TranslationConfig{
		APIURL: strings.TrimSpace(apiURL.String),
		APIKey: strings.TrimSpace(apiKey.String),
		Model:  strings.TrimSpace(modelName.String),
	}, nil
}

// UpdateSettingsParams supports partial updates. Only non-nil fields will be
// updated; nil means "leave unchanged".
type UpdateSettingsParams struct {
	Locale              *string
	ArticlePageSize     *int64
	Theme               *string
	AutoMarkRead        *string
	LatinFont           *string
	CJKFont             *string
	MonoFont            *string
	TranslationAPIURL   *string
	TranslationAPIKey   *string
	TranslationModel    *string
	TranslationMode     *string
	ListTranslationMode *string
}

// UpdateSettings upserts the singleton settings row. Fields set to nil keep
// their stored value via COALESCE (excluded.* is the proposed insert row, so a
// NULL there means "not provided in this request"). A nil pointer argument is
// passed through to SQL as NULL by database/sql.
func (s *Store) UpdateSettings(params UpdateSettingsParams) error {
	if params.Locale == nil && params.ArticlePageSize == nil && params.Theme == nil &&
		params.AutoMarkRead == nil && params.LatinFont == nil && params.CJKFont == nil &&
		params.MonoFont == nil && params.TranslationAPIURL == nil &&
		params.TranslationAPIKey == nil && params.TranslationModel == nil &&
		params.TranslationMode == nil && params.ListTranslationMode == nil {
		return nil
	}

	apiURL := normalizeTrimmedParam(params.TranslationAPIURL)
	apiKey := normalizeTrimmedParam(params.TranslationAPIKey)
	modelName := normalizeTrimmedParam(params.TranslationModel)

	_, err := s.db.Exec(`
		INSERT INTO settings (
			id,
			locale,
			article_page_size,
			theme,
			auto_mark_read,
			latin_font,
			cjk_font,
			mono_font,
			translation_api_url,
			translation_api_key,
			translation_model,
			translation_mode,
			list_translation_mode
		)
		VALUES (
			1,
			:locale,
			:article_page_size,
			:theme,
			:auto_mark_read,
			:latin_font,
			:cjk_font,
			:mono_font,
			:translation_api_url,
			:translation_api_key,
			:translation_model,
			:translation_mode,
			:list_translation_mode
		)
		ON CONFLICT (id) DO UPDATE SET
			locale = COALESCE(excluded.locale, settings.locale),
			article_page_size = COALESCE(excluded.article_page_size, settings.article_page_size),
			theme = COALESCE(excluded.theme, settings.theme),
			auto_mark_read = COALESCE(excluded.auto_mark_read, settings.auto_mark_read),
			latin_font = COALESCE(excluded.latin_font, settings.latin_font),
			cjk_font = COALESCE(excluded.cjk_font, settings.cjk_font),
			mono_font = COALESCE(excluded.mono_font, settings.mono_font),
			translation_api_url = COALESCE(excluded.translation_api_url, settings.translation_api_url),
			translation_api_key = COALESCE(excluded.translation_api_key, settings.translation_api_key),
			translation_model = COALESCE(excluded.translation_model, settings.translation_model),
			translation_mode = COALESCE(excluded.translation_mode, settings.translation_mode),
			list_translation_mode = COALESCE(excluded.list_translation_mode, settings.list_translation_mode),
			updated_at = unixepoch()
	`,
		sql.Named("locale", params.Locale),
		sql.Named("article_page_size", params.ArticlePageSize),
		sql.Named("theme", params.Theme),
		sql.Named("auto_mark_read", params.AutoMarkRead),
		sql.Named("latin_font", params.LatinFont),
		sql.Named("cjk_font", params.CJKFont),
		sql.Named("mono_font", params.MonoFont),
		sql.Named("translation_api_url", apiURL),
		sql.Named("translation_api_key", apiKey),
		sql.Named("translation_model", modelName),
		sql.Named("translation_mode", params.TranslationMode),
		sql.Named("list_translation_mode", params.ListTranslationMode),
	)
	if err != nil {
		return fmt.Errorf("update settings: %w", err)
	}
	return nil
}
