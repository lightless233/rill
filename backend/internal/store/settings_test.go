package store

import (
	"testing"

	"github.com/0x2E/fusion/internal/model"
)

func strPtr(s string) *string { return &s }

func int64Ptr(i int64) *int64 { return &i }

func TestGetSettingsEmpty(t *testing.T) {
	store, _ := setupTestDB(t)
	defer closeStore(t, store)

	settings, err := store.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings() failed: %v", err)
	}
	if settings.Locale != nil || settings.ArticlePageSize != nil || settings.Theme != nil ||
		settings.AutoMarkRead != nil || settings.LatinFont != nil || settings.CJKFont != nil ||
		settings.MonoFont != nil {
		t.Errorf("expected all-nil settings on empty database, got %+v", settings)
	}
}

func TestUpdateSettingsPartial(t *testing.T) {
	store, _ := setupTestDB(t)
	defer closeStore(t, store)

	if err := store.UpdateSettings(UpdateSettingsParams{Locale: strPtr("de")}); err != nil {
		t.Fatalf("UpdateSettings(locale) failed: %v", err)
	}

	// Push updated_at back so the second update's unixepoch() write is
	// observable even within the same wall-clock second.
	if _, err := store.db.Exec(`UPDATE settings SET updated_at = 1000`); err != nil {
		t.Fatalf("rewind updated_at failed: %v", err)
	}
	first, err := store.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings() after rewind failed: %v", err)
	}
	if first.UpdatedAt != 1000 {
		t.Fatalf("expected rewound updated_at=1000, got %d", first.UpdatedAt)
	}

	if err := store.UpdateSettings(UpdateSettingsParams{Theme: strPtr("dark")}); err != nil {
		t.Fatalf("UpdateSettings(theme) failed: %v", err)
	}

	second, err := store.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings() after theme update failed: %v", err)
	}
	if second.Locale == nil || *second.Locale != "de" {
		t.Errorf("expected locale to survive an unrelated update, got %v", second.Locale)
	}
	if second.ArticlePageSize != nil {
		t.Errorf("expected article_page_size to stay unset, got %v", *second.ArticlePageSize)
	}
	if second.Theme == nil || *second.Theme != "dark" {
		t.Errorf("expected theme=dark, got %v", second.Theme)
	}
	if second.UpdatedAt <= first.UpdatedAt {
		t.Errorf("expected updated_at to advance: first=%d second=%d", first.UpdatedAt, second.UpdatedAt)
	}
}

func TestUpdateSettingsAllFields(t *testing.T) {
	store, _ := setupTestDB(t)
	defer closeStore(t, store)

	err := store.UpdateSettings(UpdateSettingsParams{
		Locale:          strPtr("zh"),
		ArticlePageSize: int64Ptr(50),
		Theme:           strPtr("system"),
		AutoMarkRead:    strPtr("open"),
		LatinFont:       strPtr("newsreader"),
		CJKFont:         strPtr("noto-serif-sc"),
		MonoFont:        strPtr("jetbrains-mono"),
	})
	if err != nil {
		t.Fatalf("UpdateSettings(all fields) failed: %v", err)
	}

	settings, err := store.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings() failed: %v", err)
	}
	want := &model.Settings{
		Locale:          strPtr("zh"),
		ArticlePageSize: int64Ptr(50),
		Theme:           strPtr("system"),
		AutoMarkRead:    strPtr("open"),
		LatinFont:       strPtr("newsreader"),
		CJKFont:         strPtr("noto-serif-sc"),
		MonoFont:        strPtr("jetbrains-mono"),
		UpdatedAt:       settings.UpdatedAt,
	}
	if *settings.Locale != *want.Locale || *settings.ArticlePageSize != *want.ArticlePageSize ||
		*settings.Theme != *want.Theme || *settings.AutoMarkRead != *want.AutoMarkRead ||
		*settings.LatinFont != *want.LatinFont || *settings.CJKFont != *want.CJKFont ||
		*settings.MonoFont != *want.MonoFont {
		t.Errorf("got %+v, want %+v", settings, want)
	}
}

// Each font column must bypass the all-nil short circuit individually.
func TestUpdateSettingsFontFieldsOnly(t *testing.T) {
	store, _ := setupTestDB(t)
	defer closeStore(t, store)

	if err := store.UpdateSettings(UpdateSettingsParams{LatinFont: strPtr("lora")}); err != nil {
		t.Fatalf("UpdateSettings(latin_font) failed: %v", err)
	}
	if err := store.UpdateSettings(UpdateSettingsParams{CJKFont: strPtr("lxgw-wenkai")}); err != nil {
		t.Fatalf("UpdateSettings(cjk_font) failed: %v", err)
	}
	if err := store.UpdateSettings(UpdateSettingsParams{MonoFont: strPtr("fira-code")}); err != nil {
		t.Fatalf("UpdateSettings(mono_font) failed: %v", err)
	}

	settings, err := store.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings() failed: %v", err)
	}
	if settings.LatinFont == nil || *settings.LatinFont != "lora" ||
		settings.CJKFont == nil || *settings.CJKFont != "lxgw-wenkai" ||
		settings.MonoFont == nil || *settings.MonoFont != "fira-code" {
		t.Fatalf("expected all three font fields saved, got %+v", settings)
	}
	if settings.Locale != nil {
		t.Errorf("expected locale to stay unset, got %v", *settings.Locale)
	}
}

// The all-nil short circuit in UpdateSettings must know every column, or a
// PATCH carrying only this field is silently dropped.
func TestUpdateSettingsAutoMarkReadOnly(t *testing.T) {
	store, _ := setupTestDB(t)
	defer closeStore(t, store)

	if err := store.UpdateSettings(UpdateSettingsParams{AutoMarkRead: strPtr("5")}); err != nil {
		t.Fatalf("UpdateSettings(auto_mark_read) failed: %v", err)
	}

	settings, err := store.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings() failed: %v", err)
	}
	if settings.AutoMarkRead == nil || *settings.AutoMarkRead != "5" {
		t.Fatalf("expected auto_mark_read=\"5\", got %+v", settings)
	}
	if settings.Locale != nil {
		t.Errorf("expected locale to stay unset, got %v", *settings.Locale)
	}
}

func TestUpdateSettingsNoFields(t *testing.T) {
	store, _ := setupTestDB(t)
	defer closeStore(t, store)

	if err := store.UpdateSettings(UpdateSettingsParams{}); err != nil {
		t.Fatalf("UpdateSettings(no fields) failed: %v", err)
	}

	settings, err := store.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings() failed: %v", err)
	}
	if settings.UpdatedAt != 0 {
		t.Errorf("expected a no-op update not to create the row, got updated_at=%d", settings.UpdatedAt)
	}
}
