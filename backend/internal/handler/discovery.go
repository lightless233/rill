package handler

// Feed discovery heuristics adapted from github.com/0x2E/feedfinder
// (MIT License, Copyright (c) 2025 Yuan) and routed strictly through
// internal/pkg/httpc so every outbound hop enforces SSRF and proxy policies.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/0x2E/fusion/internal/pkg/httpc"
	"github.com/PuerkitoBio/goquery"
	"github.com/mmcdole/gofeed"
)

const (
	maxDiscoveryHTMLProbes  = 16
	discoveryProbeWorkers   = 4
	discoveryRequestTimeout = 30 * time.Second
)

type upstreamFetchError struct {
	statusCode int
	err        error
}

func (e *upstreamFetchError) Error() string {
	if e.statusCode != 0 {
		return fmt.Sprintf("upstream returned HTTP %d", e.statusCode)
	}
	return fmt.Sprintf("upstream fetch failed: %v", e.err)
}

func (e *upstreamFetchError) Unwrap() error {
	return e.err
}

var (
	youtubeBrowseIDPattern  = regexp.MustCompile(`"browse_id"\s*,\s*"value"\s*:\s*"(UC[0-9A-Za-z_-]+)"`)
	youtubeExternalIDRegexp = regexp.MustCompile(`"externalId"\s*:\s*"(UC[0-9A-Za-z_-]+)"`)
	youtubeChannelURLRegexp = regexp.MustCompile(`https://www\.youtube\.com/channel/(UC[0-9A-Za-z_-]+)`)

	githubReservedPaths = map[string]struct{}{
		"about":         {},
		"account":       {},
		"collections":   {},
		"contact":       {},
		"enterprise":    {},
		"events":        {},
		"explore":       {},
		"features":      {},
		"issues":        {},
		"login":         {},
		"marketplace":   {},
		"notifications": {},
		"orgs":          {},
		"organizations": {},
		"pricing":       {},
		"pulls":         {},
		"search":        {},
		"security":      {},
		"settings":      {},
		"signup":        {},
		"site":          {},
		"sponsors":      {},
		"topics":        {},
		"trending":      {},
	}

	wellKnownFeedPaths = []string{
		"feed.xml",
		"atom.xml",
		"rss.xml",
		"index.xml",
		"feed.json",
		"atom.json",
		"rss.json",
		"index.json",
		"feed/",
		"rss/",
	}
)

func discoverFeeds(ctx context.Context, target, proxy string, allowPrivate bool) ([]discoveredFeed, error) {
	if err := httpc.ValidateRequestURL(ctx, target, allowPrivate); err != nil {
		return nil, err
	}
	if _, err := httpc.ValidateProxyURL(proxy); err != nil {
		return nil, err
	}

	parsedTarget, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", httpc.ErrInvalidURL, err)
	}

	if serviceFeeds := matchDirectServiceFeeds(parsedTarget); len(serviceFeeds) > 0 {
		return filterAndNormalizeDiscoveredFeeds(ctx, serviceFeeds, allowPrivate), nil
	}

	client, err := httpc.NewClient(discoveryRequestTimeout, proxy, allowPrivate)
	if err != nil {
		return nil, err
	}

	finalURL, body, err := fetchDiscoveryDocument(ctx, client, target, allowPrivate)
	if err != nil {
		return nil, err
	}

	if parsedFeed, parseErr := gofeed.NewParser().Parse(bytes.NewReader(body)); parseErr == nil && parsedFeed != nil {
		feeds := []discoveredFeed{{Title: parsedFeed.Title, Link: finalURL}}
		return filterAndNormalizeDiscoveredFeeds(ctx, feeds, allowPrivate), nil
	}

	finalParsed, err := url.Parse(finalURL)
	if err != nil {
		finalParsed = parsedTarget
	}

	if isYouTubeHost(finalParsed.Hostname()) {
		if channelID := extractYouTubeChannelID(body); channelID != "" {
			feeds := []discoveredFeed{{
				Title: extractHTMLTitle(body),
				Link:  "https://www.youtube.com/feeds/videos.xml?channel_id=" + url.QueryEscape(channelID),
			}}
			return filterAndNormalizeDiscoveredFeeds(ctx, feeds, allowPrivate), nil
		}
	}

	var rawFeeds []discoveredFeed
	if htmlCandidates := extractHTMLFeedCandidates(body, finalParsed); len(htmlCandidates) > 0 {
		rawFeeds = probeFeedCandidates(ctx, client, htmlCandidates, allowPrivate)
	}

	if len(rawFeeds) == 0 {
		wellKnownCandidates := buildWellKnownCandidates(finalParsed)
		rawFeeds = probeFeedCandidates(ctx, client, wellKnownCandidates, allowPrivate)
	}

	return filterAndNormalizeDiscoveredFeeds(ctx, rawFeeds, allowPrivate), nil
}

