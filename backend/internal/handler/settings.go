package handler

import (
	"fmt"
	"slices"

	"github.com/0x2E/fusion/internal/store"
	"github.com/0x2E/fusion/internal/translator"
	"github.com/gin-gonic/gin"
)

// Locale and theme are closed sets by contract (shipped catalogs).
// The page size is a free value within the
// range the items API can serve (maxListLimit); the five-option list is only
// a frontend UI choice, so a future custom-value picker needs no backend
// change. These allow-lists mirror the frontend preference and theme catalogs.
var (
	validLocales              = []string{"en", "zh", "de", "fr", "es", "ru", "pt", "sv"}
	validThemes               = []string{"light", "dark", "system", "sage", "parchment", "stone", "night"}
	validAutoMarkRead         = []string{"off", "open", "5", "10", "30"}
	validTranslationModes     = []string{"zh", "bilingual"}
	validListTranslationModes = []string{"original", "zh", "bilingual"}
	validLatinFonts           = []string{
		"geist",
		"inter",
		"source-sans-3",
		"oppo-sans",
		"system-sans",
		"newsreader",
		"source-serif-4",
		"lora",
		"system-serif",
	}
	validCJKFonts = []string{
		"system",
		"noto-sans-sc",
		"oppo-sans",
		"system-sans",
		"noto-serif-sc",
		"lxgw-wenkai",
		"system-serif",
		"system-kai",
	}
	validMonoFonts = []string{
		"geist-mono",
		"jetbrains-mono",
		"fira-code",
		"system-mono",
	}
)

// A missing field and an explicit null are indistinguishable after JSON
// decoding (both leave the pointer nil) and both mean "leave unchanged".
// Translation credential strings treat an empty string "" as an explicit clear.
type updateSettingsRequest struct {
	Locale              *string `json:"locale"`
	ArticlePageSize     *int64  `json:"article_page_size"`
	Theme               *string `json:"theme"`
	AutoMarkRead        *string `json:"auto_mark_read"`
	LatinFont           *string `json:"latin_font"`
	CJKFont             *string `json:"cjk_font"`
	MonoFont            *string `json:"mono_font"`
	TranslationAPIURL   *string `json:"translation_api_url"`
	TranslationAPIKey   *string `json:"translation_api_key"`
	TranslationModel    *string `json:"translation_model"`
	TranslationMode     *string `json:"translation_mode"`
	ListTranslationMode *string `json:"list_translation_mode"`
}

func (h *Handler) getSettings(c *gin.Context) {
	settings, err := h.store.GetSettings()
	if err != nil {
		internalError(c, err, "get settings")
		return
	}

	dataResponse(c, settings)
}

func (h *Handler) updateSettings(c *gin.Context) {
	var req updateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequestError(c, "invalid request")
		return
	}

	if err := validateSettingsUpdate(&req); err != nil {
		badRequestError(c, err.Error())
		return
	}

	if err := h.store.UpdateSettings(store.UpdateSettingsParams{
		Locale:              req.Locale,
		ArticlePageSize:     req.ArticlePageSize,
		Theme:               req.Theme,
		AutoMarkRead:        req.AutoMarkRead,
		LatinFont:           req.LatinFont,
		CJKFont:             req.CJKFont,
		MonoFont:            req.MonoFont,
		TranslationAPIURL:   req.TranslationAPIURL,
		TranslationAPIKey:   req.TranslationAPIKey,
		TranslationModel:    req.TranslationModel,
		TranslationMode:     req.TranslationMode,
		ListTranslationMode: req.ListTranslationMode,
	}); err != nil {
		internalError(c, err, "update settings")
		return
	}

	settings, err := h.store.GetSettings()
	if err != nil {
		internalError(c, err, "get settings after update")
		return
	}

	dataResponse(c, settings)
}

func validateSettingsUpdate(req *updateSettingsRequest) error {
	if req.Locale != nil && !slices.Contains(validLocales, *req.Locale) {
		return fmt.Errorf("invalid locale")
	}
	if req.ArticlePageSize != nil &&
		(*req.ArticlePageSize < 1 || *req.ArticlePageSize > maxListLimit) {
		return fmt.Errorf("invalid article_page_size")
	}
	if req.Theme != nil && !slices.Contains(validThemes, *req.Theme) {
		return fmt.Errorf("invalid theme")
	}
	if req.AutoMarkRead != nil && !slices.Contains(validAutoMarkRead, *req.AutoMarkRead) {
		return fmt.Errorf("invalid auto_mark_read")
	}
	if req.LatinFont != nil && !slices.Contains(validLatinFonts, *req.LatinFont) {
		return fmt.Errorf("invalid latin_font")
	}
	if req.CJKFont != nil && !slices.Contains(validCJKFonts, *req.CJKFont) {
		return fmt.Errorf("invalid cjk_font")
	}
	if req.MonoFont != nil && !slices.Contains(validMonoFonts, *req.MonoFont) {
		return fmt.Errorf("invalid mono_font")
	}
	if req.TranslationAPIURL != nil {
		if err := translator.ValidateAPIURL(*req.TranslationAPIURL); err != nil {
			return fmt.Errorf("invalid translation_api_url")
		}
	}
	if req.TranslationMode != nil && !slices.Contains(validTranslationModes, *req.TranslationMode) {
		return fmt.Errorf("invalid translation_mode")
	}
	if req.ListTranslationMode != nil && !slices.Contains(validListTranslationModes, *req.ListTranslationMode) {
		return fmt.Errorf("invalid list_translation_mode")
	}
	return nil
}
