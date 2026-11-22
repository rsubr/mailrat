package main

import (
	"fyne.io/fyne/v2/app"
	"github.com/mailrat/mailrat/internal/desktop"
	"github.com/mailrat/mailrat/internal/ui"
)

func main() {
	a := app.NewWithID("com.mailrat.app")
	ui.Run(a, desktop.NewApp())
}
