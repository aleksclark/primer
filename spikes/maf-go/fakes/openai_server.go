// Package fakes provides deterministic scripted servers for the MAF spike.
package fakes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

// OpenAIScript is one scripted Chat Completions response.
// If Stream is true, body is written as SSE chunks of content tokens.
type OpenAIScript struct {
	// Content is assistant text (non-tool).
	Content string
	// ToolName + ToolArgs emit a tool_calls finish.
	ToolName string
	ToolArgs string
	// Stream splits Content into token chunks when no tool call.
	Stream bool
	// DelayChunks pauses between stream chunks (optional).
	// (tests control via short content)
}

// OpenAIServer is a minimal OpenAI-compatible Chat Completions endpoint.
type OpenAIServer struct {
	mu      sync.Mutex
	scripts []OpenAIScript
	calls   int
	// LastAuth records Authorization header (should be dummy).
	LastAuth string
	// LastModel records requested model.
	LastModel string
	Server    *httptest.Server
}

// NewOpenAIServer starts an httptest with sequential scripts.
func NewOpenAIServer(scripts ...OpenAIScript) *OpenAIServer {
	o := &OpenAIServer{scripts: scripts}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", o.handleChat)
	mux.HandleFunc("/chat/completions", o.handleChat)
	o.Server = httptest.NewServer(mux)
	return o
}

// URL is the base URL for openai.NewClient(option.WithBaseURL(...)).
// openai-go appends /chat/completions; WithBaseURL should include /v1.
func (o *OpenAIServer) URL() string {
	return o.Server.URL + "/v1"
}

func (o *OpenAIServer) Close() { o.Server.Close() }

func (o *OpenAIServer) Calls() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.calls
}

func (o *OpenAIServer) handleChat(w http.ResponseWriter, r *http.Request) {
	o.mu.Lock()
	o.calls++
	o.LastAuth = r.Header.Get("Authorization")
	idx := o.calls - 1
	var script OpenAIScript
	if idx < len(o.scripts) {
		script = o.scripts[idx]
	} else if len(o.scripts) > 0 {
		script = o.scripts[len(o.scripts)-1]
	} else {
		script = OpenAIScript{Content: "ok"}
	}
	o.mu.Unlock()

	var req struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	o.mu.Lock()
	o.LastModel = req.Model
	o.mu.Unlock()

	stream := req.Stream || script.Stream
	if script.ToolName != "" {
		// Non-streaming tool call response is simplest for MAF toolautocall.
		writeToolCall(w, script.ToolName, script.ToolArgs, stream)
		return
	}
	if stream {
		writeTextStream(w, script.Content)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{
  "id":"chatcmpl-fake",
  "object":"chat.completion",
  "choices":[{"index":0,"message":{"role":"assistant","content":%s},"finish_reason":"stop"}],
  "usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
}`, jsonString(script.Content))
}

func writeTextStream(w http.ResponseWriter, content string) {
	w.Header().Set("Content-Type", "text/event-stream")
	flusher, _ := w.(http.Flusher)
	id := "chatcmpl-fake"
	writeChunk := func(delta string, finish string) {
		var fr any = nil
		finishJSON := "null"
		if finish != "" {
			finishJSON = fmt.Sprintf("%q", finish)
			_ = fr
		}
		payload := fmt.Sprintf(
			`{"id":%q,"object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":%s},"finish_reason":%s}]}`,
			id, jsonString(delta), finishJSON,
		)
		fmt.Fprintf(w, "data: %s\n\n", payload)
		if flusher != nil {
			flusher.Flush()
		}
	}
	// role chunk
	fmt.Fprintf(w, "data: {\"id\":%q,\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\"},\"finish_reason\":null}]}\n\n", id)
	if flusher != nil {
		flusher.Flush()
	}
	// split content into small tokens
	tokens := tokenize(content)
	if len(tokens) == 0 {
		tokens = []string{""}
	}
	for _, tok := range tokens {
		writeChunk(tok, "")
	}
	writeChunk("", "stop")
	fmt.Fprintf(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

func writeToolCall(w http.ResponseWriter, name, args string, stream bool) {
	if args == "" {
		args = "{}"
	}
	if !stream {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
  "id":"chatcmpl-tool",
  "object":"chat.completion",
  "choices":[{
    "index":0,
    "message":{
      "role":"assistant",
      "content":null,
      "tool_calls":[{
        "id":"call_1",
        "type":"function",
        "function":{"name":%s,"arguments":%s}
      }]
    },
    "finish_reason":"tool_calls"
  }]
}`, jsonString(name), jsonString(args))
		return
	}
	// streaming tool call
	w.Header().Set("Content-Type", "text/event-stream")
	flusher, _ := w.(http.Flusher)
	id := "chatcmpl-tool"
	chunks := []string{
		fmt.Sprintf(`{"id":%q,"object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":%s,"arguments":""}}]},"finish_reason":null}]}`, id, jsonString(name)),
		fmt.Sprintf(`{"id":%q,"object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":%s}}]},"finish_reason":null}]}`, id, jsonString(args)),
		fmt.Sprintf(`{"id":%q,"object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`, id),
	}
	for _, c := range chunks {
		fmt.Fprintf(w, "data: %s\n\n", c)
		if flusher != nil {
			flusher.Flush()
		}
	}
	fmt.Fprintf(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func tokenize(s string) []string {
	if s == "" {
		return nil
	}
	// Prefer space-preserving splits for readable stream tests.
	parts := strings.Split(s, " ")
	out := make([]string, 0, len(parts))
	for i, p := range parts {
		if i > 0 {
			out = append(out, " ")
		}
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
