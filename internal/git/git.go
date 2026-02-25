// Package git provides git integration for traceability.
// Tracks branches, commits, and their relationships to stories/tasks.
package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

// Client provides git operations for a repository.
type Client struct {
	repoPath string
	enabled  bool
	locker   *flock.Flock
}

// Config holds git integration configuration.
type Config struct {
	Enabled  bool   `json:"enabled"`
	RepoPath string `json:"repo_path,omitempty"`
}

// NewClient creates a new git client.
func NewClient(cfg Config) *Client {
	repoPath := cfg.RepoPath
	if repoPath == "" {
		repoPath = "."
	}
	
	lockPath := filepath.Join(repoPath, ".openexec", "git.lock")
	
	return &Client{
		repoPath: repoPath,
		enabled:  cfg.Enabled,
		locker:   flock.NewFlock(lockPath),
	}
}

// Lock acquires a filesystem-level lock for git operations.
func (c *Client) Lock() error {
	if !c.enabled {
		return nil
	}
	// Create .openexec if it doesn't exist
	dir := filepath.Dir(c.locker.Path())
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	return c.locker.Lock()
}

// Unlock releases the filesystem-level lock.
func (c *Client) Unlock() error {
	if !c.enabled {
		return nil
	}
	return c.locker.Unlock()
}

// IsEnabled returns whether git integration is enabled.
func (c *Client) IsEnabled() bool {
	return c.enabled
}

// IsRepo checks if the path is a git repository.
func (c *Client) IsRepo() bool {
	if !c.enabled {
		return false
	}
	_, err := c.run("rev-parse", "--git-dir")
	return err == nil
}

// CurrentBranch returns the current branch name.
func (c *Client) CurrentBranch() (string, error) {
	if !c.enabled {
		return "", nil
	}
	out, err := c.run("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// BranchExists checks if a branch exists.
func (c *Client) BranchExists(branch string) bool {
	if !c.enabled {
		return false
	}
	_, err := c.run("rev-parse", "--verify", branch)
	return err == nil
}

// CreateBranch creates a new branch from the current HEAD.
func (c *Client) CreateBranch(branch string) error {
	if !c.enabled {
		return nil
	}
	_, err := c.run("checkout", "-b", branch)
	return err
}

// CreateBranchFrom creates a new branch from a specific base branch.
func (c *Client) CreateBranchFrom(branch, baseBranch string) error {
	if !c.enabled {
		return nil
	}
	_, err := c.run("checkout", "-b", branch, baseBranch)
	return err
}

// Checkout switches to a branch.
func (c *Client) Checkout(branch string) error {
	if !c.enabled {
		return nil
	}
	_, err := c.run("checkout", branch)
	return err
}

// LatestCommit returns the latest commit hash on the current branch.
func (c *Client) LatestCommit() (string, error) {
	if !c.enabled {
		return "", nil
	}
	out, err := c.run("rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// CommitsSince returns commits since a specific commit hash.
func (c *Client) CommitsSince(since string) ([]CommitInfo, error) {
	if !c.enabled {
		return nil, nil
	}

	format := "%H|%s|%an|%ae|%aI"
	var out string
	var err error

	if since == "" {
		out, err = c.run("log", "--format="+format)
	} else {
		out, err = c.run("log", since+"..HEAD", "--format="+format)
	}
	if err != nil {
		return nil, err
	}

	return parseCommitLog(out), nil
}

// CommitsOnBranch returns all commits on a branch not on another branch.
func (c *Client) CommitsOnBranch(branch, notOn string) ([]CommitInfo, error) {
	if !c.enabled {
		return nil, nil
	}

	format := "%H|%s|%an|%ae|%aI"
	out, err := c.run("log", notOn+".."+branch, "--format="+format)
	if err != nil {
		return nil, err
	}

	return parseCommitLog(out), nil
}

// CommitInfo holds information about a git commit.
type CommitInfo struct {
	Hash        string    `json:"hash"`
	ShortHash   string    `json:"short_hash"`
	Subject     string    `json:"subject"`
	AuthorName  string    `json:"author_name"`
	AuthorEmail string    `json:"author_email"`
	AuthorDate  time.Time `json:"author_date"`
}

func parseCommitLog(output string) []CommitInfo {
	var commits []CommitInfo
	lines := strings.Split(strings.TrimSpace(output), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 5)
		if len(parts) < 5 {
			continue
		}

		date, _ := time.Parse(time.RFC3339, parts[4])
		hash := parts[0]
		shortHash := hash
		if len(hash) > 7 {
			shortHash = hash[:7]
		}

		commits = append(commits, CommitInfo{
			Hash:        hash,
			ShortHash:   shortHash,
			Subject:     parts[1],
			AuthorName:  parts[2],
			AuthorEmail: parts[3],
			AuthorDate:  date,
		})
	}

	return commits
}

// MergeBranch merges a branch into the current branch.
func (c *Client) MergeBranch(branch string, noFF bool) error {
	if !c.enabled {
		return nil
	}

	args := []string{"merge", branch}
	if noFF {
		args = append(args, "--no-ff")
	}
	args = append(args, "-m", fmt.Sprintf("Merge branch '%s'", branch))

	_, err := c.run(args...)
	return err
}

// CreateTag creates an annotated tag.
func (c *Client) CreateTag(tag, message string) error {
	if !c.enabled {
		return nil
	}
	_, err := c.run("tag", "-a", tag, "-m", message)
	return err
}

// TagExists checks if a tag exists.
func (c *Client) TagExists(tag string) bool {
	if !c.enabled {
		return false
	}
	_, err := c.run("rev-parse", "--verify", "refs/tags/"+tag)
	return err == nil
}

// GetTags returns all tags.
func (c *Client) GetTags() ([]string, error) {
	if !c.enabled {
		return nil, nil
	}
	out, err := c.run("tag", "-l")
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	var tags []string
	for _, line := range lines {
		if line != "" {
			tags = append(tags, line)
		}
	}
	return tags, nil
}

// GetRemoteURL returns the remote URL for origin.
func (c *Client) GetRemoteURL() (string, error) {
	if !c.enabled {
		return "", nil
	}
	out, err := c.run("remote", "get-url", "origin")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// PushTag pushes a tag to the remote.
func (c *Client) PushTag(tag string) error {
	if !c.enabled {
		return nil
	}
	_, err := c.run("push", "origin", tag)
	return err
}

// PushBranch pushes a branch to the remote.
func (c *Client) PushBranch(branch string) error {
	if !c.enabled {
		return nil
	}
	_, err := c.run("push", "-u", "origin", branch)
	return err
}

// FetchBranch fetches a specific branch from origin.
func (c *Client) FetchBranch(branch string) error {
	if !c.enabled {
		return nil
	}
	_, err := c.run("fetch", "origin", branch)
	return err
}

// GetUserIdentity returns the configured git user name and email.
func (c *Client) GetUserIdentity() (name, email string) {
	if !c.enabled {
		return "", ""
	}

	nameOut, err := c.run("config", "user.name")
	if err == nil {
		name = strings.TrimSpace(nameOut)
	}

	emailOut, err := c.run("config", "user.email")
	if err == nil {
		email = strings.TrimSpace(emailOut)
	}

	return name, email
}

// run executes a git command and returns its output.
func (c *Client) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = c.repoPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, stderr.String())
	}

	return stdout.String(), nil
}
