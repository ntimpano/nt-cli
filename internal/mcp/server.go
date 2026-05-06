package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"nt-cli/internal/app"
	"nt-cli/internal/store"
)

type Server struct {
	in  io.Reader
	out io.Writer
}

type transportMode int

const (
	transportFramed transportMode = iota
	transportJSONStream
)

func NewServer(in io.Reader, out io.Writer) *Server {
	return &Server{in: in, out: out}
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolsCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type localSaveArgs struct {
	Content  string `json:"content"`
	Title    string `json:"title,omitempty"`
	Type     string `json:"type,omitempty"`
	TopicKey string `json:"topic_key,omitempty"`
	Scope    string `json:"scope,omitempty"`
}

type localRecallArgs struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
	// PR2b: optional metadata + date-range filters. Empty/zero values
	// preserve legacy unfiltered behavior so older clients are unaffected.
	Type  string `json:"type,omitempty"`
	Since string `json:"since,omitempty"`
	Until string `json:"until,omitempty"`
}

type localContextArgs struct {
	N     int    `json:"n"`
	Scope string `json:"scope,omitempty"`
}

type localListArgs struct {
	Limit int `json:"limit"`
}

type localDeleteArgs struct {
	ID int64 `json:"id"`
}

type localGetArgs struct {
	ID int64 `json:"id"`
}

type localUpdateArgs struct {
	ID      int64  `json:"id"`
	Content string `json:"content"`
}

type localSessionArgs struct {
	SessionID string `json:"session_id"`
	Summary   string `json:"summary,omitempty"`
}

type localImportArgs struct {
	Path   string `json:"path"`
	DryRun bool   `json:"dry_run,omitempty"`
}

type localPathArgs struct {
	Path string `json:"path"`
}

type initializeParams struct {
	ProtocolVersion string `json:"protocolVersion"`
}

func (s *Server) Run() error {
	debugPath := os.Getenv("NTCLI_MCP_DEBUG")
	if debugPath == "" {
		debugPath = "/tmp/nt-cli-mcp.log"
	}
	debugLog(debugPath, "server start")

	dbPath, err := app.DefaultDBPath()
	if err != nil {
		debugLog(debugPath, fmt.Sprintf("db path error: %v", err))
		return err
	}
	repo, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		debugLog(debugPath, fmt.Sprintf("sqlite open error: %v", err))
		return err
	}
	defer repo.Close()

	svc := app.NewService(repo)
	if err := svc.Init(); err != nil {
		debugLog(debugPath, fmt.Sprintf("svc init error: %v", err))
		return err
	}

	r := bufio.NewReader(s.in)
	w := bufio.NewWriter(s.out)

	mode, err := detectTransportMode(r)
	if err != nil {
		debugLog(debugPath, fmt.Sprintf("detect mode error: %v", err))
		return err
	}
	if mode == transportFramed {
		debugLog(debugPath, "transport=framed")
	} else {
		debugLog(debugPath, "transport=json-stream")
	}

	var dec *json.Decoder
	if mode == transportJSONStream {
		dec = json.NewDecoder(r)
	}

	for {
		payload, err := readMessage(r, mode, dec)
		if err != nil {
			if err == io.EOF {
				debugLog(debugPath, "eof")
				return nil
			}
			debugLog(debugPath, fmt.Sprintf("read message error: %v", err))
			return err
		}
		debugLog(debugPath, fmt.Sprintf("recv: %s", truncateForLog(strings.TrimSpace(string(payload)), 400)))

		trimmed := strings.TrimSpace(string(payload))
		if strings.HasPrefix(trimmed, "[") {
			var batch []json.RawMessage
			if err := json.Unmarshal(payload, &batch); err != nil {
				debugLog(debugPath, fmt.Sprintf("batch parse error: %v", err))
				if err := writeMessage(w, mode, response{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: fmt.Sprintf("parse error: %v", err)}}); err != nil {
					debugLog(debugPath, fmt.Sprintf("write error: %v", err))
					return err
				}
				continue
			}
			debugLog(debugPath, fmt.Sprintf("batch size=%d", len(batch)))
			for _, item := range batch {
				resp, ok := handleRequest(item, svc)
				if !ok {
					debugLog(debugPath, "skip notification")
					continue
				}
				b, _ := json.Marshal(resp)
				debugLog(debugPath, fmt.Sprintf("send: %s", truncateForLog(string(b), 400)))
				if err := writeMessage(w, mode, resp); err != nil {
					debugLog(debugPath, fmt.Sprintf("write error: %v", err))
					return err
				}
			}
			continue
		}

		resp, ok := handleRequest(payload, svc)
		if !ok {
			debugLog(debugPath, "skip notification")
			continue
		}
		b, _ := json.Marshal(resp)
		debugLog(debugPath, fmt.Sprintf("send: %s", truncateForLog(string(b), 400)))
		if err := writeMessage(w, mode, resp); err != nil {
			debugLog(debugPath, fmt.Sprintf("write error: %v", err))
			return err
		}
	}
}

