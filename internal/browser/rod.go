package browser

import (
	"fmt"
	"path/filepath"

	"github.com/alterego/browser/internal/profile"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/stealth"
)

// LaunchRod starts Chromium using go-rod with the specified profile configuration.
func LaunchRod(cfg *profile.Config, manager *profile.Manager) (*rod.Browser, error) {
	profileDir := manager.ProfileDir(cfg.ID)
	userDataDir := filepath.Join(profileDir, "userdata")

	u := launcher.New().
		Leakless(false).
		Headless(false).
		UserDataDir(userDataDir).
		Set("disable-sync", "true").
		Set("window-size", fmt.Sprintf("%d,%d", cfg.Fingerprint.ScreenWidth, cfg.Fingerprint.ScreenHeight)).
		Set("enforce-webrtc-ip-permission-check", "true").
		Set("force-webrtc-ip-handling-policy", "disable_non_proxied_udp")

	// Set proxy if configured (SOCKS5 or HTTP)
	proxyURL := cfg.ProxyURL()
	if proxyURL != "" {
		u = u.Proxy(proxyURL)
	}

	// Set User-Agent from fingerprint
	u = u.Set("user-agent", cfg.Fingerprint.UserAgent)
	
	if len(cfg.Fingerprint.Languages) > 0 {
		u = u.Set("accept-lang", cfg.Fingerprint.Languages[0])
	}

	url, err := u.Launch()
	if err != nil {
		return nil, fmt.Errorf("launch chromium: %w", err)
	}

	browser := rod.New().ControlURL(url).MustConnect()

	// Use the stealth plugin to create the initial page
	page := stealth.MustPage(browser)
	
	// Inject our advanced JS spoofing hooks
	stealthJS := GetAdvancedStealthJS(cfg.ID)
	page.MustEvalOnNewDocument(stealthJS)

	// Navigate to a testing site to show anti-detect working
	page.MustNavigate("https://bot.sannysoft.com/")

	return browser, nil
}
