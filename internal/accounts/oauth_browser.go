package accounts

import (
	"os"
	"os/exec"
	"runtime"
)

// tryOpenBrowser attempts to open url in the system browser.
// Returns true if the command launched without error.
func tryOpenBrowser(url string) bool {
	if os.Getenv("MAILRAT_OAUTH_HEADLESS") == "1" {
		return false
	}
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd, args = "open", []string{url}
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		for _, bin := range []string{"xdg-open", "sensible-browser", "x-www-browser"} {
			if _, err := exec.LookPath(bin); err == nil {
				cmd, args = bin, []string{url}
				break
			}
		}
		if cmd == "" {
			return false
		}
	}
	return exec.Command(cmd, args...).Start() == nil
}
