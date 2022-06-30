package test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const (
	modProg      = "modprog"
	docker       = "docker"
	dockersource = "dockersource"
)

func TestMain(m *testing.M) {
	if IsModProg() {
		DoMod()
		return
	}

	dir, err := ioutil.TempDir("", "dockersource-test")
	if err != nil {
		panic(err)
	}
	cleanup := func() {
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintln(os.Stderr, "error cleaning up test bin:", err)
		}
	}

	cmd := exec.Command("make", dockersource)
	cmd.Dir = filepath.Dir(getwd())
	cmd.Env = append(os.Environ(), "OUTPUT="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error compiling dockersource bin:", string(out))
		os.Exit(1)
	}

	p := filepath.Join(dir, dockersource)
	if err := os.Symlink(p, filepath.Join(dir, docker)); err != nil {
		fmt.Fprintln(os.Stderr, "error symlinking as docker:", err)
		os.Exit(1)
	}

	os.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	code := m.Run()
	cleanup()
	os.Exit(code)
}

func getwd() string {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return dir
}

// AsModProg allows the current binary to be used as a mod program by dockersource.
func AsModProg(t *testing.T) string {
	dir := t.TempDir()

	src, err := os.Readlink("/proc/self/exe")
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, modProg)
	if err := os.Symlink(src, target); err != nil {
		t.Fatal(err)
	}
	return target
}

// IsModProg is used to determine if the current execution should be done as a mod-prog
// This is used from TestMain.
func IsModProg() bool {
	return filepath.Base(os.Args[0]) == modProg
}

// DoMod executes as a mod-prog
func DoMod() {
	ref := os.Args[1]
	configPath := os.Getenv("MOD_CONFIG")
	if configPath == "" {
		fmt.Fprintln(os.Stderr, "MOD_CONFIG not set")
		os.Exit(1)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error reading config", err)
		os.Exit(1)
	}

	config := map[string]string{}
	if err := json.Unmarshal(configData, &config); err != nil {
		fmt.Fprintln(os.Stderr, "error parsing config", err)
		os.Exit(1)
	}

	fmt.Printf(config[ref])
}
