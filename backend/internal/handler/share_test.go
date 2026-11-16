package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/0x2E/fusion/internal/model"
	"github.com/0x2E/fusion/internal/store"
	"github.com/gin-gonic/gin"
)

type shareCreateData struct {
	Token     string `json:"token"`
	Path      string `json:"path"`
	ExpiresAt int64  `json:"expires_at"`
	Passcode  string `json:"passcode"`
}

type shareCreateEnvelope struct {
	Data shareCreateData `json:"data"`
}

type shareContentData struct {
	Title     string `json:"title"`
	Content   string `json:"content"`
	Link      string `json:"link"`
	FeedName  string `json:"feed_name"`
	PubDate   int64  `json:"pub_date"`
	ExpiresAt int64  `json:"expires_at"`
}

type shareContentEnvelope struct {
	Data shareContentData `json:"data"`
}

type shareProtectedData struct {
	RequiresPasscode bool  `json:"requires_passcode"`
	ExpiresAt        int64 `json:"expires_at"`
}

type shareProtectedEnvelope struct {
	Data shareProtectedData `json:"data"`
}

func newShareTestRouter(t *testing.T) (*gin.Engine, *http.Cookie, *store.Store) {
	t.Helper()

	h, st := newFeverTestHandler(t) // password: "secret"
	r := h.SetupRouter()

	w := performRequest(r, http.MethodPost, "/api/sessions", mustJSONBody(t, map[string]string{"password": "secret"}), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected a session cookie after login")
	}

	return r, cookies[0], st
}

func createShareFixture(t *testing.T, st *store.Store, title string) (*model.Feed, *model.Item) {
	t.Helper()

	group, err := st.CreateGroup("Share Group " + title)
	if err != nil {
		t.Fatalf("CreateGroup() failed: %v", err)
	}
	feed, err := st.CreateFeed(group.ID, "Share Feed "+title, "https://example.com/"+title+"/feed", "https://example.com", "", nil)
	if err != nil {
		t.Fatalf("CreateFeed() failed: %v", err)
	}
	item, err := st.CreateItem(
		feed.ID,
		"share-guid-"+title,
		title,
		"https://example.com/"+title,
		"<p>Body for "+title+"</p>",
		1700000000,
	)
	if err != nil {
		t.Fatalf("CreateItem() failed: %v", err)
	}

	return feed, item
}

func postCreateShare(t *testing.T, r http.Handler, cookie *http.Cookie, body any) *httptest.ResponseRecorder {
	t.Helper()

	return performRequest(
		r,
		http.MethodPost,
		"/api/shares",
		mustJSONBody(t, body),
		map[string]string{"Content-Type": "application/json"},
		cookie,
	)
}

func decodeCreateShare(t *testing.T, w *httptest.ResponseRecorder) shareCreateData {
	t.Helper()

	if w.Code != http.StatusOK {
		t.Fatalf("create share status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	var envelope shareCreateEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode create share: %v (body=%s)", err, w.Body.String())
	}
	return envelope.Data
}

func TestCreateShareRequiresSession(t *testing.T) {
	_, _, st := newShareTestRouter(t)
	_, item := createShareFixture(t, st, "auth")

	h, _ := newFeverTestHandler(t)
	r := h.SetupRouter()

	w := performRequest(
		r,
		http.MethodPost,
		"/api/shares",
		mustJSONBody(t, map[string]any{"item_id": item.ID}),
		map[string]string{"Content-Type": "application/json"},
	)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without a session, got %d", w.Code)
	}
}

