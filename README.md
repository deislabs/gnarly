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
The `mod.sh` script uses `mod.json` as a lookup table for replacements.
For each ref that is found in the Dockerfile by `dockersource`, the `mod.sh` is called with the found ref as the first argument. The `mod.sh` script can return an empty string or a replacement ref.

The output of this is saved to `Dockerfile.mod` which is a special file that the syntax parser shown above will parse to handle replacements.