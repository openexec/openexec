package botseed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Signup registers a new persona against the running app's real
// signup endpoint (`POST /api/auth/signup`). It captures any
// Set-Cookie headers so the returned Bot is ready to authenticate
// subsequent requests.
//
// If the server responds with 409 Conflict (user already exists),
// Signup transparently falls back to Login and returns the
// now-authenticated bot, so repeated seed runs with the same RNG
// seed remain idempotent.
func (c *Client) Signup(ctx context.Context, p Persona) (*Bot, error) {
	bot := &Bot{
		ID:      p.Email,
		Persona: p,
	}

	body := map[string]string{
		"email":    p.Email,
		"password": p.Password,
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal signup body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/api/auth/signup", bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("build signup request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("signup request: %w", err)
	}
	defer drainAndClose(resp.Body)

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		bot.Cookies = append(bot.Cookies, resp.Cookies()...)
		bot.SessionID = extractSessionID(bot.Cookies)
		return bot, nil
	case resp.StatusCode == http.StatusConflict:
		// User already exists — try login instead.
		if err := c.Login(ctx, bot); err != nil {
			return nil, fmt.Errorf("signup 409 -> login fallback: %w", err)
		}
		return bot, nil
	default:
		slurp, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("signup returned %d: %s", resp.StatusCode, strings.TrimSpace(string(slurp)))
	}
}

// Login authenticates an existing bot and updates its cookie set.
func (c *Client) Login(ctx context.Context, bot *Bot) error {
	if bot == nil {
		return fmt.Errorf("login: nil bot")
	}
	body := map[string]string{
		"email":    bot.Persona.Email,
		"password": bot.Persona.Password,
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal login body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/api/auth/login", bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("build login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("login request: %w", err)
	}
	defer drainAndClose(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slurp, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("login returned %d: %s", resp.StatusCode, strings.TrimSpace(string(slurp)))
	}
	// Replace cookies — the server may have rotated the session.
	if cookies := resp.Cookies(); len(cookies) > 0 {
		bot.Cookies = cookies
		bot.SessionID = extractSessionID(cookies)
	}
	return nil
}

// Logout terminates the bot's session and clears its cookies.
func (c *Client) Logout(ctx context.Context, bot *Bot) error {
	if bot == nil {
		return fmt.Errorf("logout: nil bot")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/api/auth/logout", nil)
	if err != nil {
		return fmt.Errorf("build logout request: %w", err)
	}
	attachCookies(req, bot.Cookies)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("logout request: %w", err)
	}
	defer drainAndClose(resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		bot.Cookies = nil
		bot.SessionID = ""
		return nil
	}
	slurp, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("logout returned %d: %s", resp.StatusCode, strings.TrimSpace(string(slurp)))
}

// extractSessionID looks for a Lucia-style session cookie and returns
// its value. It is strictly best-effort; the bot still authenticates
// via its raw cookie set even when this returns empty.
func extractSessionID(cookies []*http.Cookie) string {
	for _, ck := range cookies {
		name := strings.ToLower(ck.Name)
		if name == "auth_session" || strings.Contains(name, "session") {
			return ck.Value
		}
	}
	return ""
}

// attachCookies copies all stored cookies onto an outgoing request.
func attachCookies(req *http.Request, cookies []*http.Cookie) {
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
}

// drainAndClose reads any leftover body and closes it so the HTTP
// transport can reuse the underlying connection.
func drainAndClose(rc io.ReadCloser) {
	if rc == nil {
		return
	}
	_, _ = io.Copy(io.Discard, rc)
	_ = rc.Close()
}
