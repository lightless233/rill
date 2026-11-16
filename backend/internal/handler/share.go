package handler

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/0x2E/fusion/internal/auth"
	"github.com/0x2E/fusion/internal/model"
	"github.com/0x2E/fusion/internal/store"
	"github.com/gin-gonic/gin"
)

const (
	defaultShareExpirySeconds = 24 * 60 * 60
	minShareExpirySeconds     = 60 * 60
	maxShareExpirySeconds     = 365 * 24 * 60 * 60
	sharePasscodeAlphabet     = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
)

type createShareRequest struct {
	ItemID           int64  `json:"item_id"`
	ExpiresInSeconds int64  `json:"expires_in_seconds"`
	PasscodeEnabled  bool   `json:"passcode_enabled"`
	Passcode         string `json:"passcode"`
}

type accessShareRequest struct {
	Passcode string `json:"passcode"`
}

type shareContentResponse struct {
	Title     string `json:"title"`
	Content   string `json:"content"`
	Link      string `json:"link"`
	FeedName  string `json:"feed_name"`
	PubDate   int64  `json:"pub_date"`
	ExpiresAt int64  `json:"expires_at"`
}

func generateShareToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate share token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func generateSharePasscode() (string, error) {
	chars := make([]byte, 4)
	max := big.NewInt(int64(len(sharePasscodeAlphabet)))
	for i := range chars {
		index, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("generate share passcode: %w", err)
		}
		chars[i] = sharePasscodeAlphabet[index.Int64()]
	}
	return string(chars), nil
}

func normalizeSharePasscode(raw string) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(raw))
	if len([]rune(code)) != 4 {
		return "", fmt.Errorf("passcode must be exactly 4 letters or digits")
	}
	for _, char := range code {
		if (char < 'A' || char > 'Z') && (char < '0' || char > '9') {
			return "", fmt.Errorf("passcode must contain only letters or digits")
		}
	}
	return code, nil
}

func (h *Handler) scrubExpiredShares() {
	if err := h.store.ScrubExpiredArticleShares(time.Now().Unix()); err != nil {
		slog.Warn("failed to scrub expired article shares", "error", err)
	}
}

func shareExpiredResponse(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{"error": "share expired"})
}

func setShareNoStore(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
}

func shareTooManyAttemptsError(c *gin.Context, retryAfterSec int64) {
	if retryAfterSec > 0 {
		c.Header("Retry-After", fmt.Sprintf("%d", retryAfterSec))
	}
	c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many passcode attempts"})
}

func shareContentFromModel(share *model.ArticleShare) shareContentResponse {
	return shareContentResponse{
		Title:     share.Title,
		Content:   share.Content,
		Link:      share.Link,
		FeedName:  share.FeedName,
		PubDate:   share.PubDate,
		ExpiresAt: share.ExpiresAt,
	}
}

func (h *Handler) createShare(c *gin.Context) {
	setShareNoStore(c)

	var req createShareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequestError(c, "invalid request")
		return
	}
	if req.ItemID <= 0 {
		badRequestError(c, "invalid item_id")
		return
	}
	if req.ExpiresInSeconds == 0 {
		req.ExpiresInSeconds = defaultShareExpirySeconds
	}
	if req.ExpiresInSeconds < minShareExpirySeconds || req.ExpiresInSeconds > maxShareExpirySeconds {
		badRequestError(c, "invalid expires_in_seconds")
		return
	}
	if !req.PasscodeEnabled && strings.TrimSpace(req.Passcode) != "" {
		badRequestError(c, "passcode requires passcode_enabled")
		return
	}

	item, err := h.store.GetItem(req.ItemID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFoundError(c, "item")
			return
		}
		internalError(c, err, "get item for share")
		return
	}

	feedName := ""
	if feed, err := h.store.GetFeed(item.FeedID); err == nil {
		feedName = feed.Name
	} else if !errors.Is(err, store.ErrNotFound) {
		internalError(c, err, "get feed for share")
		return
	}

	passcode := ""
	passcodeHash := ""
	if req.PasscodeEnabled {
		passcode = strings.TrimSpace(req.Passcode)
		if passcode == "" {
			passcode, err = generateSharePasscode()
			if err != nil {
				internalError(c, err, "generate share passcode")
				return
			}
		}
		passcode, err = normalizeSharePasscode(passcode)
		if err != nil {
			badRequestError(c, err.Error())
			return
		}
		passcodeHash, err = auth.HashPassword(passcode)
		if err != nil {
			internalError(c, err, "hash share passcode")
			return
		}
	}

	token, err := generateShareToken()
	if err != nil {
		internalError(c, err, "generate share token")
		return
	}

	h.scrubExpiredShares()
	expiresAt := time.Now().Unix() + req.ExpiresInSeconds
	if _, err := h.store.CreateArticleShare(store.CreateArticleShareParams{
		Token:        token,
		ItemID:       &item.ID,
		Title:        item.Title,
		Content:      item.Content,
		Link:         item.Link,
		FeedName:     feedName,
		PubDate:      item.PubDate,
		PasscodeHash: passcodeHash,
		ExpiresAt:    expiresAt,
	}); err != nil {
		internalError(c, err, "create article share")
		return
	}

	response := gin.H{
		"token":      token,
		"path":       "/s/" + token,
		"expires_at": expiresAt,
	}
	if req.PasscodeEnabled {
		response["passcode"] = passcode
	}
	dataResponse(c, response)
}

func (h *Handler) getShare(c *gin.Context) {
	setShareNoStore(c)
	h.scrubExpiredShares()

	share, err := h.store.GetArticleShareByToken(c.Param("token"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFoundError(c, "share")
			return
		}
		internalError(c, err, "get article share")
		return
	}
	if share.ExpiresAt <= time.Now().Unix() {
		shareExpiredResponse(c)
		return
	}
	if share.PasscodeHash != "" {
		dataResponse(c, gin.H{
			"requires_passcode": true,
			"expires_at":        share.ExpiresAt,
		})
		return
	}
	dataResponse(c, shareContentFromModel(share))
}

func (h *Handler) accessShare(c *gin.Context) {
	setShareNoStore(c)
	h.scrubExpiredShares()

	share, err := h.store.GetArticleShareByToken(c.Param("token"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFoundError(c, "share")
			return
		}
		internalError(c, err, "get article share")
		return
	}
	if share.ExpiresAt <= time.Now().Unix() {
		shareExpiredResponse(c)
		return
	}
	if share.PasscodeHash == "" {
		dataResponse(c, shareContentFromModel(share))
		return
	}

	var req accessShareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequestError(c, "invalid request")
		return
	}

	now := time.Now()
	limiterKey := share.Token + "|" + c.ClientIP()
	allowed, retryAfter := h.shareLimiter.allow(limiterKey, now)
	if !allowed {
		shareTooManyAttemptsError(c, retryAfter)
		return
	}

	passcode, err := normalizeSharePasscode(req.Passcode)
	if err != nil || auth.CheckPassword(share.PasscodeHash, passcode) != nil {
		h.shareLimiter.recordFailure(limiterKey, now)
		c.JSON(http.StatusForbidden, gin.H{"error": "invalid passcode"})
		return
	}
	h.shareLimiter.recordSuccess(limiterKey)
	dataResponse(c, shareContentFromModel(share))
}
