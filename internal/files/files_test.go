package files

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// setupRoot creates a temp case-files root with a client folder, a file, and a
// nested subfolder. It also writes a "secret" file OUTSIDE the root to prove
// traversal cannot reach it.
func setupRoot(t *testing.T) (root string, browser *Browser) {
	t.Helper()
	base := t.TempDir()
	root = filepath.Join(base, "cases")
	clientDir := filepath.Join(root, "Smith-2026", "subfolder")
	if err := os.MkdirAll(clientDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Smith-2026", "intake.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Smith-2026", "subfolder", "deep.txt"), []byte("deep"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Secret outside the root.
	if err := os.WriteFile(filepath.Join(base, "secret.txt"), []byte("TOPSECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	return root, b
}

func TestListHappyPath(t *testing.T) {
	_, b := setupRoot(t)
	entries, _, err := b.List("Smith-2026", "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name)
	}
	// Directories sort first; expect subfolder then intake.txt.
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d (%v)", len(entries), names)
	}
	if !entries[0].IsDir || entries[0].Name != "subfolder" {
		t.Errorf("expected subfolder first, got %v", names)
	}
}

func TestListNested(t *testing.T) {
	_, b := setupRoot(t)
	entries, dirRel, err := b.List("Smith-2026", "subfolder")
	if err != nil {
		t.Fatalf("List nested: %v", err)
	}
	if dirRel != "Smith-2026/subfolder" {
		t.Errorf("dirRel = %q, want Smith-2026/subfolder", dirRel)
	}
	if len(entries) != 1 || entries[0].Name != "deep.txt" {
		t.Errorf("unexpected nested entries: %+v", entries)
	}
}

// TestTraversalRejected is the important security test: every attempt to climb
// out of the root (or the client folder) must fail.
func TestTraversalRejected(t *testing.T) {
	_, b := setupRoot(t)

	badPaths := []string{
		"../../etc/passwd",
		"..%2f..%2fetc%2fpasswd", // pre-decoded form; still must not resolve
		"/etc/passwd",
		"../secret.txt",
		"Smith-2026/../../secret.txt",
		`..\..\windows\win.ini`,
		`C:\Windows\win.ini`,
		"\\\\server\\share\\file",
		"Smith-2026/../../../secret.txt",
	}
	for _, p := range badPaths {
		t.Run(p, func(t *testing.T) {
			// Via List (treat as sub of the client folder).
			if _, _, err := b.List("Smith-2026", p); err == nil {
				t.Errorf("List allowed unsafe sub %q", p)
			}
			// Via Serve (download).
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/d", nil)
			if err := b.Serve(rec, req, "Smith-2026", p); err == nil && rec.Code == http.StatusOK {
				t.Errorf("Serve allowed unsafe path %q (body=%q)", p, rec.Body.String())
			}
		})
	}
}

func TestServeHappyPath(t *testing.T) {
	_, b := setupRoot(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/d", nil)
	if err := b.Serve(rec, req, "Smith-2026", "Smith-2026/intake.txt"); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if rec.Body.String() != "hello" {
		t.Errorf("body = %q, want hello", rec.Body.String())
	}
	if cd := rec.Header().Get("Content-Disposition"); cd == "" {
		t.Errorf("missing Content-Disposition header")
	}
}

// TestServeOutsideClientFolder ensures a file that exists in the root but in a
// DIFFERENT client's folder cannot be fetched under this client.
func TestServeOutsideClientFolder(t *testing.T) {
	root, b := setupRoot(t)
	otherDir := filepath.Join(root, "Jones-2026")
	if err := os.MkdirAll(otherDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(otherDir, "private.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/d", nil)
	// Smith should not be able to read Jones's file even though it's in-root.
	if err := b.Serve(rec, req, "Smith-2026", "Jones-2026/private.txt"); err == nil {
		t.Errorf("Serve allowed cross-client access")
	}
}
