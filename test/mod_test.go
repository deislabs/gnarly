package test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var (
	testDockerfile = []byte(`
	FROM foo:1.0 AS foo1
	FROM foo as foo2
	FROM docker.io/library/foo:1.0 AS foo3
	FROM unnamed
	FROM foo:unhandled AS foo4
		`)
	extExpectedModfileOutput = Result{
		Sources: []Source{
			{Type: "docker-image", Ref: "docker.io/library/foo:1.0", Replace: "docker.io/library/bar:1.0"},
			{Type: "docker-image", Ref: "docker.io/library/foo:latest", Replace: "docker.io/library/bar:latest"},
			{Type: "docker-image", Ref: "docker.io/library/foo:unhandled"},
		},
	}
	extModConfig = []byte(`
	{
		"docker.io/library/foo:1.0": "docker.io/library/bar:1.0",
		"docker.io/library/foo:latest": "docker.io/library/bar:latest"
	}
	`)

	// This just gives the same output as the 'external' mod config
	// This is not going to test that the actual replacement engine works as expected, for that we can use unit tests.
	builtinModConfig = []byte(`
	[
		{
			"match": "docker.io/library/foo:1.0", "replace": "docker.io/library/bar:1.0"
		},
		{
			"match": "docker.io/library/foo:latest", "replace": "docker.io/library/bar:latest"
		}
	]
	`)
)

func TestModOutput(t *testing.T) {
	modFileOutput, err := json.MarshalIndent(extExpectedModfileOutput, "", "\t")
	if err != nil {
		t.Fatal(err)
	}
	modFileOutput = bytes.TrimSpace(modFileOutput)
	flagsOutput := []byte(strings.TrimSpace(extExpectedModfileOutput.AsFlags()))

	t.Run("stdin", func(t *testing.T) {
		t.Run("external mod", func(t *testing.T) {
			t.Run("modfile", testCmd(modFileOutput, withStdin(bytes.NewReader(testDockerfile)), withFormat("modfile"), withModProg, withModConfig(extModConfig)))
			t.Run("build-flags", testCmd(flagsOutput, withStdin(bytes.NewReader(testDockerfile)), withFormat("build-flags"), withModProg, withModConfig(extModConfig)))
		})
		t.Run("builtin mod", func(t *testing.T) {
			t.Run("modfile", testCmd(modFileOutput, withStdin(bytes.NewReader(testDockerfile)), withFormat("modfile"), withModConfig(builtinModConfig)))
			t.Run("build-flags", testCmd(flagsOutput, withStdin(bytes.NewReader(testDockerfile)), withFormat("build-flags"), withModConfig(builtinModConfig)))
		})
	})
	t.Run("file", func(t *testing.T) {
		dir := t.TempDir()
		dockerfilePath := filepath.Join(dir, "Dockerfile")
		if err := os.WriteFile(dockerfilePath, testDockerfile, 0644); err != nil {
			t.Fatal(err)
		}
		t.Run("external mod", func(t *testing.T) {
			t.Run("modfile", testCmd(modFileOutput, withFormat("modfile"), withModProg, withModConfig(extModConfig), withArgs(dockerfilePath)))
			t.Run("build-flags", testCmd(flagsOutput, withFormat("build-flags"), withModProg, withModConfig(extModConfig), withArgs(dockerfilePath)))
		})
	})
}
