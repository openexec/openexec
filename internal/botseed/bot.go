// Package botseed provides a lightweight traffic generator that seeds a
// running SvelteKit template app (or any compatible HTTP API) with a
// pool of realistic synthetic users. Each bot authenticates against
// the real `/api/auth/*` endpoints, carries its own cookie jar, and
// exercises a set of contract operations so a freshly-generated site
// looks populated instead of empty.
//
// The package is deliberately dependency-free: it speaks `net/http`
// and `encoding/json` directly and owns no globals.
package botseed

import (
	"net/http"
	"time"
)

// Persona describes a single synthetic user. It is generated
// deterministically from a seed so bot runs are reproducible.
type Persona struct {
	Name     string
	Email    string
	Locale   string
	Traits   []string // e.g. ["lurker", "poster", "bidder", "commenter"]
	Password string
}

// Bot couples a persona with its live session state after signup /
// login. Cookies are held per-bot (not in a shared jar) so the bots
// stay isolated from each other.
type Bot struct {
	ID        string
	Persona   Persona
	SessionID string // set after login, best-effort
	Cookies   []*http.Cookie
}

// Client is the thin HTTP façade used by all botseed operations. It
// holds no shared cookie jar on purpose — each Bot owns its own
// cookies and attaches them per-request.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewClient builds a Client bound to a base URL with sensible
// timeouts. The returned client is safe to share across goroutines.
func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
			// Explicitly disable the default cookie jar — each bot
			// manages its own cookies so sessions do not bleed.
			Jar: nil,
		},
	}
}
