package main

import (
	"io/fs"
	"os"
	"path/filepath"
)

func lookPath(name string) string {
	paths := filepath.SplitList(os.Getenv(pathEnv))

	dir := filepath.Dir(os.Args[0])
	for _, p := range paths {
		if !noFilterPath && p == dir {
			// Skip the directory where our binary is located
			continue
		}

		f := filepath.Join(p, dockerBin)
		if err := findExecutable(f); err == nil {
			return f
		}
	}

	return ""
}

func findExecutable(file string) error {
	d, err := os.Stat(file)
	if err != nil {
		return err
	}
	if m := d.Mode(); !m.IsDir() && m&0111 != 0 {
		return nil
	}
	return fs.ErrPermission
}
