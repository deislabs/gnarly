package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const (
	modProg = "modprog"
	docker  = "docker"
	gnarly  = "gnarly"

	bInfoKey     = "containerimage.buildinfo"
	imageNameKey = "image.name"
)

type Result struct {
	Sources []Source `json:"sources"`
	Image   []string `json:"-"`
}

func unmarshalResult(t *testing.T, b []byte) Result {
	r := map[string]json.RawMessage{}
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	var ret Result
	if v, ok := r[bInfoKey]; ok {
		if err := json.Unmarshal(v, &ret); err != nil {
			t.Fatal(err)
		}

		var s string
		if err := json.Unmarshal(r[imageNameKey], &s); err != nil {
			t.Fatal(err)
		}
		ret.Image = strings.Split(s, ",")
		return ret
	}

	if err := json.Unmarshal(b, &ret); err != nil {
		t.Fatal(err)
	}
	return ret
}

func marshalResult(t *testing.T, val interface{}) []byte {
	t.Helper()

	switch v := val.(type) {
	case Result:
		sort.Slice(v.Sources, func(i, j int) bool {
			return v.Sources[i].Ref < v.Sources[j].Ref
		})
		val = v
		if len(v.Image) > 0 {
			val = map[string]interface{}{
				bInfoKey:     v,
				imageNameKey: strings.Join(v.Image, ","),
			}
		}
	case map[string]interface{}:
		vv, ok := v[bInfoKey]
		if ok {
			vr := vv.(Result)
			sort.Slice(vr.Sources, func(i, j int) bool {
				return vr.Sources[i].Ref < vr.Sources[j].Ref
			})
			v[bInfoKey] = vr
		}
		val = v
	}

	b, err := json.MarshalIndent(val, "", "\t")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type Source struct {
	Type    string `json:"type"`
	Ref     string `json:"ref"`
	Replace string `json:"replace,omitempty"`
}

func (s Source) AsFlag() string {
	if s.Replace == "" {
		return ""
	}
	return fmt.Sprintf("--build-context %s=%s://%s", s.Ref, s.Type, s.Replace)
}

func (r Result) AsFlags() string {
	sb := &strings.Builder{}
	for _, s := range r.Sources {
		sb.WriteString(s.AsFlag())
		sb.WriteString(" ")
	}
	return sb.String()
}

type cmdOpt func(t *testing.T, cfg *cmdConfig)

func withStdin(t *testing.T, cfg *cmdConfig) {
	cfg.Stdin = true
}

func withDockerfile(dockerfile io.Reader) cmdOpt {
	return func(t *testing.T, cfg *cmdConfig) {
		cfg.Dockerfile = dockerfile
	}
}

func withFormat(format string) cmdOpt {
	return func(t *testing.T, cfg *cmdConfig) {
		cfg.Format = format
	}
}

func withModProg(t *testing.T, cfg *cmdConfig) {
	cfg.ModProg = AsModProg(t)
}

func withModConfig(config []byte) cmdOpt {
	return func(t *testing.T, cfg *cmdConfig) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "config")
		if err := ioutil.WriteFile(configPath, config, 0644); err != nil {
			t.Fatal(err)
		}
		cfg.ModConfig = configPath
	}
}

func withDockerArgs(args ...string) cmdOpt {
	return func(t *testing.T, cfg *cmdConfig) {
		cfg.DockerArgs = args
	}
}

func withTags(tags ...string) cmdOpt {
	return func(t *testing.T, cfg *cmdConfig) {
		cfg.Tags = append(cfg.Tags, tags...)
	}
}

func withOutput(output ...string) cmdOpt {
	return func(t *testing.T, cfg *cmdConfig) {
		cfg.Output = append(cfg.Output, output...)
	}
}

func withModfile(r Result) cmdOpt {
	return func(t *testing.T, cfg *cmdConfig) {
		p := filepath.Join(t.TempDir(), "modfile")
		if err := os.WriteFile(p, marshalResult(t, r), 0600); err != nil {
			t.Fatal(err)
		}
		cfg.Modfile = p
	}
}

func withAlt(alt []byte) cmdOpt {
	return func(t *testing.T, cfg *cmdConfig) {
		cfg.expectedAlt = alt
	}
}

type cmdConfig struct {
	Format      string
	AsDocker    bool
	DockerArgs  []string
	Dockerfile  io.Reader
	Stdin       bool
	ModProg     string
	ModConfig   string
	Modfile     string
	expectedAlt []byte
	Tags        []string
	Output      []string
}

var openOnce sync.Once

// This is a hack to bust the go test cache automatically
func bustCmdCache(t *testing.T) {
	openOnce.Do(func() {
		// Opening the root dir busts the cache any time anything changes
		// We could probably scope this down to just non-test .go files, but we'd need to list them using `ls` or something, because `os.ReadDir` will open the dir anyway.
		f, err := os.Open(filepath.Dir(getwd()))
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
	})
}

