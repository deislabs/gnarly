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
	"strings"

	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/cpuguy83/dockercfg"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/frontend/dockerfile/dockerfile2llb"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func main() {
	buildArgs := &argFlag{}
	flag.Var(buildArgs, "build-arg", "set build args to pass through -- these are required if the dockerfie uses args to determine an image source")
	modProg := flag.String("mod-prog", os.Getenv("DOCKERFILE_MOD_PROG"), "Set program to execute to modify a reference as a replace rule")
	flag.Parse()

	var dt []byte
	var err error
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

	targets, err := dockerfile2llb.ListTargets(context.TODO(), dt)
	if err != nil {
		panic(err)
	}

	r := newResolver()
	for _, target := range targets.Targets {
		_, err = dockerfile2llb.Dockefile2Outline(context.TODO(), dt, dockerfile2llb.ConvertOpt{
			BuildArgs: func() map[string]string {
				args := *buildArgs
				if len(args) > 0 {
					return args
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
	for ref := range r.meta {
		s := Source{Type: "docker-image", Ref: ref}
		if *modProg != "" {
			buf.Reset()
			stderr.Reset()

			cmd := exec.Command(*modProg, ref)
			cmd.Stdout = buf
			cmd.Stderr = stderr
			if err := cmd.Run(); err != nil {
				if stderr.Len() == 0 {
					stderr.WriteString("<no output from program>")
				}
				panic(fmt.Sprintf("%s: %v", stderr, err))
			}

			s.Replace = strings.TrimSpace(buf.String())
		}
		result.Sources = append(result.Sources, s)
	}

	data, err := json.MarshalIndent(result, "", "\t")
	if err != nil {
		panic(err)
	}

	fmt.Println(string(data))
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

func newResolver() *metaResolver {
	cfg, err := dockercfg.LoadDefaultConfig()
	if err != nil {
		panic(err)
	}
	return &metaResolver{
		r: docker.NewResolver(docker.ResolverOptions{
			Hosts: docker.ConfigureDefaultRegistries(
				docker.WithAuthorizer(docker.NewDockerAuthorizer(
					docker.WithAuthCreds(cfg.GetRegistryCredentials),
				)),
			)}),
		meta: make(map[string]v1.Descriptor),
	}
}

type metaResolver struct {
	r    remotes.Resolver
	meta map[string]v1.Descriptor
}

func (r *metaResolver) ResolveImageConfig(ctx context.Context, ref string, opt llb.ResolveImageConfigOpt) (digest.Digest, []byte, error) {
	name, desc, err := r.r.Resolve(ctx, ref)
	if err != nil {
		return "", nil, err
	}

	r.meta[ref] = desc

	f, err := r.r.Fetcher(ctx, name)
	if err != nil {
		return "", nil, err
	}

	rdr, err := f.Fetch(ctx, desc)
	if err != nil {
		return "", nil, err
	}
	defer rdr.Close()

	lr := io.LimitReader(rdr, desc.Size)

	data, err := ioutil.ReadAll(lr)
	if err != nil {
		return "", nil, err
	}

	return desc.Digest, data, nil
}
