package testutils

import (
	"os"
	"path/filepath"
	"runtime"
)

func ProjectRoot() string {
	_, filename, _, _ := runtime.Caller(1)
	dir := filepath.Dir(filename)

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			panic("go.mod not found")
		}
		dir = parent
	}
}
