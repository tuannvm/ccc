package watch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	configpkg "github.com/tuannvm/ccc/pkg/config"
)

type CCCRepoResolver struct {
	Config *configpkg.Config
}

func (r CCCRepoResolver) Resolve(ctx context.Context, repoRef string) (ResolvedRepo, error) {
	repoRef = strings.TrimSpace(repoRef)
	if repoRef == "" {
		return ResolvedRepo{}, fmt.Errorf("missing repo field")
	}
	if configpkg.IsGitURL(repoRef) {
		name := configpkg.ExtractRepoName(repoRef)
		if name == "" {
			return ResolvedRepo{}, fmt.Errorf("invalid git URL: could not extract repository name")
		}
		workDir := filepath.Join(configpkg.GetProjectsDir(r.Config), name)
		cloneCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if _, err := configpkg.CloneRepo(cloneCtx, repoRef, workDir); err != nil {
			return ResolvedRepo{}, fmt.Errorf("resolve git repo %s: %w", configpkg.RedactGitURL(repoRef), err)
		}
		return ResolvedRepo{Path: workDir, Name: name}, nil
	}

	path := configpkg.ExpandPath(repoRef)
	if !filepath.IsAbs(path) {
		path = configpkg.ResolveProjectPath(r.Config, path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return ResolvedRepo{}, fmt.Errorf("repo path %s: %w", path, err)
	}
	if !info.IsDir() {
		return ResolvedRepo{}, fmt.Errorf("repo path is not a directory: %s", path)
	}
	return ResolvedRepo{Path: path, Name: filepath.Base(path)}, nil
}

func SessionName(ticketRef, repoName string) string {
	return safePart(ticketRef) + "-" + safePart(repoName)
}

func safePart(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "ticket"
	}
	return out
}
