package main

import (
	"context"
	"io"
	"io/ioutil"
	"os"

	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/cpuguy83/dockercfg"
	"github.com/moby/buildkit/client/llb"
	"github.com/opencontainers/go-digest"
)

func newResolver() *metaResolver {
	cfg, err := dockercfg.LoadDefaultConfig()
	if err != nil {
		if !os.IsNotExist(err) {
			panic(err)
		}
		return &metaResolver{r: docker.NewResolver(docker.ResolverOptions{
			Hosts: docker.ConfigureDefaultRegistries(),
		}),
			refs: make(map[string]string),
		}
	}
	return &metaResolver{
		r: docker.NewResolver(docker.ResolverOptions{
			Hosts: docker.ConfigureDefaultRegistries(
				docker.WithAuthorizer(docker.NewDockerAuthorizer(
					docker.WithAuthCreds(cfg.GetRegistryCredentials),
				)),
			)}),
		refs:     make(map[string]string),
		resolved: make(map[string]resolved),
	}
}

type resolved struct {
	config []byte
	digest digest.Digest
}

type metaResolver struct {
	r        remotes.Resolver
	refs     map[string]string
	resolved map[string]resolved
}

func (r *metaResolver) ResolveImageConfig(ctx context.Context, ref string, opt llb.ResolveImageConfigOpt) (digest.Digest, []byte, error) {
	if res, ok := r.resolved[ref]; ok {
		return res.digest, res.config, nil
	}

	name, desc, err := r.r.Resolve(ctx, ref)
	if err != nil {
		return "", nil, err
	}

	if res, ok := r.resolved[name]; ok {
		r.resolved[ref] = res
		return res.digest, res.config, nil
	}

	r.refs[ref] = name

	var res resolved
	res.digest = desc.Digest

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

	res.config = data

	r.resolved[ref] = res
	r.resolved[name] = res

	return desc.Digest, data, nil
}
