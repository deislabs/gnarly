package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

const (
	formatModfile    = "modfile"
	formatBuildFlags = "build-flags"
)

var (
	modProg   = os.Getenv("DOCKERFILE_MOD_PROG")
	modConfig = os.Getenv("DOCKERFILE_MOD_CONFIG")
)

func main() {
	if IsDocker() {
		InvokeDocker()
		return
	}

	buildArgs := argFlag{}
	format := os.Getenv("DOCKERFILE_MOD_FORMAT")
	if format == "" {
		format = formatBuildFlags
	}

	flag.Var(&buildArgs, "build-arg", "set build args to pass through -- these are required if the dockerfie uses args to determine an image source")
	flag.StringVar(&modProg, "mod-prog", modProg, "Set program to execute to modify a reference as a replace rule")
	flag.StringVar(&modConfig, "mod-config", modConfig, "Set the config file to pass to mod prog")
	flag.StringVar(&format, "format", format, "Set the output format. Formats: modfile, build-flags")

	flag.Parse()

	var (
		err error
		dt  []byte
	)
	if flag.NArg() == 0 || flag.Arg(0) == "-" {
		stat, e := os.Stdin.Stat()
		if e != nil {
			panic(err)
		}
		if stat.Mode()&os.ModeCharDevice == 0 {
			dt, err = ioutil.ReadAll(os.Stdin)
		} else {
			dt, err = ioutil.ReadFile("Dockerfile")
		}
	} else {
		dt, err = ioutil.ReadFile(flag.Arg(0))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error reading dockerfile:", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	result, err := Generate(ctx, dt, buildArgs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error generating mods:", err)
		os.Exit(2)
	}

	switch format {
	case formatModfile:
		data, err := json.MarshalIndent(result, "", "\t")
		if err != nil {
			panic(err)
		}
		fmt.Println(string(data))
		return
	case formatBuildFlags:
		sb := &strings.Builder{}

		for k, v := range buildArgs {
			sb.WriteString(fmt.Sprintf("--build-arg %s=%s ", k, v))
		}

		for _, resolved := range result.Sources {
			if resolved.Replace != "" {
				sb.WriteString(fmt.Sprintf("--build-context %s=docker-image://%s ", resolved.Ref, resolved.Replace))
			}
		}
		fmt.Print(sb.String())
		return
	default:
		fmt.Fprintln(os.Stderr, "unknown format:", format)
		os.Exit(1)
	}
}

type argFlag map[string]string

func (f *argFlag) Set(val string) error {
	v := strings.SplitN(val, "=", 2)
	if len(v) != 2 {
		return fmt.Errorf("expected format <key>=<value>")
	}
	(*f)[v[0]] = v[1]
	return nil
}

func (f *argFlag) String() string {
	fv := *f
	vals := make([]string, 0, len(fv))

	for k, v := range fv {
		vals = append(vals, k+"="+v)
	}
	return strings.Join(vals, " ")
}
