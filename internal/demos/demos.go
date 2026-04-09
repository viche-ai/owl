package demos

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed all:bug-hunt
var bugHuntFS embed.FS

// Available returns the names of available demos.
func Available() []string {
	return []string{"bug-hunt"}
}

// Scaffold copies demo files for the named demo into targetDir.
func Scaffold(name, targetDir string) error {
	var demoFS embed.FS
	switch name {
	case "bug-hunt":
		demoFS = bugHuntFS
	default:
		return fmt.Errorf("unknown demo %q — available demos: %s", name, strings.Join(Available(), ", "))
	}

	return fs.WalkDir(demoFS, name, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == name {
			return nil
		}

		rel := path[len(name)+1:]
		dest := filepath.Join(targetDir, filepath.FromSlash(rel))

		if d.IsDir() {
			return os.MkdirAll(dest, 0755)
		}

		content, err := demoFS.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, content, 0644)
	})
}