func TestShareSnapshotSurvivesSourceDeletion(t *testing.T) {
	r, cookie, st := newShareTestRouter(t)
	feed, item := createShareFixture(t, st, "snapshot")

	created := decodeCreateShare(t, postCreateShare(t, r, cookie, map[string]any{
		"item_id":            item.ID,
		"expires_in_seconds": 3600,
	}))
	if created.Token == "" || created.Path != "/s/"+created.Token {
		t.Fatalf("unexpected share token/path: %+v", created)
	}
	if created.Passcode != "" {
		t.Fatalf("expected no passcode, got %q", created.Passcode)
	}
	if created.ExpiresAt < time.Now().Unix()+3500 {
		t.Fatalf("unexpected expiry: %d", created.ExpiresAt)
	}

	getShare := func() *httptest.ResponseRecorder {
		return performRequest(r, http.MethodGet, "/api/shares/"+created.Token, nil, nil)
	}

	w := getShare()
	if w.Code != http.StatusOK {
		t.Fatalf("public GET status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	if cacheControl := w.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", cacheControl)
	}
	var content shareContentEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &content); err != nil {
		t.Fatalf("decode share content: %v", err)
	}
	if content.Data.Title != item.Title || content.Data.Content != item.Content || content.Data.FeedName != feed.Name {
		t.Fatalf("unexpected public content: %+v", content.Data)
	}

	if err := st.DeleteFeed(feed.ID); err != nil {
		t.Fatalf("DeleteFeed() failed: %v", err)
	}

	w = getShare()
	if w.Code != http.StatusOK {
		t.Fatalf("public GET after source deletion status = %d (body=%s)", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &content); err != nil {
		t.Fatalf("decode share content after deletion: %v", err)
	}
	if content.Data.Title != item.Title || content.Data.Content != item.Content {
		t.Fatalf("snapshot changed after source deletion: %+v", content.Data)
	}
}

func TestProtectedShareRequiresPasscodeWithoutLeakingContent(t *testing.T) {
	r, cookie, st := newShareTestRouter(t)
	_, item := createShareFixture(t, st, "protected")

	created := decodeCreateShare(t, postCreateShare(t, r, cookie, map[string]any{
		"item_id":            item.ID,
		"expires_in_seconds": 3600,
		"passcode_enabled":   true,
		"passcode":           "a7k2",
	}))
	if created.Passcode != "A7K2" {
		t.Fatalf("expected normalized passcode A7K2, got %q", created.Passcode)
	}

	w := performRequest(r, http.MethodGet, "/api/shares/"+created.Token, nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("protected GET status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	if cacheControl := w.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", cacheControl)
	}
	var protected shareProtectedEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &protected); err != nil {
		t.Fatalf("decode protected metadata: %v", err)
	}
	if !protected.Data.RequiresPasscode || protected.Data.ExpiresAt != created.ExpiresAt {
		t.Fatalf("unexpected protected metadata: %+v", protected.Data)
	}
	for _, leaked := range []string{item.Title, item.Content, item.Link, "feed_name", `"title"`, `"content"`} {
		if strings.Contains(w.Body.String(), leaked) {
			t.Fatalf("protected response leaked %q: %s", leaked, w.Body.String())
		}
	}

	w = performRequest(
		r,
		http.MethodPost,
		"/api/shares/"+created.Token+"/access",
		mustJSONBody(t, map[string]string{"passcode": "zzzz"}),
		map[string]string{"Content-Type": "application/json"},
	)
	if w.Code != http.StatusForbidden {
		t.Fatalf("wrong passcode status = %d, want 403 (body=%s)", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), item.Title) || strings.Contains(w.Body.String(), item.Content) {
		t.Fatalf("wrong passcode leaked content: %s", w.Body.String())
	}
	if strings.Contains(w.Header().Get("Set-Cookie"), "session=") {
		t.Fatalf("wrong passcode must not set a session cookie")
	}

	w = performRequest(
		r,
		http.MethodPost,
		"/api/shares/"+created.Token+"/access",
		mustJSONBody(t, map[string]string{"passcode": "a7k2"}),
		map[string]string{"Content-Type": "application/json"},
	)
	if w.Code != http.StatusOK {
		t.Fatalf("correct passcode status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	var content shareContentEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &content); err != nil {
		t.Fatalf("decode unlocked content: %v", err)
	}
	if content.Data.Title != item.Title || content.Data.Content != item.Content {
		t.Fatalf("unexpected unlocked content: %+v", content.Data)
	}
}

