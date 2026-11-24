package main

import (
	"flag"
	"os"
	"runtime/debug"

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
	a := app.NewWithID("com.mailrat.app")
	ui.RegisterRemoteImages()
	ui.Run(a, desktop.NewApp())
}
