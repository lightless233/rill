package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/0x2E/fusion/internal/config"
)

type validateFeedBody struct {
	Data struct {
		Feeds []struct {
			Title string `json:"title"`
			Link  string `json:"link"`
		} `json:"feeds"`
	} `json:"data"`
	Error string `json:"error"`
}

func postValidateFeed(t *testing.T, h *Handler, url string) (int, validateFeedBody) {
	return postValidateFeedWithProxy(t, h, url, "")
}

func postValidateFeedWithProxy(t *testing.T, h *Handler, targetURL, proxy string) (int, validateFeedBody) {
	t.Helper()

	r := newTestRouter()
	r.POST("/api/feeds/validate", h.validateFeed)
	payload, err := json.Marshal(map[string]string{
		"url":   targetURL,
		"proxy": proxy,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	w := performRequest(
		r,
		http.MethodPost,
		"/api/feeds/validate",
		strings.NewReader(string(payload)),
		map[string]string{"Content-Type": "application/json"},
	)

	var body validateFeedBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response (%d %q): %v", w.Code, w.Body.String(), err)
	}
	return w.Code, body
}

// The add-feed dialog calls validate before creating a feed and blocks the
// creation when no feed is found, so a malformed document must yield an empty
// feeds list rather than an error or a fabricated entry.
func TestValidateFeedMalformedXMLReturnsNoFeeds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = fmt.Fprint(w, `<?xml version="1.0"?><rss><channel><title>Broken`)
	}))
	defer server.Close()

	h := &Handler{config: &config.Config{AllowPrivateFeeds: true}}

	code, body := postValidateFeed(t, h, server.URL)
	if code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", code)
	}
	if len(body.Data.Feeds) != 0 {
		t.Fatalf("expected 0 feeds for malformed XML, got %d: %+v", len(body.Data.Feeds), body.Data.Feeds)
	}
}

func TestValidateFeedValidRSSReturnsFeed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Serve the feed only at its real path so the finder's common-path
		// guesses (index.xml, atom.xml, ...) miss like they would in production.
		if r.URL.Path != "/feed.xml" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Tech Daily</title><link>https://example.com</link>
<item><guid>g1</guid><title>Item</title><link>https://example.com/1</link></item>
</channel></rss>`)
	}))
	defer server.Close()

	h := &Handler{config: &config.Config{AllowPrivateFeeds: true}}

	code, body := postValidateFeed(t, h, server.URL+"/feed.xml")
	if code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", code)
	}
	if len(body.Data.Feeds) != 1 {
		t.Fatalf("expected 1 feed, got %d: %+v", len(body.Data.Feeds), body.Data.Feeds)
	}
	if body.Data.Feeds[0].Title != "Tech Daily" {
		t.Fatalf("expected title %q, got %q", "Tech Daily", body.Data.Feeds[0].Title)
	}
	if body.Data.Feeds[0].Link != server.URL+"/feed.xml" {
		t.Fatalf("expected link %q, got %q", server.URL+"/feed.xml", body.Data.Feeds[0].Link)
	}
}

func TestValidateFeedUnreachableURLReturnsBadGateway(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	url := server.URL
	server.Close()

	h := &Handler{config: &config.Config{AllowPrivateFeeds: true}}

	code, body := postValidateFeed(t, h, url)
	if code != http.StatusBadGateway {
		t.Fatalf("expected status 502, got %d (error=%q)", code, body.Error)
	}
	if body.Error == "" {
		t.Fatal("expected non-empty error message for unreachable URL")
	}
}

// Discovery probes common paths on the host, so a feed elsewhere on the host
// makes /feeds/validate return a non-empty list even though the entered URL is
// not a feed itself. The frontend must therefore match the entered URL against
// the returned links instead of treating a non-empty list as approval.
func TestValidateFeedReturnsCommonPathFeedForInvalidPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feed.xml":
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Tech Daily</title><link>https://example.com</link>
<item><guid>g1</guid><title>Item</title><link>https://example.com/1</link></item>
</channel></rss>`)
		default:
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprint(w, `<?xml version="1.0"?><rss><channel><title>Broken`)
		}
	}))
	defer server.Close()

	h := &Handler{config: &config.Config{AllowPrivateFeeds: true}}

	code, body := postValidateFeed(t, h, server.URL+"/broken.xml")
	if code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", code)
	}
	found := false
	for _, feed := range body.Data.Feeds {
		if feed.Link == server.URL+"/feed.xml" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected discovery to surface %q for a broken entered path, got %+v", server.URL+"/feed.xml", body.Data.Feeds)
	}
}

