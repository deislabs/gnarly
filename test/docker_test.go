package test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestDocker(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping docker test in short mode")
	}

	testDockerfile := []byte(`
FROM busybox AS busy
FROM alpine AS alp

# Just some dummy things to make sure both the above targets are built
FROM scratch
COPY --from=busy / /tmp /tmp-busy
COPY --from=alp / /tmp /tmp-alpine
	`)

	// This are pinning sha's because the buildkit metadata adds the sha for contexts provided with `--build-context`
	expected := marshalResult(t, Result{
		Sources: []Source{
			{Type: "docker-image", Ref: "docker.io/library/alpine:latest"},
			{Type: "docker-image", Ref: "docker.io/library/alpine@sha256:686d8c9dfa6f3ccfc8230bc3178d23f84eeaf7e457f36f271ab1acc53015037c"},
		},
	})

	extModConfig := []byte(`
{
	"docker.io/library/busybox:latest": "docker.io/library/alpine:latest@sha256:686d8c9dfa6f3ccfc8230bc3178d23f84eeaf7e457f36f271ab1acc53015037c"
}
`)

	builtinModCfg := []byte(`
	[
		{"match": "docker.io/library/busybox:latest", "replace": "docker.io/library/alpine:latest@sha256:686d8c9dfa6f3ccfc8230bc3178d23f84eeaf7e457f36f271ab1acc53015037c"}
	]
	`)

	t.Run("with invoke docker env", func(t *testing.T) {
		t.Run("non-build commands", func(t *testing.T) {
			t.Run("docker run", testCmd([]byte("hello"), withDockerArgs("run", "--rm", "--tmpfs=/tmp", "--pids-limit", "100", "busybox", "echo", "hello")))
		})
		t.Run("build commands", func(t *testing.T) {
			if os.Getenv("TEST_NO_SKIP_BUILD") != "1" {
				t.Skip("Build tests are currently buggy to due to non-deterministic output from buildkit's metadata file. Enable these tests by setting TEST_NO_SKIP_BUILD=1")
			}
			t.Run("pre-generate", func(t *testing.T) {
				// pre-generate the mods instead of doing it on the fly when invoking docker.
				modfilePath := filepath.Join(t.TempDir(), "modfile")
				modfile := marshalResult(t, Result{
					Sources: []Source{
						{Type: "docker-image", Ref: "docker.io/library/busybox:latest", Replace: "docker.io/library/alpine:latest@sha256:686d8c9dfa6f3ccfc8230bc3178d23f84eeaf7e457f36f271ab1acc53015037c"},
					},
				})
				if err := os.WriteFile(modfilePath, modfile, 0644); err != nil {
					t.Fatal(err)
				}

				t.Run("context dir", func(t *testing.T) {
					t.Run("without buildx", testCmd(expected, withDockerfile(bytes.NewReader(testDockerfile)), withModfile(modfilePath), withDockerArgs("build")))
					t.Run("with buildx", testCmd(expected, withDockerfile(bytes.NewReader(testDockerfile)), withModfile(modfilePath), withDockerArgs("buildx", "build")))
				})
				t.Run("context stdin", func(t *testing.T) {
					t.Run("without buildx", testCmd(expected, withStdin, withDockerfile(bytes.NewReader(testDockerfile)), withModfile(modfilePath), withDockerArgs("build")))
					t.Run("with buildx", testCmd(expected, withStdin, withDockerfile(bytes.NewReader(testDockerfile)), withModfile(modfilePath), withDockerArgs("buildx", "build")))
				})
			})
			t.Run("generate", func(t *testing.T) {
				t.Run("context stdin", func(t *testing.T) {
					t.Run("external mod", func(t *testing.T) {
						t.Run("without buildx", testCmd(expected, withStdin, withDockerfile(bytes.NewReader(testDockerfile)), withModProg, withModConfig(extModConfig), withDockerArgs("build")))
						t.Run("with buildx", testCmd(expected, withStdin, withDockerfile(bytes.NewReader(testDockerfile)), withModProg, withModConfig(extModConfig), withDockerArgs("buildx", "build")))
					})
					t.Run("builtin mod", func(t *testing.T) {
						t.Run("without buildx", testCmd(expected, withStdin, withDockerfile(bytes.NewReader(testDockerfile)), withModConfig(builtinModCfg), withDockerArgs("build")))
						t.Run("with buildx", testCmd(expected, withStdin, withDockerfile(bytes.NewReader(testDockerfile)), withModConfig(builtinModCfg), withDockerArgs("buildx", "build")))
					})
				})
			})
		})
	})
}
