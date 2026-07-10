package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	sourceDir = "web/dist"
	targetDir = "internal/webui/assets"
)

func main() {
	if err := os.RemoveAll(targetDir); err != nil {
		fail(err)
	}
	if err := copyDirectory(sourceDir, targetDir); err != nil {
		fail(err)
	}
}

func copyDirectory(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relativePath, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relativePath)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}

		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()

		output, err := os.Create(destination)
		if err != nil {
			return err
		}
		defer output.Close()
		_, err = io.Copy(output, input)
		return err
	})
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
