package execution

import (
	"encoding/json"
	"time"
	"unicode/utf8"

	"github.com/openexec/openexec/pkg/agent"
)

// ContextPolicy is how one provider's request chain is kept inside its model's
// window. Small local models fail on size, not on reasoning: twelve of the
// Qwen failures the console recorded were context_overflow (one of them
// dispatched, so the guard let an oversized request through), three were
// header timeouts on a 1 MB body, and six were budget endings. The policy is a
// property of the provider, so a hosted model with a bigger window checkpoints
// later; nothing here knows which model it is.
//
// A request chain is the run of requests that share one growing conversation.
// Past the soft threshold the model is asked to update the run's handover
// document; past the hard threshold, or when the assembled request cannot be
// processed inside the request timeout, the chain ends and the next one starts
// from the system prompt, the task and the handover only (handover.go).
type ContextPolicy struct {
	// Disabled restores the unbounded chain (tests that prove the failure).
	Disabled bool `json:"disabled,omitempty"`
	// ContextTokens is the provider's real window (131072 for the local Qwen).
	ContextTokens int64 `json:"context_tokens,omitempty"`
	// SoftRatio and HardRatio are shares of the effective window.
	SoftRatio float64 `json:"checkpoint_soft,omitempty"`
	HardRatio float64 `json:"checkpoint_hard,omitempty"`
	// PromptTokensPerSecond is the measured or configured prompt-processing
	// speed (about 100 for the local Qwen); zero means unknown, no time bound.
	PromptTokensPerSecond float64 `json:"prompt_tokens_per_second,omitempty"`
	// RequestTimeout is how long one request may take; zero means unknown.
	RequestTimeout time.Duration `json:"-"`
	// MaxInlineToolBytes is the largest tool result that enters a request whole.
	MaxInlineToolBytes int `json:"max_inline_tool_bytes,omitempty"`
	// MaxRequestBytes bounds one assembled request independently of the token
	// estimate (a header timeout was seen on a 1 MB body); zero derives it from
	// the hard threshold.
	MaxRequestBytes int64 `json:"max_request_bytes,omitempty"`
	// MaxChains bounds the chains of one Execute; past it the pass ends as a
	// context checkpoint and the next pass continues from the handover.
	MaxChains int `json:"max_chains,omitempty"`
}

const (
	defaultSoftRatio          = 0.5
	defaultHardRatio          = 0.7
	defaultLoopbackContext    = int64(MaxAPIContextTokens)
	defaultHostedContext      = int64(128000)
	defaultMaxInlineToolBytes = 12 << 10
	defaultMaxChains          = 8
	// A request must be processed in well under its timeout: generation and
	// queueing share it.
	timeShare = 0.8
	// Bytes per token behind the derived byte bound: a backstop just past the
	// token estimator's own ASCII rate (about 2.4), so dense text that the
	// estimator undercounts is still caught by size.
	hardBytesPerToken = 2.5
)

// WithDefaults fills what the provider did not configure.
func (c ContextPolicy) WithDefaults(loopback bool) ContextPolicy {
	if c.ContextTokens <= 0 {
		c.ContextTokens = defaultHostedContext
		if loopback {
			c.ContextTokens = defaultLoopbackContext
		}
	}
	if c.SoftRatio <= 0 || c.SoftRatio >= 1 {
		c.SoftRatio = defaultSoftRatio
	}
	if c.HardRatio <= c.SoftRatio || c.HardRatio >= 1 {
		c.HardRatio = defaultHardRatio
		if c.HardRatio <= c.SoftRatio {
			c.HardRatio = (1 + c.SoftRatio) / 2
		}
	}
	if c.MaxInlineToolBytes <= 0 {
		c.MaxInlineToolBytes = defaultMaxInlineToolBytes
	}
	if c.MaxChains <= 0 {
		c.MaxChains = defaultMaxChains
	}
	return c
}

// effective narrows the window by what the run itself may use: the hard-token
// path never sends more than MaxAPIContextTokens (or the caller's ceiling).
func (c ContextPolicy) effective(limit int64) ContextPolicy {
	if limit > 0 && limit < c.ContextTokens {
		c.ContextTokens = limit
	}
	return c
}

func (c ContextPolicy) softTokens() int64 { return int64(float64(c.ContextTokens) * c.SoftRatio) }
func (c ContextPolicy) hardTokens() int64 { return int64(float64(c.ContextTokens) * c.HardRatio) }
func (c ContextPolicy) hardBytes() int64 {
	if c.MaxRequestBytes > 0 {
		return c.MaxRequestBytes
	}
	return int64(float64(c.hardTokens()) * hardBytesPerToken)
}

// timeTokens is the most prompt tokens the request timeout allows at the
// configured speed, or 0 when either is unknown.
func (c ContextPolicy) timeTokens() int64 {
	if c.PromptTokensPerSecond <= 0 || c.RequestTimeout <= 0 {
		return 0
	}
	return int64(c.PromptTokensPerSecond * c.RequestTimeout.Seconds() * timeShare)
}

type admission int

const (
	admitOK   admission = iota
	admitSoft           // ask the model to update the handover
	admitHard           // end the chain, restart from the handover
)

// admit judges one assembled request. Bytes, the padded token estimate and the
// time the endpoint needs to read it are three separate bounds.
func (c ContextPolicy) admit(tokens, bytes int64) (admission, string) {
	if t := c.timeTokens(); t > 0 && tokens > t {
		return admitHard, "prompt processing would not finish inside the request timeout"
	}
	if tokens > c.hardTokens() {
		return admitHard, "estimated tokens past the hard threshold"
	}
	if bytes > c.hardBytes() {
		return admitHard, "request bytes past the hard threshold"
	}
	if tokens > c.softTokens() {
		return admitSoft, "estimated tokens past the soft threshold"
	}
	if t := c.timeTokens(); t > 0 && tokens > t/2 {
		return admitSoft, "prompt processing past half of the request timeout"
	}
	return admitOK, ""
}

// estimateRequest sizes the complete request the way the adapters serialize
// it (system, messages with tool calls and results, tool schemas), with the
// same padded token estimate as agent.ContextOverflowError.
func estimateRequest(r agent.Request) (tokens, bytes int64) {
	body, err := json.Marshal(struct {
		System   string                 `json:"system"`
		Messages []agent.Message        `json:"messages"`
		Tools    []agent.ToolDefinition `json:"tools"`
	}{r.System, r.Messages, r.Tools})
	if err != nil {
		return 1 << 40, 1 << 40
	}
	ascii, other := 0, 0
	for _, b := range body {
		if b < 128 {
			ascii++
		} else {
			other++
		}
	}
	return int64((ascii*5+11)/12 + other + 256 + (len(r.Messages)+len(r.Tools))*32), int64(len(body))
}

// headTail splits text into its first head and last tail bytes on rune
// boundaries and reports how many bytes lie between them.
func headTail(text string, head, tail int) (string, string, int) {
	if len(text) <= head+tail {
		return text, "", 0
	}
	h := head
	for h > 0 && !utf8.RuneStart(text[h]) {
		h--
	}
	t := len(text) - tail
	for t < len(text) && !utf8.RuneStart(text[t]) {
		t++
	}
	return text[:h], text[t:], t - h
}
