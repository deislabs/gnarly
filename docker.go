package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	dockerBin = "docker"
	pathEnv   = "PATH"
)

// Env vars which are used when dockersource is invoked as a wrapper for the docker binary.
var (
	// Pass through a custom syntax parser to the docker build command.
	parser = os.Getenv("BUILDKIT_SYNTAX")

	// Set a path to a pre-existing modfile instead of calculating one on each invocation.
	modPath = os.Getenv("DOCKERFILE_MOD_PATH")

	// Enable debug logging
	dockerDebug = os.Getenv("DEBUG")
)

var (
	// When IsDocker is true because of the special env var instead of based on argv[0], we don't want to filter the path.
	noFilterPath bool
)

func IsDocker() bool {
	if filepath.Base(os.Args[0]) == dockerBin {
		return true
	}
	if os.Getenv("DOCKERFILE_MOD_INVOKE_DOCKER") == "1" {
		noFilterPath = true
		return true
	}
	return false
}

func InvokeDocker() {
	if err := invokeDocker(newContext()); err != nil {
		fmt.Fprintln(os.Stderr, "[dockersource]: Error while wrapping docker cli:", err)
		fmt.Fprintln(os.Stderr, "[dockersource]: The oepration you are trying to perform may not be supported by this version of dockersource.")
		os.Exit(1)
	}
}

func debug(args ...interface{}) {
	if dockerDebug != "" {
		fmt.Fprintln(os.Stderr, append([]interface{}{"[dockersource]:"}, args...)...)
	}
}

type dockerArgs struct {
	BuildArgs      map[string]string
	DockerfileName string
	Build          bool
	Buildx         bool
}

func newDoockerArgs() dockerArgs {
	return dockerArgs{
		BuildArgs:      make(map[string]string),
		DockerfileName: "Dockerfile",
	}
}

// Expects all args that would be passed to dodcker except argv[0] itself.
// e.g. if argv is "docker build -t foo -f bar", the args would be "build -t foo -f bar"
func parseDockerArgs(args []string) dockerArgs {
	var (
		skipNext bool
		dArgs    = newDoockerArgs()
	)

	for i, arg := range args {
		switch arg {
		case "build":
			dArgs.Build = true
		case "buildx":
			dArgs.Buildx = true
		}

		if skipNext {
			skipNext = false
			continue
		}

		if arg[0] == '-' {
			splitArg := strings.SplitN(arg, "=", 2)
			hasValue := len(splitArg) == 2
			var value string
			if hasValue {
				value = splitArg[1]
			} else {
				if i < len(args)-1 {
					value = args[i+1]
				}
			}

			debug(arg, value)
			if dArgs.Build {
				switch splitArg[0] {
				case "--build-arg":
					split := strings.SplitN(value, "=", 2)
					var v string
					if len(split) == 2 {
						v = split[1]
					}
					debug("setting build arg", split[0], v)
					dArgs.BuildArgs[split[0]] = v
				case "-f", "--file":
					dArgs.DockerfileName = value
				}
			}

			if !hasValue && len(args)-1 > i+1 {
				a := args[i+1]
				if len(a) > 0 && a[0] != '-' {
					// This is a value for the current option, which we've already captured, so skip it.
					skipNext = true
				}
			}
			continue
		}
	}

	return dArgs
}

func invokeDocker(ctx context.Context) error {
	d := lookPath(dockerBin)
	if d == "" {
		return &exec.Error{Name: dockerBin, Err: exec.ErrNotFound}
	}

	var (
		lastArg string
		args    []string
	)
	if len(os.Args) > 1 {
		lastArg = os.Args[len(os.Args)-1]
		args = os.Args[1 : len(os.Args)-1]
	}

	dArgs := parseDockerArgs(args)

	if dArgs.Build {
		if dArgs.Build && !dArgs.Buildx && parser == "" {
			return fmt.Errorf("legacy `docker build` invcoation detected, but no dockerfile parser was specified. Please set the `BUILDKIT_SYNTAX` environment variable to the name of the parser to use and add a Dockerfile.mod adjacent to the specified Dockerfile")
		}

		if parser != "" {
			args = append(args, "--build-arg=BUILDKIT_SYNTAX="+parser)
		}

		var result Result
		if modPath != "" {
			data, err := os.ReadFile(modPath)
			if err != nil {
				return fmt.Errorf("error reading specified modfile path: %w", err)
			}

			if err := json.Unmarshal(data, &result); err != nil {
				return fmt.Errorf("error parsing specified modfile: %w", err)
			}
		} else {
			if dArgs.Buildx {
				dt, err := getDockerfile(lastArg, dArgs.DockerfileName)
				if err != nil {
					return err
				}

				result, err = Generate(ctx, dt, dArgs.BuildArgs)
				if err != nil {
					return err
				}
			}
		}

		for _, s := range result.Sources {
			if s.Replace != "" {
				args = append(args, fmt.Sprintf("--build-context=%s=%s://%s", s.Ref, s.Type, s.Replace))
			}
		}
	}

	select {
	case <-ctx.Done():
	default:
	}

	args = append(args, lastArg)

	debug(d, strings.Join(args, " "))
	if err := syscall.Exec(d, append([]string{filepath.Base(d)}, args...), os.Environ()); err != nil {
		return fmt.Errorf("error executing actual docker bin: %w", err)
	}

	return nil
}