func testCmd(expected []byte, opts ...cmdOpt) func(t *testing.T) {
	return func(t *testing.T) {
		t.Parallel()
		bustCmdCache(t)

		var cfg cmdConfig
		for _, o := range opts {
			o(t, &cfg)
		}

		prog := gnarly
		if cfg.AsDocker {
			prog = docker
		}
		cmd := exec.Command(prog, cfg.DockerArgs...)
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, "DEBUG=1")

		if len(cfg.Tags) > 0 {
			tags := "BUILDKIT_TAG=" + strings.Join(cfg.Tags, ",")
			cmd.Env = append(cmd.Env, tags)
		}

		if len(cfg.DockerArgs) > 0 {
			if !cfg.AsDocker {
				cmd.Env = append(cmd.Env, "DOCKERFILE_MOD_INVOKE_DOCKER=1")
			} else {
				dir := t.TempDir()
				p := filepath.Join(dir, docker)
				if err := os.Symlink(gnarlyPath, p); err != nil {
					t.Fatal(err)
				}
				t.Setenv("PATH", p+":"+os.Getenv("PATH"))
			}
		}

		metdataFilePath := filepath.Join(t.TempDir(), "metadata.json")
		if len(cfg.DockerArgs) > 0 || cfg.AsDocker {
			cmd.Env = append(cmd.Env, "BUILDKIT_METADATA_FILE="+metdataFilePath)
		}

		if cfg.ModProg != "" {
			cmd.Env = append(cmd.Env, "DOCKERFILE_MOD_PROG="+cfg.ModProg)
		}
		if cfg.ModConfig != "" {
			cmd.Env = append(cmd.Env, "DOCKERFILE_MOD_CONFIG="+cfg.ModConfig)
		}
		if cfg.Format != "" {
			cmd.Env = append(cmd.Env, "DOCKERFILE_MOD_FORMAT="+cfg.Format)
		}
		if cfg.Modfile != "" {
			cmd.Env = append(cmd.Env, "DOCKERFILE_MOD_PATH="+cfg.Modfile)
		}
		if len(cfg.Output) > 0 {
			cmd.Env = append(cmd.Env, "BUILDKIT_OUTPUT="+strings.Join(cfg.Output, "\n"))
		}

		if cfg.Dockerfile != nil {
			if len(cfg.DockerArgs) > 0 {
				cmd.Args = append(cmd.Args, "--output=type=docker,dest="+filepath.Join(t.TempDir(), "img.tar"))
			}
			if cfg.Stdin {
				cmd.Stdin = cfg.Dockerfile
				if cfg.AsDocker || len(cfg.DockerArgs) > 0 {
					cmd.Args = append(cmd.Args, "-")
				}
			} else {
				data, err := ioutil.ReadAll(cfg.Dockerfile)
				if err != nil {
					t.Fatal(err)
				}

				p := filepath.Join(t.TempDir(), "Dockerfile")
				if err := os.WriteFile(p, data, 0644); err != nil {
					t.Fatal(err)
				}

				if cfg.AsDocker || len(cfg.DockerArgs) > 0 {
					p = filepath.Dir(p)
				}
				cmd.Args = append(cmd.Args, p)
			}
		}

		stdout := bytes.NewBuffer(nil)
		stderr := bytes.NewBuffer(nil)

		defer func() {
			if t.Failed() {
				t.Log(cmd.Args)
				if stdout.Len() > 0 {
					t.Log(stdout)
				}
				if stderr.Len() > 0 {
					t.Log(stderr)
				}
			}
		}()

		cmd.Stdout = stdout
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}

		out := stdout.Bytes()
		if cfg.AsDocker || len(cfg.DockerArgs) > 0 {
			if cfg.DockerArgs[0] == "build" || (cfg.DockerArgs[0] == "buildx" && cfg.DockerArgs[1] == "build") {
				meta, err := os.ReadFile(metdataFilePath)
				if err != nil {
					t.Fatalf("error reading docker build metadata file: %v", err)
				}

				actualR := unmarshalResult(t, meta)
				out = marshalResult(t, actualR)

				exepctedR := unmarshalResult(t, expected)
				expected = marshalResult(t, exepctedR)
			}
		}

		for _, o := range cfg.Output {
			split := strings.SplitN(o, "dest=", 2)
			if len(split) > 1 {
				_, err := os.Stat(strings.SplitN(split[1], ",", 2)[0])
				if err != nil {
					t.Errorf("could not stat output file: %v", err)
				}
			}
		}

		out = bytes.TrimSpace(out)
		if !bytes.Equal(out, expected) {
			var allowAlt bool
			if cfg.expectedAlt != nil {
				var err error
				if allowAltEnv := os.Getenv("TEST_ALLOW_ALT_META"); allowAltEnv != "" {
					allowAlt, err = strconv.ParseBool(allowAltEnv)
					if err != nil {
						t.Log(err)
					}
				}
			}
			if !allowAlt {
				t.Fatalf("expected %s, got %s", string(expected), string(out))
			}
			if !bytes.Equal(out, cfg.expectedAlt) {
				t.Errorf("expected %s, got %s", string(expected), string(out))
				t.Fatalf("expected %s, got %s", string(cfg.expectedAlt), string(out))
			}
		}
	}
}

func createBuildx(t *testing.T) {
	builderName := strings.ToLower(strings.Replace(t.Name(), "/", "_", -1))
	if _, err := exec.Command(docker, "buildx", "create", "--name", builderName).CombinedOutput(); err != nil {
		exec.Command(docker, "buildx", "create", "--name", builderName).Run()
		out, err := exec.Command(docker, "buildx", "create", "--name", builderName).CombinedOutput()
		if err != nil {
			t.Fatal(string(out))
		}
	}
	t.Cleanup(func() {
		out, err := exec.Command("docker", "buildx", "rm", builderName).CombinedOutput()
		if err != nil {
			t.Log(string(out))
		}
	})

	out, err := exec.Command(docker, "buildx", "inspect", "--bootstrap", builderName).CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}
	t.Setenv("BUILDX_BUILDER", builderName)
}
