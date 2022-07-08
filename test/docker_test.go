package test

import (
	"bytes"
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
	expected := marshalResult(t, map[string]interface{}{
		bInfoKey: Result{
			Sources: []Source{
				{Type: "docker-image", Ref: "docker.io/library/alpine:latest"},
				{Type: "docker-image", Ref: "docker.io/library/alpine@sha256:686d8c9dfa6f3ccfc8230bc3178d23f84eeaf7e457f36f271ab1acc53015037c"},
			},
		},
		imageNameKey: "docker.io/library/snowflake:latest,docker.io/library/flurry:latest",
	})

	// Work around for non-determinism of metadata file
	// In order to use the test process must be executed with TEST_ALLOW_ALT_META=1
	expectedAlt := marshalResult(t, map[string]interface{}{
		bInfoKey: Result{
			Sources: []Source{
				{Type: "docker-image", Ref: "docker.io/library/alpine:latest"},
				{Type: "docker-image", Ref: "docker.io/library/alpine:latest@sha256:686d8c9dfa6f3ccfc8230bc3178d23f84eeaf7e457f36f271ab1acc53015037c"},
			},
		},
		imageNameKey: "docker.io/library/snowflake:latest,docker.io/library/flurry:latest",
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
			createBuildx(t)
			buildOpts := func(buildx bool) cmdOpt {
				return func(t *testing.T, cfg *cmdConfig) {
					withDockerfile(bytes.NewReader(testDockerfile))(t, cfg)
					args := []string{"build", "-t", "filtered"}
					if buildx {
						args = append([]string{"buildx"}, args...)
					}
					withDockerArgs(args...)(t, cfg)
					withAlt(expectedAlt)(t, cfg)
					withTags("snowflake", "flurry")(t, cfg)
				}
			}
			t.Run("pre-generate", func(t *testing.T) {
				modfile := Result{
					Sources: []Source{
						{Type: "docker-image", Ref: "docker.io/library/busybox:latest", Replace: "docker.io/library/alpine:latest@sha256:686d8c9dfa6f3ccfc8230bc3178d23f84eeaf7e457f36f271ab1acc53015037c"},
					},
				}
				// pre-generate the mods instead of doing it on the fly when invoking docker.
				t.Run("context dir", func(t *testing.T) {
					t.Run("without buildx", testCmd(expected, buildOpts(false), withModfile(modfile)))
					t.Run("with buildx", testCmd(expected, buildOpts(true), withModfile(modfile)))
				})
				t.Run("context stdin", func(t *testing.T) {
					t.Run("without buildx", testCmd(expected, withStdin, buildOpts(false), withModfile(modfile)))
					t.Run("with buildx", testCmd(expected, withStdin, buildOpts(true), withModfile(modfile)))
				})
			})
			t.Run("generate", func(t *testing.T) {
				t.Run("context stdin", func(t *testing.T) {
					t.Run("external mod", func(t *testing.T) {
						t.Run("without buildx", testCmd(expected, withStdin, withModProg, withModConfig(extModConfig), buildOpts(false)))
						t.Run("with buildx", testCmd(expected, withStdin, withModProg, withModConfig(extModConfig), buildOpts(false)))
					})
					t.Run("builtin mod", func(t *testing.T) {
						t.Run("without buildx", testCmd(expected, withStdin, withModConfig(builtinModCfg), buildOpts(false)))
						t.Run("with buildx", testCmd(expected, withStdin, withModConfig(builtinModCfg), buildOpts(true)))
					})
				})
			})
		})
	})
}
