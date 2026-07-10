package main

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/goccy/go-yaml"
)

var markdownLinkPattern = regexp.MustCompile(`\[[^\]]*\]\(([^)]+)\)`)

func main() {
	markdownCount, err := checkMarkdownLinks(".")
	if err != nil {
		fail(err)
	}
	if err := checkOpenAPI("api/openapi.yaml"); err != nil {
		fail(err)
	}
	fmt.Printf("documentation checks passed: %d Markdown files, OpenAPI YAML valid\n", markdownCount)
}

func checkMarkdownLinks(root string) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && shouldSkipDirectory(path) {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		count++
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range markdownLinkPattern.FindAllStringSubmatch(string(content), -1) {
			if err := checkLocalLink(path, match[1]); err != nil {
				return err
			}
		}
		return nil
	})
	return count, err
}

func shouldSkipDirectory(path string) bool {
	clean := filepath.ToSlash(path)
	return clean == ".git" ||
		clean == "bin" ||
		clean == "web/node_modules" ||
		clean == "web/dist"
}

func checkLocalLink(sourcePath, rawLink string) error {
	link := strings.Trim(rawLink, "<>")
	parsed, err := url.Parse(link)
	if err != nil {
		return fmt.Errorf("parse link in %s: %q: %w", sourcePath, rawLink, err)
	}
	if parsed.Scheme != "" || parsed.Host != "" || parsed.Path == "" {
		return nil
	}
	target := filepath.Clean(filepath.Join(filepath.Dir(sourcePath), filepath.FromSlash(parsed.Path)))
	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("broken Markdown link: %s -> %s: %w", sourcePath, rawLink, err)
	}
	return nil
}

func checkOpenAPI(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var document struct {
		OpenAPI string                 `yaml:"openapi"`
		Info    map[string]any         `yaml:"info"`
		Paths   map[string]interface{} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(content, &document); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if document.OpenAPI != "3.1.0" {
		return fmt.Errorf("%s openapi must be 3.1.0", path)
	}
	if len(document.Info) == 0 || len(document.Paths) == 0 {
		return fmt.Errorf("%s must define info and paths", path)
	}
	return nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
