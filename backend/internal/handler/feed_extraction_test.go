package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/0x2E/fusion/internal/config"
	"github.com/0x2E/fusion/internal/store"
	"github.com/gin-gonic/gin"
)

type previewExtractionEnvelope struct {
	Data struct {
		Content string `json:"content"`
		URL     string `json:"url"`
	} `json:"data"`
	Error string `json:"error"`
}

func newPreviewTestHandler(t *testing.T, cfg *config.Config) (*Handler, *store.Store, *gin.Engine, *http.Cookie) {
	t.Helper()
	if cfg.Password == "" {
		cfg.Password = "secret"
	}
	if cfg.PullTimeout == 0 {
		cfg.PullTimeout = 5
	}
	if cfg.LoginRateLimit == 0 {
		cfg.LoginRateLimit = 10
	}
	if cfg.LoginWindow == 0 {
		cfg.LoginWindow = 60
	}
	if cfg.LoginBlock == 0 {
		cfg.LoginBlock = 300
	}
	cfg.AllowPrivateFeeds = true

	h, st := newTestHandlerWithConfig(t, cfg)
	r := h.SetupRouter()

	w := performRequest(r, http.MethodPost, "/api/sessions", mustJSONBody(t, map[string]string{"password": "secret"}), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected a session cookie after login")
	}
	return h, st, r, cookies[0]
}

func previewRequest(t *testing.T, r http.Handler, cookie *http.Cookie, feedID int64, body any) *httptest.ResponseRecorder {
	t.Helper()
	return performRequest(
		r,
		http.MethodPost,
		fmt.Sprintf("/api/feeds/%d/extraction/preview", feedID),
		mustJSONBody(t, body),
		map[string]string{"Content-Type": "application/json"},
		cookie,
	)
}

func decodePreview(t *testing.T, w *httptest.ResponseRecorder) previewExtractionEnvelope {
	t.Helper()
	var envelope previewExtractionEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode preview response: %v (body: %s)", err, w.Body.String())
	}
	return envelope
}

func TestFeedUpdateContentSelectors(t *testing.T) {
	_, st, r, cookie := newPreviewTestHandler(t, &config.Config{})
	group, err := st.CreateGroup("G")
	if err != nil {
		t.Fatalf("CreateGroup() failed: %v", err)
	}
	feed, err := st.CreateFeed(group.ID, "Feed", "https://example.com/feed", "", "", nil)
	if err != nil {
		t.Fatalf("CreateFeed() failed: %v", err)
	}

	patch := func(body map[string]any) *httptest.ResponseRecorder {
		return performRequest(
			r,
			http.MethodPatch,
			fmt.Sprintf("/api/feeds/%d", feed.ID),
			mustJSONBody(t, body),
			map[string]string{"Content-Type": "application/json"},
			cookie,
		)
	}

	if w := patch(map[string]any{
		"content_selector": "article .body",
		"exclude_selector": ".ad",
	}); w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	saved, err := st.GetFeed(feed.ID)
	if err != nil {
		t.Fatalf("GetFeed() failed: %v", err)
	}
	if saved.ContentSelector != "article .body" || saved.ExcludeSelector != ".ad" {
		t.Fatalf("selectors = %q / %q", saved.ContentSelector, saved.ExcludeSelector)
	}

	if w := patch(map[string]any{"content_selector": "div["}); w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid selector, got %d: %s", w.Code, w.Body.String())
	}
	saved, err = st.GetFeed(feed.ID)
	if err != nil {
		t.Fatalf("GetFeed() after invalid selector failed: %v", err)
	}
	if saved.ContentSelector != "article .body" || saved.ExcludeSelector != ".ad" {
		t.Fatalf("invalid update changed selectors: %q / %q", saved.ContentSelector, saved.ExcludeSelector)
	}

	if w := patch(map[string]any{"content_selector": "main"}); w.Code != http.StatusOK {
		t.Fatalf("expected partial update 200, got %d: %s", w.Code, w.Body.String())
	}
	saved, err = st.GetFeed(feed.ID)
	if err != nil {
		t.Fatalf("GetFeed() after partial update failed: %v", err)
	}
	if saved.ContentSelector != "main" || saved.ExcludeSelector != ".ad" {
		t.Fatalf("partial selectors = %q / %q", saved.ContentSelector, saved.ExcludeSelector)
	}

	if w := patch(map[string]any{
		"content_selector": "",
		"exclude_selector": " ",
	}); w.Code != http.StatusOK {
		t.Fatalf("expected clear 200, got %d: %s", w.Code, w.Body.String())
	}
	saved, err = st.GetFeed(feed.ID)
	if err != nil {
		t.Fatalf("GetFeed() after clear failed: %v", err)
	}
	if saved.ContentSelector != "" || saved.ExcludeSelector != "" {
		t.Fatalf("expected cleared selectors, got %q / %q", saved.ContentSelector, saved.ExcludeSelector)
	}
}

