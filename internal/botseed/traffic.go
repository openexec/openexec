package botseed

import (
	"context"
	"errors"
	"fmt"
	mathrand "math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/openexec/openexec/internal/contracts"
)

// TrafficConfig drives a single seed run: how many bots to register,
// how fast they should send requests, and how long the run should
// last.
type TrafficConfig struct {
	Bots     int
	Rate     float64 // requests per second across the whole pool
	Duration time.Duration
	BaseURL  string
	Contract *contracts.ContractFile
	Seed     int64
}

// OperationStats collects per-operation observations across a run.
type OperationStats struct {
	Attempts  int
	Successes int
	Failures  int
	P50ms     int64
	P95ms     int64
}

// TrafficReport summarises a completed traffic run.
type TrafficReport struct {
	BotsRegistered int
	Operations     int
	Successes      int
	Failures       int
	ByOperation    map[string]OperationStats
	StartedAt      time.Time
	FinishedAt     time.Time
}

// RunTraffic is the single entry point for generating synthetic
// traffic against a running server. It is intentionally single-
// goroutine for its request loop — correctness and observability
// over raw throughput — which keeps reasoning about race conditions
// and cancellation easy.
func RunTraffic(ctx context.Context, cfg TrafficConfig) (*TrafficReport, error) {
	if cfg.Bots <= 0 {
		return nil, errors.New("traffic: Bots must be > 0")
	}
	if cfg.Rate <= 0 {
		cfg.Rate = 1.0
	}
	if cfg.Duration <= 0 {
		cfg.Duration = 30 * time.Second
	}
	if cfg.BaseURL == "" {
		return nil, errors.New("traffic: BaseURL required")
	}
	if cfg.Contract == nil || cfg.Contract.IsEmpty() {
		return nil, errors.New("traffic: contract is empty; nothing to exercise")
	}

	report := &TrafficReport{
		ByOperation: make(map[string]OperationStats),
		StartedAt:   time.Now(),
	}

	client := NewClient(cfg.BaseURL)
	rng := mathrand.New(mathrand.NewSource(cfg.Seed))

	// Register bots. Failures during signup are logged into the
	// report's Failures counter but do not abort the run — a partial
	// pool is still useful.
	personas := GeneratePersonas(cfg.Bots, cfg.Seed)
	bots := make([]*Bot, 0, len(personas))
	for _, p := range personas {
		if ctx.Err() != nil {
			break
		}
		bot, err := client.Signup(ctx, p)
		if err != nil {
			report.Failures++
			continue
		}
		bots = append(bots, bot)
		report.BotsRegistered++
	}

	if len(bots) == 0 {
		report.FinishedAt = time.Now()
		return report, errors.New("traffic: no bots successfully registered")
	}

	// Collect all operations for quick random selection.
	allOps := flattenOps(cfg.Contract)
	if len(allOps) == 0 {
		report.FinishedAt = time.Now()
		return report, errors.New("traffic: contract has no operations")
	}

	// Per-operation timing buckets.
	timings := make(map[string][]int64)
	var mu sync.Mutex // guards report + timings; only the main goroutine writes but future-proofed

	interval := time.Duration(float64(time.Second) / cfg.Rate)
	if interval <= 0 {
		interval = time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	deadline := time.Now().Add(cfg.Duration)

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case now := <-ticker.C:
			if now.After(deadline) {
				break loop
			}
			bot := bots[rng.Intn(len(bots))]
			op := pickOperation(rng, bot.Persona.Traits, allOps)
			payload := fakePayload(rng, op)

			start := time.Now()
			resp, err := client.Execute(ctx, bot, op, payload)
			elapsed := time.Since(start).Milliseconds()

			mu.Lock()
			stats := report.ByOperation[op.ID]
			stats.Attempts++
			report.Operations++
			if err == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 400 {
				stats.Successes++
				report.Successes++
			} else {
				stats.Failures++
				report.Failures++
			}
			report.ByOperation[op.ID] = stats
			timings[op.ID] = append(timings[op.ID], elapsed)
			mu.Unlock()
		}
	}

	// Finalise percentiles.
	for id, samples := range timings {
		stats := report.ByOperation[id]
		stats.P50ms = percentile(samples, 50)
		stats.P95ms = percentile(samples, 95)
		report.ByOperation[id] = stats
	}
	report.FinishedAt = time.Now()
	return report, nil
}

