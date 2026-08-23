// Command build-images builds every mailez mail image from the
// component sources under deploy/images (shared infrastructure) and
// deploy/engines/<engine>/<component> (engine-specific services).
//
// Components are discovered from the directory layout, so adding a new
// engine (e.g. engines/stalwart) requires no changes to this command other
// than registering its image names below.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// imageName maps component directory names to docker image names.
// Shared infrastructure directories describe the component's role (gateway,
// mail-filter, ...) while engine directories use the software name directly
// (postfix, dovecot). Image names are the stable compose contract
// (mailez/nginx, mailez/postfix, ...) and must not change without updating
// the compose files.
var imageName = map[string]string{
	"gateway":       "nginx",
	"mail-filter":   "rspamd",
	"resolver":      "unbound",
	"macro-scanner": "macro-scanner",
	"postfix":       "postfix",
	"dovecot":       "dovecot",
}

// component describes one buildable image.
type component struct {
	dir  string
	name string
}

func main() {
	version := flag.String("version", "local", "image version tag")
	flag.Parse()

	root, err := repoRoot()
	if err != nil {
		fatal(err)
	}
	docker, err := findDocker()
	if err != nil {
		fatal(err)
	}

	components, err := discoverComponents(root)
	if err != nil {
		fatal(err)
	}
	if len(components) == 0 {
		fatal(fmt.Errorf("no components found under %s", filepath.Join(root, "deploy")))
	}

	fmt.Printf("==> %d images to build (version=%s)\n", len(components), *version)
	for _, c := range components {
		buildImage(docker, root, c, *version)
	}
	fmt.Println("==> all images built")
}

// repoRoot walks up from the working directory to the repository root
// (identified by backend/go.mod), so the command runs from anywhere.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "backend", "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("repository root not found (no backend/go.mod in any parent)")
		}
		dir = parent
	}
}

// findDocker locates the docker CLI: $DOCKER_CMD wins, then PATH, then the
// Docker Desktop locations on Windows.
func findDocker() (string, error) {
	if v := os.Getenv("DOCKER_CMD"); v != "" {
		return v, nil
	}
	if p, err := exec.LookPath("docker"); err == nil {
		return p, nil
	}
	if runtime.GOOS == "windows" {
		candidates := []string{
			`C:\Users\admin\AppData\Local\Programs\DockerDesktop\resources\bin\docker.exe`,
			`C:\Program Files\Docker\Docker\resources\bin\docker.exe`,
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				return c, nil
			}
		}
	}
	return "", fmt.Errorf("docker CLI not found; set DOCKER_CMD to its path")
}

// discoverComponents scans deploy/images/* (shared) and deploy/engines/*/*
// (per engine) for component directories carrying a Dockerfile, registering
// each against the image-name map.
func discoverComponents(root string) ([]component, error) {
	var dirs []string
	imagesRoot := filepath.Join(root, "deploy", "images")
	enginesRoot := filepath.Join(root, "deploy", "engines")

	if entries, err := os.ReadDir(imagesRoot); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				dirs = append(dirs, filepath.Join(imagesRoot, e.Name()))
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read %s: %w", imagesRoot, err)
	}

	if engines, err := os.ReadDir(enginesRoot); err == nil {
		for _, eng := range engines {
			if !eng.IsDir() {
				continue
			}
			components, err := os.ReadDir(filepath.Join(enginesRoot, eng.Name()))
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", filepath.Join(enginesRoot, eng.Name()), err)
			}
			for _, c := range components {
				if c.IsDir() {
					dirs = append(dirs, filepath.Join(enginesRoot, eng.Name(), c.Name()))
				}
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read %s: %w", enginesRoot, err)
	}

	sort.Strings(dirs)
	var out []component
	for _, dir := range dirs {
		if _, err := os.Stat(filepath.Join(dir, "Dockerfile")); err != nil {
			continue
		}
		name, ok := imageName[filepath.Base(dir)]
		if !ok {
			return nil, fmt.Errorf(
				"component %s has no registered image name; add it to imageName in build-images",
				dir,
			)
		}
		out = append(out, component{dir: dir, name: name})
	}
	return out, nil
}

// buildImage runs docker build with the repository root as context so
// multi-stage Dockerfiles can COPY backend sources.
func buildImage(docker, root string, c component, version string) {
	dockerfile := filepath.Join(c.dir, "Dockerfile")
	tag := fmt.Sprintf("mailez/%s:%s", c.name, version)
	fmt.Printf("==> building %s from %s\n", tag, c.dir)
	cmd := exec.Command(docker, "build", "-f", dockerfile,
		"--build-arg", "VERSION="+version,
		"-t", tag,
		root,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fatal(fmt.Errorf("build failed: %s", strings.TrimSpace(tag)))
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "build-images: %v\n", err)
	os.Exit(1)
}