func fetchDiscoveryDocument(ctx context.Context, client *http.Client, rawURL string, allowPrivate bool) (string, []byte, error) {
	if err := httpc.ValidateRequestURL(ctx, rawURL, allowPrivate); err != nil {
		return "", nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %w", httpc.ErrInvalidURL, err)
	}
	httpc.SetDefaultHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, httpc.ErrInvalidURL) || errors.Is(err, httpc.ErrPrivateTarget) || errors.Is(err, httpc.ErrResolveHost) {
			return "", nil, err
		}
		return "", nil, &upstreamFetchError{err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", nil, &upstreamFetchError{statusCode: resp.StatusCode}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, &upstreamFetchError{err: err}
	}

	finalURL := rawURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	return finalURL, body, nil
}

func probeFeedCandidates(ctx context.Context, client *http.Client, candidates []string, allowPrivate bool) []discoveredFeed {
	if len(candidates) == 0 {
		return nil
	}

	results := make([]*discoveredFeed, len(candidates))
	sem := make(chan struct{}, discoveryProbeWorkers)
	var wg sync.WaitGroup

	for i, candidate := range candidates {
		wg.Add(1)
		go func(idx int, candidateURL string) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			finalURL, body, err := fetchDiscoveryDocument(ctx, client, candidateURL, allowPrivate)
			if err != nil {
				return
			}
			parsedFeed, err := gofeed.NewParser().Parse(bytes.NewReader(body))
			if err != nil || parsedFeed == nil {
				return
			}
			results[idx] = &discoveredFeed{
				Title: parsedFeed.Title,
				Link:  finalURL,
			}
		}(i, candidate)
	}
	wg.Wait()

	feeds := make([]discoveredFeed, 0, len(results))
	for _, item := range results {
		if item != nil {
			feeds = append(feeds, *item)
		}
	}
	return feeds
}

func matchDirectServiceFeeds(u *url.URL) []discoveredFeed {
	if u == nil {
		return nil
	}
	host := strings.ToLower(u.Hostname())

	switch host {
	case "github.com", "www.github.com":
		trimmedPath := strings.Trim(u.Path, "/")
		if trimmedPath == "" {
			return nil
		}
		parts := strings.Split(trimmedPath, "/")
		if _, reserved := githubReservedPaths[strings.ToLower(parts[0])]; reserved {
			return nil
		}
		last := strings.ToLower(parts[len(parts)-1])
		if strings.HasSuffix(last, ".atom") || strings.HasSuffix(last, ".rss") || strings.HasSuffix(last, ".xml") {
			return nil
		}
		if len(parts) == 1 {
			return []discoveredFeed{{
				Title: parts[0],
				Link:  fmt.Sprintf("https://github.com/%s.atom", url.PathEscape(parts[0])),
			}}
		}
		owner, repo := url.PathEscape(parts[0]), url.PathEscape(parts[1])
		base := fmt.Sprintf("https://github.com/%s/%s", owner, repo)
		return []discoveredFeed{
			{Title: parts[1] + " releases", Link: base + "/releases.atom"},
			{Title: parts[1] + " commits", Link: base + "/commits.atom"},
			{Title: parts[1] + " tags", Link: base + "/tags.atom"},
		}

	case "reddit.com", "www.reddit.com", "old.reddit.com", "np.reddit.com":
		cleanPath := strings.TrimSuffix(u.Path, "/")
		if strings.HasSuffix(strings.ToLower(cleanPath), ".rss") {
			return nil
		}
		return []discoveredFeed{{
			Title: "Reddit",
			Link:  fmt.Sprintf("https://%s%s.rss", host, cleanPath),
		}}

	case "youtube.com", "www.youtube.com", "m.youtube.com":
		if u.Path == "/playlist" {
			if listID := strings.TrimSpace(u.Query().Get("list")); listID != "" {
				return []discoveredFeed{{
					Title: "YouTube Playlist",
					Link:  "https://www.youtube.com/feeds/videos.xml?playlist_id=" + url.QueryEscape(listID),
				}}
			}
		}
		if strings.HasPrefix(u.Path, "/channel/") {
			channelID := strings.Trim(strings.TrimPrefix(u.Path, "/channel/"), "/")
			if channelID != "" && !strings.Contains(channelID, "/") {
				return []discoveredFeed{{
					Title: "YouTube Channel",
					Link:  "https://www.youtube.com/feeds/videos.xml?channel_id=" + url.QueryEscape(channelID),
				}}
			}
		}
	}

	return nil
}

