# AlterEgo Anti-Detect Browser

AlterEgo is a lightweight, simple yet powerful anti-detect browser built with Go (`go-rod`) designed for seamless multi-accounting and basic fingerprint spoofing.

## Features

- **🚀 Lightweight Local Web GUI**: A stunning premium Dark Mode UI (HTML/CSS/JS) served directly from the Go binary. No Node.js required!
- **👥 True Multi-Accounting**: Every profile gets its own fully isolated `userdata` directory. Cookies, local storage, history, and active sessions never leak between profiles.
- **🔌 Proxy Support**: Route your profiles through HTTP or SOCKS5 proxies easily from the UI.
- **🛡️ WebRTC IP-Leak Protection**: Built-in strict Chromium flags (`--enforce-webrtc-ip-permission-check`) prevent UDP proxy bypasses, keeping your real IP hidden.
- **🎨 Advanced Fingerprint Spoofing**:
  - **Canvas & WebGL Noise**: Injects micro-noise into pixel rendering and spoofs WebGL renderer/vendor strings.
  - **AudioContext & Fonts**: Alters audio frequency arrays and font measurement (`measureText`) to bypass standard tracking.
  - **Consistency**: Noise generation is seeded by the unique Profile ID, meaning fingerprints stay consistent for the same profile across reboots.
- **🤖 Stealth Plugin**: Integrates `go-rod/stealth` to automatically mask `navigator.webdriver` and bypass common bot-detection scripts (like Cloudflare or Datadome).

## Getting Started

### Prerequisites
- [Go 1.21+](https://go.dev/dl/)
- Chromium (Automatically downloaded by `go-rod` on first launch)

### Installation & Run

1. Clone this repository.
2. Run the application:
   ```bash
   go run ./cmd/alterego
   ```
3. Your default web browser will automatically open the AlterEgo Dashboard at `http://localhost:8080`.

### Usage

1. Click **New Profile** in the dashboard.
2. Click the Edit (✎) icon to set a name and configure your Proxy (HTTP/SOCKS5). Leave the proxy fields empty for a direct connection.
3. Click **Open Browser**. A fully isolated, spoofed Chromium window will launch!

## Architecture

AlterEgo abandons heavy UI libraries in favor of a hybrid approach:
- **Backend (Go):** Runs a local REST API server and orchestrates Chromium instances using `go-rod`.
- **Frontend (Vanilla Web):** Uses pure HTML/CSS/JS embedded directly into the Go binary using `go:embed`.
- **Fingerprinting (JS Hooks):** Applies CDP (Chrome DevTools Protocol) to inject `EvalOnNewDocument` scripts that override hardware APIs before the page loads.

## Disclaimer

This project is for educational and research purposes only.

## ❤️ Support & Donation

If you find this project useful and would like to support its development, you can donate here:

👉 **[Donate Link](https://adiru3.github.io/Donate/)**

---
