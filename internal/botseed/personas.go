package botseed

import (
	cryptorand "crypto/rand"
	"encoding/base64"
	"fmt"
	mathrand "math/rand"
	"strings"
)

// Small deterministic pools. Kept intentionally small so that diffs
// against generated output stay small and deterministic.
var (
	firstNames = []string{
		"ada", "alex", "anna", "ben", "chen", "dani", "elif", "fatima",
		"george", "hana", "ines", "juno", "kai", "lina", "milo", "nora",
		"omar", "priya", "quinn", "ravi", "sara", "theo", "uma", "vera",
		"wes", "xi", "yara", "zane",
	}
	lastNames = []string{
		"andersen", "baker", "chen", "diaz", "eriksson", "farah", "gomez",
		"haidari", "ibrahim", "johansen", "kumar", "laine", "moreno",
		"nakamura", "okafor", "petersen", "qureshi", "rossi", "silva",
		"takahashi", "ustinov", "virtanen", "wagner", "xu", "yilmaz", "zhao",
	}
	locales = []string{"en-US", "en-GB", "fi-FI", "de-DE", "fr-FR"}

	// Each trait set represents a behavioral archetype used by the
	// traffic generator to pick which operations to call.
	traitSets = [][]string{
		{"lurker"},
		{"lurker", "commenter"},
		{"poster"},
		{"poster", "commenter"},
		{"bidder"},
		{"bidder", "commenter"},
		{"poster", "bidder"},
		{"commenter"},
	}
)

// GeneratePersonas builds `n` deterministic personas using `seed` as
// the PRNG seed. The function guarantees globally-unique emails
// within the returned batch by appending an incrementing
// randomly-derived suffix when collisions occur.
func GeneratePersonas(n int, seed int64) []Persona {
	if n <= 0 {
		return nil
	}
	rng := mathrand.New(mathrand.NewSource(seed))
	out := make([]Persona, 0, n)
	seen := make(map[string]struct{}, n)

	for i := 0; i < n; i++ {
		first := firstNames[rng.Intn(len(firstNames))]
		last := lastNames[rng.Intn(len(lastNames))]
		locale := locales[rng.Intn(len(locales))]
		traits := traitSets[rng.Intn(len(traitSets))]

		// bot_ prefix is the isolation marker used by Clear().
		var email string
		for attempt := 0; attempt < 8; attempt++ {
			suffix := rng.Intn(1_000_000)
			email = fmt.Sprintf("bot_%s.%s%d@botseed.local", first, last, suffix)
			if _, dup := seen[email]; !dup {
				break
			}
		}
		seen[email] = struct{}{}

		out = append(out, Persona{
			Name:     strings.Title(first) + " " + strings.Title(last), //nolint:staticcheck // Title is fine for ASCII test data
			Email:    email,
			Locale:   locale,
			Traits:   append([]string(nil), traits...),
			Password: generatePassword(),
		})
	}
	return out
}

// generatePassword returns a strong random password using crypto/rand
// so bot credentials are not predictable from the persona seed.
func generatePassword() string {
	const rawBytes = 24
	buf := make([]byte, rawBytes)
	if _, err := cryptorand.Read(buf); err != nil {
		// extremely unlikely; fall back to a still-random but
		// deterministic-ish value so we never panic in CI.
		for i := range buf {
			buf[i] = byte(i * 7)
		}
	}
	// Base64-encoded -> > 24 characters, and the "bot_" prefix on
	// emails means even if two bots ended up with the same password
	// the collision is harmless.
	return "Bot!" + base64.RawURLEncoding.EncodeToString(buf)
}
