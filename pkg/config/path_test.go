package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPathUtilities tests path resolution functions
func TestPathUtilities(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-path-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	config := &Config{
		ProjectsDir: "~/CustomProjects",
	}

	// Test GetProjectsDir
	projectsDir := GetProjectsDir(config)
	expected := filepath.Join(tmpDir, "CustomProjects")
	if projectsDir != expected {
		t.Errorf("GetProjectsDir: got %q, want %q", projectsDir, expected)
	}

	// Test ResolveProjectPath with absolute path
	path := ResolveProjectPath(config, "/absolute/path")
	if path != "/absolute/path" {
		t.Errorf("ResolveProjectPath(absolute): got %q, want '/absolute/path'", path)
	}

	// Test ResolveProjectPath with home-relative path
	path = ResolveProjectPath(config, "~/from/home")
	expected = filepath.Join(tmpDir, "from/home")
	if path != expected {
		t.Errorf("ResolveProjectPath(~/from/home): got %q, want %q", path, expected)
	}

	// Test ResolveProjectPath with relative path
	path = ResolveProjectPath(config, "relative/path")
	expected = filepath.Join(tmpDir, "CustomProjects", "relative/path")
	if path != expected {
		t.Errorf("ResolveProjectPath(relative): got %q, want %q", path, expected)
	}

	// Test ExpandPath
	path = ExpandPath("~/test/path")
	expected = filepath.Join(tmpDir, "test/path")
	if path != expected {
		t.Errorf("ExpandPath(~/test/path): got %q, want %q", path, expected)
	}

	// Test ExpandPath with non-tilde path
	path = ExpandPath("/absolute/path")
	if path != "/absolute/path" {
		t.Errorf("ExpandPath(absolute): got %q, want '/absolute/path'", path)
	}
}
