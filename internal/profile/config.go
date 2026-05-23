package profile

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"
)

// ProxyConfig holds proxy connection details.
type ProxyConfig struct {
	Type string `json:"type"` // "socks5", "http", or ""
	Host string `json:"host"`
	Port int    `json:"port"`
	User string `json:"user"`
	Pass string `json:"pass"`
}

// Fingerprint describes the browser fingerprint spoofed for a profile.
type Fingerprint struct {
	TLSPreset           string   `json:"tls_preset"`
	UserAgent           string   `json:"user_agent"`
	Platform            string   `json:"platform"`
	ScreenWidth         int      `json:"screen_width"`
	ScreenHeight        int      `json:"screen_height"`
	Timezone            string   `json:"timezone"`
	Languages           []string `json:"languages"`
	HardwareConcurrency int      `json:"hardware_concurrency"`
	DeviceMemory        int      `json:"device_memory"`
	CanvasNoiseSeed     int64    `json:"canvas_noise_seed"`
	WebGLVendor         string   `json:"webgl_vendor"`
	WebGLRenderer       string   `json:"webgl_renderer"`
}

// Config is the top-level profile configuration.
type Config struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Created     time.Time   `json:"created"`
	Proxy       ProxyConfig `json:"proxy"`
	Fingerprint Fingerprint `json:"fingerprint"`
	Theme       string      `json:"theme"`
}

// DefaultFingerprints contains built-in browser fingerprint presets.
var DefaultFingerprints = map[string]Fingerprint{
	"chrome_120": {
		TLSPreset:           "chrome_120",
		UserAgent:           "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		Platform:            "Win32",
		ScreenWidth:         1920,
		ScreenHeight:        1080,
		Timezone:            "America/New_York",
		Languages:           []string{"en-US", "en"},
		HardwareConcurrency: 8,
		DeviceMemory:        8,
		CanvasNoiseSeed:     0, // set at runtime
		WebGLVendor:         "Google Inc. (NVIDIA)",
		WebGLRenderer:       "ANGLE (NVIDIA, NVIDIA GeForce RTX 3060 Direct3D11 vs_5_0 ps_5_0)",
	},
	"firefox_121": {
		TLSPreset:           "firefox_121",
		UserAgent:           "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0",
		Platform:            "Win32",
		ScreenWidth:         1920,
		ScreenHeight:        1080,
		Timezone:            "America/New_York",
		Languages:           []string{"en-US", "en"},
		HardwareConcurrency: 8,
		DeviceMemory:        8,
		CanvasNoiseSeed:     0,
		WebGLVendor:         "Mozilla",
		WebGLRenderer:       "Mozilla",
	},
	"safari_17": {
		TLSPreset:           "safari_17",
		UserAgent:           "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_2_1) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15",
		Platform:            "MacIntel",
		ScreenWidth:         2560,
		ScreenHeight:        1600,
		Timezone:            "America/Los_Angeles",
		Languages:           []string{"en-US", "en"},
		HardwareConcurrency: 10,
		DeviceMemory:        16,
		CanvasNoiseSeed:     0,
		WebGLVendor:         "Apple Inc.",
		WebGLRenderer:       "Apple M2",
	},
}

// GenerateID creates a unique profile ID from the current timestamp + 6 random hex chars.
func GenerateID() string {
	ts := time.Now().UnixMilli()
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%d_%s", ts, hex.EncodeToString(b))
}

// randomInt64 generates a cryptographically random int64.
func randomInt64() (int64, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return 0, err
	}
	return int64(binary.LittleEndian.Uint64(b)), nil
}

// NewConfig creates a new Config with the chrome_120 preset and a random canvas noise seed.
func NewConfig(name string) (*Config, error) {
	id := GenerateID()

	fp := DefaultFingerprints["chrome_120"]

	seed, err := randomInt64()
	if err != nil {
		return nil, fmt.Errorf("profile: generate canvas seed: %w", err)
	}
	fp.CanvasNoiseSeed = seed

	return &Config{
		ID:          id,
		Name:        name,
		Created:     time.Now().UTC(),
		Proxy:       ProxyConfig{},
		Fingerprint: fp,
		Theme:       "gray",
	}, nil
}

// ProxyURL returns a formatted proxy URL for the profile, or "" if no proxy is set.
func (c *Config) ProxyURL() string {
	p := c.Proxy
	if p.Type == "" || p.Host == "" {
		return ""
	}

	auth := ""
	if p.User != "" || p.Pass != "" {
		auth = fmt.Sprintf("%s:%s@", p.User, p.Pass)
	}

	return fmt.Sprintf("%s://%s%s:%d", p.Type, auth, p.Host, p.Port)
}
