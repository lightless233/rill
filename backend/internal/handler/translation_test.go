package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestTranslationFlowLookupStreamAndCache(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		for _, chunk := range []string{
			"[[title]]\n测试文章",
			"\n[[0]]\n第一段翻译。\n",
		} {
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
	defer upstream.Close()

	r, cookie := newSettingsTestRouter(t)

	// Without configured key, translate returns 400 JSON before opening stream.
	reqBody := `{"title":"Test Article","blocks":[{"id":0,"text":"First paragraph."}]}`
	w := performRequest(r, http.MethodPost, "/api/translations", strings.NewReader(reqBody), nil, cookie)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when API key missing, got %d: %s", w.Code, w.Body.String())
	}

	// Lookup misses cleanly before translation.
	w = performRequest(r, http.MethodPost, "/api/translations/lookup", strings.NewReader(reqBody), nil, cookie)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"found":false`) {
		t.Fatalf("expected cache miss, got %d: %s", w.Code, w.Body.String())
	}

	// Save translation config pointing at mock upstream.
	patchBody := fmt.Sprintf(`{"translation_api_url":%q,"translation_api_key":"sk-mock-key","translation_model":"deepseek-chat"}`, upstream.URL)
	w = performRequest(r, http.MethodPatch, "/api/settings", strings.NewReader(patchBody), nil, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH settings failed: %d: %s", w.Code, w.Body.String())
	}

	// Stream translation from mock upstream.
	w = performRequest(r, http.MethodPost, "/api/translations", strings.NewReader(reqBody), nil, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/translations failed: %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "测试文章") || !strings.Contains(w.Body.String(), "第一段翻译。") || !strings.Contains(w.Body.String(), "event: done") {
		t.Fatalf("unexpected SSE stream body: %s", w.Body.String())
	}
	if upstreamCalls.Load() != 1 {
		t.Fatalf("expected 1 upstream call, got %d", upstreamCalls.Load())
	}

	// Lookup now hits cache and second translate hits cache without upstream call.
	w = performRequest(r, http.MethodPost, "/api/translations/lookup", strings.NewReader(reqBody), nil, cookie)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"found":true`) || !strings.Contains(w.Body.String(), "第一段翻译。") {
		t.Fatalf("expected cache hit on lookup, got %d: %s", w.Code, w.Body.String())
	}

	w = performRequest(r, http.MethodPost, "/api/translations", strings.NewReader(reqBody), nil, cookie)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"cached":true`) {
		t.Fatalf("expected cached stream replay, got %d: %s", w.Code, w.Body.String())
	}
	if upstreamCalls.Load() != 1 {
		t.Fatalf("expected cache replay to avoid upstream, got %d calls", upstreamCalls.Load())
	}

	// force=true bypasses cache and re-queries upstream.
	forceBody := `{"title":"Test Article","blocks":[{"id":0,"text":"First paragraph."}],"force":true}`
	w = performRequest(r, http.MethodPost, "/api/translations", strings.NewReader(forceBody), nil, cookie)
	if w.Code != http.StatusOK || upstreamCalls.Load() != 2 {
		t.Fatalf("expected force=true to trigger upstream call 2, code=%d calls=%d", w.Code, upstreamCalls.Load())
	}
}

func TestTranslationTestEndpoint(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-override" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"wrong key"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"OK"}}]}`))
	}))
	defer upstream.Close()

	r, cookie := newSettingsTestRouter(t)

	// Unsaved override key works directly.
	body := fmt.Sprintf(`{"api_url":%q,"api_key":"sk-override","model":"deepseek-chat"}`, upstream.URL)
	w := performRequest(r, http.MethodPost, "/api/translations/test", strings.NewReader(body), nil, cookie)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("expected test connection 200, got %d: %s", w.Code, w.Body.String())
	}

	// Wrong key returns 502 with upstream error message.
	badBody := fmt.Sprintf(`{"api_url":%q,"api_key":"sk-bad"}`, upstream.URL)
	w = performRequest(r, http.MethodPost, "/api/translations/test", strings.NewReader(badBody), nil, cookie)
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "wrong key") {
		t.Fatalf("expected 502 with upstream error, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSummaryFlowLookupStreamAndCache(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		for _, chunk := range []string{"核心结论摘要。", "\n\n- 第一点\n- 第二点"} {
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
	defer upstream.Close()

	r, cookie := newSettingsTestRouter(t)
	reqBody := `{"title":"Edge Article","content":"Edge caching reduces origin load significantly."}`

	// Without configured key, summarize returns 400 JSON before SSE stream begins.
	w := performRequest(r, http.MethodPost, "/api/summaries", strings.NewReader(reqBody), nil, cookie)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when API key missing, got %d: %s", w.Code, w.Body.String())
	}

	// Lookup misses cleanly before summary exists.
	w = performRequest(r, http.MethodPost, "/api/summaries/lookup", strings.NewReader(reqBody), nil, cookie)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"found":false`) {
		t.Fatalf("expected cache miss, got %d: %s", w.Code, w.Body.String())
	}

	patchBody := fmt.Sprintf(`{"translation_api_url":%q,"translation_api_key":"sk-mock-key","translation_model":"deepseek-chat"}`, upstream.URL)
	w = performRequest(r, http.MethodPatch, "/api/settings", strings.NewReader(patchBody), nil, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH settings failed: %d: %s", w.Code, w.Body.String())
	}

	// First summary call streams from upstream and caches result.
	w = performRequest(r, http.MethodPost, "/api/summaries", strings.NewReader(reqBody), nil, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/summaries failed: %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "核心结论摘要。") || !strings.Contains(w.Body.String(), "event: delta") || !strings.Contains(w.Body.String(), "event: done") {
		t.Fatalf("unexpected summary SSE body: %s", w.Body.String())
	}
	if upstreamCalls.Load() != 1 {
		t.Fatalf("expected 1 upstream call, got %d", upstreamCalls.Load())
	}

	// Lookup now hits cache, and second summarize replays cached summary without upstream call.
	w = performRequest(r, http.MethodPost, "/api/summaries/lookup", strings.NewReader(reqBody), nil, cookie)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"found":true`) || !strings.Contains(w.Body.String(), "核心结论摘要。") {
		t.Fatalf("expected lookup hit, got %d: %s", w.Code, w.Body.String())
	}

	w = performRequest(r, http.MethodPost, "/api/summaries", strings.NewReader(reqBody), nil, cookie)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"cached":true`) || upstreamCalls.Load() != 1 {
		t.Fatalf("expected cached replay without upstream, code=%d calls=%d body=%s", w.Code, upstreamCalls.Load(), w.Body.String())
	}

	// force=true bypasses cache.
	forceBody := `{"title":"Edge Article","content":"Edge caching reduces origin load significantly.","force":true}`
	w = performRequest(r, http.MethodPost, "/api/summaries", strings.NewReader(forceBody), nil, cookie)
	if w.Code != http.StatusOK || upstreamCalls.Load() != 2 {
		t.Fatalf("expected force=true to trigger upstream call 2, code=%d calls=%d", w.Code, upstreamCalls.Load())
	}
}

func TestIncompleteTranslationAndTruncatedSummaryNotCached(t *testing.T) {
	var mode atomic.Int32 // 0 = missing segment, 1 = finish_reason length, 2 = complete
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		switch mode.Load() {
		case 0:
			payload, _ := json.Marshal(map[string]any{
				"choices": []map[string]any{{
					"delta":         map[string]string{"content": "[[title]]\n标题\n[[0]]\n第一段\n"},
					"finish_reason": "stop",
				}},
			})
			_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", payload)
		case 1:
			chunk1, _ := json.Marshal(map[string]any{
				"choices": []map[string]any{{"delta": map[string]string{"content": "截断摘要"}}},
			})
			chunk2, _ := json.Marshal(map[string]any{
				"choices": []map[string]any{{"delta": map[string]string{}, "finish_reason": "length"}},
			})
			_, _ = fmt.Fprintf(w, "data: %s\n\ndata: %s\n\ndata: [DONE]\n\n", chunk1, chunk2)
		default:
			payload, _ := json.Marshal(map[string]any{
				"choices": []map[string]any{{
					"delta":         map[string]string{"content": "[[title]]\n标题\n[[0]]\n第一段\n[[1]]\n第二段\n"},
					"finish_reason": "stop",
				}},
			})
			_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", payload)
		}
	}))
	defer upstream.Close()

	r, cookie := newSettingsTestRouter(t)
	patchBody := fmt.Sprintf(`{"translation_api_url":%q,"translation_api_key":"sk-mock-key"}`, upstream.URL)
	w := performRequest(r, http.MethodPatch, "/api/settings", strings.NewReader(patchBody), nil, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH settings failed: %d: %s", w.Code, w.Body.String())
	}

	transReq := `{"title":"Title","blocks":[{"id":0,"text":"First paragraph."},{"id":1,"text":"Second paragraph."}]}`
	mode.Store(0)
	w = performRequest(r, http.MethodPost, "/api/translations", strings.NewReader(transReq), nil, cookie)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "event: error") || strings.Contains(w.Body.String(), "event: done") {
		t.Fatalf("expected error SSE without done on missing segment, got %d: %s", w.Code, w.Body.String())
	}

	w = performRequest(r, http.MethodPost, "/api/translations/lookup", strings.NewReader(transReq), nil, cookie)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"found":false`) {
		t.Fatalf("expected incomplete translation not to be cached, got %d: %s", w.Code, w.Body.String())
	}

	// Retry once upstream returns all segments -> succeeds and enters cache.
	mode.Store(2)
	w = performRequest(r, http.MethodPost, "/api/translations", strings.NewReader(transReq), nil, cookie)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "event: done") {
		t.Fatalf("expected complete translation to succeed, got %d: %s", w.Code, w.Body.String())
	}
	w = performRequest(r, http.MethodPost, "/api/translations/lookup", strings.NewReader(transReq), nil, cookie)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"found":true`) || !strings.Contains(w.Body.String(), "第二段") {
		t.Fatalf("expected complete translation cached, got %d: %s", w.Code, w.Body.String())
	}

	// Summary truncation via finish_reason: length -> SSE error and no cache entry.
	sumReq := `{"title":"Long Article","content":"Lots of technical content."}`
	mode.Store(1)
	w = performRequest(r, http.MethodPost, "/api/summaries", strings.NewReader(sumReq), nil, cookie)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "event: error") || strings.Contains(w.Body.String(), "event: done") {
		t.Fatalf("expected error SSE without done on truncated summary, got %d: %s", w.Code, w.Body.String())
	}
	w = performRequest(r, http.MethodPost, "/api/summaries/lookup", strings.NewReader(sumReq), nil, cookie)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"found":false`) {
		t.Fatalf("expected truncated summary not to be cached, got %d: %s", w.Code, w.Body.String())
	}
}
