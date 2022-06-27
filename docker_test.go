package main

import (
	"reflect"
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
	dArgs := parseDockerArgs([]string{"run", "-it", "--rm", "busybox", "sh"})
	if !reflect.DeepEqual(dArgs, newDockerArgs()) {
		t.Errorf("Got unexpected docker args, should be empty: %+v", dArgs)
	}

	dArgs = parseDockerArgs([]string{"run", "-it", "--rm", "--name", "build", "busybox", "sh"})
	if dArgs.Build {
		t.Errorf("Got unexpected docker args, should not be build: %+v", dArgs)
	}
	dArgs = parseDockerArgs([]string{"run", "-it", "--rm", "build", "sh"})
	if dArgs.Build {
		t.Errorf("Got unexpected docker args, should not be build: %+v", dArgs)
	}
	dArgs = parseDockerArgs([]string{"--tls", "run", "-it", "--rm", "build", "sh"})
	if dArgs.Build {
		t.Errorf("Got unexpected docker args, should not be build: %+v", dArgs)
	}

	dArgs = parseDockerArgs([]string{"run", "-it", "--rm", "buildx", "sh"})
	if dArgs.Buildx {
		t.Errorf("Got unexpected docker args, should not be build: %+v", dArgs)
	}

	dArgs = parseDockerArgs([]string{"build", ".", "-t", "test", "-f", "Dockerfile.test"})
	if dArgs.Context != "." {
		t.Errorf("Got unexpected context path, expected ., got: %s", dArgs.Context)
	}
	if dArgs.DockerfileName != "Dockerfile.test" {
		t.Errorf("Got unexpected dockerfile name, expected Dockerfile.test, got: %s", dArgs.DockerfileName)
	}

	osArgs := []string{"build", "--build-arg", "foo=bar", "--bool-flag", "--other-flag", "some value", "--build-arg=baz=quux", "--file", t.Name(), "."}
	dArgs = parseDockerArgs(osArgs)

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