// flattenOps returns every operation in the contract, tagging each
// with its feature name so IDs are globally unique.
func flattenOps(cf *contracts.ContractFile) []contracts.Operation {
	var out []contracts.Operation
	for feature, ops := range cf.Features {
		for _, op := range ops {
			if op.ID == "" {
				op.ID = fmt.Sprintf("%s:%s_%s", feature, op.Method, op.Path)
			}
			out = append(out, op)
		}
	}
	// Sort for determinism inside the pool (selection itself is still
	// randomised via rng).
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// pickOperation weights an operation list by the bot's persona
// traits so different archetypes gravitate to different endpoints.
// A random uniform fallback is used when nothing strongly matches.
func pickOperation(rng *mathrand.Rand, traits []string, ops []contracts.Operation) contracts.Operation {
	traitSet := make(map[string]struct{}, len(traits))
	for _, t := range traits {
		traitSet[t] = struct{}{}
	}

	weights := make([]int, len(ops))
	total := 0
	for i, op := range ops {
		w := 1 // baseline — every op always has some chance
		path := strings.ToLower(op.Path)
		method := strings.ToUpper(op.Method)

		if _, ok := traitSet["lurker"]; ok {
			if method == "GET" {
				w += 6
			}
		}
		if _, ok := traitSet["poster"]; ok {
			if method == "POST" && (strings.Contains(path, "post") || strings.Contains(path, "article") || strings.Contains(path, "listing") || strings.Contains(path, "content")) {
				w += 6
			} else if method == "POST" {
				w += 2
			}
		}
		if _, ok := traitSet["bidder"]; ok {
			if method == "POST" && strings.Contains(path, "bid") {
				w += 8
			}
		}
		if _, ok := traitSet["commenter"]; ok {
			if method == "POST" && strings.Contains(path, "comment") {
				w += 6
			}
		}
		weights[i] = w
		total += w
	}

	if total <= 0 {
		return ops[rng.Intn(len(ops))]
	}
	pick := rng.Intn(total)
	for i, w := range weights {
		if pick < w {
			return ops[i]
		}
		pick -= w
	}
	return ops[len(ops)-1]
}

// fakePayload builds a small map of plausible values for an
// operation. Contracts don't (yet) carry a payload schema so this is
// best-effort: we populate a handful of common field names based on
// the path and method.
func fakePayload(rng *mathrand.Rand, op contracts.Operation) map[string]any {
	payload := map[string]any{}

	// Path params — fill any `:name` placeholder with a random id-ish
	// string so Execute can resolve them.
	for _, seg := range strings.Split(op.Path, "/") {
		if strings.HasPrefix(seg, ":") && len(seg) > 1 {
			payload[seg[1:]] = fmt.Sprintf("seed-%d", rng.Intn(10_000))
		}
	}

	method := strings.ToUpper(op.Method)
	path := strings.ToLower(op.Path)

	if method == "GET" || method == "DELETE" || method == "HEAD" {
		return payload
	}

	// Body-ish heuristics. These are intentionally small and safe.
	if strings.Contains(path, "bid") {
		payload["amount"] = rng.Intn(1000) + 1
	}
	if strings.Contains(path, "comment") {
		payload["body"] = randomSentence(rng)
	}
	if strings.Contains(path, "post") || strings.Contains(path, "article") || strings.Contains(path, "listing") {
		payload["title"] = randomWord(rng) + " " + randomWord(rng)
		payload["body"] = randomSentence(rng)
	}
	if strings.Contains(path, "email") {
		payload["email"] = fmt.Sprintf("bot_sample%d@botseed.local", rng.Intn(10_000))
	}
	// Generic fallback so the body is never empty for mutations.
	if len(payload) == 0 || (method != "GET" && len(payload) == 1) {
		payload["note"] = randomSentence(rng)
	}
	return payload
}

// tiny word bank used for payload generation
var wordBank = []string{
	"apple", "river", "cloud", "mountain", "forest", "ocean", "garden",
	"window", "bridge", "harbor", "meadow", "thunder", "lantern",
	"compass", "signal", "horizon", "canyon", "breeze",
}

func randomWord(rng *mathrand.Rand) string {
	return wordBank[rng.Intn(len(wordBank))]
}

func randomSentence(rng *mathrand.Rand) string {
	n := 4 + rng.Intn(6)
	parts := make([]string, n)
	for i := 0; i < n; i++ {
		parts[i] = wordBank[rng.Intn(len(wordBank))]
	}
	return strings.Join(parts, " ") + "."
}

// percentile returns an approximate Nth percentile from a slice of
// milliseconds. It sorts a copy so the caller's slice is untouched.
func percentile(samples []int64, p int) int64 {
	if len(samples) == 0 {
		return 0
	}
	dup := append([]int64(nil), samples...)
	sort.Slice(dup, func(i, j int) bool { return dup[i] < dup[j] })
	idx := (p * (len(dup) - 1)) / 100
	if idx < 0 {
		idx = 0
	}
	if idx >= len(dup) {
		idx = len(dup) - 1
	}
	return dup[idx]
}
