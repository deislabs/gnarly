package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/moby/buildkit/frontend/dockerfile/dockerfile2llb"
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
		panic(err)
	}

	result := Run(dt, buildArgs)

	switch format {
	case formatModfile:
		data, err := json.MarshalIndent(result, "", "\t")
		if err != nil {
			panic(err)
		}
		fmt.Println(string(data))
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
	default:
		panic(fmt.Sprintf("unknown format: %s", format))
	}
}

func Run(dt []byte, buildArgs map[string]string) Result {
	targets, err := dockerfile2llb.ListTargets(context.TODO(), dt)
	if err != nil {
		panic(err)
	}

	r := newResolver()
	for _, target := range targets.Targets {
		_, err = dockerfile2llb.Dockefile2Outline(context.TODO(), dt, dockerfile2llb.ConvertOpt{
			BuildArgs: func() map[string]string {
				if len(buildArgs) > 0 {
					return buildArgs
				}
				return nil
			}(),
			Target:       target.Name,
			MetaResolver: r,
		})
		if err != nil {
			panic(err)
		}
	}

	var result Result

	buf := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)

	type matchRule struct {
		Match   string `json:"match"`
		Replace string `json:"replace"`
		regex   *regexp.Regexp
	}

	var matchers []matchRule
	if modProg == "" {
		data, err := os.ReadFile(modConfig)
		if err != nil && !os.IsNotExist(err) {
			panic(err)
		}
		if len(data) > 0 {
			if err := json.Unmarshal(data, &matchers); err != nil {
				panic(err)
			}
			for i, v := range matchers {
				matchers[i].regex = regexp.MustCompile(v.Match)
			}
		}
	}

	replace := func(ref string) string {
		if modProg == "" {
			for _, rule := range matchers {
				if !rule.regex.MatchString(ref) {
					continue
				}
				return rule.regex.ReplaceAllString(ref, rule.Replace)
			}
			return ""
		}

		buf.Reset()
		stderr.Reset()
		cmdWithArgs := strings.Fields(modProg)
		cmdWithArgs = append(cmdWithArgs, ref)
		cmd := exec.Command(cmdWithArgs[0], cmdWithArgs[1:]...)
		cmd.Stdout = buf
		cmd.Stderr = stderr
		if modConfig != "" {
			cmd.Env = append(os.Environ(), "MOD_CONFIG="+modConfig)
		}
		if err := cmd.Run(); err != nil {
			if stderr.Len() == 0 {
				stderr.WriteString("<no output from program>")
			}
			panic(fmt.Sprintf("%s: %v", stderr, err))
		}

		if stderr.Len() > 0 {
			io.Copy(os.Stderr, stderr)
		}

		return strings.TrimSpace(buf.String())
	}

	for _, resolved := range r.refs {
		s := Source{Type: "docker-image", Ref: resolved, Replace: replace(resolved)}
		result.Sources = append(result.Sources, s)
	}
	return result
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

type Source struct {
	Type    string `json:"type"`
	Ref     string `json:"ref"`
	Replace string `json:"replace,omitempty"`
}

type Result struct {
	Sources []Source `json:"sources"`
}
