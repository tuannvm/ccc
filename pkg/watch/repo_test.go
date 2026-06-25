package watch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	configpkg "github.com/tuannvm/ccc/pkg/config"
)

func TestRepoResolverLocalAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	got, err := (CCCRepoResolver{}).Resolve(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != dir || got.Name != filepath.Base(dir) {
		t.Fatalf("got %+v", got)
	}
}

func TestRepoResolverHomeRelativePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, "src", "repo")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	got, err := (CCCRepoResolver{}).Resolve(context.Background(), "~/src/repo")
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != dir || got.Name != "repo" {
		t.Fatalf("got %+v", got)
	}
}

func TestRepoResolverRelativePathUsesProjectsDir(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "repo")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	got, err := (CCCRepoResolver{Config: &configpkg.Config{ProjectsDir: base}}).Resolve(context.Background(), "repo")
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != dir {
		t.Fatalf("Path = %q, want %q", got.Path, dir)
	}
}

func TestSessionNameForGitURLRepoName(t *testing.T) {
	repoName := configpkg.ExtractRepoName("git@github.com:liftoff/ccc.git")
	if repoName != "liftoff-ccc" {
		t.Fatalf("repoName = %q", repoName)
	}
	if got := SessionName("JIRA-123", repoName); got != "jira-123-liftoff-ccc" {
		t.Fatalf("SessionName = %q", got)
	}
}

func TestRepoResolverMissingRepoField(t *testing.T) {
	_, err := (CCCRepoResolver{}).Resolve(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "missing repo field") {
		t.Fatalf("err = %v", err)
	}
}
