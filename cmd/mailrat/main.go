package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/mailrat/mailrat/internal/cli"
	"github.com/mailrat/mailrat/internal/desktop"
	"github.com/spf13/cobra"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend
var rawAssets embed.FS

func main() {
	frontendFS, err := fs.Sub(rawAssets, "frontend")
	if err != nil {
		panic(err)
	}

	root := cli.NewRootCmd(func(cmd *cobra.Command, args []string) error {
		return launchDesktop(frontendFS)
	})
	root.AddCommand(cli.NewTUICmd())
	root.AddCommand(newWebCmd(frontendFS))

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// launchDesktop starts the Wails desktop app.
func launchDesktop(frontendFS fs.FS) error {
	app := desktop.NewApp()
	return wails.Run(&options.App{
		Title:     "mailrat",
		Width:     1280,
		Height:    800,
		MinWidth:  960,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: frontendFS,
		},
		OnStartup:  app.OnStartup,
		OnShutdown: app.OnShutdown,
		Bind:       []interface{}{app},
	})
}

// ── "web" subcommand ─────────────────────────────────────────────────────────

func newWebCmd(embeddedFS fs.FS) *cobra.Command {
	var addr, staticDir string
	cmd := &cobra.Command{
		Use:   "web",
		Short: "Serve the web UI over HTTP for remote browser access",
		RunE: func(cmd *cobra.Command, args []string) error {
			serveFS := embeddedFS
			if staticDir != "" {
				serveFS = os.DirFS(staticDir)
			}
			return runWebServer(addr, serveFS)
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "0.0.0.0:8080", "listen address (host:port)")
	cmd.Flags().StringVar(&staticDir, "static", "", "serve frontend from path instead of embedded (for development)")
	return cmd
}

// ── Web server implementation ────────────────────────────────────────────────

func runWebServer(addr string, frontendFS fs.FS) error {
	app := desktop.NewApp()
	app.StartWeb(context.Background())

	events := newHub()
	mux := http.NewServeMux()

	// Static frontend (embedded or from --static path)
	mux.Handle("/", http.FileServer(http.FS(frontendFS)))

	// SSE event stream
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

	handleGET(mux, "/api/accounts", func(r *http.Request) (any, error) {
		return app.GetAccounts()
	})

	handleGET(mux, "/api/inbox", func(r *http.Request) (any, error) {
		return app.GetInbox(qp(r, "accountId"), 60, qpi(r, "offset"))
	})

	handleGET(mux, "/api/search", func(r *http.Request) (any, error) {
		return app.SearchInbox(qp(r, "q"), 50)
	})

	handleGET(mux, "/api/contacts/search", func(r *http.Request) (any, error) {
		return app.SearchContacts(qp(r, "accountId"), qp(r, "q"), 8)
	})

	handleGET(mux, "/api/label-threads", func(r *http.Request) (any, error) {
		return app.GetLabelThreads(qp(r, "accountId"), qp(r, "labelId"), 60, qpi(r, "offset"))
	})

	handleGET(mux, "/api/thread", func(r *http.Request) (any, error) {
		return app.GetThread(qp(r, "accountId"), qp(r, "threadId"))
	})

	handleGET(mux, "/api/message", func(r *http.Request) (any, error) {
		return app.GetMessage(qp(r, "accountId"), qp(r, "messageId"))
	})

	mux.HandleFunc("/api/send", func(w http.ResponseWriter, r *http.Request) {
		if !requirePOST(w, r) {
			return
		}
		var msg desktop.OutboundMsg
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			errJSON(w, fmt.Errorf("bad request: %w", err))
			return
		}
		if err := app.SendMessage(msg); err != nil {
			errJSON(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	mux.HandleFunc("/api/sync", func(w http.ResponseWriter, r *http.Request) {
		if !requirePOST(w, r) {
			return
		}
		events.emit("sync:progress", map[string]string{"status": "started"})
		if err := app.Sync(qp(r, "accountId")); err != nil {
			events.emit("sync:complete", nil)
			errJSON(w, err)
			return
		}
		app.DrainPendingOps()
		events.emit("sync:complete", nil)
		events.emit("inbox:updated", nil)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	handleGET(mux, "/api/labels", func(r *http.Request) (any, error) {
		return app.GetLabels(qp(r, "accountId"))
	})

	mux.HandleFunc("/api/mark-read", func(w http.ResponseWriter, r *http.Request) {
		if !requirePOST(w, r) {
			return
		}
		if err := app.MarkMessageRead(qp(r, "accountId"), qp(r, "messageId")); err != nil {
			errJSON(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	mux.HandleFunc("/api/mark-unread", func(w http.ResponseWriter, r *http.Request) {
		if !requirePOST(w, r) {
			return
		}
		if err := app.MarkMessageUnread(qp(r, "accountId"), qp(r, "messageId")); err != nil {
			errJSON(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	mux.HandleFunc("/api/trash", func(w http.ResponseWriter, r *http.Request) {
		if !requirePOST(w, r) {
			return
		}
		if err := app.TrashThread(qp(r, "accountId"), qp(r, "threadId")); err != nil {
			errJSON(w, err)
			return
		}
		events.emit("inbox:updated", nil)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	mux.HandleFunc("/api/spam", func(w http.ResponseWriter, r *http.Request) {
		if !requirePOST(w, r) {
			return
		}
		if err := app.MarkThreadSpam(qp(r, "accountId"), qp(r, "threadId")); err != nil {
			errJSON(w, err)
			return
		}
		events.emit("inbox:updated", nil)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	mux.HandleFunc("/api/not-spam", func(w http.ResponseWriter, r *http.Request) {
		if !requirePOST(w, r) {
			return
		}
		if err := app.MarkThreadNotSpam(qp(r, "accountId"), qp(r, "threadId")); err != nil {
			errJSON(w, err)
			return
		}
		events.emit("inbox:updated", nil)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	log.Printf("mailrat web → http://%s", addr)
	return http.ListenAndServe(addr, mux)
}

// ── SSE hub ──────────────────────────────────────────────────────────────────

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
	payload, err := json.Marshal(map[string]any{"event": event, "data": data})
	if err != nil {
		return
	}
	h.mu.Lock()
	for ch := range h.clients {
		select {
		case ch <- payload:
		default:
		}
	}
	h.mu.Unlock()
}

// ── HTTP helpers ─────────────────────────────────────────────────────────────

func handleGET(mux *http.ServeMux, path string, h func(*http.Request) (any, error)) {
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		result, err := h(r)
		if err != nil {
			errJSON(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
}

func requirePOST(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

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

func qp(r *http.Request, key string) string {
	return r.URL.Query().Get(key)
}

func qpi(r *http.Request, key string) int {
	v, _ := strconv.Atoi(r.URL.Query().Get(key))
	return v
}
