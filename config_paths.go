package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// configDir returns ~/.config/ccc (created if needed)
func configDir() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".config", "ccc")
	os.MkdirAll(dir, 0755)
	return dir
}

// cacheDir returns ~/Library/Caches/ccc (created if needed)
func cacheDir() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, "Library", "Caches", "ccc")
	os.MkdirAll(dir, 0755)
	return dir
}

// getConfigPath returns the path to config.json, with migration from old location
func getConfigPath() string {
	// Migrate from old path if needed
	home, _ := os.UserHomeDir()
	oldPath := filepath.Join(home, ".ccc.json")
	newPath := filepath.Join(configDir(), "config.json")
	if _, err := os.Stat(newPath); os.IsNotExist(err) {
		if _, err := os.Stat(oldPath); err == nil {
			data, _ := os.ReadFile(oldPath)
			os.WriteFile(newPath, data, 0600)
			os.Remove(oldPath)
		}
	}
	return newPath
}

// getProjectsDir returns the base directory for projects
func getProjectsDir(config *Config) string {
	if config.ProjectsDir != "" {
		// Expand ~ to home directory
		if strings.HasPrefix(config.ProjectsDir, "~/") {
			home, _ := os.UserHomeDir()
			return filepath.Join(home, config.ProjectsDir[2:])
		}
		return config.ProjectsDir
	}
	// Default to ~/Projects
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Projects")
}

// resolveProjectPath resolves the full path for a project
// If name starts with / or ~/, it's treated as absolute/home-relative path
// Otherwise, it's relative to projects_dir
func resolveProjectPath(config *Config, name string) string {
	// Absolute path
	if strings.HasPrefix(name, "/") {
		return name
	}
	// Home-relative path (~/something or just ~)
	if strings.HasPrefix(name, "~/") || name == "~" {
		home, _ := os.UserHomeDir()
		if name == "~" {
			return home
		}
		return filepath.Join(home, name[2:])
	}
	// Relative to projects_dir
	return filepath.Join(getProjectsDir(config), name)
}

// expandPath expands ~ to home directory
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

// isGitURL detects if the input string is a git repository URL
func isGitURL(s string) bool {
	// HTTPS URLs
	if strings.HasPrefix(s, "https://") {
		return true
	}
	// SSH URLs (git@host:user/repo.git or ssh://git@host/repo)
	if strings.HasPrefix(s, "git@") || strings.HasPrefix(s, "ssh://") {
		return true
	}
	// Git protocol
	if strings.HasPrefix(s, "git://") {
		return true
	}
	// SCP-style SSH URLs (user@host:path/repo.git) - detect pattern: contains @ followed by : later
	// This catches formats like alice@git.example.com:team/repo.git
	if atIdx := strings.Index(s, "@"); atIdx > 0 {
		// Check if there's a colon after the @ sign (but not part of a protocol like http://)
		if colonIdx := strings.Index(s[atIdx:], ":"); colonIdx > 0 {
			// Make sure there's no :// before the @ (which would indicate a protocol)
			if !strings.Contains(s[:atIdx], "://") {
				return true
			}
		}
	}
	return false
}

// redactGitURL removes credentials from git URLs for safe display
// Handles: https://user:pass@host/repo, https://token@host/repo, ssh://user:pass@host/repo
// Returns a safe URL with credentials removed
func redactGitURL(url string) string {
	// HTTPS/HTTP URLs with credentials
	if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://") {
		// Find the @ after the protocol prefix
		rest := url
		protocol := ""
		if strings.HasPrefix(rest, "https://") {
			rest = rest[8:]
			protocol = "https://"
		} else if strings.HasPrefix(rest, "http://") {
			rest = rest[7:]
			protocol = "http://"
		}

		if atIdx := strings.Index(rest, "@"); atIdx > 0 {
			// Found credentials - extract host and rebuild URL
			hostPart := rest[atIdx+1:]
			return protocol + hostPart
		}
	}

	// SSH URLs with credentials (ssh://user:pass@host/repo)
	if strings.HasPrefix(url, "ssh://") {
		rest := url[6:] // Strip "ssh://"
		if atIdx := strings.Index(rest, "@"); atIdx > 0 {
			// Found credentials - extract host and rebuild URL
			hostPart := rest[atIdx+1:]
			return "ssh://" + hostPart
		}
	}

	// SCP-style URLs (git@host:repo or user@host:repo)
	// These are already safe (@ is the standard format, not a credential)
	// git@, user@ format doesn't include passwords

	return url
}

