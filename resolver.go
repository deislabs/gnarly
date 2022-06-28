package main

import (
	"context"
	"sync"

	"github.com/moby/buildkit/client/llb"
	"github.com/opencontainers/go-digest"
)

func newResolver() *metaResolver {
	return &metaResolver{
		refs: make(map[string]string),
	}
}

type metaResolver struct {
	mu   sync.Mutex
	refs map[string]string
}

const (
	emptyDigest = digest.Digest("sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a")
	emptyConfig = "{}"
)

func (r *metaResolver) ResolveImageConfig(ctx context.Context, ref string, opt llb.ResolveImageConfigOpt) (digest.Digest, []byte, error) {
	r.mu.Lock()
	r.refs[ref] = ref
	r.mu.Unlock()
	return emptyDigest, []byte(emptyConfig), nil
}