func TestPreviewFeedExtractionRequiresAuth(t *testing.T) {
	_, st, r, _ := newPreviewTestHandler(t, &config.Config{})
	group, err := st.CreateGroup("G")
	if err != nil {
		t.Fatalf("CreateGroup() failed: %v", err)
	}
	feed, err := st.CreateFeed(group.ID, "Feed", "https://example.com/feed", "", "", nil)
	if err != nil {
		t.Fatalf("CreateFeed() failed: %v", err)
	}

	w := performRequest(
		r,
		http.MethodPost,
		fmt.Sprintf("/api/feeds/%d/extraction/preview", feed.ID),
		mustJSONBody(t, map[string]string{"url": "https://example.com/article", "content_selector": "body"}),
		map[string]string{"Content-Type": "application/json"},
	)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPreviewFeedExtractionExtractsAndDoesNotPersist(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><body>
<article id="body"><p>Preview body</p><div class="ad">Advertisement</div>
<a href="/next">Next</a></article>
</body></html>`)
	}))
	defer origin.Close()

	_, st, r, cookie := newPreviewTestHandler(t, &config.Config{})
	group, err := st.CreateGroup("G")
	if err != nil {
		t.Fatalf("CreateGroup() failed: %v", err)
	}
	feed, err := st.CreateFeed(group.ID, "Feed", "https://example.com/feed", "", "", nil)
	if err != nil {
		t.Fatalf("CreateFeed() failed: %v", err)
	}

	w := previewRequest(t, r, cookie, feed.ID, map[string]string{
		"url":              origin.URL + "/article",
		"content_selector": "#body",
		"exclude_selector": ".ad",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	result := decodePreview(t, w).Data
	if !strings.Contains(result.Content, "Preview body") {
		t.Fatalf("expected preview body, got %q", result.Content)
	}
	if strings.Contains(result.Content, "Advertisement") {
		t.Fatalf("expected excluded content, got %q", result.Content)
	}
	if !strings.Contains(result.Content, `href="`+origin.URL+`/next"`) {
		t.Fatalf("expected absolute link, got %q", result.Content)
	}
	if result.URL != origin.URL+"/article" {
		t.Fatalf("final URL = %q, want %q", result.URL, origin.URL+"/article")
	}

	saved, err := st.GetFeed(feed.ID)
	if err != nil {
		t.Fatalf("GetFeed() failed: %v", err)
	}
	if saved.ContentSelector != "" || saved.ExcludeSelector != "" {
		t.Fatalf("preview persisted selectors: %+v", saved)
	}
	items, err := st.ListItems(store.ListItemsParams{FeedID: &feed.ID})
	if err != nil {
		t.Fatalf("ListItems() failed: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("preview inserted items: %d", len(items))
	}
}

func TestPreviewFeedExtractionValidationAndErrorMapping(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/no-match":
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `<html><body><div>Text</div></body></html>`)
		case "/upstream":
			http.Error(w, "unavailable", http.StatusInternalServerError)
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `<html><body><div id="body">Text</div></body></html>`)
		}
	}))
	defer origin.Close()

	_, st, r, cookie := newPreviewTestHandler(t, &config.Config{})
	group, err := st.CreateGroup("G")
	if err != nil {
		t.Fatalf("CreateGroup() failed: %v", err)
	}
	feed, err := st.CreateFeed(group.ID, "Feed", "https://example.com/feed", "", "", nil)
	if err != nil {
		t.Fatalf("CreateFeed() failed: %v", err)
	}

	tests := []struct {
		name string
		body map[string]string
		want int
	}{
		{
			name: "invalid selector",
			body: map[string]string{"url": origin.URL + "/article", "content_selector": "div["},
			want: http.StatusBadRequest,
		},
		{
			name: "no match",
			body: map[string]string{"url": origin.URL + "/no-match", "content_selector": "#missing"},
			want: http.StatusBadRequest,
		},
		{
			name: "upstream failure",
			body: map[string]string{"url": origin.URL + "/upstream", "content_selector": "body"},
			want: http.StatusBadGateway,
		},
		{
			name: "invalid proxy",
			body: map[string]string{"url": origin.URL + "/article", "content_selector": "body", "proxy": "file:///tmp/proxy"},
			want: http.StatusBadRequest,
		},
		{
			name: "invalid url",
			body: map[string]string{"url": "file:///tmp/article", "content_selector": "body"},
			want: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := previewRequest(t, r, cookie, feed.ID, tt.body)
			if w.Code != tt.want {
				t.Fatalf("expected %d, got %d: %s", tt.want, w.Code, w.Body.String())
			}
			if decodePreview(t, w).Error == "" {
				t.Fatal("expected an error message")
			}
		})
	}

	w := previewRequest(t, r, cookie, 99999, map[string]string{
		"url": origin.URL + "/article", "content_selector": "body",
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing feed, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPreviewFeedExtractionUsesDraftAndSavedProxy(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test-Proxy") != "1" {
			http.Error(w, "proxy header missing", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body><div id="body">Proxied</div></body></html>`)
	}))
	defer origin.Close()

	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL == nil || !r.URL.IsAbs() {
			http.Error(w, "expected absolute proxy URL", http.StatusBadRequest)
			return
		}
		req, err := http.NewRequestWithContext(r.Context(), r.Method, r.URL.String(), r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		req.Header = r.Header.Clone()
		req.Header.Set("X-Test-Proxy", "1")
		resp, err := http.DefaultTransport.RoundTrip(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for key, values := range resp.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	defer proxy.Close()

	_, st, r, cookie := newPreviewTestHandler(t, &config.Config{})
	group, err := st.CreateGroup("G")
	if err != nil {
		t.Fatalf("CreateGroup() failed: %v", err)
	}
	feed, err := st.CreateFeed(group.ID, "Feed", "https://example.com/feed", "", "", nil)
	if err != nil {
		t.Fatalf("CreateFeed() failed: %v", err)
	}

	t.Run("draft proxy", func(t *testing.T) {
		w := previewRequest(t, r, cookie, feed.ID, map[string]string{
			"url": origin.URL + "/article", "content_selector": "#body", "proxy": proxy.URL,
		})
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(decodePreview(t, w).Data.Content, "Proxied") {
			t.Fatal("expected proxied preview content")
		}
		saved, err := st.GetFeed(feed.ID)
		if err != nil {
			t.Fatalf("GetFeed() failed: %v", err)
		}
		if saved.Proxy != "" {
			t.Fatalf("preview persisted proxy: %q", saved.Proxy)
		}
	})

	t.Run("saved proxy", func(t *testing.T) {
		proxyURL := proxy.URL
		if err := st.UpdateFeed(feed.ID, store.UpdateFeedParams{Proxy: &proxyURL}); err != nil {
			t.Fatalf("UpdateFeed() failed: %v", err)
		}
		w := previewRequest(t, r, cookie, feed.ID, map[string]string{
			"url": origin.URL + "/article", "content_selector": "#body",
		})
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestPreviewFeedExtractionDeadlineAfterDownloadDuringParsing(t *testing.T) {
	articleBody := "<html><body><article id=\"body\">" +
		strings.Repeat("<p>Long article paragraph.</p>", 60000) +
		"</article></body></html>"
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, articleBody)
	}))
	defer origin.Close()

	h, st, r, cookie := newPreviewTestHandler(t, &config.Config{PullTimeout: 5})
	h.previewTimeout = 40 * time.Millisecond

	group, err := st.CreateGroup("G")
	if err != nil {
		t.Fatalf("CreateGroup() failed: %v", err)
	}
	feed, err := st.CreateFeed(group.ID, "Feed", "https://example.com/feed", "", "", nil)
	if err != nil {
		t.Fatalf("CreateFeed() failed: %v", err)
	}

	server := httptest.NewUnstartedServer(r)
	server.Config.WriteTimeout = 500 * time.Millisecond
	server.Start()
	defer server.Close()

	payload, err := json.Marshal(map[string]string{
		"url": origin.URL + "/article", "content_selector": "#body",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req, err := http.NewRequest(
		http.MethodPost,
		fmt.Sprintf("%s/api/feeds/%d/extraction/preview", server.URL, feed.ID),
		bytes.NewReader(payload),
	)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)

	client := &http.Client{Timeout: 5 * time.Second}
	started := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("preview request failed at connection level: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	t.Logf("work_budget=40ms elapsed=%s HTTP_status=%d body_bytes=%d", time.Since(started), resp.StatusCode, len(body))
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", resp.StatusCode, body)
	}
	var envelope previewExtractionEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, body)
	}
	if envelope.Error == "" {
		t.Fatalf("expected a structured error, got %s", body)
	}
}

