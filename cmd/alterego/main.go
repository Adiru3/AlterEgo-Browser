package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/alterego/browser/internal/api"
	"github.com/alterego/browser/internal/profile"
	"github.com/alterego/browser/web"
)

func main() {
	profilesDir := "./profiles"
	if abs, err := filepath.Abs(profilesDir); err == nil {
		profilesDir = abs
	}
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		log.Fatalf("Failed to create profiles dir: %v", err)
	}

	mgr := profile.NewManager(profilesDir)
	server := api.NewServer(mgr, web.StaticFiles)

	port := "8080"
	url := "http://localhost:" + port

	go func() {
		log.Printf("Starting AlterEgo Server on %s", url)
		if err := http.ListenAndServe(":"+port, server.Router()); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	openBrowser(url)

	// Block forever
	select {}
}

func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		log.Printf("Could not open browser automatically: %v", err)
	}
}
