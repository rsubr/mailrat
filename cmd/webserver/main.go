// cmd/webserver serves the mailrat frontend over HTTP for remote browser testing.
// The Go backend methods are exposed as a JSON REST API; sync events are pushed
// via Server-Sent Events so the Alpine.js store stays in sync.
//
// Usage:
//
//	go run ./cmd/webserver -addr 0.0.0.0:8080 -static cmd/desktop/frontend
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/mailrat/mailrat/internal/desktop"
)

// ── SSE event hub ────────────────────────────────────────────────────────────

type hub struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

func newHub() *hub { return &hub{clients: make(map[chan []byte]struct{})} }

func (h *hub) subscribe() chan []byte {
	ch := make(chan []byte, 8)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *hub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.clients, ch)
	close(ch)
	h.mu.Unlock()
}

func (h *hub) emit(event string, data any) {
	payload, _ := json.Marshal(map[string]any{"event": event, "data": data})
	h.mu.Lock()
	for ch := range h.clients {
		select {
		case ch <- payload:
		default: // drop if client is slow
		}
	}
	h.mu.Unlock()
}

// ── HTTP helpers ─────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON: %v", err)
	}
}

func errJSON(w http.ResponseWriter, err error) {
	log.Printf("API error: %v", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
}

func queryParam(r *http.Request, key string) string {
	return r.URL.Query().Get(key)
}

// ── Main ─────────────────────────────────────────────────────────────────────

func main() {
	addr      := flag.String("addr",   "0.0.0.0:8080",        "listen address (host:port)")
	staticDir := flag.String("static", "cmd/desktop/frontend", "path to frontend static files")
	flag.Parse()

	// Initialise the backend app (same store + account manager as CLI/TUI/desktop).
	app := desktop.NewApp()
	app.OnStartup(context.Background())

	events := newHub()
	mux := http.NewServeMux()

	// ── Static frontend ───────────────────────────────────────────────────
	mux.Handle("/", http.FileServer(http.Dir(*staticDir)))

	// ── Server-Sent Events ────────────────────────────────────────────────
	mux.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		ch := events.subscribe()
		defer events.unsubscribe(ch)

		// Send a keep-alive comment immediately so the browser knows we're alive.
		fmt.Fprint(w, ": connected\n\n")
		flusher.Flush()

		for {
			select {
			case msg, ok := <-ch:
				if !ok {
					return
				}
				fmt.Fprintf(w, "data: %s\n\n", msg)
				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	})

	// ── API: accounts ─────────────────────────────────────────────────────
	mux.HandleFunc("/api/accounts", func(w http.ResponseWriter, r *http.Request) {
		accs, err := app.GetAccounts()
		if err != nil { errJSON(w, err); return }
		writeJSON(w, http.StatusOK, accs)
	})

	// ── API: inbox ────────────────────────────────────────────────────────
	mux.HandleFunc("/api/inbox", func(w http.ResponseWriter, r *http.Request) {
		threads, err := app.GetInbox(queryParam(r, "accountId"), 60)
		if err != nil { errJSON(w, err); return }
		writeJSON(w, http.StatusOK, threads)
	})

	// ── API: search ───────────────────────────────────────────────────────
	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		threads, err := app.SearchInbox(queryParam(r, "q"), 50)
		if err != nil { errJSON(w, err); return }
		writeJSON(w, http.StatusOK, threads)
	})

	// ── API: thread ───────────────────────────────────────────────────────
	mux.HandleFunc("/api/thread", func(w http.ResponseWriter, r *http.Request) {
		msgs, err := app.GetThread(queryParam(r, "accountId"), queryParam(r, "threadId"))
		if err != nil { errJSON(w, err); return }
		writeJSON(w, http.StatusOK, msgs)
	})

	// ── API: message (lazy body fetch) ────────────────────────────────────
	mux.HandleFunc("/api/message", func(w http.ResponseWriter, r *http.Request) {
		msg, err := app.GetMessage(queryParam(r, "accountId"), queryParam(r, "messageId"))
		if err != nil { errJSON(w, err); return }
		writeJSON(w, http.StatusOK, msg)
	})

	// ── API: send ─────────────────────────────────────────────────────────
	mux.HandleFunc("/api/send", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var msg desktop.OutboundMsg
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			errJSON(w, fmt.Errorf("bad request: %w", err))
			return
		}
		if err := app.SendMessage(msg); err != nil { errJSON(w, err); return }
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	// ── API: sync ─────────────────────────────────────────────────────────
	// Runs synchronously; broadcasts SSE events when done.
	mux.HandleFunc("/api/sync", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		events.emit("sync:progress", map[string]string{"status": "started"})
		if err := app.Sync(queryParam(r, "accountId")); err != nil {
			events.emit("sync:complete", nil)
			errJSON(w, err)
			return
		}
		events.emit("sync:complete", nil)
		events.emit("inbox:updated", nil)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	// ── API: labels ───────────────────────────────────────────────────────
	mux.HandleFunc("/api/labels", func(w http.ResponseWriter, r *http.Request) {
		labels, err := app.GetLabels(queryParam(r, "accountId"))
		if err != nil { errJSON(w, err); return }
		writeJSON(w, http.StatusOK, labels)
	})

	log.Printf("mailrat web server → http://%s  (static: %s)", *addr, *staticDir)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}