func isYouTubeHost(host string) bool {
	switch strings.ToLower(host) {
	case "youtube.com", "www.youtube.com", "m.youtube.com":
		return true
	default:
		return false
	}
}

func extractYouTubeChannelID(body []byte) string {
	for _, re := range []*regexp.Regexp{youtubeBrowseIDPattern, youtubeExternalIDRegexp, youtubeChannelURLRegexp} {
		if matches := re.FindSubmatch(body); len(matches) > 1 {
			return string(matches[1])
		}
	}
	return ""
}

func extractHTMLTitle(body []byte) string {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(doc.Find("title").First().Text())
}

func extractHTMLFeedCandidates(body []byte, baseURL *url.URL) []string {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil
	}

	seen := make(map[string]struct{})
	var candidates []string
	addCandidate := func(rawHref string) {
		if len(candidates) >= maxDiscoveryHTMLProbes {
			return
		}
		resolved := resolveCandidateURL(baseURL, rawHref)
		if resolved == "" {
			return
		}
		if _, exists := seen[resolved]; exists {
			return
		}
		seen[resolved] = struct{}{}
		candidates = append(candidates, resolved)
	}

	doc.Find("link[href]").Each(func(_ int, sel *goquery.Selection) {
		href, ok := sel.Attr("href")
		if !ok {
			return
		}
		mimeType, _ := sel.Attr("type")
		rel, _ := sel.Attr("rel")
		if isFeedLinkTag(mimeType, rel) {
			addCandidate(href)
		}
	})

	doc.Find("body a[href]").Each(func(_ int, sel *goquery.Selection) {
		text := strings.ToLower(strings.TrimSpace(sel.Text()))
		if !strings.Contains(text, "rss") {
			return
		}
		if href, ok := sel.Attr("href"); ok {
			addCandidate(href)
		}
	})

	return candidates
}

func isFeedLinkTag(mimeType, rel string) bool {
	mediaType := strings.TrimSpace(strings.ToLower(strings.Split(mimeType, ";")[0]))
	switch mediaType {
	case "application/rss+xml", "application/atom+xml", "application/feed+json":
		return true
	case "application/json", "application/xml", "text/xml":
		for _, token := range strings.Fields(strings.ToLower(rel)) {
			if token == "alternate" {
				return true
			}
		}
	}
	return false
}

func buildWellKnownCandidates(baseURL *url.URL) []string {
	if baseURL == nil {
		return nil
	}

	baseStr := baseURL.String()
	seen := map[string]struct{}{baseStr: {}}
	candidates := make([]string, 0, len(wellKnownFeedPaths)*2)

	var bases []*url.URL
	dirURL := *baseURL
	if baseURL.Path == "" || strings.HasSuffix(baseURL.Path, "/") {
		dirURL.Path = baseURL.Path
		if dirURL.Path == "" {
			dirURL.Path = "/"
		}
	} else {
		subDirURL := *baseURL
		subDirURL.Path = strings.TrimSuffix(baseURL.Path, "/") + "/"
		subDirURL.RawQuery = ""
		subDirURL.Fragment = ""
		bases = append(bases, &subDirURL)

		dir := path.Dir(baseURL.Path)
		if !strings.HasSuffix(dir, "/") {
			dir += "/"
		}
		dirURL.Path = dir
	}
	dirURL.RawQuery = ""
	dirURL.Fragment = ""
	bases = append(bases, &dirURL)

	rootURL := &url.URL{Scheme: baseURL.Scheme, Host: baseURL.Host, Path: "/"}
	bases = append(bases, rootURL)

	for _, b := range bases {
		for _, suffix := range wellKnownFeedPaths {
			ref, err := url.Parse(suffix)
			if err != nil {
				continue
			}
			resolved := b.ResolveReference(ref).String()
			if _, exists := seen[resolved]; exists {
				continue
			}
			seen[resolved] = struct{}{}
			candidates = append(candidates, resolved)
		}
	}

	return candidates
}

func resolveCandidateURL(baseURL *url.URL, rawHref string) string {
	href := strings.TrimSpace(rawHref)
	if href == "" {
		return ""
	}
	parsed, err := url.Parse(href)
	if err != nil {
		return ""
	}
	if baseURL != nil {
		parsed = baseURL.ResolveReference(parsed)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	return parsed.String()
}

func filterAndNormalizeDiscoveredFeeds(ctx context.Context, found []discoveredFeed, allowPrivate bool) []discoveredFeed {
	normalized := normalizeDiscoveredFeeds(found)
	if allowPrivate {
		return normalized
	}

	filtered := make([]discoveredFeed, 0, len(normalized))
	for _, feed := range normalized {
		if err := httpc.ValidateRequestURL(ctx, feed.Link, false); err == nil {
			filtered = append(filtered, feed)
		}
	}
	return filtered
}
