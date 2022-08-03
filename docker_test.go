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
	dArgs := newDockerArgs()
	parseDockerArgs([]string{"run", "-it", "--rm", "busybox", "sh"}, &dArgs)
	if !reflect.DeepEqual(dArgs, newDockerArgs()) {
		t.Errorf("Got unexpected docker args, should be empty: %+v", dArgs)
	}

	dArgs = newDockerArgs()
	parseDockerArgs([]string{"run", "-it", "--rm", "--name", "build", "busybox", "sh"}, &dArgs)
	if dArgs.Build {
		t.Errorf("Got unexpected docker args, should not be build: %+v", dArgs)
	}
	dArgs = newDockerArgs()
	parseDockerArgs([]string{"run", "-it", "--rm", "build", "sh"}, &dArgs)
	if dArgs.Build {
		t.Errorf("Got unexpected docker args, should not be build: %+v", dArgs)
	}
	dArgs = newDockerArgs()
	parseDockerArgs([]string{"--tls", "run", "-it", "--rm", "build", "sh"}, &dArgs)
	if dArgs.Build {
		t.Errorf("Got unexpected docker args, should not be build: %+v", dArgs)
	}

	dArgs = newDockerArgs()
	parseDockerArgs([]string{"run", "-it", "--rm", "buildx", "sh"}, &dArgs)
	if dArgs.Buildx {
		t.Errorf("Got unexpected docker args, should not be build: %+v", dArgs)
	}

	dArgs = newDockerArgs()
	parseDockerArgs([]string{"build", ".", "-t", "test", "-f", "Dockerfile.test"}, &dArgs)
	if dArgs.Context != "." {
		t.Errorf("Got unexpected context path, expected ., got: %s", dArgs.Context)
	}
	if dArgs.DockerfileName != "Dockerfile.test" {
		t.Errorf("Got unexpected dockerfile name, expected Dockerfile.test, got: %s", dArgs.DockerfileName)
	}

	dArgs = newDockerArgs()
	parseDockerArgs([]string{"build", "-"}, &dArgs)
	if dArgs.Context != "-" {
		t.Errorf("Got unexpected context path, expected -, got: %s", dArgs.Context)
	}

	osArgs := []string{"build", "--build-arg", "foo=bar", "--bool-flag", "-t", "foo", "--output=type=registry,dest=bar", "--other-flag", "some value", "--build-arg=baz=quux", "--file", t.Name(), "."}
	dArgs = newDockerArgs()
	parseDockerArgs(osArgs, &dArgs)

	if !dArgs.Build {
		t.Error("Expected `build` to be true")
	}
	if dArgs.Buildx {
		t.Error("Expected `buildx` to be false")
	}

	dArgs = newDockerArgs()
	dArgs.Tags = []string{"asdf"}
	parseDockerArgs(append([]string{"buildx"}, osArgs...), &dArgs)

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

	if len(dArgs.FilterFlags) != 3 {
		t.Fatalf("Expected 3 filter flags, got: %v", dArgs.FilterFlags)
	}
	if dArgs.FilterFlags[0] != 5 {
		t.Errorf("Expected 4, got %d", dArgs.FilterFlags[0])
	}
	if dArgs.FilterFlags[1] != 6 {
		t.Errorf("Expected 5, got %d", dArgs.FilterFlags[1])
	}
	if dArgs.FilterFlags[2] != 7 {
		t.Errorf("Expected 6, got %d", dArgs.FilterFlags[2])
	}

	dArgs = newDockerArgs()
	parseDockerArgs([]string{"run", "-it", "--rm", "golang:1.18", "go", "build"}, &dArgs)
	if dArgs.Build {
		t.Error("Expected `build` to be false since it is a not a docker build command")
	}
}
