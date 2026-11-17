package extract

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func newArticleServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func runArticle(t *testing.T, rawURL, contentSelector, excludeSelector string) (*Result, error) {
	t.Helper()
	return Article(context.Background(), Options{
		URL:             rawURL,
		ContentSelector: contentSelector,
		ExcludeSelector: excludeSelector,
		AllowPrivate:    true,
		Timeout:         5 * time.Second,
	})
}

func TestArticleExtractsSelectedRootAndExcludesContent(t *testing.T) {
	server := newArticleServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><body>
<article id="body">
  <p>Keep this paragraph.</p>
  <div class="ad">Remove this advertisement.</div>
  <table><tbody><tr><td>Cell</td></tr></tbody></table>
</article>
<div>Outside</div>
</body></html>`)
	})

	result, err := runArticle(t, server.URL+"/article", "#body", ".ad")
	if err != nil {
		t.Fatalf("Article() failed: %v", err)
	}
	if !strings.Contains(result.Content, "<article") {
		t.Fatalf("expected outer article root, got %q", result.Content)
	}
	if !strings.Contains(result.Content, "Keep this paragraph.") {
		t.Fatalf("expected selected paragraph, got %q", result.Content)
	}
	if strings.Contains(result.Content, "Remove this advertisement.") {
		t.Fatalf("expected excluded advertisement to be removed, got %q", result.Content)
	}
	if strings.Contains(result.Content, "Outside") {
		t.Fatalf("expected content outside the selector to be omitted, got %q", result.Content)
	}
	if !strings.Contains(result.Content, "<table>") {
		t.Fatalf("expected table structure, got %q", result.Content)
	}
}

func TestArticlePreservesDirectTableAndListRoots(t *testing.T) {
	server := newArticleServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><body>
<table id="table-body"><tbody><tr><td>Cell</td></tr></tbody></table>
<ul id="list-body"><li>One</li><li>Two</li></ul>
</body></html>`)
	})

	table, err := runArticle(t, server.URL+"/article", "#table-body", "")
	if err != nil {
		t.Fatalf("table Article() failed: %v", err)
	}
	if !strings.HasPrefix(table.Content, "<table") || !strings.Contains(table.Content, "<tr>") {
		t.Fatalf("expected outer table root, got %q", table.Content)
	}

	list, err := runArticle(t, server.URL+"/article", "#list-body", "")
	if err != nil {
		t.Fatalf("list Article() failed: %v", err)
	}
	if !strings.HasPrefix(list.Content, "<ul") || !strings.Contains(list.Content, "<li>One</li>") {
		t.Fatalf("expected outer list root, got %q", list.Content)
	}
}

func TestArticleNestedMatchesAndDocumentOrder(t *testing.T) {
	server := newArticleServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><body>
<div class="second">Second</div>
<div class="outer"><p>Nested text</p></div>
<div class="first">First</div>
</body></html>`)
	})

	result, err := runArticle(t, server.URL+"/article", ".second, .first", "")
	if err != nil {
		t.Fatalf("Article() failed: %v", err)
	}
	if strings.Index(result.Content, "Second") > strings.Index(result.Content, "First") {
		t.Fatalf("expected document order, got %q", result.Content)
	}

	nested, err := runArticle(t, server.URL+"/article", "div, p", "")
	if err != nil {
		t.Fatalf("nested Article() failed: %v", err)
	}
	if strings.Count(nested.Content, "Nested text") != 1 {
		t.Fatalf("expected nested match once, got %q", nested.Content)
	}
}

func TestArticleResolvesRelativeURLsAgainstFinalURL(t *testing.T) {
	server := newArticleServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/article" {
			http.Redirect(w, r, "/final/page", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><body><article id="body">
<a href="../next">Next</a><img src="img.png" alt="Image">
</article></body></html>`)
	})

	result, err := runArticle(t, server.URL+"/article", "#body", "")
	if err != nil {
		t.Fatalf("Article() failed: %v", err)
	}
	if !strings.Contains(result.Content, `href="`+server.URL+`/next"`) {
		t.Fatalf("expected absolute link, got %q", result.Content)
	}
	if !strings.Contains(result.Content, `src="`+server.URL+`/final/img.png"`) {
		t.Fatalf("expected absolute image source, got %q", result.Content)
	}
	if result.FinalURL != server.URL+"/final/page" {
		t.Fatalf("final URL = %q, want %q", result.FinalURL, server.URL+"/final/page")
	}
}