// redactGitURLsInText finds and redacts credentials in any git URLs within text
// Returns text with credentials removed from HTTP/HTTPS/SSH URLs
func redactGitURLsInText(text string) string {
	var result strings.Builder
	remaining := text

	for {
		// Find next https://, http://, or ssh:// (whichever comes first)
		var prefixIdx int = -1
		prefixLen := 0

		httpsIdx := strings.Index(remaining, "https://")
		httpIdx := strings.Index(remaining, "http://")
		sshIdx := strings.Index(remaining, "ssh://")

		// Pick the earliest match
	 earliestIdx := -1
		if httpsIdx >= 0 && (earliestIdx == -1 || httpsIdx < earliestIdx) {
			earliestIdx = httpsIdx
		}
		if httpIdx >= 0 && (earliestIdx == -1 || httpIdx < earliestIdx) {
			earliestIdx = httpIdx
		}
		if sshIdx >= 0 && (earliestIdx == -1 || sshIdx < earliestIdx) {
			earliestIdx = sshIdx
		}

		if earliestIdx >= 0 {
			prefixIdx = earliestIdx
			if prefixIdx == httpsIdx {
				prefixLen = 8
			} else if prefixIdx == httpIdx {
				prefixLen = 7
			} else if prefixIdx == sshIdx {
				prefixLen = 6
			}
		}

		if prefixIdx == -1 {
			// No more URLs, append remaining text and return
			result.WriteString(remaining)
			break
		}

		// Append text before this URL
		result.WriteString(remaining[:prefixIdx])

		// Find end of URL (space or end of string)
		urlStart := prefixIdx
		urlEnd := strings.IndexAny(remaining[prefixIdx:], " \n")
		if urlEnd == -1 {
			urlEnd = len(remaining)
		} else {
			urlEnd += prefixIdx
		}

		url := remaining[urlStart:urlEnd]

		// Check if URL contains @ after protocol (has credentials)
		if atIdx := strings.Index(url, "@"); atIdx > prefixLen {
			// Redact credentials
			credLessURL := redactGitURL(url)
			result.WriteString(credLessURL)
		} else {
			// No credentials, keep original URL
			result.WriteString(url)
		}

		// Move past this URL
		if urlEnd >= len(remaining) {
			break
		}
		remaining = remaining[urlEnd:]
	}

	return result.String()
}

// extractRepoName extracts the repository name from a git URL
// Handles URLs like:
// - https://github.com/user/repo.git -> user-repo
// - https://github.com/user/repo -> user-repo
// - https://github.com/user/repo/ -> user-repo (trailing slash handled)
// - https://gitlab.com/org/subgroup/repo.git -> org-subgroup-repo (full namespace)
// - ssh://git@host:2222/user/repo.git -> user-repo (SSH with port)
// - git@github.com:user/repo.git -> user-repo
// - git@github.com:user/repo -> user-repo
// - alice@git.example.com:team/repo.git -> team-repo (generic SCP URLs)
// Returns empty string for malformed URLs or unsafe paths (., .., contains slashes)
func extractRepoName(url string) string {
	// Remove .git suffix if present
	url = strings.TrimSuffix(url, ".git")

	// Remove trailing slashes to handle URLs like "https://github.com/user/repo/"
	url = strings.TrimSuffix(url, "/")

	var namespace []string
	var repoName string

	// Handle ssh:// URLs (including those with ports like ssh://git@host:2222/path)
	if strings.HasPrefix(url, "ssh://") {
		// Strip ssh:// prefix and parse as standard URL path
		rest := url[6:] // Strip "ssh://"
		// Find the path part (everything after the host:port or host)
		if slashIdx := strings.Index(rest, "/"); slashIdx >= 0 {
			pathPart := rest[slashIdx+1:] // Get path after host/port
			parts := strings.Split(pathPart, "/")
			if len(parts) > 0 {
				repoName = parts[len(parts)-1]
			}
			if len(parts) > 1 {
				namespace = parts[:len(parts)-1]
			}
		}
		// If we found a repo name, skip to validation
		if repoName != "" {
			goto validateAndReturn
		}
	}

	// For SSH URLs with colon (SCP-style: user@host:path/repo), find the last colon
	// Only apply if NOT an ssh:// URL (those are handled above)
	if !strings.HasPrefix(url, "ssh://") {
		if atIdx := strings.Index(url, "@"); atIdx > 0 {
			// Check if there's a colon after the @ (SCP-style URL)
			afterAt := url[atIdx+1:]
			if colonIdx := strings.Index(afterAt, ":"); colonIdx > 0 {
				// Check if this is SCP-style (colon before slash, not a URL with port)
				slashIdx := strings.Index(afterAt, "/")
				if slashIdx == -1 || colonIdx < slashIdx {
					// SCP-style: user@host:path/repo
					pathPart := afterAt[colonIdx+1:] // Get everything after the colon (e.g., "user/repo" or "org/team/repo")
					// Split the path to get namespace parts and repo name
					parts := strings.Split(pathPart, "/")
					if len(parts) > 0 {
						repoName = parts[len(parts)-1]
					}
					if len(parts) > 1 {
						// Get all segments except the last as namespace
						namespace = parts[:len(parts)-1]
					}
				}
			}
		}
	}

	// If we didn't find SCP-style colon, handle HTTPS and SSH-with-slash formats
	if repoName == "" {
		if idx := strings.LastIndex(url, "/"); idx > 0 {
			repoName = url[idx+1:] // Get the repo name
			// Get everything before the repo name
			beforeRepo := url[:idx]
			// Find the first slash after the host to get the full namespace
			// First, find the host (everything after :// and before the first /)
			hostEnd := -1
			if protoIdx := strings.Index(beforeRepo, "://"); protoIdx >= 0 {
				hostEnd = strings.Index(beforeRepo[protoIdx+3:], "/")
				if hostEnd >= 0 {
					hostEnd += protoIdx + 3
				}
			}

			var namespaceStr string
			if hostEnd >= 0 && hostEnd < idx {
				namespaceStr = beforeRepo[hostEnd+1:]
			} else {
				namespaceStr = beforeRepo
			}

			if namespaceStr != "" {
				namespace = strings.Split(namespaceStr, "/")
			}
		}
	}

validateAndReturn:
	// Validate: require at least 2 path segments (at least one namespace level + repo name)
	// Reject malformed URLs like "https://github.com/repo.git" (missing org/user)
	if repoName == "" || len(namespace) == 0 {
		return ""
	}

	// Combine namespace parts with hyphens, then add repo name
	// e.g., ["org", "subgroup"] + "repo" -> "org-subgroup-repo"
	var result string
	if len(namespace) > 0 {
		result = strings.Join(namespace, "-") + "-" + repoName
	} else {
		result = repoName
	}

	// Reject path traversal components and unsafe values
	if result == "." || result == ".." || strings.Contains(result, "/") || strings.Contains(result, "\\") {
		return ""
	}

	return result
}

