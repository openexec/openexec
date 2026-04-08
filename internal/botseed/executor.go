package botseed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/openexec/openexec/internal/contracts"
)

// Response is the minimal wire result returned by Execute. It is
// intentionally not a full http.Response so tests can construct one
// without involving a real transport.
type Response struct {
	StatusCode int
	Body       []byte
}

// Execute performs a single contract operation on behalf of a bot.
//
// The payload is interpreted in two ways:
//
//  1. Any key whose value substring appears as a `:name` placeholder
//     in op.Path is used to fill that placeholder (e.g. {"id":"abc"}
//     against "/api/listings/:id/bids" → "/api/listings/abc/bids").
//     Consumed keys are removed from the remaining payload so they
//     do not leak into query strings or bodies.
//  2. For GET/DELETE, any leftover payload is encoded as a query
//     string; for POST/PUT/PATCH it is JSON-encoded as the request
//     body.
//
// The bot's cookies are attached so the request executes as that
// authenticated user.
func (c *Client) Execute(ctx context.Context, bot *Bot, op contracts.Operation, payload map[string]any) (*Response, error) {
	if bot == nil {
		return nil, fmt.Errorf("execute: nil bot")
	}

	// Copy payload so we can destructively consume path params.
	remaining := make(map[string]any, len(payload))
	for k, v := range payload {
		remaining[k] = v
	}

	resolvedPath := resolvePathParams(op.Path, remaining)

	method := strings.ToUpper(op.Method)
	if method == "" {
		method = http.MethodGet
	}

	var (
		reqURL string
		body   io.Reader
		ctype  string
	)

	switch method {
	case http.MethodGet, http.MethodDelete, http.MethodHead:
		reqURL = c.BaseURL + resolvedPath
		if len(remaining) > 0 {
			q := url.Values{}
			for k, v := range remaining {
				q.Set(k, fmt.Sprint(v))
			}
			reqURL += "?" + q.Encode()
		}
	default:
		reqURL = c.BaseURL + resolvedPath
		buf, err := json.Marshal(remaining)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
		body = bytes.NewReader(buf)
		ctype = "application/json"
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	req.Header.Set("Accept", "application/json")
	attachCookies(req, bot.Cookies)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer drainAndClose(resp.Body)

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Cookies may be refreshed mid-session by the server (e.g. slide
	// expiry) — keep the bot up to date.
	if cookies := resp.Cookies(); len(cookies) > 0 {
		bot.Cookies = mergeCookies(bot.Cookies, cookies)
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Body:       raw,
	}, nil
}

// resolvePathParams walks an operation path and substitutes `:name`
// tokens with stringified values from `payload`, removing any
// consumed keys from the map.
func resolvePathParams(path string, payload map[string]any) string {
	if !strings.Contains(path, ":") {
		return path
	}
	segments := strings.Split(path, "/")
	for i, seg := range segments {
		if !strings.HasPrefix(seg, ":") || len(seg) < 2 {
			continue
		}
		key := seg[1:]
		if v, ok := payload[key]; ok {
			segments[i] = fmt.Sprint(v)
			delete(payload, key)
		}
	}
	return strings.Join(segments, "/")
}

// mergeCookies keeps the most recent value for a given cookie name.
func mergeCookies(existing, fresh []*http.Cookie) []*http.Cookie {
	byName := make(map[string]*http.Cookie, len(existing)+len(fresh))
	for _, ck := range existing {
		byName[ck.Name] = ck
	}
	for _, ck := range fresh {
		byName[ck.Name] = ck
	}
	out := make([]*http.Cookie, 0, len(byName))
	for _, ck := range byName {
		out = append(out, ck)
	}
	return out
}
