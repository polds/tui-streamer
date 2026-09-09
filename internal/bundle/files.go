package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// stagedAssetsDirName is the directory name used both in packaged
// Contents/Resources/tui and in the runtime cache.
const stagedAssetsDirName = "tui"

// PackagedTUIPath returns Contents/Resources/tui next to the executable when
// that directory exists, otherwise "".
func PackagedTUIPath() string {
	dir := PackagedResourcesDir()
	if dir == "" {
		return ""
	}
	p := filepath.Join(dir, stagedAssetsDirName)
	st, err := os.Stat(p)
	if err != nil || !st.IsDir() {
		return ""
	}
	return p
}

// ResolveTUIPath returns the directory that should be exported as TUI_PATH.
// Packaged Resources/tui wins when present so a .app is self-contained.
// Otherwise listed files are copied from paths relative to bundlePath.
func ResolveTUIPath(bundlePath string, files []FileEntry) (string, error) {
	if p := PackagedTUIPath(); p != "" {
		return p, nil
	}
	if len(files) == 0 {
		return "", nil
	}
	if bundlePath == "" {
		return "", fmt.Errorf("bundle files listed but no on-disk bundle path to resolve them from")
	}
	return StageFiles(bundlePath, files)
}

// StageFiles copies each FileEntry from next to the bundle YAML into a
// per-bundle cache directory and returns that directory (TUI_PATH).
// Execute bits on the source are preserved.
func StageFiles(bundlePath string, files []FileEntry) (string, error) {
	bundleDir, err := filepath.Abs(filepath.Dir(bundlePath))
	if err != nil {
		return "", fmt.Errorf("bundle dir: %w", err)
	}
	destRoot, err := stageRoot(bundlePath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(destRoot, 0o755); err != nil {
		return "", fmt.Errorf("create TUI_PATH: %w", err)
	}

	for i, entry := range files {
		if err := stageEntry(bundleDir, destRoot, entry); err != nil {
			return "", fmt.Errorf("files[%d]: %w", i, err)
		}
	}
	return destRoot, nil
}

func stageRoot(bundlePath string) (string, error) {
	abs, err := filepath.Abs(bundlePath)
	if err != nil {
		return "", fmt.Errorf("bundle path: %w", err)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		cache = os.TempDir()
	}
	sum := sha256.Sum256([]byte(abs))
	return filepath.Join(cache, "tui-streamer", hex.EncodeToString(sum[:8]), stagedAssetsDirName), nil
}

func stageEntry(bundleDir, destRoot string, entry FileEntry) error {
	src, err := resolveSource(bundleDir, entry.Source)
	if err != nil {
		return err
	}
	destRel, err := sanitizeDest(entry.Dest, src)
	if err != nil {
		return err
	}
	dst := filepath.Join(destRoot, destRel)
	// Join + Clean must still stay under destRoot.
	if !isWithin(destRoot, dst) {
		return fmt.Errorf("dest %q escapes TUI_PATH", entry.Dest)
	}
	return copyPath(src, dst)
}

func resolveSource(bundleDir, source string) (string, error) {
	if strings.TrimSpace(source) == "" {
		return "", fmt.Errorf("source is required")
	}
	p := source
	if !filepath.IsAbs(p) {
		p = filepath.Join(bundleDir, source)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("source %q: %w", source, err)
	}
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("source %q: %w", source, err)
	}
	return abs, nil
}

// DestName returns the relative path under TUI_PATH for entry.
func DestName(entry FileEntry) (string, error) {
	src := entry.Source
	if src == "" {
		src = entry.Dest
	}
	return sanitizeDest(entry.Dest, src)
}

func sanitizeDest(dest, absSource string) (string, error) {
	if dest == "" {
		dest = filepath.Base(absSource)
	}
	if filepath.IsAbs(dest) || strings.HasPrefix(filepath.ToSlash(dest), "/") {
		return "", fmt.Errorf("dest must be a relative path, got %q", dest)
	}
	dest = filepath.ToSlash(dest)
	clean := filepath.Clean(dest)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid dest %q", dest)
	}
	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("dest must be a relative path, got %q", dest)
	}
	return clean, nil
}

func isWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func copyPath(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("stat %q: %w", src, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := filepath.EvalSymlinks(src)
		if err != nil {
			return fmt.Errorf("resolve symlink %q: %w", src, err)
		}
		return copyPath(target, dst)
	}
	if info.IsDir() {
		return copyDir(src, dst, info.Mode())
	}
	return copyFile(src, dst, info.Mode())
}

func copyDir(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(dst, mode.Perm()|0o755); err != nil {
		return fmt.Errorf("mkdir %q: %w", dst, err)
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("readdir %q: %w", src, err)
	}
	for _, e := range entries {
		if err := copyPath(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir %q: %w", filepath.Dir(dst), err)
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %q: %w", src, err)
	}
	defer in.Close()

	// Preserve the source permission bits (including execute).
	perm := mode.Perm()
	if perm == 0 {
		perm = 0o644
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("create %q: %w", dst, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %q → %q: %w", src, dst, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close %q: %w", dst, err)
	}
	if err := os.Chmod(dst, perm); err != nil {
		return fmt.Errorf("chmod %q: %w", dst, err)
	}
	return nil
}
