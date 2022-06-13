This is a tool to get a list of image sources out of a Dockerfile.

It can also optionally run another program to add a "replace" directive to test out a feature I'm working on to replace images with a different source reference.

This tool only generates metadata that can be fed into other tools.
It is based on a lot of in progress work and is only here as a placeholder.

### Example Usage

```terminal
$ ./dockersource --mod-prog=./mod.sh | tee Dockerfile.mod
{
        "sources": [
                {
                        "type": "docker-image",
                        "ref": "docker.io/library/golang:1.18",
                        "replace": "mcr.microsoft.com/oss/go/microsoft/golang:1.18"
                }
        ]
}
$ docker buildx build --build-arg BUILDKIT_SYNTAX=mcr.microsoft.com/oss/moby/dockerfile:modfile0 .
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