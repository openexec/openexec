// Package git is the PUBLIC seam exposing the OpenExec git client to
// out-of-tree modules. It re-exports only the branch/push operations a product
// layer needs for PR workflows, so internal/git stays MIT and self-contained
// while the public surface remains small and stable.
package git

import internalgit "github.com/openexec/openexec/internal/git"

// Config configures a Client. Enabled=false makes mutating operations no-ops;
// RepoPath defaults to "." when empty.
type Config struct {
	Enabled  bool
	RepoPath string
}

// Client wraps the OpenExec git client with the subset of operations modules
// need. It is safe to construct with NewClient only.
type Client struct{ inner *internalgit.Client }

// NewClient constructs a git client rooted at cfg.RepoPath.
func NewClient(cfg Config) *Client {
	return &Client{inner: internalgit.NewClient(internalgit.Config{
		Enabled:  cfg.Enabled,
		RepoPath: cfg.RepoPath,
	})}
}

// CurrentBranch returns the currently checked-out branch.
func (c *Client) CurrentBranch() (string, error) { return c.inner.CurrentBranch() }

// PushBranch pushes the named branch to origin.
func (c *Client) PushBranch(branch string) error { return c.inner.PushBranch(branch) }
