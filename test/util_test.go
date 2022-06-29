package test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type Result struct {
	Sources []Source `json:"sources"`
}

type Source struct {
	Type    string `json:"type"`
	Ref     string `json:"ref"`
	Replace string `json:"replace,omitempty"`
}

func (s Source) AsFlag() string {
	if s.Replace == "" {
		return ""
	}
	return fmt.Sprintf("--build-context %s=%s://%s", s.Ref, s.Type, s.Replace)
}

func (r Result) AsFlags() string {
	sb := &strings.Builder{}
	for _, s := range r.Sources {
		sb.WriteString(s.AsFlag())
		sb.WriteString(" ")
	}
	return sb.String()
}

type cmdOpt func(t *testing.T, cmd *exec.Cmd)

func withStdin(r io.Reader) cmdOpt {
	return func(t *testing.T, cmd *exec.Cmd) {
		cmd.Stdin = r
	}
}

func withFormat(format string) cmdOpt {
	return func(t *testing.T, cmd *exec.Cmd) {
		cmd.Args = append(cmd.Args, "--format", format)
	}
}

func withModProg(t *testing.T, cmd *exec.Cmd) {
	cmd.Args = append(cmd.Args, "--mod-prog="+AsModProg(t))
}

func withArgs(args ...string) cmdOpt {
	return func(t *testing.T, cmd *exec.Cmd) {
		cmd.Args = append(cmd.Args, args...)
	}
}

func withModConfig(config []byte) cmdOpt {
	return func(t *testing.T, cmd *exec.Cmd) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "config")
		if err := ioutil.WriteFile(configPath, config, 0644); err != nil {
			t.Fatal(err)
		}
		cmd.Args = append(cmd.Args, "--mod-config="+configPath)
	}
}

func testCmd(expected []byte, opts ...cmdOpt) func(t *testing.T) {
	return func(t *testing.T) {
		cmd := exec.Command(dockersource)
		for _, o := range opts {
			o(t, cmd)
		}

		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v", string(out), err)
		}
		if !bytes.Equal(bytes.TrimSpace(out), expected) {
			t.Fatalf("expected %s, got %s", string(expected), string(out))
		}
	}
}
