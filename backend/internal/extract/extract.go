// Package extract fetches an article page and extracts the user-selected body
// with the same behaviour for preview and automatic feed pulling.
package extract

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/0x2E/fusion/internal/pkg/httpc"
	"github.com/PuerkitoBio/goquery"
	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

// MaxArticleBytes bounds the decompressed article response before decoding.
const MaxArticleBytes int64 = 10 << 20

// Extraction failures are matched by callers through errors.Is.
var (
	ErrContentSelectorRequired = errors.New("content selector is required")
	ErrInvalidSelector         = errors.New("invalid selector")
	ErrNoMatch                 = errors.New("no matching content")
	ErrEmptyContent            = errors.New("empty extracted content")
	ErrTooLarge                = errors.New("article page exceeds size limit")
	ErrUnsupportedContentType  = errors.New("unsupported article content type")
	ErrUpstream                = errors.New("article fetch failed")
)

// Options describes one article extraction request.
type Options struct {
	URL             string
	ContentSelector string
	ExcludeSelector string
	Proxy           string
	AllowPrivate    bool
	Timeout         time.Duration
}

// Result is the extracted HTML and the final response URL.
type Result struct {
	Content  string
	FinalURL string
}

// ValidateSelector compiles a nonempty CSS selector using Cascadia.
func ValidateSelector(selector string) error {
	trimmed := strings.TrimSpace(selector)
	if trimmed == "" {
		return ErrContentSelectorRequired
	}
	if _, err := cascadia.Compile(trimmed); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSelector, err)
	}
	return nil
}

// Article fetches opts.URL and returns the selected roots as HTML.
func Article(ctx context.Context, opts Options) (*Result, error) {
	contentSelector := strings.TrimSpace(opts.ContentSelector)
	if contentSelector == "" {
		return nil, ErrContentSelectorRequired
	}
	contentMatcher, err := cascadia.Compile(contentSelector)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSelector, err)
	}

	var excludeMatcher cascadia.Selector
	if excludeSelector := strings.TrimSpace(opts.ExcludeSelector); excludeSelector != "" {
		excludeMatcher, err = cascadia.Compile(excludeSelector)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidSelector, err)
		}
	}

	articleURL := strings.TrimSpace(opts.URL)
	if err := httpc.ValidateRequestURL(ctx, articleURL, opts.AllowPrivate); err != nil {
		return nil, err
	}

	client, err := httpc.NewClient(opts.Timeout, opts.Proxy, opts.AllowPrivate)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, articleURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build article request: %w", err)
	}
	httpc.SetDefaultHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		if isHTTPPolicyError(err) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: HTTP %d", ErrUpstream, resp.StatusCode)
	}

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if !isHTMLContentType(contentType) {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedContentType, contentType)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxArticleBytes+1))
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	if int64(len(body)) > MaxArticleBytes {
		return nil, ErrTooLarge
	}

	decoded, err := charset.NewReader(bytes.NewReader(body), contentType)
	if err != nil {
		return nil, fmt.Errorf("%w: decode: %v", ErrUpstream, err)
	}
	doc, err := goquery.NewDocumentFromReader(decoded)
	if err != nil {
		return nil, fmt.Errorf("%w: parse: %v", ErrUpstream, err)
	}

	doc.Find("script, style, noscript").Remove()
	if excludeMatcher != nil {
		doc.FindMatcher(excludeMatcher).Remove()
	}

	matches := doc.FindMatcher(contentMatcher)
	if matches.Length() == 0 {
		return nil, ErrNoMatch
	}

	finalURL := articleURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	base, err := url.Parse(finalURL)
	if err != nil {
		return nil, fmt.Errorf("%w: final url: %v", ErrUpstream, err)
	}

	var (
		parts    []string
		retained []*html.Node
		buildErr error
	)
	matches.EachWithBreak(func(_ int, selection *goquery.Selection) bool {
		if len(selection.Nodes) == 0 {
			return true
		}
		node := selection.Nodes[0]
		for _, root := range retained {
			if isDescendant(node, root) {
				return true
			}
		}
		if !hasReadableContent(selection) {
			return true
		}

		resolveRelativeURLs(selection, base)
		outer, err := goquery.OuterHtml(selection)
		if err != nil {
			buildErr = fmt.Errorf("serialize extracted content: %w", err)
			return false
		}
		retained = append(retained, node)
		parts = append(parts, outer)
		return true
	})
	if buildErr != nil {
		return nil, buildErr
	}

	content := strings.Join(parts, "\n")
	if strings.TrimSpace(content) == "" {
		return nil, ErrEmptyContent
	}
	return &Result{Content: content, FinalURL: finalURL}, nil
}

func isHTTPPolicyError(err error) bool {
	return errors.Is(err, httpc.ErrInvalidURL) ||
		errors.Is(err, httpc.ErrPrivateTarget) ||
		errors.Is(err, httpc.ErrInvalidProxy)
}

func isHTMLContentType(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return true
	}
	mediaType, _, err := mime.ParseMediaType(raw)
	if err != nil {
		return false
	}
	switch strings.ToLower(mediaType) {
	case "text/html", "application/xhtml+xml":
		return true
	default:
		return false
	}
}

func isDescendant(node, root *html.Node) bool {
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if parent == root {
			return true
		}
	}
	return false
}

func hasReadableContent(selection *goquery.Selection) bool {
	if strings.TrimSpace(selection.Text()) != "" {
		return true
	}
	if selection.Is("img") {
		if src, ok := selection.Attr("src"); ok && strings.TrimSpace(src) != "" {
			return true
		}
	}
	hasImage := false
	selection.Find("img").EachWithBreak(func(_ int, img *goquery.Selection) bool {
		if src, ok := img.Attr("src"); ok && strings.TrimSpace(src) != "" {
			hasImage = true
			return false
		}
		return true
	})
	return hasImage
}

func resolveRelativeURLs(root *goquery.Selection, base *url.URL) {
	resolve := func(selection *goquery.Selection) {
		if href, ok := selection.Attr("href"); ok {
			selection.SetAttr("href", resolveReference(base, href))
		}
		if src, ok := selection.Attr("src"); ok {
			selection.SetAttr("src", resolveReference(base, src))
		}
	}
	resolve(root)
	root.Find("[href], [src]").Each(func(_ int, selection *goquery.Selection) {
		resolve(selection)
	})
}

func resolveReference(base *url.URL, raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return raw
	}
	return base.ResolveReference(parsed).String()
}
