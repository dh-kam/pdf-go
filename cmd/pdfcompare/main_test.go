package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRefusesRepositoryRootOutput(t *testing.T) {
	err := run(config{
		repoRoot: ".",
		outDir:   ".",
		dpi:      150,
		workers:  1,
	})
	if err == nil {
		t.Fatal("expected repository-root output directory to be rejected")
	}
	if !strings.Contains(err.Error(), "refusing to use repository root or ancestor") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateOutputDirectoryRefusesRepositoryAncestor(t *testing.T) {
	root := t.TempDir()
	repoRoot := filepath.Join(root, "repo")
	if err := os.Mkdir(repoRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	err := validateOutputDirectory(repoRoot, root, "")
	if err == nil {
		t.Fatal("expected repository ancestor output directory to be rejected")
	}
	if !strings.Contains(err.Error(), "refusing to use repository root or ancestor") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateOutputDirectoryRefusesScanRootAncestor(t *testing.T) {
	repoRoot := t.TempDir()
	scanRoot := filepath.Join("test", "2nd")
	outDir := filepath.Join(repoRoot, "test")

	err := validateOutputDirectory(repoRoot, outDir, scanRoot)
	if err == nil {
		t.Fatal("expected scan-root ancestor output directory to be rejected")
	}
	if !strings.Contains(err.Error(), "refusing to use scan root or ancestor") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateOutputDirectoryRefusesScanRootItself(t *testing.T) {
	repoRoot := t.TempDir()
	outDir := filepath.Join(repoRoot, "test", "2nd")

	err := validateOutputDirectory(repoRoot, outDir, "test/2nd")
	if err == nil {
		t.Fatal("expected scan-root output directory to be rejected")
	}
	if !strings.Contains(err.Error(), "refusing to use scan root or ancestor") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateOutputDirectoryRefusesSymlinkedScanRootAncestor(t *testing.T) {
	root := t.TempDir()
	repoRoot := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(repoRoot, "test", "2nd"), 0o755); err != nil {
		t.Fatal(err)
	}
	repoLink := filepath.Join(root, "repo-link")
	if err := os.Symlink(repoRoot, repoLink); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	err := validateOutputDirectory(repoRoot, filepath.Join(repoLink, "test"), "test/2nd")
	if err == nil {
		t.Fatal("expected symlinked scan-root ancestor output directory to be rejected")
	}
	if !strings.Contains(err.Error(), "refusing to use scan root or ancestor") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateOutputDirectoryAllowsRepositoryChild(t *testing.T) {
	repoRoot := t.TempDir()
	outDir := filepath.Join(repoRoot, "tmp", "compare")

	if err := validateOutputDirectory(repoRoot, outDir, "test/2nd"); err != nil {
		t.Fatalf("expected repository child output directory to be allowed: %v", err)
	}
}