func debugLog(path, msg string) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339Nano), msg)
}

func truncateForLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func handleRequest(payload []byte, svc *app.Service) (response, bool) {
	var req request
	if err := json.Unmarshal(payload, &req); err != nil {
		return response{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: fmt.Sprintf("parse error: %v", err)}}, true
	}

	isNotification := len(req.ID) == 0

	switch req.Method {
		case "initialize":
			if isNotification {
				return response{}, false
			}
			protocolVersion := "2024-11-05"
			if len(req.Params) > 0 {
				var p initializeParams
				if err := json.Unmarshal(req.Params, &p); err == nil && strings.TrimSpace(p.ProtocolVersion) != "" {
					protocolVersion = strings.TrimSpace(p.ProtocolVersion)
				}
			}
			result := map[string]interface{}{
				"protocolVersion": protocolVersion,
				"serverInfo": map[string]interface{}{
					"name":    "nt-cli",
					"version": "0.1.0",
				},
				"capabilities": map[string]interface{}{
					"tools": map[string]interface{}{},
				},
			}
			return response{JSONRPC: "2.0", ID: req.ID, Result: result}, true

		case "notifications/initialized", "$/cancelRequest":
			return response{}, false

		case "resources/list":
			if isNotification {
				return response{}, false
			}
			return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]interface{}{"resources": []interface{}{}}}, true

		case "prompts/list":
			if isNotification {
				return response{}, false
			}
			return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]interface{}{"prompts": []interface{}{}}}, true

		case "tools":
			if isNotification {
				return response{}, false
			}
			return response{JSONRPC: "2.0", ID: req.ID, Result: toolsListResult()}, true

		case "tools/list":
			if isNotification {
				return response{}, false
			}
			return response{JSONRPC: "2.0", ID: req.ID, Result: toolsListResult()}, true

		case "tools/call":
			if isNotification {
				return response{}, false
			}
			var params toolsCallParams
			if err := json.Unmarshal(req.Params, &params); err != nil {
				return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32602, Message: "invalid params"}}, true
			}

			switch params.Name {
			case "local_save":
				var args localSaveArgs
				if err := json.Unmarshal(params.Arguments, &args); err != nil {
					return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32602, Message: "invalid arguments"}}, true
				}
				// Route to SaveWithMeta only when the caller provided at
				// least one metadata field. This preserves the legacy
				// JSON-RPC contract for older clients and keeps test fakes
				// that don't implement MetadataStore working.
				hasMeta := args.Title != "" || args.Type != "" || args.TopicKey != "" || args.Scope != ""
				var (
					id    int64
					saveErr error
				)
				if hasMeta {
					id, saveErr = svc.SaveWithMeta(app.SaveRequest{
						Content:  args.Content,
						Title:    args.Title,
						Type:     args.Type,
						TopicKey: args.TopicKey,
						Scope:    args.Scope,
					})
				} else {
					id, saveErr = svc.Save(args.Content)
				}
				if saveErr != nil {
					return response{JSONRPC: "2.0", ID: req.ID, Result: toolError(saveErr.Error())}, true
				}
				return response{JSONRPC: "2.0", ID: req.ID, Result: toolText(fmt.Sprintf("saved #%d", id))}, true

			case "local_recall":
				var args localRecallArgs
				if err := json.Unmarshal(params.Arguments, &args); err != nil {
					return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32602, Message: "invalid arguments"}}, true
				}
				hasFilter := strings.TrimSpace(args.Type) != "" ||
					strings.TrimSpace(args.Since) != "" ||
					strings.TrimSpace(args.Until) != ""
				var (
					items []app.MemoryItem
					err   error
				)
				if hasFilter {
					opts := app.RecallOptions{
						Query: args.Query,
						Type:  args.Type,
						Limit: args.Limit,
					}
					if args.Since != "" {
						t, perr := parseDateArg(args.Since)
						if perr != nil {
							return response{JSONRPC: "2.0", ID: req.ID, Result: toolError("invalid since: " + perr.Error())}, true
						}
						opts.Since = t
					}
					if args.Until != "" {
						t, perr := parseDateArg(args.Until)
						if perr != nil {
							return response{JSONRPC: "2.0", ID: req.ID, Result: toolError("invalid until: " + perr.Error())}, true
						}
						opts.Until = t
					}
					items, err = svc.RecallWithOptions(opts)
				} else {
					items, err = svc.Recall(args.Query, args.Limit)
				}
				if err != nil {
					return response{JSONRPC: "2.0", ID: req.ID, Result: toolError(err.Error())}, true
				}
				b, _ := json.Marshal(memoryItemsPayload(items))
				return response{JSONRPC: "2.0", ID: req.ID, Result: toolText(string(b))}, true

			case "local_context":
				var args localContextArgs
				_ = json.Unmarshal(params.Arguments, &args)
				items, err := svc.Context(args.N, args.Scope)
				if err != nil {
					return response{JSONRPC: "2.0", ID: req.ID, Result: toolError(err.Error())}, true
				}
				b, _ := json.Marshal(memoryItemsPayload(items))
				return response{JSONRPC: "2.0", ID: req.ID, Result: toolText(string(b))}, true


			case "local_list":
				var args localListArgs
				_ = json.Unmarshal(params.Arguments, &args)
				items, err := svc.List(args.Limit)
				if err != nil {
					return response{JSONRPC: "2.0", ID: req.ID, Result: toolError(err.Error())}, true
				}
				b, _ := json.Marshal(items)
				return response{JSONRPC: "2.0", ID: req.ID, Result: toolText(string(b))}, true

			case "local_delete":
				var args localDeleteArgs
				if err := json.Unmarshal(params.Arguments, &args); err != nil {
					return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32602, Message: "invalid arguments"}}, true
				}
				deleted, err := svc.Delete(args.ID)
				if err != nil {
					return response{JSONRPC: "2.0", ID: req.ID, Result: toolError(err.Error())}, true
				}
				if !deleted {
					return response{JSONRPC: "2.0", ID: req.ID, Result: toolText(fmt.Sprintf("note #%d not found", args.ID))}, true
				}
				return response{JSONRPC: "2.0", ID: req.ID, Result: toolText(fmt.Sprintf("deleted #%d", args.ID))}, true

			case "local_get":
				var args localGetArgs
				if err := json.Unmarshal(params.Arguments, &args); err != nil {
					return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32602, Message: "invalid arguments"}}, true
				}
				item, err := svc.Get(args.ID)
				if err != nil {
					return response{JSONRPC: "2.0", ID: req.ID, Result: toolError(err.Error())}, true
				}
				b, _ := json.Marshal(memoryItemPayload(item))
				return response{JSONRPC: "2.0", ID: req.ID, Result: toolText(string(b))}, true

			case "local_update":
				var args localUpdateArgs
				if err := json.Unmarshal(params.Arguments, &args); err != nil {
					return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32602, Message: "invalid arguments"}}, true
				}
				ok, err := svc.Update(args.ID, args.Content)
				if err != nil {
					return response{JSONRPC: "2.0", ID: req.ID, Result: toolError(err.Error())}, true
				}
				if !ok {
					return response{JSONRPC: "2.0", ID: req.ID, Result: toolError(fmt.Sprintf("note #%d not found", args.ID))}, true
				}
				return response{JSONRPC: "2.0", ID: req.ID, Result: toolText(fmt.Sprintf("updated #%d", args.ID))}, true

			case "local_session_start":
				var args localSessionArgs
				if err := json.Unmarshal(params.Arguments, &args); err != nil {
					return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32602, Message: "invalid arguments"}}, true
				}
				if err := svc.SessionStart(args.SessionID); err != nil {
					return response{JSONRPC: "2.0", ID: req.ID, Result: toolError(err.Error())}, true
				}
				return response{JSONRPC: "2.0", ID: req.ID, Result: toolText(fmt.Sprintf("session started %s", strings.TrimSpace(args.SessionID)))}, true

			case "local_session_end":
				var args localSessionArgs
				if err := json.Unmarshal(params.Arguments, &args); err != nil {
					return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32602, Message: "invalid arguments"}}, true
				}
				if err := svc.SessionEnd(args.SessionID); err != nil {
					return response{JSONRPC: "2.0", ID: req.ID, Result: toolError(err.Error())}, true
				}
				return response{JSONRPC: "2.0", ID: req.ID, Result: toolText(fmt.Sprintf("session ended %s", strings.TrimSpace(args.SessionID)))}, true

			case "local_session_summary":
				var args localSessionArgs
				if err := json.Unmarshal(params.Arguments, &args); err != nil {
					return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32602, Message: "invalid arguments"}}, true
				}
				if err := svc.SessionSummary(args.SessionID, args.Summary); err != nil {
					return response{JSONRPC: "2.0", ID: req.ID, Result: toolError(err.Error())}, true
				}
				return response{JSONRPC: "2.0", ID: req.ID, Result: toolText(fmt.Sprintf("session summary %s", strings.TrimSpace(args.SessionID)))}, true

			case "local_import":
				var args localImportArgs
				if err := json.Unmarshal(params.Arguments, &args); err != nil {
					return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32602, Message: "invalid arguments"}}, true
				}
				path := strings.TrimSpace(args.Path)
				if path == "" {
					return response{JSONRPC: "2.0", ID: req.ID, Result: toolError("path is required")}, true
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return response{JSONRPC: "2.0", ID: req.ID, Result: toolError(fmt.Sprintf("read file: %v", err))}, true
				}
				res, err := svc.ImportJSON(data, args.DryRun)
				if err != nil {
					return response{JSONRPC: "2.0", ID: req.ID, Result: toolError(err.Error())}, true
				}
				prefix := "import"
				if args.DryRun {
					prefix = "import (dry-run)"
				}
				return response{JSONRPC: "2.0", ID: req.ID, Result: toolText(fmt.Sprintf("%s: inserted=%d skipped=%d", prefix, res.Inserted, res.Skipped))}, true

			case "local_backup":
				var args localPathArgs
				if err := json.Unmarshal(params.Arguments, &args); err != nil {
					return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32602, Message: "invalid arguments"}}, true
				}
				if err := svc.Backup(args.Path); err != nil {
					return response{JSONRPC: "2.0", ID: req.ID, Result: toolError(err.Error())}, true
				}
				return response{JSONRPC: "2.0", ID: req.ID, Result: toolText(fmt.Sprintf("backup written to %s", strings.TrimSpace(args.Path)))}, true

			case "local_restore":
				var args localPathArgs
				if err := json.Unmarshal(params.Arguments, &args); err != nil {
					return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32602, Message: "invalid arguments"}}, true
				}
				if err := svc.Restore(args.Path); err != nil {
					return response{JSONRPC: "2.0", ID: req.ID, Result: toolError(err.Error())}, true
				}
				return response{JSONRPC: "2.0", ID: req.ID, Result: toolText(fmt.Sprintf("restored from %s", strings.TrimSpace(args.Path)))}, true

			case "local_doctor":
				report, err := svc.Doctor()
				if err != nil {
					return response{JSONRPC: "2.0", ID: req.ID, Result: toolError(err.Error())}, true
				}
				out := report.Summary
				for _, msg := range report.IntegrityMessages {
					out += "\n  integrity: " + msg
				}
				return response{JSONRPC: "2.0", ID: req.ID, Result: toolText(out)}, true

			default:
				return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32601, Message: "tool not found"}}, true
			}

		case "ping":
			if isNotification {
				return response{}, false
			}
			return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]string{"pong": "pong"}}, true

		default:
			if isNotification {
				return response{}, false
			}
			return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32601, Message: "method not found"}}, true
		}

}