func TestShareAutoGeneratesPasscode(t *testing.T) {
	r, cookie, st := newShareTestRouter(t)
	_, item := createShareFixture(t, st, "generated")

	created := decodeCreateShare(t, postCreateShare(t, r, cookie, map[string]any{
		"item_id":            item.ID,
		"expires_in_seconds": 3600,
		"passcode_enabled":   true,
	}))
	if len(created.Passcode) != 4 {
		t.Fatalf("expected 4-character generated passcode, got %q", created.Passcode)
	}

	w := performRequest(
		r,
		http.MethodPost,
		"/api/shares/"+created.Token+"/access",
		mustJSONBody(t, map[string]string{"passcode": created.Passcode}),
		map[string]string{"Content-Type": "application/json"},
	)
	if w.Code != http.StatusOK {
		t.Fatalf("generated passcode access status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
}

func TestSharePasscodeRateLimitAndSuccessReset(t *testing.T) {
	r, cookie, st := newShareTestRouter(t)
	_, item := createShareFixture(t, st, "rate-limit")

	wrongPasscode := func(token string) *httptest.ResponseRecorder {
		return performRequest(
			r,
			http.MethodPost,
			"/api/shares/"+token+"/access",
			mustJSONBody(t, map[string]string{"passcode": "zzzz"}),
			map[string]string{"Content-Type": "application/json"},
		)
	}

	created := decodeCreateShare(t, postCreateShare(t, r, cookie, map[string]any{
		"item_id":            item.ID,
		"expires_in_seconds": 3600,
		"passcode_enabled":   true,
		"passcode":           "A7K2",
	}))
	for i := 0; i < 5; i++ {
		if w := wrongPasscode(created.Token); w.Code != http.StatusForbidden {
			t.Fatalf("wrong passcode attempt %d status = %d, want 403 (body=%s)", i+1, w.Code, w.Body.String())
		}
	}
	w := wrongPasscode(created.Token)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("rate-limited attempt status = %d, want 429 (body=%s)", w.Code, w.Body.String())
	}
	if retryAfter := w.Header().Get("Retry-After"); retryAfter == "" || retryAfter == "0" {
		t.Fatalf("expected positive Retry-After, got %q", retryAfter)
	}
	if strings.Contains(w.Body.String(), item.Title) || strings.Contains(w.Body.String(), item.Content) {
		t.Fatalf("rate-limited response leaked content: %s", w.Body.String())
	}

	w = performRequest(
		r,
		http.MethodPost,
		"/api/shares/"+created.Token+"/access",
		mustJSONBody(t, map[string]string{"passcode": "A7K2"}),
		map[string]string{"Content-Type": "application/json"},
	)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("correct passcode while blocked status = %d, want 429", w.Code)
	}

	second := decodeCreateShare(t, postCreateShare(t, r, cookie, map[string]any{
		"item_id":            item.ID,
		"expires_in_seconds": 3600,
		"passcode_enabled":   true,
		"passcode":           "B8M3",
	}))
	for i := 0; i < 4; i++ {
		if w := wrongPasscode(second.Token); w.Code != http.StatusForbidden {
			t.Fatalf("second share wrong attempt %d status = %d, want 403", i+1, w.Code)
		}
	}
	w = performRequest(
		r,
		http.MethodPost,
		"/api/shares/"+second.Token+"/access",
		mustJSONBody(t, map[string]string{"passcode": "B8M3"}),
		map[string]string{"Content-Type": "application/json"},
	)
	if w.Code != http.StatusOK {
		t.Fatalf("correct passcode status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	for i := 0; i < 4; i++ {
		if w := wrongPasscode(second.Token); w.Code != http.StatusForbidden {
			t.Fatalf("post-success wrong attempt %d status = %d, want 403 (failure state not cleared)", i+1, w.Code)
		}
	}
}

func TestExpiredShareReturnsGoneAndScrubsContent(t *testing.T) {
	r, _, st := newShareTestRouter(t)
	_, item := createShareFixture(t, st, "expired")
	item.Title = "Secret snapshot title"
	item.Content = "<p>Secret snapshot body</p>"

	share, err := st.CreateArticleShare(store.CreateArticleShareParams{
		Token:     "expired-token",
		ItemID:    &item.ID,
		Title:     item.Title,
		Content:   item.Content,
		Link:      item.Link,
		FeedName:  "Share Feed expired",
		PubDate:   item.PubDate,
		ExpiresAt: time.Now().Add(-time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("CreateArticleShare() failed: %v", err)
	}

	for _, target := range []string{"/api/shares/" + share.Token} {
		w := performRequest(r, http.MethodGet, target, nil, nil)
		if w.Code != http.StatusGone {
			t.Fatalf("GET %s status = %d, want 410 (body=%s)", target, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), item.Title) || strings.Contains(w.Body.String(), item.Content) {
			t.Fatalf("expired response leaked content: %s", w.Body.String())
		}
	}

	w := performRequest(
		r,
		http.MethodPost,
		"/api/shares/"+share.Token+"/access",
		mustJSONBody(t, map[string]string{"passcode": "AAAA"}),
		map[string]string{"Content-Type": "application/json"},
	)
	if w.Code != http.StatusGone {
		t.Fatalf("expired access status = %d, want 410", w.Code)
	}

	tombstone, err := st.GetArticleShareByToken(share.Token)
	if err != nil {
		t.Fatalf("expected expired tombstone, got error: %v", err)
	}
	if tombstone.Title != "" || tombstone.Content != "" || tombstone.FeedName != "" {
		t.Fatalf("expected expired content scrubbed, got %+v", tombstone)
	}

	w = performRequest(r, http.MethodGet, "/api/shares/unknown-token", nil, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown token status = %d, want 404 (body=%s)", w.Code, w.Body.String())
	}
}

func TestCreateShareValidation(t *testing.T) {
	r, cookie, st := newShareTestRouter(t)
	_, item := createShareFixture(t, st, "validation")

	tests := []struct {
		name string
		body map[string]any
		want int
	}{
		{
			name: "missing item",
			body: map[string]any{"item_id": item.ID + 9999},
			want: http.StatusNotFound,
		},
		{
			name: "expiry below minimum",
			body: map[string]any{"item_id": item.ID, "expires_in_seconds": 60},
			want: http.StatusBadRequest,
		},
		{
			name: "expiry above maximum",
			body: map[string]any{"item_id": item.ID, "expires_in_seconds": 365*24*60*60 + 1},
			want: http.StatusBadRequest,
		},
		{
			name: "passcode without enable flag",
			body: map[string]any{"item_id": item.ID, "passcode": "A7K2"},
			want: http.StatusBadRequest,
		},
		{
			name: "custom passcode too short",
			body: map[string]any{"item_id": item.ID, "passcode_enabled": true, "passcode": "A7K"},
			want: http.StatusBadRequest,
		},
		{
			name: "custom passcode has invalid character",
			body: map[string]any{"item_id": item.ID, "passcode_enabled": true, "passcode": "A7-2"},
			want: http.StatusBadRequest,
		},
		{
			name: "expiry defaults to 24 hours",
			body: map[string]any{"item_id": item.ID},
			want: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := postCreateShare(t, r, cookie, tt.body)
			if w.Code != tt.want {
				t.Fatalf("status = %d, want %d (body=%s)", w.Code, tt.want, w.Body.String())
			}
			if tt.want == http.StatusOK {
				created := decodeCreateShare(t, w)
				if created.ExpiresAt < time.Now().Unix()+23*60*60 {
					t.Fatalf("expected default 24h expiry, got %d", created.ExpiresAt)
				}
				if created.Passcode != "" {
					t.Fatalf("expected no passcode, got %q", created.Passcode)
				}
			}
		})
	}
}

func TestPublicShareRoutesSkipAuthMiddleware(t *testing.T) {
	r, cookie, st := newShareTestRouter(t)
	_, item := createShareFixture(t, st, "public")

	created := decodeCreateShare(t, postCreateShare(t, r, cookie, map[string]any{
		"item_id":            item.ID,
		"expires_in_seconds": 3600,
	}))
	w := performRequest(r, http.MethodGet, fmt.Sprintf("/api/shares/%s", created.Token), nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("public share GET without session status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
}
