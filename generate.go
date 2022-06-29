package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	"github.com/moby/buildkit/frontend/dockerfile/dockerfile2llb"
)

type Source struct {
	Type    string `json:"type"`
	Ref     string `json:"ref"`
	Replace string `json:"replace,omitempty"`
}

type Result struct {
	Sources []Source `json:"sources"`
}

func Generate(ctx context.Context, dt []byte, buildArgs map[string]string) (Result, error) {
	targets, err := dockerfile2llb.ListTargets(context.TODO(), dt)
	if err != nil {
		return Result{}, fmt.Errorf("error listing dockerfile targets: %w", err)
	}

	r := newResolver()
	for _, target := range targets.Targets {
		_, err = dockerfile2llb.Dockefile2Outline(ctx, dt, dockerfile2llb.ConvertOpt{
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
			return Result{}, fmt.Errorf("error parsing dockerfile: %w", err)
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
	if modProg == "" && modConfig != "" {
		data, err := os.ReadFile(modConfig)
		if err != nil {
			return Result{}, fmt.Errorf("error reading mod config: %w", err)
		}
		if len(data) > 0 {
			if err := json.Unmarshal(data, &matchers); err != nil {
				return Result{}, fmt.Errorf("error parsing mod config for builtin matcher: %w", err)
			}
			for i, v := range matchers {
				matchers[i].regex, err = regexp.Compile(v.Match)
				if err != nil {
					return Result{}, fmt.Errorf("error compiling matcher regex from mod config: %w", err)
				}
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
		cmd := exec.CommandContext(ctx, cmdWithArgs[0], cmdWithArgs[1:]...)
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
		debug("resolved", s.Ref, "with replacement:", s.Replace)
		result.Sources = append(result.Sources, s)
	}

	// Sort for stable output for testing
	sort.Slice(result.Sources, func(i, j int) bool {
		return result.Sources[i].Ref < result.Sources[j].Ref
	})

	return result, nil
}