func detectTransportMode(r *bufio.Reader) (transportMode, error) {
	b, err := r.Peek(1)
	if err != nil {
		return transportFramed, err
	}
	if len(b) > 0 && (b[0] == '{' || b[0] == '[') {
		return transportJSONStream, nil
	}
	return transportFramed, nil
}

func readMessage(r *bufio.Reader, mode transportMode, dec *json.Decoder) ([]byte, error) {
	if mode == transportJSONStream {
		if dec == nil {
			return nil, fmt.Errorf("json decoder is nil")
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		return raw, nil
	}
	return readMCPMessage(r)
}

func writeMessage(w *bufio.Writer, mode transportMode, v interface{}) error {
	if mode == transportJSONStream {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		if _, err := w.Write(append(b, '\n')); err != nil {
			return err
		}
		return w.Flush()
	}
	return writeMCPMessage(w, v)
}

func readMCPMessage(r *bufio.Reader) ([]byte, error) {
	contentLength := 0
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		val := strings.TrimSpace(parts[1])
		if key == "content-length" {
			n, err := strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("invalid content-length: %w", err)
			}
			contentLength = n
		}
	}
	if contentLength <= 0 {
		return nil, fmt.Errorf("missing content-length")
	}
	buf := make([]byte, contentLength)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func writeMCPMessage(w *bufio.Writer, v interface{}) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	if _, err := w.Write(body); err != nil {
		return err
	}
	return w.Flush()
}