const (
	Uncompressed = iota
	Bzip2
	Gzip
	Xz
)

func detectCompression(magic []byte) int {
	for compression, m := range map[int][]byte{
		Bzip2: {0x42, 0x5A, 0x68},
		Gzip:  {0x1F, 0x8B, 0x08},
		Xz:    {0xFD, 0x37, 0x7A, 0x58, 0x5A, 0x00},
	} {
		if len(magic) < len(m) {
			continue
		}
		if bytes.Equal(m, magic[:len(m)]) {
			return compression
		}
	}

	return Uncompressed
}

type unsupportedURLContext struct {
	scheme string
}

func (e unsupportedURLContext) Error() string {
	return "unsupported context scheme: " + e.scheme
}

func getDockerfile(context, p string) ([]byte, error) {
	if context == "-" {
		f, err := os.CreateTemp("", "dockermod-"+context)
		if err != nil {
			return nil, fmt.Errorf("error creating temp file to pipe from stdin: %w", err)
		}
		defer f.Close()

		rdr := io.TeeReader(os.Stdin, f)

		dt, err := dockerfileFromReader(rdr, p)
		if err != nil {
			return nil, err
		}

		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return nil, fmt.Errorf("error seeking to start of temp context file: %w", err)
		}

		if err := syscall.Dup3(int(f.Fd()), int(os.Stdin.Fd()), 0); err != nil {
			return nil, fmt.Errorf("error duping temp context file to stdin: %w", err)
		}

		return dt, nil
	}

	u, err := url.Parse(context)
	if err == nil {
		switch u.Scheme {
		case "http", "https":
			return nil, unsupportedURLContext{u.Scheme}
		case "git":
			return nil, unsupportedURLContext{u.Scheme}
		}
	}

	if _, err := os.Stat(context); err == nil {
		return os.ReadFile(filepath.Join(context, p))
	}

	return nil, fmt.Errorf("unable to locate %s in context %s", p, context)
}

func xzStream(in io.Reader) (io.Reader, error) {
	cmd := exec.Command("xz", "-d", "-c", "-q")
	cmd.Stdin = in

	pr, pw := io.Pipe()
	cmd.Stdout = pw

	errBuf := bytes.NewBuffer(nil)
	cmd.Stderr = errBuf

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	go func() {
		if err := cmd.Wait(); err != nil {
			pr.CloseWithError(fmt.Errorf("%s: %w", errBuf, err))
		}
	}()

	return pr, nil
}

func dockerfileFromReader(f io.Reader, p string) ([]byte, error) {
	bufReader := bufio.NewReader(f)
	var rdr io.Reader = bufReader

	magic, err := bufReader.Peek(1024)
	if err != nil && err != io.EOF {
		return nil, err
	}

	switch detectCompression(magic) {
	case Bzip2:
		rdr = bzip2.NewReader(rdr)
	case Gzip:
		rdr, err = gzip.NewReader(rdr)
		if err != nil {
			return nil, err
		}
	case Xz:
		rdr, err = xzStream(rdr)
		if err != nil {
			return nil, err
		}
	}

	tr := tar.NewReader(bytes.NewBuffer(magic))
	if _, err := tr.Next(); err != nil {
		// Not an archive
		return ioutil.ReadAll(rdr)
	}

	tr = tar.NewReader(rdr)
	for {
		th, err := tr.Next()
		if err != nil {
			return nil, err
		}

		if th.Name != p {
			continue
		}

		return ioutil.ReadAll(tr)
	}
}
