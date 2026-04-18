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

	mux.HandleFunc("/api/accounts", func(w http.ResponseWriter, r *http.Request) {
		accs, err := app.GetAccounts()
		if err != nil { errJSON(w, err); return }
		writeJSON(w, http.StatusOK, accs)
	})

	mux.HandleFunc("/api/inbox", func(w http.ResponseWriter, r *http.Request) {
		threads, err := app.GetInbox(qp(r, "accountId"), 60)
		if err != nil { errJSON(w, err); return }
		writeJSON(w, http.StatusOK, threads)
	})

	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		threads, err := app.SearchInbox(qp(r, "q"), 50)
		if err != nil { errJSON(w, err); return }
		writeJSON(w, http.StatusOK, threads)
	})

	mux.HandleFunc("/api/contacts/search", func(w http.ResponseWriter, r *http.Request) {
		contacts, err := app.SearchContacts(qp(r, "accountId"), qp(r, "q"), 8)
		if err != nil { errJSON(w, err); return }
		writeJSON(w, http.StatusOK, contacts)
	})

	mux.HandleFunc("/api/label-threads", func(w http.ResponseWriter, r *http.Request) {
		threads, err := app.GetLabelThreads(qp(r, "accountId"), qp(r, "labelId"), 60)
		if err != nil { errJSON(w, err); return }
		writeJSON(w, http.StatusOK, threads)
	})

	mux.HandleFunc("/api/thread", func(w http.ResponseWriter, r *http.Request) {
		msgs, err := app.GetThread(qp(r, "accountId"), qp(r, "threadId"))
		if err != nil { errJSON(w, err); return }
		writeJSON(w, http.StatusOK, msgs)
	})

	mux.HandleFunc("/api/message", func(w http.ResponseWriter, r *http.Request) {
		msg, err := app.GetMessage(qp(r, "accountId"), qp(r, "messageId"))
		if err != nil { errJSON(w, err); return }
		writeJSON(w, http.StatusOK, msg)
	})

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

	mux.HandleFunc("/api/sync", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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

	mux.HandleFunc("/api/labels", func(w http.ResponseWriter, r *http.Request) {
		labels, err := app.GetLabels(qp(r, "accountId"))
		if err != nil { errJSON(w, err); return }
		writeJSON(w, http.StatusOK, labels)
	})

	mux.HandleFunc("/api/mark-read", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := app.MarkMessageRead(qp(r, "accountId"), qp(r, "messageId")); err != nil {
			errJSON(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	mux.HandleFunc("/api/mark-unread", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := app.MarkMessageUnread(qp(r, "accountId"), qp(r, "messageId")); err != nil {
			errJSON(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	mux.HandleFunc("/api/trash", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := app.TrashThread(qp(r, "accountId"), qp(r, "threadId")); err != nil {
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
	payload, _ := json.Marshal(map[string]any{"event": event, "data": data})
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
