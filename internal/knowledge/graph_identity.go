package knowledge

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// EnsureRepositoryIdentity resolves or creates the repository, checkout, and
// worktree identities for one local root. repositoryHint may link a new clone
// to a repository identity already known by the caller.
func (s *Store) EnsureRepositoryIdentity(ctx context.Context, rootPath, repositoryHint string) (RepositoryIdentity, error) {
	return s.EnsureRepositoryIdentityWithOptions(ctx, rootPath, RepositoryIdentityOptions{RepositoryHint: repositoryHint})
}

type RepositoryIdentityOptions struct {
	RepositoryHint       string
	ForkedFromRepository string
}

// EnsureRepositoryIdentityWithOptions makes fork lineage explicit. A fork is
// never inferred merely because two roots look similar.
func (s *Store) EnsureRepositoryIdentityWithOptions(ctx context.Context, rootPath string, options RepositoryIdentityOptions) (RepositoryIdentity, error) {
	root, err := filepath.Abs(rootPath)
	if err != nil {
		return RepositoryIdentity{}, fmt.Errorf("resolve repository root: %w", err)
	}
	if evaluated, evalErr := filepath.EvalSymlinks(root); evalErr == nil {
		root = evaluated
	}
	root = filepath.Clean(root)

	var identity RepositoryIdentity
	err = s.db.QueryRowContext(ctx, `
		SELECT r.id, c.id, w.id, w.root_path
		FROM worktrees w
		JOIN checkouts c ON c.id = w.checkout_id
		JOIN repositories r ON r.id = w.repository_id
		WHERE w.root_path = ? AND w.removed_at IS NULL`, root).
		Scan(&identity.RepositoryID, &identity.CheckoutID, &identity.WorktreeID, &identity.RootPath)
	if err == nil {
		identity.BaseCommit = gitOutput(ctx, root, "rev-parse", "HEAD")
		return identity, nil
	}
	if err != sql.ErrNoRows {
		return RepositoryIdentity{}, fmt.Errorf("lookup repository identity: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RepositoryIdentity{}, fmt.Errorf("begin repository identity: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	remote := normalizeRemote(gitOutput(ctx, root, "config", "--get", "remote.origin.url"))
	gitDir := resolvedGitDirectory(ctx, root, "--git-common-dir")
	repositoryID := options.RepositoryHint
	checkoutID := ""
	if repositoryID != "" {
		var exists int
		if scanErr := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM repositories WHERE id = ?`, repositoryID).Scan(&exists); scanErr != nil {
			return RepositoryIdentity{}, fmt.Errorf("check repository hint: %w", scanErr)
		}
		if exists == 0 {
			repositoryID = ""
		}
	}
	if options.ForkedFromRepository != "" {
		var parentExists int
		if scanErr := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM repositories WHERE id = ?`, options.ForkedFromRepository).Scan(&parentExists); scanErr != nil || parentExists != 1 {
			return RepositoryIdentity{}, fmt.Errorf("fork parent repository is not known")
		}
		// A fork is a new logical source lineage even when a discovery alias is
		// shared or missing.
		repositoryID = ""
	}
	if repositoryID == "" && gitDir != "" && options.ForkedFromRepository == "" {
		_ = tx.QueryRowContext(ctx, `SELECT repository_id, checkout_id FROM worktrees WHERE git_dir = ? AND removed_at IS NULL ORDER BY created_at LIMIT 1`, gitDir).Scan(&repositoryID, &checkoutID)
	}
	if repositoryID == "" && remote != "" && options.ForkedFromRepository == "" {
		// A normalized remote is a discovery hint, never the repository's
		// primary identity. Reuse it only when it names exactly one known lineage.
		rows, queryErr := tx.QueryContext(ctx, `SELECT DISTINCT repository_id FROM repository_aliases WHERE alias_type = 'remote' AND alias_value = ? ORDER BY repository_id LIMIT 2`, remote)
		if queryErr != nil {
			return RepositoryIdentity{}, queryErr
		}
		var matches []string
		for rows.Next() {
			var match string
			if scanErr := rows.Scan(&match); scanErr != nil {
				rows.Close()
				return RepositoryIdentity{}, scanErr
			}
			matches = append(matches, match)
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			rows.Close()
			return RepositoryIdentity{}, rowsErr
		}
		if closeErr := rows.Close(); closeErr != nil {
			return RepositoryIdentity{}, closeErr
		}
		if len(matches) == 1 {
			repositoryID = matches[0]
		}
	}
	if repositoryID == "" {
		persisted := uuid.NewString()
		repositoryID = "repo_" + persisted
		if _, err := tx.ExecContext(ctx, `INSERT INTO repositories (id, persisted_uuid, discovery_remote, forked_from_repository_id) VALUES (?, ?, ?, NULLIF(?, ''))`, repositoryID, persisted, remote, options.ForkedFromRepository); err != nil {
			return RepositoryIdentity{}, fmt.Errorf("create repository identity: %w", err)
		}
	}
	if remote != "" {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO repository_aliases (repository_id, alias_type, alias_value) VALUES (?, 'remote', ?)`, repositoryID, remote); err != nil {
			return RepositoryIdentity{}, fmt.Errorf("record repository alias: %w", err)
		}
	}

	if checkoutID == "" {
		checkoutID = "checkout_" + uuid.NewString()
		if _, err := tx.ExecContext(ctx, `INSERT INTO checkouts (id, repository_id, root_path) VALUES (?, ?, ?)`, checkoutID, repositoryID, root); err != nil {
			return RepositoryIdentity{}, fmt.Errorf("create checkout identity: %w", err)
		}
	}
	worktreeID := "worktree_" + uuid.NewString()
	if _, err := tx.ExecContext(ctx, `INSERT INTO worktrees (id, repository_id, checkout_id, root_path, git_dir) VALUES (?, ?, ?, ?, ?)`, worktreeID, repositoryID, checkoutID, root, gitDir); err != nil {
		return RepositoryIdentity{}, fmt.Errorf("create worktree identity: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return RepositoryIdentity{}, fmt.Errorf("commit repository identity: %w", err)
	}

	return RepositoryIdentity{
		RepositoryID: repositoryID,
		CheckoutID:   checkoutID,
		WorktreeID:   worktreeID,
		RootPath:     root,
		BaseCommit:   gitOutput(ctx, root, "rev-parse", "HEAD"),
	}, nil
}

func resolvedGitDirectory(ctx context.Context, root, argument string) string {
	directory := gitOutput(ctx, root, "rev-parse", argument)
	if directory == "" {
		return ""
	}
	if directory != "" && !filepath.IsAbs(directory) {
		directory = filepath.Join(root, directory)
	}
	if evaluated, err := filepath.EvalSymlinks(directory); err == nil {
		directory = evaluated
	}
	return filepath.Clean(directory)
}

func gitOutput(ctx context.Context, root string, args ...string) string {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func normalizeRemote(remote string) string {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return ""
	}
	if parsed, err := url.Parse(remote); err == nil && parsed.Host != "" {
		return strings.ToLower(strings.TrimSuffix(parsed.Host+"/"+strings.TrimPrefix(parsed.Path, "/"), ".git"))
	}
	// Git's scp-like form: user@host:owner/repository.git.
	if at, colon := strings.LastIndex(remote, "@"), strings.Index(remote, ":"); at >= 0 && colon > at {
		return strings.ToLower(strings.TrimSuffix(remote[at+1:colon]+"/"+remote[colon+1:], ".git"))
	}
	return strings.ToLower(strings.TrimSuffix(filepath.ToSlash(filepath.Clean(remote)), ".git"))
}