// CloneResult represents the outcome of a clone operation
type CloneResult int

const (
	// CloneResultCloned indicates a new repository was cloned
	CloneResultCloned CloneResult = iota
	// CloneResultAlreadyExists indicates the repository already exists as the same git repo
	CloneResultAlreadyExists
)

// cloneRepo clones a git repository to the specified path
// Returns (CloneResult, nil) on success, with CloneResult indicating if it was newly cloned or already existed
// Returns an error if cloning fails or if directory exists with unexpected content
// Uses context for timeout control to prevent blocking indefinitely
func cloneRepo(ctx context.Context, url, targetPath string) (CloneResult, error) {
	// Check if directory already exists
	if _, err := os.Stat(targetPath); err == nil {
		// Directory exists, check if it's a git repo
		gitCmd := exec.CommandContext(ctx, "git", "-C", targetPath, "rev-parse", "--git-dir")
		if err := gitCmd.Run(); err != nil {
			// Directory exists but is not a git repository - unsafe to proceed
			return CloneResultCloned, fmt.Errorf("directory exists but is not a git repository: %s", targetPath)
		}

		// It is a git repo - verify remote matches to avoid silently using wrong repo
		// Get the origin remote URL
		gitCmd = exec.CommandContext(ctx, "git", "-C", targetPath, "remote", "get-url", "origin")
		output, err := gitCmd.Output()
		if err != nil {
			// Directory is a git repo but has no origin remote - unsafe to proceed
			return CloneResultCloned, fmt.Errorf("directory is a git repository but has no origin remote: %s", targetPath)
		}

		existingRemote := strings.TrimSpace(string(output))
		// Normalize URLs for comparison (remove .git suffix, trailing slashes, normalize protocols)
		normalizeURL := func(u string) string {
			u = strings.TrimSuffix(strings.TrimSuffix(u, "/"), ".git")
			// Remove protocol prefixes FIRST (before handling SSH @ syntax)
			u = strings.TrimPrefix(u, "https://")
			u = strings.TrimPrefix(u, "http://")
			u = strings.TrimPrefix(u, "ssh://")
			u = strings.TrimPrefix(u, "git://")
			// Strip credentials from all URLs (user@ or user:pass@)
			// Handles: git@host:path, token@host/path, user:pass@host/path
			if atIdx := strings.Index(u, "@"); atIdx > 0 {
				afterAt := u[atIdx+1:]
				// Check if this is SCP-style (has colon before any slash)
				colonIdx := strings.Index(afterAt, ":")
				slashIdx := strings.Index(afterAt, "/")
				if colonIdx > 0 && (slashIdx == -1 || colonIdx < slashIdx) {
					// SCP-style: user@host:path -> host/path
					u = afterAt // Strip username@
					u = u[:colonIdx] + "/" + u[colonIdx+1:] // Replace colon with slash
				} else {
					// HTTPS-style or SSH-style with slash: user@host/path -> host/path
					u = afterAt // Strip username@, path already has slashes
				}
			}
			return u
		}

		if normalizeURL(existingRemote) != normalizeURL(url) {
			// Different remote - warn but don't fail (user may have intentionally forked/renamed)
			// Return a specific error that caller can handle
			return CloneResultCloned, fmt.Errorf("directory exists as a different git repository (has %s, want %s)", existingRemote, url)
		}

		// Same repository, skip cloning
		return CloneResultAlreadyExists, nil
	}

	// Create parent directory if needed
	parentDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return CloneResultCloned, err
	}

	// Clone the repository
	gitCmd := exec.CommandContext(ctx, "git", "clone", url, targetPath)
	if err := gitCmd.Run(); err != nil {
		return CloneResultCloned, err
	}

	return CloneResultCloned, nil
}