func toolText(text string) map[string]interface{} {
	return map[string]interface{}{
		"content": []map[string]string{
			{"type": "text", "text": text},
		},
	}
}

func toolError(msg string) map[string]interface{} {
	return map[string]interface{}{
		"content": []map[string]string{
			{"type": "text", "text": msg},
		},
		"isError": true,
	}
}

// toolsListResult returns the canonical advertised tools payload, used by
// both `tools` and `tools/list` JSON-RPC methods to keep them in sync.
func toolsListResult() map[string]interface{} {
	return map[string]interface{}{
		"tools": []map[string]interface{}{
			{
				"name":        "local_save",
				"description": "Guarda una nota local en SQLite (local-only; no usa Engram).",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"content":   map[string]interface{}{"type": "string"},
						"title":     map[string]interface{}{"type": "string"},
						"type":      map[string]interface{}{"type": "string"},
						"topic_key": map[string]interface{}{"type": "string"},
						"scope":     map[string]interface{}{"type": "string"},
					},
					"required": []string{"content"},
				},
			},
			{
				"name":        "local_recall",
				"description": "Busca notas locales por texto en SQLite (local-only; no consulta Engram). Acepta filtros opcionales por type y rango de fechas (since/until).",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{"type": "string"},
						"limit": map[string]interface{}{"type": "integer", "minimum": 1},
						"type":  map[string]interface{}{"type": "string"},
						"since": map[string]interface{}{"type": "string", "description": "YYYY-MM-DD or RFC3339 (UTC)"},
						"until": map[string]interface{}{"type": "string", "description": "YYYY-MM-DD or RFC3339 (UTC)"},
					},
					"required": []string{"query"},
				},
			},
			{
				"name":        "local_context",
				"description": "Devuelve las N notas más recientes desde SQLite (local-only; no consulta Engram). Acepta filtro opcional por scope.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"n":     map[string]interface{}{"type": "integer", "minimum": 1},
						"scope": map[string]interface{}{"type": "string"},
					},
				},
			},
			{
				"name":        "local_list",
				"description": "Lista notas recientes desde SQLite (local-only; no incluye Engram).",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"limit": map[string]interface{}{"type": "integer", "minimum": 1},
					},
				},
			},
			{
				"name":        "local_delete",
				"description": "Elimina una nota local por id en SQLite (local-only; no afecta Engram).",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{"type": "integer", "minimum": 1},
					},
					"required": []string{"id"},
				},
			},
			{
				"name":        "local_get",
				"description": "Obtiene una nota local por id desde SQLite (local-only; no consulta Engram).",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{"type": "integer", "minimum": 1},
					},
					"required": []string{"id"},
				},
			},
			{
				"name":        "local_update",
				"description": "Actualiza el contenido de una nota local por id en SQLite (local-only; no afecta Engram).",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id":      map[string]interface{}{"type": "integer", "minimum": 1},
						"content": map[string]interface{}{"type": "string"},
					},
					"required": []string{"id", "content"},
				},
			},
			{
				"name":        "local_session_start",
				"description": "Marca el inicio de una sesión en el log local SQLite (local-only; no afecta Engram).",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"session_id": map[string]interface{}{"type": "string"},
					},
					"required": []string{"session_id"},
				},
			},
			{
				"name":        "local_session_end",
				"description": "Marca el cierre de una sesión en el log local SQLite (local-only; no afecta Engram).",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"session_id": map[string]interface{}{"type": "string"},
					},
					"required": []string{"session_id"},
				},
			},
			{
				"name":        "local_session_summary",
				"description": "Adjunta un resumen a una sesión en el log local SQLite (local-only; no afecta Engram).",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"session_id": map[string]interface{}{"type": "string"},
						"summary":    map[string]interface{}{"type": "string"},
					},
					"required": []string{"session_id", "summary"},
				},
			},
			{
				"name":        "local_import",
				"description": "Importa observaciones desde un archivo JSON al store local SQLite (local-only; no afecta Engram). Idempotente: deduplica por (topic_key, hash de content).",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path":    map[string]interface{}{"type": "string"},
						"dry_run": map[string]interface{}{"type": "boolean"},
					},
					"required": []string{"path"},
				},
			},
			{
				"name":        "local_backup",
				"description": "Crea un snapshot portable de la base local SQLite en la ruta indicada (local-only; no afecta Engram). Usa VACUUM INTO atómico.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{"type": "string"},
					},
					"required": []string{"path"},
				},
			},
			{
				"name":        "local_restore",
				"description": "Restaura la base local SQLite desde un snapshot previamente creado (local-only; no afecta Engram). Sobrescribe la base activa.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{"type": "string"},
					},
					"required": []string{"path"},
				},
			},
			{
				"name":        "local_doctor",
				"description": "Diagnóstico read-only del store local SQLite (local-only; no afecta Engram). Reporta schema_version, salud de FTS5, integrity_check y row counts en una línea de resumen.",
				"inputSchema": map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
	}
}

