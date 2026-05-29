// Package files provides path-traversal-safe browsing and download of a
// client's case-file folder, which lives under a single configured root.
//
// Safety model:
//   - The configured root is opened once with os.OpenRoot (Go 1.24+), whose
//     Open/Stat are guaranteed not to escape the root, including via symlinks.
//   - All user-supplied path segments are additionally validated with a
//     filepath.Rel containment check and rejected if they contain traversal,
//     absolute paths, drive letters, UNC prefixes, NUL bytes, or (on Windows)
//     reserved device names.
package files

import (
	"errors"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Browser serves files from a single root directory.
type Browser struct {
	root     *os.Root
	rootPath string
}

// Entry is one item in a directory listing.
type Entry struct {
	Name    string
	RelPath string // path relative to the root, forward-slashed, for URLs
	IsDir   bool
	Size    int64
	ModTime time.Time
}

// ErrUnsafePath is returned when a requested path fails validation.
var ErrUnsafePath = errors.New("unsafe path")

// Open prepares a Browser rooted at rootPath. The directory must exist.
func Open(rootPath string) (*Browser, error) {
	abs, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, err
	}
	abs = filepath.Clean(abs)
	r, err := os.OpenRoot(abs)
	if err != nil {
		return nil, err
	}
	return &Browser{root: r, rootPath: abs}, nil
}

// Close releases the underlying root handle.
func (b *Browser) Close() error { return b.root.Close() }

// cleanRel validates and normalizes a relative path supplied (in part) by the
// user. It returns a slash-free-of-traversal relative path suitable for use
// with os.Root, or ErrUnsafePath.
//
// segments are joined in order; any may be empty. A typical call is
// cleanRel(client.FolderPath, userSubPath).
func cleanRel(segments ...string) (string, error) {
	joined := path.Join(toSlashAll(segments)...)
	// path.Join already cleans; an empty result means the root itself.
	if joined == "" || joined == "." {
		return ".", nil
	}
	// Reject obvious attacks before the containment check.
	if strings.ContainsRune(joined, 0) {
		return "", ErrUnsafePath
	}
	if strings.HasPrefix(joined, "/") || strings.HasPrefix(joined, "\\") {
		return "", ErrUnsafePath // absolute or UNC
	}
	if hasDriveLetter(joined) {
		return "", ErrUnsafePath // e.g. C:\...
	}
	// Containment: the cleaned relative path must not climb out of root.
	cleaned := path.Clean(joined)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", ErrUnsafePath
	}
	if isReservedWindowsName(cleaned) {
		return "", ErrUnsafePath
	}
	// os.Root wants an OS-separated path; it enforces containment again.
	return filepath.FromSlash(cleaned), nil
}

// List returns the entries of the directory at clientFolder/sub. Entries are
// sorted directories-first then name. parentRel is the rel path of the parent
// (empty if sub is already the folder root) for "up" navigation.
func (b *Browser) List(clientFolder, sub string) (entries []Entry, dirRel string, err error) {
	rel, err := cleanRel(clientFolder, sub)
	if err != nil {
		return nil, "", err
	}
	f, err := b.root.Open(rel)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, "", err
	}
	if !info.IsDir() {
		return nil, "", ErrUnsafePath
	}

	dirEntries, err := f.ReadDir(-1)
	if err != nil {
		return nil, "", err
	}

	// rel is relative to the configured root and includes the client folder;
	// we want RelPath the same way so download/browse links are self-contained.
	relSlash := filepath.ToSlash(rel)
	if relSlash == "." {
		relSlash = ""
	}
	for _, de := range dirEntries {
		fi, ierr := de.Info()
		if ierr != nil {
			continue
		}
		// Skip anything that isn't a regular file or directory (e.g. symlinks,
		// devices) — os.Root won't traverse them but we also hide them.
		if !fi.Mode().IsRegular() && !fi.IsDir() {
			continue
		}
		child := de.Name()
		childRel := child
		if relSlash != "" {
			childRel = relSlash + "/" + child
		}
		entries = append(entries, Entry{
			Name:    child,
			RelPath: childRel,
			IsDir:   de.IsDir(),
			Size:    fi.Size(),
			ModTime: fi.ModTime(),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir // dirs first
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	return entries, relSlash, nil
}

// Serve streams the file at the given root-relative path to w with a download
// disposition. The clientFolder is required so we can confirm the requested
// path actually lives within that client's folder.
func (b *Browser) Serve(w http.ResponseWriter, r *http.Request, clientFolder, relPath string) error {
	// Confirm relPath is within the client's folder, not just within root.
	if !withinFolder(clientFolder, relPath) {
		return ErrUnsafePath
	}
	rel, err := cleanRel(relPath)
	if err != nil {
		return err
	}
	f, err := b.root.Open(rel)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.IsDir() || !info.Mode().IsRegular() {
		return ErrUnsafePath
	}

	name := sanitizeHeaderValue(filepath.Base(rel))
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// http.ServeContent sets Content-Type/Length and supports range requests.
	// *os.File implements io.ReadSeeker.
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
	return nil
}

// withinFolder reports whether relPath is inside clientFolder (or equal to it).
func withinFolder(clientFolder, relPath string) bool {
	cf := path.Clean(filepath.ToSlash(strings.TrimSpace(clientFolder)))
	rp := path.Clean(filepath.ToSlash(strings.TrimSpace(relPath)))
	if cf == "." || cf == "" {
		return true // no folder constraint configured for the client
	}
	return rp == cf || strings.HasPrefix(rp, cf+"/")
}

func toSlashAll(segments []string) []string {
	out := make([]string, 0, len(segments))
	for _, s := range segments {
		out = append(out, filepath.ToSlash(strings.TrimSpace(s)))
	}
	return out
}

func hasDriveLetter(p string) bool {
	// Matches "C:" style prefixes (and "C:/..." / "C:\...").
	if len(p) >= 2 && p[1] == ':' {
		c := p[0]
		return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
	}
	return false
}

var reservedWindows = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

func isReservedWindowsName(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		base := seg
		if i := strings.IndexByte(seg, '.'); i != -1 {
			base = seg[:i]
		}
		if reservedWindows[strings.ToUpper(base)] {
			return true
		}
	}
	return false
}

// sanitizeHeaderValue strips characters that could enable header injection.
func sanitizeHeaderValue(s string) string {
	return strings.NewReplacer("\r", "", "\n", "", `"`, "").Replace(s)
}
