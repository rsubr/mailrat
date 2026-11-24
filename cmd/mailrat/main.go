package main

import (
	"flag"
	"log"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"time"

	"fyne.io/fyne/v2/app"
	"github.com/mailrat/mailrat/internal/desktop"
	"github.com/mailrat/mailrat/internal/ui"
)

func main() {
	dataDir := flag.String("data-dir", "", "profile directory (default ~/.mailrat; env MAILRAT_DATA_DIR)")
	flag.Parse()
	if *dataDir != "" {
		os.Setenv("MAILRAT_DATA_DIR", *dataDir)
	}
	debug.SetGCPercent(25)
	debug.SetMemoryLimit(100 << 20)
	go freeIdleMemory()
	if os.Getenv("MAILRAT_MEMLOG") != "" {
		go memLog()
	}
	a := app.NewWithID("com.mailrat.app")
	ui.RegisterRemoteImages()
	ui.Run(a, desktop.NewApp())
}

// memLog prints Go runtime memory stats every 5s (debug aid, MAILRAT_MEMLOG=1).
func memLog() {
	var m runtime.MemStats
	for range time.Tick(5 * time.Second) {
		runtime.ReadMemStats(&m)
		if f := os.Getenv("MAILRAT_HEAPPROF"); f != "" {
			if w, err := os.Create(f); err == nil {
				pprof.Lookup("heap").WriteTo(w, 0)
				w.Close()
			}
		}
		log.Printf("MEM heapInuse=%dK heapIdle=%dK released=%dK stack=%dK sys=%dK goroutines=%d",
			m.HeapInuse>>10, m.HeapIdle>>10, m.HeapReleased>>10, m.StackInuse>>10, m.Sys>>10, runtime.NumGoroutine())
	}
}

// freeIdleMemory returns freed heap to the OS every minute; the Go scavenger
// alone releases it slowly and the 25% GC target keeps the heap small anyway.
func freeIdleMemory() {
	for range time.Tick(time.Minute) {
		debug.FreeOSMemory()
	}
}