func TestArticleAcceptsImageOnlyAndShortContent(t *testing.T) {
	server := newArticleServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><body><div id="image"><img src="photo.jpg"></div><div id="short">Hi</div></body></html>`)
	})

	image, err := runArticle(t, server.URL+"/article", "#image", "")
	if err != nil {
		t.Fatalf("image Article() failed: %v", err)
	}
	if !strings.Contains(image.Content, "<img") {
		t.Fatalf("expected image content, got %q", image.Content)
	}

	short, err := runArticle(t, server.URL+"/article", "#short", "")
	if err != nil {
		t.Fatalf("short Article() failed: %v", err)
	}
	if !strings.Contains(short.Content, "Hi") {
		t.Fatalf("expected short content, got %q", short.Content)
	}
}

func TestArticleRejectsInvalidSelector(t *testing.T) {
	server := newArticleServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body><div>Body</div></body></html>`)
	})

	_, err := runArticle(t, server.URL+"/article", "div[", "")
	if !errors.Is(err, ErrInvalidSelector) {
		t.Fatalf("expected ErrInvalidSelector, got %v", err)
	}
}

func TestArticleNoMatchAndEmptyContent(t *testing.T) {
	server := newArticleServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body><div id="empty"><span></span></div><div id="body">Text</div></body></html>`)
	})

	if _, err := runArticle(t, server.URL+"/article", "#missing", ""); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("expected ErrNoMatch, got %v", err)
	}
	if _, err := runArticle(t, server.URL+"/article", "#empty", ""); !errors.Is(err, ErrEmptyContent) {
		t.Fatalf("expected ErrEmptyContent, got %v", err)
	}
}

func TestArticleRejectsUnsupportedContentType(t *testing.T) {
	server := newArticleServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.7"))
	})

	_, err := runArticle(t, server.URL+"/article", "body", "")
	if !errors.Is(err, ErrUnsupportedContentType) {
		t.Fatalf("expected ErrUnsupportedContentType, got %v", err)
	}
}

func TestArticleUpstreamFailure(t *testing.T) {
	server := newArticleServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	})

	_, err := runArticle(t, server.URL+"/article", "body", "")
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("expected ErrUpstream, got %v", err)
	}
}

func TestArticleContextCancellation(t *testing.T) {
	server := newArticleServer(t, func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body><div>Body</div></body></html>`)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := Article(ctx, Options{
		URL:             server.URL + "/article",
		ContentSelector: "body",
		AllowPrivate:    true,
		Timeout:         5 * time.Second,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline, got %v", err)
	}
}

func TestArticleSizeLimit(t *testing.T) {
	prefix := `<html><body><div id="body">ok</div><!--`
	suffix := `--></body></html>`
	padding := int(MaxArticleBytes) - len(prefix) - len(suffix)
	if padding < 0 {
		t.Fatal("size-limit fixture is larger than the limit")
	}

	t.Run("exact limit is eligible", func(t *testing.T) {
		server := newArticleServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, prefix+strings.Repeat("x", padding)+suffix)
		})
		result, err := runArticle(t, server.URL+"/article", "#body", "")
		if err != nil {
			t.Fatalf("Article() failed: %v", err)
		}
		if !strings.Contains(result.Content, "ok") {
			t.Fatalf("expected content, got %q", result.Content)
		}
	})

	t.Run("oversized response fails", func(t *testing.T) {
		server := newArticleServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, prefix+strings.Repeat("x", padding+1)+suffix)
		})
		if _, err := runArticle(t, server.URL+"/article", "#body", ""); !errors.Is(err, ErrTooLarge) {
			t.Fatalf("expected ErrTooLarge, got %v", err)
		}
	})
}

func TestArticleDecodesGBKHeaderAndMeta(t *testing.T) {
	gbk := simplifiedchinese.GBK.NewEncoder()
	encode := func(t *testing.T, raw string) []byte {
		t.Helper()
		body, err := gbk.Bytes([]byte(raw))
		if err != nil {
			t.Fatalf("encode GBK: %v", err)
		}
		return body
	}

	t.Run("HTTP header declares GBK", func(t *testing.T) {
		body := encode(t, `<html><body><div id="body">中文内容</div></body></html>`)
		server := newArticleServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=gbk")
			_, _ = w.Write(body)
		})
		result, err := runArticle(t, server.URL+"/article", "#body", "")
		if err != nil {
			t.Fatalf("Article() failed: %v", err)
		}
		if !strings.Contains(result.Content, "中文内容") {
			t.Fatalf("expected decoded Chinese, got %q", result.Content)
		}
	})

	t.Run("meta declares GBK without HTTP charset", func(t *testing.T) {
		body := encode(t, `<html><head><meta charset="gbk"></head><body><div id="body">中文内容</div></body></html>`)
		server := newArticleServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write(body)
		})
		result, err := runArticle(t, server.URL+"/article", "#body", "")
		if err != nil {
			t.Fatalf("Article() failed: %v", err)
		}
		if !strings.Contains(result.Content, "中文内容") {
			t.Fatalf("expected decoded Chinese, got %q", result.Content)
		}
	})
}
