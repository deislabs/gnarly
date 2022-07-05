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
	"os/signal"
	"path/filepath"
	"strconv"
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

	// Set the path to store the metadata output from docker build
	metaPath = os.Getenv("DOCKERFILE_MOD_META_PATH")

	// Enable debug logging
	dockerDebug = os.Getenv("DEBUG")
)

var (
	// When IsDocker is true because of the special env var instead of based on argv[0], we don't want to filter the path.
	noFilterPath bool

	knownBoolFlags = map[string]bool{
		"--load":      true,
		"--no-cache":  true,
		"--pull":      true,
		"--push":      true,
		"-q":          true,
		"--quiet":     true,
		"--tls":       true,
		"--tlsverify": true,
	}
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
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := invokeDocker(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "[dockersource]: Error while wrapping docker cli:", err)
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
	BuildPos       int
	Buildx         bool
	Context        string
	MetaData       string
}

func newDockerArgs() dockerArgs {
	return dockerArgs{
		BuildArgs:      make(map[string]string),
		DockerfileName: "Dockerfile",
	}
}

// Returns true if next is consumed here and should be skipped when itterating args
func handleDockerFlag(arg, next string, dArgs *dockerArgs) bool {
	splitArg := strings.SplitN(arg, "=", 2)
	hasValue := len(splitArg) == 2
	var value string
	if hasValue {
		value = splitArg[1]
	} else {
		value = next
	}

	debug(splitArg[0], value)
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
		case "--metadata-file":
			debug("setting metadata file", arg, value)
			dArgs.MetaData = value
		}
	}

	if !hasValue && value != "" {
		if knownBoolFlags[splitArg[0]] {
			// We have a known bool flag, check if the next arg is a value passed to the bool flag
			if isBoolV, _ := strconv.ParseBool(value); isBoolV {
				return true
			}
		}
		if len(value) > 0 && value[0] != '-' {
			// This is a value for the current option, which we've already captured, so skip it.
			return true
		}
	}

	return false
}

// Expects all args that would be passed to dodcker except argv[0] itself.
// e.g. if argv is "docker build -t foo -f bar", the args would be "build -t foo -f bar"
func parseDockerArgs(args []string) dockerArgs {
	var (
		skipNext bool
		dArgs    = newDockerArgs()
	)

	for i, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		switch arg {
		case "build":
			dArgs.Build = true
			dArgs.BuildPos = i
			continue
		case "buildx":
			// `buildx` must come before `build` to be considered
			if !dArgs.Build {
				dArgs.Buildx = true
			}
			continue
		}

		if arg[0] == '-' {
			if arg == "-" {
				// for builds, this means read the context from stdin
				dArgs.Context = "-"
				continue
			}
			var next string
			if i < len(args)-1 {
				next = args[i+1]
			}
			skipNext = handleDockerFlag(arg, next, &dArgs)
			continue
		}

		if dArgs.Build {
			if dArgs.Context != "" {
				panic("[dockersource]: found multiple contexts -- this is a bug in the argument parser")
			}
			dArgs.Context = arg
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
		args []string
	)
	if len(os.Args) > 1 {
		args = os.Args[1:]
	}

	dArgs := parseDockerArgs(args)

	var metaCopy bool
	if dArgs.Build {
		if dArgs.Context == "" {
			return fmt.Errorf("could not find context for build in command line arguments")
		}
		if dArgs.Build && !dArgs.Buildx {
			out, err := exec.CommandContext(ctx, d, "build", "--help").CombinedOutput()
			if err != nil {
				debug("error while checking if `docker build` supports --build-context:", err, ":", string(out))
			}

			// Newer versions of docker *may* support --build-context, but that depends on a number of factors... so just check if `docker build --help` says it supports it.
			// If not then inject buildx into the args.
			if !strings.Contains(string(out), "--build-context") {
				debug("injecting buildx into args")
				args = append(args[:dArgs.BuildPos], append([]string{"buildx"}, args[dArgs.BuildPos:]...)...)
			}
		}

		if metaPath != "" {
			debug("injecting metadata file into args")
			if dArgs.MetaData != "" && metaPath != dArgs.MetaData {
				debug("build arguments already specified a metadata file, creating helper to copy it")
				metaCopy = true
			}
			args = append(args, "--metadata-file", metaPath)
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
			if dArgs.Build {
				dt, err := getDockerfile(dArgs.Context, dArgs.DockerfileName)
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

	debug(d, strings.Join(args, " "))
	if !metaCopy {
		if err := syscall.Exec(d, append([]string{filepath.Base(d)}, args...), os.Environ()); err != nil {
			return fmt.Errorf("error executing actual docker bin: %w", err)
		}
		// Nothing happens in our code after this.
		// `syscall.Exec` takes over the whole process
	}

	cmd := exec.CommandContext(ctx, d, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGTERM,
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error executing actual docker bin: %w", err)
	}

	f1, err := os.Open(metaPath)
	if err != nil {
		return fmt.Errorf("error opening metadata file: %w", err)
	}
	defer f1.Close()

	f2, err := os.Create(dArgs.MetaData)
	if err != nil {
		return fmt.Errorf("error creating metadata file: %w", err)
	}
	defer f2.Close()

	_, err = io.Copy(f2, f1)
	return err
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
