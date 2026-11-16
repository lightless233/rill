package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type settingsEnvelope struct {
	Data struct {
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
	} `json:"data"`
}

func decodeSettings(t *testing.T, w *httptest.ResponseRecorder) settingsEnvelope {
	t.Helper()

	var envelope settingsEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode settings response failed: %v (body: %s)", err, w.Body.String())
	}
	return envelope
}

// newSettingsTestRouter returns the full router plus a valid session cookie.
func newSettingsTestRouter(t *testing.T) (*gin.Engine, *http.Cookie) {
	t.Helper()

	h, _ := newFeverTestHandler(t) // password: "secret"
	r := h.SetupRouter()

	w := performRequest(r, http.MethodPost, "/api/sessions", mustJSONBody(t, map[string]string{"password": "secret"}), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", w.Code, w.Body.String())
	}

	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected a session cookie after login")
	}
	return r, cookies[0]
}

func TestGetSettingsEmpty(t *testing.T) {
	r, cookie := newSettingsTestRouter(t)

	w := performRequest(r, http.MethodGet, "/api/settings", nil, nil, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	settings := decodeSettings(t, w).Data
	if settings.Locale != nil || settings.ArticlePageSize != nil || settings.Theme != nil ||
		settings.AutoMarkRead != nil || settings.LatinFont != nil || settings.CJKFont != nil ||
		settings.MonoFont != nil || settings.ListTranslationMode != nil {
		t.Errorf("expected all-null settings, got %+v", settings)
	}

	// Pointer decoding cannot distinguish a missing key from a JSON null, so
	// assert on the raw body that all seven keys are explicitly present.
	for _, key := range []string{
		`"locale":null`, `"article_page_size":null`, `"theme":null`, `"auto_mark_read":null`,
		`"latin_font":null`, `"cjk_font":null`, `"mono_font":null`,
		`"list_translation_mode":null`,
	} {
		if !strings.Contains(w.Body.String(), key) {
			t.Errorf("expected body to contain %s, got %s", key, w.Body.String())
		}
	}
}

func TestUpdateSettingsExplicitNullLeavesValueUnchanged(t *testing.T) {
	r, cookie := newSettingsTestRouter(t)

	w := performRequest(r, http.MethodPatch, "/api/settings", strings.NewReader(`{"locale":"de"}`), nil, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH locale: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = performRequest(r, http.MethodPatch, "/api/settings", strings.NewReader(`{"locale":null,"theme":"dark","auto_mark_read":null}`), nil, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH null locale: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	settings := decodeSettings(t, w).Data
	if settings.Locale == nil || *settings.Locale != "de" {
		t.Errorf("expected explicit null to leave locale unchanged, got %+v", settings)
	}
	if settings.Theme == nil || *settings.Theme != "dark" {
		t.Errorf("expected theme=dark, got %+v", settings)
	}
	if settings.AutoMarkRead != nil {
		t.Errorf("expected explicit null to leave auto_mark_read unchanged, got %+v", settings)
	}
}

func TestSettingsRequireAuth(t *testing.T) {
	r, _ := newSettingsTestRouter(t)

	for _, method := range []string{http.MethodGet, http.MethodPatch} {
		w := performRequest(r, method, "/api/settings", nil, nil)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s without cookie: expected 401, got %d", method, w.Code)
		}
	}
}

func TestUpdateSettingsSingleField(t *testing.T) {
	r, cookie := newSettingsTestRouter(t)

	w := performRequest(r, http.MethodPatch, "/api/settings", strings.NewReader(`{"locale":"de"}`), nil, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH locale: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	settings := decodeSettings(t, w).Data
	if settings.Locale == nil || *settings.Locale != "de" {
		t.Errorf("PATCH locale: expected locale=de in response, got %+v", settings)
	}

	w = performRequest(r, http.MethodPatch, "/api/settings", strings.NewReader(`{"theme":"dark"}`), nil, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH theme: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	settings = decodeSettings(t, w).Data
	if settings.Locale == nil || *settings.Locale != "de" {
		t.Errorf("PATCH theme: expected locale to survive, got %+v", settings)
	}
	if settings.Theme == nil || *settings.Theme != "dark" {
		t.Errorf("PATCH theme: expected theme=dark, got %+v", settings)
	}

	w = performRequest(r, http.MethodGet, "/api/settings", nil, nil, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("GET after updates: expected 200, got %d", w.Code)
	}
	settings = decodeSettings(t, w).Data
	if settings.UpdatedAt == 0 {
		t.Error("expected updated_at to be set once a preference is stored")
	}
}

func TestUpdateSettingsValidation(t *testing.T) {
	r, cookie := newSettingsTestRouter(t)

	tests := []struct {
		name string
		body string
		want string
	}{
		{"invalid locale", `{"locale":"xx"}`, "invalid locale"},
		{"empty locale", `{"locale":""}`, "invalid locale"},
		{"locale wrong type", `{"locale":5}`, "invalid request"},
		{"page size below range", `{"article_page_size":0}`, "invalid article_page_size"},
		{"page size above range", `{"article_page_size":101}`, "invalid article_page_size"},
		{"negative page size", `{"article_page_size":-10}`, "invalid article_page_size"},
		{"page size as string", `{"article_page_size":"20"}`, "invalid request"},
		{"invalid theme", `{"theme":"blue"}`, "invalid theme"},
		{"empty theme", `{"theme":""}`, "invalid theme"},
		{"invalid auto mark read", `{"auto_mark_read":"sometimes"}`, "invalid auto_mark_read"},
		{"empty auto mark read", `{"auto_mark_read":""}`, "invalid auto_mark_read"},
		{"auto mark read wrong type", `{"auto_mark_read":5}`, "invalid request"},
		{"auto mark read numeric delay not in set", `{"auto_mark_read":"15"}`, "invalid auto_mark_read"},
		{"invalid latin font", `{"latin_font":"comic-sans"}`, "invalid latin_font"},
		{"empty latin font", `{"latin_font":""}`, "invalid latin_font"},
		{"invalid cjk font", `{"cjk_font":"unknown"}`, "invalid cjk_font"},
		{"empty cjk font", `{"cjk_font":""}`, "invalid cjk_font"},
		{"invalid mono font", `{"mono_font":"courier"}`, "invalid mono_font"},
		{"empty mono font", `{"mono_font":""}`, "invalid mono_font"},
		{"invalid translation url", `{"translation_api_url":"ftp://example.com"}`, "invalid translation_api_url"},
		{"malformed translation url", `{"translation_api_url":"not-a-url"}`, "invalid translation_api_url"},
		{"invalid translation mode", `{"translation_mode":"en"}`, "invalid translation_mode"},
		{"empty translation mode", `{"translation_mode":""}`, "invalid translation_mode"},
		{"invalid list translation mode", `{"list_translation_mode":"en"}`, "invalid list_translation_mode"},
		{"empty list translation mode", `{"list_translation_mode":""}`, "invalid list_translation_mode"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := performRequest(r, http.MethodPatch, "/api/settings", strings.NewReader(tt.body), nil, cookie)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
			var errBody struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &errBody); err != nil {
				t.Fatalf("decode error body failed: %v", err)
			}
			if errBody.Error != tt.want {
				t.Errorf("expected error %q, got %q", tt.want, errBody.Error)
			}
		})
	}
}

func TestUpdateSettingsCustomPageSize(t *testing.T) {
	r, cookie := newSettingsTestRouter(t)

	// The five-option list is a frontend UI choice; any value the items API
	// can serve (1..maxListLimit) must roundtrip.
	w := performRequest(r, http.MethodPatch, "/api/settings", strings.NewReader(`{"article_page_size":25}`), nil, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH custom page size: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	settings := decodeSettings(t, w).Data
	if settings.ArticlePageSize == nil || *settings.ArticlePageSize != 25 {
		t.Errorf("expected article_page_size=25, got %+v", settings)
	}

	w = performRequest(r, http.MethodGet, "/api/settings", nil, nil, cookie)
	settings = decodeSettings(t, w).Data
	if settings.ArticlePageSize == nil || *settings.ArticlePageSize != 25 {
		t.Errorf("expected article_page_size=25 after GET, got %+v", settings)
	}
}

func TestUpdateSettingsOPPOSansRoundTrip(t *testing.T) {
	r, cookie := newSettingsTestRouter(t)

	w := performRequest(r, http.MethodPatch, "/api/settings", strings.NewReader(`{"cjk_font":"oppo-sans"}`), nil, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH OPPO Sans: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = performRequest(r, http.MethodGet, "/api/settings", nil, nil, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("GET settings: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	settings := decodeSettings(t, w).Data
	if settings.CJKFont == nil || *settings.CJKFont != "oppo-sans" {
		t.Fatalf("expected persisted OPPO Sans selection, got %+v", settings.CJKFont)
	}
}

func TestUpdateSettingsEmptyBody(t *testing.T) {
	r, cookie := newSettingsTestRouter(t)

	w := performRequest(r, http.MethodPatch, "/api/settings", strings.NewReader(`{}`), nil, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	settings := decodeSettings(t, w).Data
	if settings.Locale != nil || settings.ArticlePageSize != nil || settings.Theme != nil ||
		settings.AutoMarkRead != nil || settings.LatinFont != nil || settings.CJKFont != nil ||
		settings.MonoFont != nil || settings.ListTranslationMode != nil {
		t.Errorf("expected empty PATCH to be a no-op, got %+v", settings)
	}
	if settings.UpdatedAt != 0 {
		t.Errorf("expected empty PATCH not to create the row, got updated_at=%d", settings.UpdatedAt)
	}
}

// A PATCH that only sets auto_mark_read used to be swallowed by the store's
// all-nil short circuit (which only knew the original three fields).
func TestUpdateSettingsAutoMarkReadSingleField(t *testing.T) {
	r, cookie := newSettingsTestRouter(t)

	w := performRequest(r, http.MethodPatch, "/api/settings", strings.NewReader(`{"auto_mark_read":"10"}`), nil, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH auto_mark_read: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	settings := decodeSettings(t, w).Data
	if settings.AutoMarkRead == nil || *settings.AutoMarkRead != "10" {
		t.Fatalf("expected auto_mark_read=\"10\" in response, got %+v", settings)
	}
	if settings.UpdatedAt == 0 {
		t.Error("expected the row to be created by a new-field-only PATCH")
	}

	// Round-trip through GET, and confirm the value survives an unrelated PATCH.
	w = performRequest(r, http.MethodGet, "/api/settings", nil, nil, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("GET: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	settings = decodeSettings(t, w).Data
	if settings.AutoMarkRead == nil || *settings.AutoMarkRead != "10" {
		t.Errorf("expected auto_mark_read=\"10\" after GET, got %+v", settings)
	}

	w = performRequest(r, http.MethodPatch, "/api/settings", strings.NewReader(`{"locale":"de"}`), nil, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH locale: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	settings = decodeSettings(t, w).Data
	if settings.AutoMarkRead == nil || *settings.AutoMarkRead != "10" {
		t.Errorf("expected auto_mark_read to survive an unrelated PATCH, got %+v", settings)
	}
}

func TestUpdateSettingsFontFieldsSingleAndRoundTrip(t *testing.T) {
	r, cookie := newSettingsTestRouter(t)

	w := performRequest(
		r,
		http.MethodPatch,
		"/api/settings",
		strings.NewReader(`{"latin_font":"newsreader","cjk_font":"lxgw-wenkai","mono_font":"jetbrains-mono"}`),
		nil,
		cookie,
	)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH font fields: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	settings := decodeSettings(t, w).Data
	if settings.LatinFont == nil || *settings.LatinFont != "newsreader" ||
		settings.CJKFont == nil || *settings.CJKFont != "lxgw-wenkai" ||
		settings.MonoFont == nil || *settings.MonoFont != "jetbrains-mono" {
		t.Fatalf("expected font fields updated, got %+v", settings)
	}

	w = performRequest(r, http.MethodPatch, "/api/settings", strings.NewReader(`{"theme":"dark"}`), nil, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH theme: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	settings = decodeSettings(t, w).Data
	if settings.LatinFont == nil || *settings.LatinFont != "newsreader" ||
		settings.CJKFont == nil || *settings.CJKFont != "lxgw-wenkai" ||
		settings.MonoFont == nil || *settings.MonoFont != "jetbrains-mono" {
		t.Errorf("expected font fields to survive an unrelated PATCH, got %+v", settings)
	}
}

func TestUpdateSettingsTranslationDoesNotExposeRawKey(t *testing.T) {
	r, cookie := newSettingsTestRouter(t)

	const secret = "sk-deepseek-secret-9876"
	w := performRequest(
		r,
		http.MethodPatch,
		"/api/settings",
		strings.NewReader(`{"translation_api_url":"https://api.deepseek.com","translation_api_key":"`+secret+`","translation_model":"deepseek-chat","translation_mode":"zh","list_translation_mode":"bilingual"}`),
		nil,
		cookie,
	)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH translation fields: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), secret) {
		t.Fatalf("settings response leaked raw API key: %s", w.Body.String())
	}

	settings := decodeSettings(t, w).Data
	if !settings.TranslationAPIKeySet || settings.TranslationAPIKeyMask == "" {
		t.Fatalf("expected masked key info, got %+v", settings)
	}
	if settings.TranslationMode == nil || *settings.TranslationMode != "zh" {
		t.Fatalf("expected translation_mode=zh, got %+v", settings)
	}
	if settings.ListTranslationMode == nil || *settings.ListTranslationMode != "bilingual" {
		t.Fatalf("expected list_translation_mode=bilingual, got %+v", settings)
	}
}
