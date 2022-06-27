package main

import (
	"io/fs"
	"os"
	"path/filepath"
)

func lookPath(name string) string {
	paths := filepath.SplitList(os.Getenv(pathEnv))

	dir := filepath.Dir(os.Args[0])
	debug("current bin:", os.Args[0])
	for _, p := range paths {
		debug("checking for docker in path:", p)
		if !noFilterPath && p == dir {
			// Skip the directory where our binary is located
			debug("Skipping docker bin lookup for:", p)
			continue
		}

		f := filepath.Join(p, dockerBin)
		if err := findExecutable(f); err == nil {
			debug("found docker:", f)
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
