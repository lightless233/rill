package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
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

func TestListTranslationFlowStreamAndCache(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		for _, chunk := range []string{
			"[[0]]\n标题一",
			"\n[[1]]\n摘要一",
			"\n[[2]]\n标题二",
			"\n[[3]]\n摘要二\n",
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
	body := `{"items":[{"item_id":-1,"title":"Title one","summary":"Summary one"},{"item_id":2,"title":"Title two","summary":"Summary two"}]}`

	// Missing key is rejected before the stream begins.
	w := performRequest(r, http.MethodPost, "/api/translations/items", strings.NewReader(body), nil, cookie)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without API key, got %d: %s", w.Code, w.Body.String())
	}

	patchBody := fmt.Sprintf(`{"translation_api_url":%q,"translation_api_key":"sk-mock-key","translation_model":"deepseek-chat"}`, upstream.URL)
	w = performRequest(r, http.MethodPatch, "/api/settings", strings.NewReader(patchBody), nil, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH settings failed: %d: %s", w.Code, w.Body.String())
	}

	w = performRequest(r, http.MethodPost, "/api/translations/items", strings.NewReader(body), nil, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/translations/items failed: %d: %s", w.Code, w.Body.String())
	}
	streamBody := w.Body.String()
	for _, want := range []string{
		"event: meta",
		`"item_id":-1`,
		`"translated_title":"标题一"`,
		`"translated_summary":"摘要一"`,
		`"cached":false`,
		"event: done",
	} {
		if !strings.Contains(streamBody, want) {
			t.Fatalf("expected stream to contain %q, got: %s", want, streamBody)
		}
	}
	if upstreamCalls.Load() != 1 {
		t.Fatalf("expected one upstream call, got %d", upstreamCalls.Load())
	}

	// The second request is served from the translation cache.
	w = performRequest(r, http.MethodPost, "/api/translations/items", strings.NewReader(body), nil, cookie)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"cached":true`) || !strings.Contains(w.Body.String(), "event: done") {
		t.Fatalf("expected cached replay with done, got %d: %s", w.Code, w.Body.String())
	}
	if upstreamCalls.Load() != 1 {
		t.Fatalf("expected cache hit to avoid upstream, got %d calls", upstreamCalls.Load())
	}

	// force bypasses the cache.
	forceBody := strings.Replace(body, "{", `{"force":true,`, 1)
	w = performRequest(r, http.MethodPost, "/api/translations/items", strings.NewReader(forceBody), nil, cookie)
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), `"cached":true`) {
		t.Fatalf("expected force request to bypass cache, got %d: %s", w.Code, w.Body.String())
	}
	if upstreamCalls.Load() != 2 {
		t.Fatalf("expected force to call upstream again, got %d calls", upstreamCalls.Load())
	}
}

func TestListTranslationValidation(t *testing.T) {
	r, cookie := newSettingsTestRouter(t)

	tooMany := make([]map[string]any, maxListTranslationItems+1)
	for i := range tooMany {
		tooMany[i] = map[string]any{"item_id": i + 1, "title": "Title"}
	}

	longTitle := strings.Repeat("a", maxListTranslationTitleRunes+1)
	cases := []struct {
		name string
		body any
	}{
		{"empty items", map[string]any{"items": []any{}}},
		{"duplicate item id", map[string]any{"items": []map[string]any{
			{"item_id": 1, "title": "One"},
			{"item_id": 1, "title": "Two"},
		}}},
		{"empty item", map[string]any{"items": []map[string]any{{"item_id": 1}}}},
		{"too many items", map[string]any{"items": tooMany}},
		{"title too long", map[string]any{"items": []map[string]any{{"item_id": 1, "title": longTitle}}}},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			w := performRequest(r, http.MethodPost, "/api/translations/items", mustJSONBody(t, tt.body), nil, cookie)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestListTranslationPartialFailureDoesNotEmitDone(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		payload, _ := json.Marshal(map[string]any{
			"choices": []map[string]any{{"delta": map[string]string{"content": "[[0]]\n标题一\n"}}},
		})
		_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", payload)
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	r, cookie := newSettingsTestRouter(t)
	patchBody := fmt.Sprintf(`{"translation_api_url":%q,"translation_api_key":"sk-mock-key","translation_model":"deepseek-chat"}`, upstream.URL)
	w := performRequest(r, http.MethodPatch, "/api/settings", strings.NewReader(patchBody), nil, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH settings failed: %d: %s", w.Code, w.Body.String())
	}

	body := `{"items":[{"item_id":1,"title":"Title one"},{"item_id":2,"title":"Title two"}]}`
	w = performRequest(r, http.MethodPost, "/api/translations/items", strings.NewReader(body), nil, cookie)
	streamBody := w.Body.String()
	if !strings.Contains(streamBody, `"item_id":1`) {
		t.Fatalf("expected the completed item to be emitted, got: %s", streamBody)
	}
	if strings.Contains(streamBody, "event: done") {
		t.Fatalf("partial failure must not emit done, got: %s", streamBody)
	}
	if !strings.Contains(streamBody, "event: error") {
		t.Fatalf("expected an error event, got: %s", streamBody)
	}
}

func TestArticleBlockTranslationFlowAndCache(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		userPrompt := ""
		if messages, ok := body["messages"].([]any); ok && len(messages) > 0 {
			if message, ok := messages[len(messages)-1].(map[string]any); ok {
				userPrompt, _ = message["content"].(string)
			}
		}
		ids := []string{}
		for _, match := range regexp.MustCompile(`\[\[(title|\d+)\]\]`).FindAllStringSubmatch(userPrompt, -1) {
			ids = append(ids, match[1])
		}

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		var chunks []string
		for _, id := range ids {
			chunks = append(chunks, "[["+id+"]]\n译文"+id+"\n")
		}
		payload, _ := json.Marshal(map[string]any{
			"choices": []map[string]any{{"delta": map[string]string{"content": strings.Join(chunks, "")}}},
		})
		_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", payload)
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	r, cookie := newSettingsTestRouter(t)
	body := `{"title":"Article title","blocks":[{"id":0,"text":"First block."},{"id":2,"text":"Second block."}],"translate_title":true}`

	w := performRequest(r, http.MethodPost, "/api/translations/blocks", strings.NewReader(body), nil, cookie)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without API key, got %d: %s", w.Code, w.Body.String())
	}

	patchBody := fmt.Sprintf(`{"translation_api_url":%q,"translation_api_key":"sk-mock-key","translation_model":"deepseek-chat"}`, upstream.URL)
	w = performRequest(r, http.MethodPatch, "/api/settings", strings.NewReader(patchBody), nil, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH settings failed: %d: %s", w.Code, w.Body.String())
	}

	w = performRequest(r, http.MethodPost, "/api/translations/blocks", strings.NewReader(body), nil, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/translations/blocks failed: %d: %s", w.Code, w.Body.String())
	}
	streamBody := w.Body.String()
	for _, want := range []string{
		"event: meta",
		`"key":"title"`,
		`"key":"0"`,
		`"key":"2"`,
		"event: done",
	} {
		if !strings.Contains(streamBody, want) {
			t.Fatalf("expected stream to contain %q, got: %s", want, streamBody)
		}
	}
	if upstreamCalls.Load() != 1 {
		t.Fatalf("expected one upstream call, got %d", upstreamCalls.Load())
	}

	// Repeating the same blocks and title is served entirely from the block cache.
	w = performRequest(r, http.MethodPost, "/api/translations/blocks", strings.NewReader(body), nil, cookie)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "event: done") {
		t.Fatalf("expected cached replay, got %d: %s", w.Code, w.Body.String())
	}
	if upstreamCalls.Load() != 1 {
		t.Fatalf("expected cache hit to avoid upstream, got %d calls", upstreamCalls.Load())
	}

	// force bypasses cached block/title entries.
	forceBody := strings.Replace(body, "{", `{"force":true,`, 1)
	w = performRequest(r, http.MethodPost, "/api/translations/blocks", strings.NewReader(forceBody), nil, cookie)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "event: done") {
		t.Fatalf("expected force translation, got %d: %s", w.Code, w.Body.String())
	}
	if upstreamCalls.Load() != 2 {
		t.Fatalf("expected force to call upstream again, got %d calls", upstreamCalls.Load())
	}
}

func TestArticleBlockTranslationValidation(t *testing.T) {
	r, cookie := newSettingsTestRouter(t)
	cases := []struct {
		name string
		body string
	}{
		{"empty request", `{"blocks":[]}`},
		{"duplicate block", `{"blocks":[{"id":1,"text":"a"},{"id":1,"text":"b"}]}`},
		{"negative block", `{"blocks":[{"id":-1,"text":"a"}]}`},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			w := performRequest(r, http.MethodPost, "/api/translations/blocks", strings.NewReader(tt.body), nil, cookie)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}