func TestPreviewFeedExtractionSlowOriginReturns502BeforeWriteTimeout(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body><div id="body">Slow</div></body></html>`)
	}))
	defer origin.Close()

	h, st, r, cookie := newPreviewTestHandler(t, &config.Config{PullTimeout: 1})
	h.previewTimeout = 50 * time.Millisecond

	group, err := st.CreateGroup("G")
	if err != nil {
		t.Fatalf("CreateGroup() failed: %v", err)
	}
	feed, err := st.CreateFeed(group.ID, "Feed", "https://example.com/feed", "", "", nil)
	if err != nil {
		t.Fatalf("CreateFeed() failed: %v", err)
	}

	server := httptest.NewUnstartedServer(r)
	server.Config.WriteTimeout = 100 * time.Millisecond
	server.Start()
	defer server.Close()

	payload, err := json.Marshal(map[string]string{
		"url": origin.URL + "/article", "content_selector": "#body",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req, err := http.NewRequest(
		http.MethodPost,
		fmt.Sprintf("%s/api/feeds/%d/extraction/preview", server.URL, feed.ID),
		bytes.NewReader(payload),
	)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("preview request failed at connection level: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", resp.StatusCode, body)
	}
	var envelope previewExtractionEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, body)
	}
	if envelope.Error == "" {
		t.Fatalf("expected a structured error, got %s", body)
	}
}
