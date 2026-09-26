package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// filePrepender produces the front matter the docs site reads, so the title
// it derives from the filename is the contract.
func TestFilePrepender(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		filename  string
		wantTitle string
	}{
		{"top-level command", "apppack_version.md", "version"},
		{"subcommand", "apppack_version_check.md", "version check"},
		{"three levels", "apppack_create_database.md", "create database"},
		// The else branch appends to the parts slice rather than
		// replacing it, so the binary name stays in the title. Pinned as
		// it is -- these titles are published on the docs site.
		{"root", "apppack.md", "apppack (base command)"},
		{"full path is ignored", "/tmp/docs/apppack_ps.md", "ps"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := filePrepender(tt.filename)

			want := "title: " + tt.wantTitle + "\n"
			if !strings.Contains(got, want) {
				t.Errorf("filePrepender(%q) = %q, want it to contain %q", tt.filename, got, want)
			}

			if !strings.HasPrefix(got, "---\n") || !strings.Contains(got, "\n---\n") {
				t.Errorf("front matter is not delimited:\n%q", got)
			}
		})
	}
}

func TestGenerateDocs(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "docs")

	if err := generateDocs(newTestRootCmd(), dir); err != nil {
		t.Fatalf("generateDocs: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading output directory: %v", err)
	}

	if len(entries) == 0 {
		t.Fatal("no files were written")
	}

	// The root page is always produced, and carries the (base command) title.
	root, err := os.ReadFile(filepath.Join(dir, "apppack.md"))
	if err != nil {
		t.Fatalf("reading apppack.md: %v", err)
	}

	if !strings.Contains(string(root), "title: apppack (base command)") {
		t.Errorf("apppack.md is missing its front matter:\n%s", root)
	}
}

// The output directory is created rather than assumed, so `docgen -d` into a
// fresh path works.
func TestGenerateDocsCreatesTheDirectory(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "a", "b", "c")

	if err := generateDocs(newTestRootCmd(), dir); err != nil {
		t.Fatalf("generateDocs into a missing directory: %v", err)
	}

	if _, err := os.Stat(dir); err != nil {
		t.Errorf("directory was not created: %v", err)
	}
}

func TestDocgenIsHidden(t *testing.T) {
	t.Parallel()

	for _, c := range newTestRootCmd().Commands() {
		if c.Name() == "docgen" {
			if !c.Hidden {
				t.Error("docgen should stay out of the top-level help")
			}

			return
		}
	}

	t.Fatal("docgen is not registered")
}
