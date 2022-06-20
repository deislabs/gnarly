This is a tool to get a list of image sources out of a Dockerfile.

It can also optionally run another program to add a "replace" directive to test out a feature I'm working on to replace images with a different source reference.

This tool only generates metadata that can be fed into other tools.
It is based on a lot of in progress work and is only here as a placeholder.

### Example Usage

This tool can output data in two formats:

- `--format=build-flags` - Used to output flags directly to `docker buildx build`
- `--format=modfile` - Experimental file which requires a custom syntax parser (`--build-arg BUILDKIT_SYNTAX="mcr.microsoft.com/oss/moby/dockerfile:modfile1"`). The file is passed along with the build context and repalcements are by the parser during build.

The default format is `build-flags`.

```console
$ ./dockersource --mod-prog=./mod.sh
--build-context docker.io/library/golang:1.18=docker-image://mcr.microsoft.com/oss/go/microsoft/golang:1.18 $
```

Only sources that have a replacement will be output.
Notice it does not print a newline character so it can be passed directly to `docker buildx build`.

When using this format, whatever `--build-args` you pass to this tool will also be part of the output so you don't have to specify build-args to both this tool and to `docker buildx build.

For `--format=modfile`:

```console
$ ./dockersource --format=modfile --mod-prog=./mod.sh | tee Dockerfile.mod
{
        "sources": [
                {
                        "type": "docker-image",
                        "ref": "docker.io/library/golang:1.18",
                        "replace": "mcr.microsoft.com/oss/go/microsoft/golang:1.18"
                }
        ]
}
$ docker buildx build --build-arg BUILDKIT_SYNTAX=mcr.microsoft.com/oss/moby/dockerfile:modfile1 .
```

Here we've told `dockersource` to use `mod.sh` as a tool to handle replacements.
The `mod.sh` script uses `lookup.json` as a lookup table for replacements.
For each ref that is found in the Dockerfile by `dockersource`, the `mod.sh` is called with the found ref as the first argument. The `mod.sh` script can return an empty string or a replacement ref.
You can specify a path to a config file to use, which will be passed along to the mod-prog as an environment variable `MOD_CONFIG`.

The output of this is saved to `Dockerfile.mod` which is a special file that the syntax parser shown above will parse to handle replacements.

This also supports a built-in replacement generator.
This takes a config file (passed via `--mod-config`) with a list of match/replace rules.
The first match is used for each ref.

```json
[
        {"match": "<some matcher>", "replace": "<some value>"}
]
```

