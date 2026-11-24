package main

import (
	"flag"
	"os"

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
	a := app.NewWithID("com.mailrat.app")
	ui.Run(a, desktop.NewApp())
}
