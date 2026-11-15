package translator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolveCompletionsURL(t *testing.T) {
	tests := []struct {
		raw     string
		want    string
		wantErr bool
	}{
		{"", "https://api.deepseek.com/v1/chat/completions", false},
		{"https://api.deepseek.com", "https://api.deepseek.com/v1/chat/completions", false},
		{"https://api.deepseek.com/", "https://api.deepseek.com/v1/chat/completions", false},
		{"https://api.deepseek.com/v1", "https://api.deepseek.com/v1/chat/completions", false},
		{"https://api.deepseek.com/v1/", "https://api.deepseek.com/v1/chat/completions", false},
		{"https://api.deepseek.com/chat/completions", "https://api.deepseek.com/chat/completions", false},
		{"https://ark.cn-beijing.volces.com/api/v3", "https://ark.cn-beijing.volces.com/api/v3/chat/completions", false},
		{"http://127.0.0.1:8080/proxy", "http://127.0.0.1:8080/proxy/v1/chat/completions", false},
		{"ftp://example.com", "", true},
		{"not a url", "", true},
	}

	for _, tt := range tests {
		got, err := ResolveCompletionsURL(tt.raw)
		if (err != nil) != tt.wantErr {
			t.Errorf("ResolveCompletionsURL(%q) err=%v wantErr=%v", tt.raw, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("ResolveCompletionsURL(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestBuildBatchesAndHash(t *testing.T) {
	blocks := []Segment{
		{ID: 0, Text: strings.Repeat("a", 60)},
		{ID: 1, Text: strings.Repeat("b", 60)},
		{ID: 2, Text: strings.Repeat("c", 60)},
	}
	batches := BuildBatches("Hello Title", blocks, 100, 2)
	if len(batches) != 4 {
		t.Fatalf("expected 4 batches with maxRunes=100, got %d (%+v)", len(batches), batches)
	}
	if batches[0][0].key != "title" {
		t.Errorf("expected first item to be title, got %q", batches[0][0].key)
	}

	h1 := ComputeContentHash("Title", blocks)
	h2 := ComputeContentHash("Title", blocks)
	h3 := ComputeContentHash("Changed", blocks)
	if h1 != h2 {
		t.Fatalf("expected deterministic hash")
	}
	if h1 == h3 {
		t.Fatalf("expected different hash when title changes")
	}
}

func TestTranslateArticleStreamingAndMultiBatch(t *testing.T) {
	var requestCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test-key" {
			t.Fatalf("unexpected Authorization header: %q", got)
		}

		var body chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Model != "deepseek-flash" {
			t.Fatalf("expected default model deepseek-flash, got %q", body.Model)
		}

		requestCount++
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		chunks := []string{
			"[[title]]\n中文",
			"标题\n[[0]]\n第一段包含 <a0>链接</a0> 与 ",
			"<code>zar</code>。\n[[1]]\n第二段。\n",
		}
		for _, c := range chunks {
			payload, _ := json.Marshal(map[string]any{
				"choices": []map[string]any{
					{"delta": map[string]string{"content": c}},
				},
			})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			if flusher != nil {
				flusher.Flush()
			}
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	client := NewClient()
	var seen []string
	res, err := client.TranslateArticle(
		context.Background(),
		Config{APIURL: srv.URL, APIKey: "sk-test-key"},
		"English Title",
		[]Segment{
			{ID: 0, Text: "First paragraph with <a0>link</a0> and <code>zar</code>."},
			{ID: 1, Text: "Second paragraph."},
		},
		func(key, text string) error {
			seen = append(seen, key+"="+text)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("TranslateArticle failed: %v", err)
	}
	if requestCount != 1 {
		t.Fatalf("expected 1 batch request, got %d", requestCount)
	}
	if res.TranslatedTitle != "中文标题" {
		t.Errorf("TranslatedTitle = %q, want %q", res.TranslatedTitle, "中文标题")
	}
	if len(res.Blocks) != 2 || res.Blocks[0].Text != "第一段包含 <a0>链接</a0> 与 <code>zar</code>。" || res.Blocks[1].Text != "第二段。" {
		t.Errorf("unexpected blocks: %+v", res.Blocks)
	}
	if len(seen) != 3 {
		t.Errorf("expected 3 streamed callbacks, got %v", seen)
	}
}

func TestTranslateArticleErrorAndMissingKey(t *testing.T) {
	client := NewClient()
	if _, err := client.TranslateArticle(context.Background(), Config{}, "Title", nil, nil); !errors.Is(err, ErrMissingAPIKey) {
		t.Fatalf("expected ErrMissingAPIKey, got %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer srv.Close()

	if err := client.TestConnection(context.Background(), Config{APIURL: srv.URL, APIKey: "bad"}); err == nil || !strings.Contains(err.Error(), "invalid api key") {
		t.Fatalf("expected upstream message in error, got %v", err)
	}
}

func TestSummarizeArticleStreaming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk-sum-key" {
			t.Fatalf("unexpected Authorization header: %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		for _, chunk := range []string{"本文总结了核心技术方案。", "\n\n- 关键点 1", "\n- 关键点 2"} {
			payload, _ := json.Marshal(map[string]any{
				"choices": []map[string]any{{"delta": map[string]string{"content": chunk}}},
			})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			if flusher != nil {
				flusher.Flush()
			}
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	client := NewClient()
	var deltas strings.Builder
	summary, modelName, err := client.SummarizeArticle(
		context.Background(),
		Config{APIURL: srv.URL, APIKey: "sk-sum-key"},
		"Edge Caching",
		"Full article text explaining edge caching and origin shields.",
		func(delta string) error {
			deltas.WriteString(delta)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("SummarizeArticle failed: %v", err)
	}
	if modelName != DefaultModel {
		t.Fatalf("modelName = %q, want %q", modelName, DefaultModel)
	}
	want := "本文总结了核心技术方案。\n\n- 关键点 1\n- 关键点 2"
	if summary != want || deltas.String() != want {
		t.Fatalf("unexpected summary=%q deltas=%q", summary, deltas.String())
	}
	if ComputeSummaryHash("A", "B") == ComputeSummaryHash("A", "C") {
		t.Fatalf("expected distinct summary hashes when body changes")
	}
}
