// Command fetchexternal imports the outside datasets and test suites listed in the private corpus's fixtures/private/external/manifest.json from their sources, at the pinned commit or checksum, into fixtures/private/external/<id>/. It is a maintainer's tool: CI and other maintainers fetch the result from the bucket with scripts/private-corpus fetch-external, and a maintainer uploads it with scripts/private-corpus push-external (docs/private-corpus.md). Run it from the repository root:
//
//	go run ./internal/tools/fetchexternal [id-or-name ...]
//
// With arguments, only those sources are imported. A source already present is left alone; delete its directory to import it again.
package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
)

const dir = "fixtures/private/external"

type source struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Archive string   `json:"archive"`
	MD5     string   `json:"md5"`
	Skip    []string `json:"skip"` // archive members the tests do not read, left packed
	Git     string   `json:"git"`
	Commit  string   `json:"commit"`
	Paths   []string `json:"paths"`
}

func main() {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		fail(err)
	}
	var m struct{ Sources []source }
	if err := json.Unmarshal(data, &m); err != nil {
		fail(err)
	}
	for _, s := range m.Sources {
		if len(os.Args) > 1 && !slices.Contains(os.Args[1:], s.ID) && !slices.Contains(os.Args[1:], s.Name) {
			continue
		}
		d := filepath.Join(dir, s.ID)
		if _, err := os.Stat(filepath.Join(d, ".fetched")); err == nil {
			fmt.Printf("%s: present\n", s.ID)
			continue
		}
		fmt.Printf("%s: importing\n", s.ID)
		if err := os.MkdirAll(d, 0o700); err != nil {
			fail(err)
		}
		switch {
		case s.Git != "":
			err = fetchGit(s, d)
		case s.Archive != "":
			err = fetchArchive(s, d)
		}
		if err != nil {
			fail(fmt.Errorf("%s: %w", s.ID, err))
		}
		if err := os.WriteFile(filepath.Join(d, ".fetched"), nil, 0o644); err != nil {
			fail(err)
		}
	}
}

// fetchGit checks out only the listed paths of the pinned commit.
func fetchGit(s source, dir string) error {
	repo := filepath.Join(dir, "repo")
	steps := [][]string{
		{"git", "init", "-q", repo},
		{"git", "-C", repo, "remote", "add", "origin", s.Git},
		{"git", "-C", repo, "sparse-checkout", "set", "--no-cone"},
		{"git", "-C", repo, "fetch", "-q", "--depth", "1", "--filter=blob:none", "origin", s.Commit},
	}
	steps[2] = append(steps[2], s.Paths...)
	steps = append(steps, []string{"git", "-C", repo, "checkout", "-q", s.Commit})
	for _, st := range steps {
		if err := run(st...); err != nil {
			return err
		}
	}
	return nil
}

// fetchArchive downloads a .tar.gz, checks its MD5 and unpacks it, leaving the skipped members packed.
func fetchArchive(s source, dir string) error {
	file := filepath.Join(dir, "archive.tar.gz")
	if sum, err := md5sum(file); err != nil || sum != s.MD5 {
		if err := run("curl", "-sSfL", "--retry", "20", "-o", file, s.Archive); err != nil {
			return err
		}
	}
	sum, err := md5sum(file)
	if err != nil {
		return err
	}
	if sum != s.MD5 {
		return fmt.Errorf("archive md5 %s, want %s", sum, s.MD5)
	}
	args := []string{"tar", "-xzf", file, "-C", dir}
	for _, p := range s.Skip {
		args = append(args, "--exclude="+p)
	}
	return run(args...)
}

func md5sum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func run(args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "fetchexternal:", err)
	os.Exit(1)
}