func TestValidateFeedBlocksPrivateTargetsAndInvalidProxies(t *testing.T) {
	h := &Handler{config: &config.Config{AllowPrivateFeeds: false}}

	for _, target := range []string{
		"http://127.0.0.1/feed.xml",
		"http://localhost/feed.xml",
		"http://10.0.0.5/feed.xml",
		"file:///etc/passwd",
	} {
		code, body := postValidateFeed(t, h, target)
		if code != http.StatusBadRequest {
			t.Fatalf("target %q: expected 400, got %d (error=%q)", target, code, body.Error)
		}
	}

	for _, badProxy := range []string{
		"file:///etc/passwd",
		"not-a-proxy",
		"http:///missing-host",
	} {
		code, body := postValidateFeedWithProxy(t, h, "http://93.184.216.34/feed.xml", badProxy)
		if code != http.StatusBadRequest {
			t.Fatalf("proxy %q: expected 400, got %d (error=%q)", badProxy, code, body.Error)
		}
	}
}

func TestValidateFeedUpstreamFailuresReturnBadGateway(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/missing":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	hAllowPrivate := &Handler{config: &config.Config{AllowPrivateFeeds: true}}
	for _, path := range []string{"/missing", "/boom"} {
		code, _ := postValidateFeed(t, hAllowPrivate, server.URL+path)
		if code != http.StatusBadGateway {
			t.Fatalf("path %q: expected 502, got %d", path, code)
		}
	}

	hDefault := &Handler{config: &config.Config{AllowPrivateFeeds: false}}
	code, _ := postValidateFeed(t, hDefault, "http://nonexistent.fusion-test.invalid/feed.xml")
	if code != http.StatusBadGateway {
		t.Fatalf("DNS failure: expected 502, got %d", code)
	}
}

func TestValidateFeedPageWithNoFeedsAnd404ProbesReturnsEmptyOK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `<!doctype html><html><head>
<title>Plain Site</title>
<link rel="alternate" type="application/rss+xml" href="/missing-feed.xml">
</head><body><p>No feeds here</p></body></html>`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	h := &Handler{config: &config.Config{AllowPrivateFeeds: true}}

	code, body := postValidateFeed(t, h, server.URL+"/")
	if code != http.StatusOK {
		t.Fatalf("expected 200 for valid HTML page with missing probes, got %d (error=%q)", code, body.Error)
	}
	if len(body.Data.Feeds) != 0 {
		t.Fatalf("expected 0 feeds, got %+v", body.Data.Feeds)
	}
}

func TestValidateFeedProxyPolicyAndRedirectBlocking(t *testing.T) {
	var internalHits atomic.Int32
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host == "127.0.0.1" {
			internalHits.Add(1)
			w.WriteHeader(http.StatusTeapot)
			return
		}
		switch r.URL.Path {
		case "/feed.xml", "/public-feed.xml":
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Proxied Feed</title><link>http://93.184.216.34/</link>
<item><guid>g1</guid><title>Item</title><link>http://93.184.216.34/1</link></item>
</channel></rss>`)
		case "/redirect-private":
			http.Redirect(w, r, "http://127.0.0.1/secret.xml", http.StatusFound)
		case "/html-with-mixed-links":
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `<!doctype html><html><head>
<link rel="alternate" type="application/rss+xml" href="http://127.0.0.1/secret.xml">
<link rel="alternate" type="application/rss+xml" href="/public-feed.xml">
</head><body></body></html>`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer proxyServer.Close()

	h := &Handler{config: &config.Config{AllowPrivateFeeds: false}}

	t.Run("private proxy works for public feed target", func(t *testing.T) {
		code, body := postValidateFeedWithProxy(t, h, "http://93.184.216.34/feed.xml", proxyServer.URL)
		if code != http.StatusOK {
			t.Fatalf("expected 200, got %d (error=%q)", code, body.Error)
		}
		if len(body.Data.Feeds) != 1 || body.Data.Feeds[0].Title != "Proxied Feed" {
			t.Fatalf("unexpected feeds: %+v", body.Data.Feeds)
		}
	})

	t.Run("primary redirect to private host returns 400", func(t *testing.T) {
		before := internalHits.Load()
		code, _ := postValidateFeedWithProxy(t, h, "http://93.184.216.34/redirect-private", proxyServer.URL)
		if code != http.StatusBadRequest {
			t.Fatalf("expected 400 on blocked primary redirect, got %d", code)
		}
		if internalHits.Load() != before {
			t.Fatal("expected blocked redirect target not to be requested")
		}
	})

	t.Run("html discovery filters private candidates before probing", func(t *testing.T) {
		before := internalHits.Load()
		code, body := postValidateFeedWithProxy(t, h, "http://93.184.216.34/html-with-mixed-links", proxyServer.URL)
		if code != http.StatusOK {
			t.Fatalf("expected 200, got %d (error=%q)", code, body.Error)
		}
		if internalHits.Load() != before {
			t.Fatal("expected private HTML candidate not to be probed")
		}
		if len(body.Data.Feeds) != 1 || body.Data.Feeds[0].Link != "http://93.184.216.34/public-feed.xml" {
			t.Fatalf("unexpected discovered feeds: %+v", body.Data.Feeds)
		}
	})
}
