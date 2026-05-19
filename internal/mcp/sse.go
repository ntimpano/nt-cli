package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// RunSSE starts the MCP server over SSE transport.
func (s *Server) RunSSE(port, bind string) error {
	if strings.TrimSpace(port) == "" {
		port = "7878"
	}
	if strings.TrimSpace(bind) == "" {
		bind = "127.0.0.1"
	}

	if _, _, err := s.ensureService(); err != nil {
		return err
	}

	addr := fmt.Sprintf("%s:%s", bind, port)
	mux := http.NewServeMux()
	mux.HandleFunc("/sse", s.handleSSE)
	mux.HandleFunc("/message", s.handleMessage)
	return http.ListenAndServe(addr, mux)
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	_, _ = fmt.Fprintf(w, "event: endpoint\ndata: /message\n\n")
	flusher.Flush()

	<-r.Context().Done()
}

func (s *Server) handleMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if s.svc == nil {
		http.Error(w, "service unavailable", http.StatusInternalServerError)
		return
	}

	resp, ok := handleRequest(payload, s.svc)
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, "encode error", http.StatusInternalServerError)
	}
}