// memoryItemPayload renders a MemoryItem as a JSON-serialisable map with
// UTC ISO-8601 timestamps, used by both CLI and MCP surfaces.
func memoryItemPayload(it app.MemoryItem) map[string]interface{} {
	return map[string]interface{}{
		"id":         it.ID,
		"content":    it.Content,
		"created_at": it.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at": it.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

// memoryItemsPayload renders a slice of items as JSON-serialisable maps
// with metadata fields (title/type/topic_key/scope) included so MCP
// callers using the M2 filter and context surfaces can read structured
// fields without parsing Go's default capital-cased struct keys.
func memoryItemsPayload(items []app.MemoryItem) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(items))
	for _, it := range items {
		row := memoryItemPayload(it)
		row["title"] = it.Title
		row["type"] = it.Type
		row["topic_key"] = it.TopicKey
		row["scope"] = it.Scope
		out = append(out, row)
	}
	return out
}

// parseDateArg accepts YYYY-MM-DD (interpreted as UTC midnight) or RFC3339.
// Mirrors the CLI runner's parseDateFlag so both surfaces stay consistent.
func parseDateArg(raw string) (time.Time, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t.UTC(), nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, fmt.Errorf("expected YYYY-MM-DD or RFC3339, got %q", v)
	}
	return t.UTC(), nil
}
