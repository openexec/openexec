package planner

import (
	"regexp"
	"strings"
)

// falseGreenPatterns are verification-script shapes that report success even
// when the real check failed. Detecting them deterministically complements the
// plan prompt (which asks the model to avoid them) and the AI reviewers (which
// only sometimes catch them) — the reviewer recommended a deterministic linter.
var falseGreenPatterns = []struct {
	name string
	re   *regexp.Regexp
	// orElse marks a pattern whose match ends at `||`: it is no mask when the
	// fallback after it fails the script itself.
	orElse bool
}{
	{"masks failure: a test/assert command followed by `|| <fallback>` (the fallback can pass while the real check failed)",
		regexp.MustCompile(`(?i)(vitest|jest|pytest|\bnpm test\b|\bgo test\b|\bgrep\b)[^\n|]*\|\|`), true},
	{"hides errors: `2>/dev/null` on the checked command",
		regexp.MustCompile(`2>\s*/dev/null`), false},
	// A pipe is one `|`; `grep -q x || exit 1` keeps the exit status. The
	// pipe must belong to the grep's own command: `&&` and `;` end it, so
	// `grep -q x f && go tool cover -func=p | python3 ...` pipes only cover.
	{"discards exit status: a quiet grep (`grep -q`) piped into another command",
		regexp.MustCompile(`grep\s+-\w*q\w*\b[^\n|&;]*\|(?:[^|]|\z)`), false},
	{"masks failure: assertions chained as `A && B || C` (C passing hides an A/B failure)",
		regexp.MustCompile(`&&[^\n|]*\|\|`), true},
}

// failingFallback is a fallback that fails the script: `|| exit 1`,
// `|| false`, `|| return 1`, or a `{ ...; }` group ending in one of them.
// `cmd || { echo "missing $k"; exit 1; }` reports the failure loudly; it is
// the idiom the lint asks for, not a mask.
var failingFallback = regexp.MustCompile(`^\s*(?:\{(?:[^{}\n]*?;)?\s*` + failCommand + `\s*;?\s*\}|` + failCommand + `\s*(?:;|\)|\n|\z))`)

const failCommand = `(?:exit\s+[1-9][0-9]*|false|return\s+[1-9][0-9]*)`

// StaleBaseRefIssue reports a verification script that diffs against a bare
// local `main`/`master`. OpenExec runs tasks in linked candidate worktrees
// synced to origin/<default>, where the local branch of that name is stale or
// missing, so such a script fails for correct work or passes for wrong work.
// Unlike LintVerificationScript this is a hard gate, not a warning, so it only
// inspects executable git revision arguments: the script is tokenized as shell,
// and main/master in comments, in data for other commands, as option values or
// as pathspecs after `--` are not refs. Returns "" when the script is acceptable.
func StaleBaseRefIssue(script string) string {
	for _, argv := range shellCommands(script) {
		for _, rev := range gitRevisionArgs(argv) {
			for _, ref := range revisionEndpoints(rev) {
				if ref == "main" || ref == "master" {
					return "verification script diffs against the bare local `" + ref + "` ref; use `origin/" + ref +
						"` — candidate worktrees are synced to origin/<default>, where the local `" + ref + "` is stale or missing"
				}
			}
		}
	}
	return ""
}

// gitRevisionSubcommands are the git subcommands whose positional arguments
// before `--` are revisions.
var gitRevisionSubcommands = map[string]bool{
	"diff": true, "log": true, "show": true, "merge-base": true, "rev-list": true,
	"rev-parse": true, "cherry": true, "range-diff": true,
}

// gitGlobalOptionsWithValue take their value as the next argument.
var gitGlobalOptionsWithValue = map[string]bool{
	"-C": true, "-c": true, "--git-dir": true, "--work-tree": true, "--namespace": true,
}

// gitOptionsWithValue are subcommand options whose separate next argument is a
// count, pattern or date — never a revision.
var gitOptionsWithValue = map[string]bool{
	"-n": true, "--max-count": true, "--skip": true, "--grep": true, "--author": true,
	"--committer": true, "-S": true, "-G": true, "-L": true, "-O": true,
	"--since": true, "--until": true, "--before": true, "--after": true,
}

// shellCommandPrefixes may precede a command's real argv[0].
var shellCommandPrefixes = map[string]bool{
	"!": true, "{": true, "if": true, "then": true, "else": true, "elif": true,
	"while": true, "until": true, "do": true, "time": true, "command": true, "exec": true,
}

// gitRevisionArgs returns the revision arguments of one command's argv when it
// runs a revision-taking git subcommand, else nil.
func gitRevisionArgs(argv []string) []string {
	i := 0
	for i < len(argv) && (shellCommandPrefixes[argv[i]] || isShellAssignment(argv[i])) {
		i++
	}
	if i >= len(argv) || argv[i] != "git" {
		return nil
	}
	for i++; i < len(argv) && strings.HasPrefix(argv[i], "-"); i++ {
		if gitGlobalOptionsWithValue[argv[i]] {
			i++
		}
	}
	if i >= len(argv) || !gitRevisionSubcommands[argv[i]] {
		return nil
	}
	var revs []string
	for i++; i < len(argv); i++ {
		arg := argv[i]
		if arg == "--" {
			break
		}
		if strings.HasPrefix(arg, "-") {
			if gitOptionsWithValue[arg] {
				i++
			}
			continue
		}
		revs = append(revs, arg)
	}
	return revs
}

