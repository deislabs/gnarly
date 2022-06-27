package main

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
)

func lookPath(name string) string {
	paths := filepath.SplitList(os.Getenv(pathEnv))

	execPath := os.Args[0]
	if !filepath.IsAbs(execPath) {
		if _, err := os.Stat(execPath); err != nil {
			if !os.IsNotExist(err) {
				debug("lookPath: stat error %s: %v", execPath, err)
				return ""
			}

			execPath, err = exec.LookPath(execPath)
			if err != nil {
				debug("exec.LookPath error: %v", err)
				return ""
			}
		}
	}

	dir := filepath.Dir(execPath)
	debug("current bin:", execPath)
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
