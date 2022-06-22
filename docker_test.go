package main

import (
	"strings"
	"testing"
)

func TestDockerfileFromReader(t *testing.T) {
	t.Run("raw dockerfile", func(t *testing.T) {
		dockerfile := `
FROM busybox
RUN echo hello
	`
		rdr := strings.NewReader(dockerfile)

		dt, err := dockerfileFromReader(rdr, "")
		if err != nil {
			t.Fatal(err)
		}

		if string(dt) != dockerfile {
			t.Fatalf("Expected %s, got %s", dockerfile, string(dt))
		}
	})
}

func TestParseDockerArgs(t *testing.T) {
	osArgs := []string{"build", "--build-arg", "foo=bar", "--bool-flag", "--other-flag", "some value", "--build-arg=baz=quux", "--file", t.Name(), "."}
	dArgs := parseDockerArgs(osArgs)

	if !dArgs.Build {
		t.Error("Expected `build` to be true")
	}
	if dArgs.Buildx {
		t.Error("Expected `buildx` to be false")
	}

	dArgs = parseDockerArgs(append([]string{"buildx"}, osArgs...))

	if !dArgs.Build {
		t.Error("Expected `build` to be true")
	}
	if !dArgs.Buildx {
		t.Error("Expected `buildx` to be true")
	}

	if dArgs.DockerfileName != t.Name() {
		t.Fatalf("Expected dockerfile name %s, got %s", t.Name(), dArgs.DockerfileName)
	}

	if len(dArgs.BuildArgs) != 2 {
		t.Errorf("Expected 2 build args: %v", dArgs.BuildArgs)
	}

	if v, ok := dArgs.BuildArgs["foo"]; v != "bar" {
		t.Errorf("Expected 'bar', got '%s', isSet: %v", dArgs.BuildArgs["foo"], ok)
	}

	if v, ok := dArgs.BuildArgs["baz"]; v != "quux" {
		t.Errorf("Expected 'quux', got '%s', isSet: %v", v, ok)
	}
}