// isShellAssignment reports a `NAME=value` word prefixing a command.
func isShellAssignment(word string) bool {
	eq := strings.IndexByte(word, '=')
	if eq <= 0 {
		return false
	}
	for j, c := range word[:eq] {
		if !(c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (j > 0 && c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// revisionEndpoints splits a revision argument on `...`/`..` and strips each
// endpoint's leading `^` exclusion and its `^`/`~N`/`@{..}`/`:path` suffixes,
// so `^main`, `main~2..HEAD` and `master@{u}...HEAD` all yield the bare name.
func revisionEndpoints(rev string) []string {
	var out []string
	for _, ep := range strings.Split(strings.ReplaceAll(rev, "...", ".."), "..") {
		ep = strings.TrimPrefix(ep, "^")
		if k := strings.Index(ep, "@{"); k >= 0 {
			ep = ep[:k]
		}
		if k := strings.IndexAny(ep, "^~:"); k >= 0 {
			ep = ep[:k]
		}
		out = append(out, ep)
	}
	return out
}

// shellCommands is a small shell tokenizer returning the argv of every simple
// command in script. It honours single/double quotes and backslash escapes,
// drops `#` comments that start a word outside quotes, drops redirection
// targets, and splits commands on newline, `;`, `&&`, `||`, `|`, `&`, `(`,
// `)`, `$(` and backticks — including substitutions inside double quotes,
// whose contents run. It is not a full shell parser (no heredocs or
// expansion); it only has to find git's argv reliably.
func shellCommands(script string) [][]string {
	type frame struct {
		backtick bool // opened by a backtick rather than `$(` or `(`
		inDQ     bool // the enclosing context was inside double quotes
	}
	var (
		cmds     [][]string
		argv     []string
		word     strings.Builder
		inWord   bool // a word is open, possibly empty (`''`)
		inDQ     bool
		skipNext bool // the next word is a redirection target
		stack    []frame
	)
	endWord := func() {
		if inWord {
			if skipNext {
				skipNext = false
			} else {
				argv = append(argv, word.String())
			}
		}
		word.Reset()
		inWord = false
	}
	endCommand := func() {
		endWord()
		skipNext = false
		if len(argv) > 0 {
			cmds = append(cmds, argv)
		}
		argv = nil
	}
	openSub := func(backtick bool) {
		endCommand()
		stack = append(stack, frame{backtick: backtick, inDQ: inDQ})
		inDQ = false
	}
	closeSub := func() {
		endCommand()
		if n := len(stack); n > 0 {
			inDQ = stack[n-1].inDQ
			stack = stack[:n-1]
		}
	}
	for i := 0; i < len(script); i++ {
		c := script[i]
		switch {
		case c == '\\':
			if i+1 >= len(script) {
				continue
			}
			i++
			next := script[i]
			if next == '\n' {
				continue // line continuation
			}
			if inDQ && strings.IndexByte("$`\"\\", next) < 0 {
				word.WriteByte('\\')
			}
			word.WriteByte(next)
			inWord = true
		case c == '$' && i+1 < len(script) && script[i+1] == '(':
			i++
			openSub(false)
		case c == '`':
			if n := len(stack); n > 0 && stack[n-1].backtick && !inDQ {
				closeSub()
			} else {
				openSub(true)
			}
		case inDQ:
			if c == '"' {
				inDQ = false
			} else {
				word.WriteByte(c)
			}
			inWord = true
		case c == '"':
			inDQ = true
			inWord = true
		case c == '\'':
			inWord = true
			for i++; i < len(script) && script[i] != '\''; i++ {
				word.WriteByte(script[i])
			}
		case c == '#' && !inWord:
			for i+1 < len(script) && script[i+1] != '\n' {
				i++
			}
		case c == ' ' || c == '\t' || c == '\r':
			endWord()
		case c == '\n' || c == ';' || c == '&' || c == '|':
			endCommand()
		case c == '(':
			openSub(false)
		case c == ')':
			closeSub()
		case c == '<' || c == '>':
			if isFileDescriptor(word.String()) {
				word.Reset()
				inWord = false
			} else {
				endWord()
			}
			for i+1 < len(script) && strings.IndexByte("<>&|", script[i+1]) >= 0 {
				i++
			}
			skipNext = true
		default:
			word.WriteByte(c)
			inWord = true
		}
	}
	endCommand()
	return cmds
}

// isFileDescriptor reports an all-digit word such as the `2` in `2>file`.
func isFileDescriptor(word string) bool {
	if word == "" {
		return false
	}
	for _, c := range word {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// LintVerificationScript returns human-readable descriptions of false-green
// anti-patterns found in a verification script. An empty result means none of
// the known anti-patterns matched — not a guarantee of soundness.
func LintVerificationScript(script string) []string {
	if strings.TrimSpace(script) == "" {
		return nil
	}
	var issues []string
	for _, p := range falseGreenPatterns {
		for _, loc := range p.re.FindAllStringIndex(script, -1) {
			if p.orElse && failingFallback.MatchString(script[loc[1]:]) {
				continue
			}
			issues = append(issues, p.name)
			break
		}
	}
	return issues
}

// LintPlanVerification lints every verification script in a plan and returns
// warnings keyed by the owning story/task id, so a caller can surface them for
// human review before approval.
func LintPlanVerification(plan *ProjectPlan) map[string][]string {
	if plan == nil {
		return nil
	}
	out := map[string][]string{}
	for _, st := range plan.Stories {
		if issues := LintVerificationScript(st.VerificationScript); len(issues) > 0 {
			out[st.ID] = issues
		}
		for _, tk := range st.Tasks {
			if issues := LintVerificationScript(tk.VerificationScript); len(issues) > 0 {
				out[tk.ID] = issues
			}
		}
	}
	return out
}
