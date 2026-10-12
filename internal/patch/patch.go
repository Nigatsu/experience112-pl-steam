// Package patch installs the Polish localisation of the eXperience112
// retail release into the Steam release, and restores the original files.
//
// User-facing messages (and therefore error texts) are in Polish, because
// they are shown verbatim to the players this tool is for.
package patch

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// BackupDirName is the folder (inside the game folder) holding the original
// files replaced or moved by the patch.
const BackupDirName = "PL_patch_backup"

// UI is how the patch communicates with the user.
type UI interface {
	// Printf prints a message (no newline is added).
	Printf(format string, args ...any)
	// Confirm asks a yes/no question; def is the default answer.
	Confirm(question string, def bool) bool
	// Prompt asks for a line of text (e.g. a path); "" means no answer.
	Prompt(question string) string
}

// ErrAborted is returned when the user declines to continue.
var ErrAborted = errors.New("przerwano")

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// moveFile renames src to dst, falling back to copy+delete across volumes.
func moveFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}

// moveTree moves every file under src into dst (an existing file in dst is
// kept, so the first backup wins) and removes the emptied source folders.
func moveTree(src, dst string) error {
	var dirs []string
	err := filepath.Walk(src, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case fi.IsDir():
			dirs = append(dirs, p)
			return os.MkdirAll(target, 0o755)
		case exists(target):
			return os.Remove(p)
		default:
			return moveFile(p, target)
		}
	})
	if err != nil {
		return err
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		os.Remove(dirs[i]) // only succeeds when empty, which is intended
	}
	return nil
}