The `match` field can be a regex, and the `replace` value can make use of capture groups from the regex.
See [regexp.ReplaceAllString](https://pkg.go.dev/regexp#Regexp.ReplaceAllString) for more details.
As an example, see `mod-builtin.json`.

In some cases you may not want to modify the main build context with a Dockerfile.mod, which could dirty the git tree or potentially interfere with the actual build. For this case you can use a special "named" context with the mod file in it.


```console
$ dir="$(mktemp -d)" # Make a temp dir where we'll store the Dockerfile.mod
$ ./dockersource --format=modfile --mod-prog=./mod.sh | tee "${dir}/Dockerfile.mod" # Generate the Dockerfile.mod and store it in the temp dir created above.
{
        "sources": [
                {
                        "type": "docker-image",
                        "ref": "docker.io/library/golang:1.18",
                        "replace": "mcr.microsoft.com/oss/go/microsoft/golang:1.18"
                }
        ]
}
$ docker buildx build --build-arg BUILDKIT_SYNTAX=mcr.microsoft.com/oss/moby/dockerfile:modfile1 --build-context "dockerfile-mod=${dir}" .
[+] Building 2.2s (17/17) FINISHED                                                                                                                                                                                                                                                            
 => [internal] load .dockerignore                                                                                                                                                                                                                                                        0.0s
 => => transferring context: 2B                                                                                                                                                                                                                                                          0.0s
 => [internal] load build definition from Dockerfile                                                                                                                                                                                                                                     0.0s
 => => transferring dockerfile: 496B                                                                                                                                                                                                                                                     0.0s
 => resolve image config for mcr.microsoft.com/oss/moby/dockerfile:modfile1                                                                                                                                                                                                              0.3s
 => CACHED docker-image://mcr.microsoft.com/oss/moby/dockerfile:modfile1@sha256:ddccaae065a61196876e89b99eb88ac66cfbc4e21daea9e90f0588dab02420ae                                                                                                                                         0.0s
 => => resolve mcr.microsoft.com/oss/moby/dockerfile:modfile1@sha256:ddccaae065a61196876e89b99eb88ac66cfbc4e21daea9e90f0588dab02420ae                                                                                                                                                    0.0s
 => [internal] load build definition from Dockerfile                                                                                                                                                                                                                                     0.0s
 => => transferring dockerfile: 496B                                                                                                                                                                                                                                                     0.0s
 => [context dockerfile-mod] load .dockerignore                                                                                                                                                                                                                                          0.0s
 => => transferring dockerfile-mod: 2B                                                                                                                                                                                                                                                   0.0s
 => [context dockerfile-mod] load from client                                                                                                                                                                                                                                            0.0s
 => => transferring dockerfile-mod: 36B                                                                                                                                                                                                                                                  0.0s
 => [internal] load metadata for mcr.microsoft.com/oss/go/microsoft/golang:1.18                                                                                                                                                                                                          0.1s
 => [build 1/7] FROM mcr.microsoft.com/oss/go/microsoft/golang:1.18@sha256:fba12e22cb828665f844f123c5bfd5143f8e9c00c960d6abd4653b1b0e35df6c                                                                                                                                              0.0s
 => => resolve mcr.microsoft.com/oss/go/microsoft/golang:1.18@sha256:fba12e22cb828665f844f123c5bfd5143f8e9c00c960d6abd4653b1b0e35df6c                                                                                                                                                    0.0s
 => [internal] load build context                                                                                                                                                                                                                                                        0.0s
 => => transferring context: 19.10kB                                                                                                                                                                                                                                                     0.0s
 => CACHED [build 2/7] WORKDIR /go/src/github.com/cpuguy83/dockersource                                                                                                                                                                                                                  0.0s
 => CACHED [build 3/7] COPY go.mod .                                                                                                                                                                                                                                                     0.0s
 => CACHED [build 4/7] COPY go.sum .                                                                                                                                                                                                                                                     0.0s
 => CACHED [build 5/7] RUN     --mount=type=cache,target=/go/pkg/mod     --mount=type=cache,target=/root/.cache/go-build     go mod download                                                                                                                                             0.0s
 => [build 6/7] COPY . .                                                                                                                                                                                                                                                                 0.2s
 => [build 7/7] RUN     --mount=type=cache,target=/go/pkg/mod     --mount=type=cache,target=/root/.cache/go-build     CGO_ENABLED=0 go build .                                                                                                                                           1.2s
 => [stage-1 1/1] COPY --from=build /go/src/github.com/cpuguy83/dockersource/dockersource /
$ rm -rf "${dir}"
```

Here we have passed a `--build-context` with a value of `"dockerfile-mod=${dir}"`.
This is an extra context that buildx will send called `dockerfile-mod`.
In the specified `BUILDKIT_SYNTAX`, the `dockerfile-mod` named context is a special context it will use to look for a `Dockerfile.mod`.
This can be a directory, an image, or even a URL. See https://www.docker.com/blog/dockerfiles-now-support-multiple-build-contexts/ for more details on named contexts.
You can also specify a custom name for the named context, but you'll need to set a `--build-arg` to tell the builder about that name.

```console
$ docker buildx build --build-arg BUILDKIT_SYNTAX=mcr.microsoft.com/oss/moby/dockerfile:modfile1 --build-context "my-custom-name=${dir}" --build-arg BUILDKIT_MOD_CONTEXT=my-custom-name .
```