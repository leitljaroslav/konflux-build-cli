package tknbundle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/bmatcuk/doublestar/v4"
)

// resolveFiles applies comma- or whitespace-separated context selections in order.
// Directories contribute their YAML files recursively; explicit files are included
// as given. A lone exclusion starts with all YAML files under sourceDir.
// Returned paths are absolute and retain selection order and duplicates.
func resolveFiles(sourceDir, context string) ([]string, error) {
	sourceDir, err := filepath.Abs(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("resolving source directory: %w", err)
	}
	// split context by , or whitespace
	paths := strings.FieldsFunc(context, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})

	// context is only exclusion without a source. Use . as source -> !foo becomes '. !sub'
	if len(paths) == 1 && strings.HasPrefix(paths[0], "!") {
		paths = append([]string{"."}, paths...)
	}

	var selected []string
	for _, expression := range paths {
		path, exclude := strings.CutPrefix(expression, "!")
		if path == "" {
			return nil, errors.New("context exclusion must specify a path")
		}
		files, err := resolveContextPath(sourceDir, path)
		if exclude && errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("resolving context path %q: %w", path, err)
		}
		if exclude {
			selected = slices.DeleteFunc(selected, func(file string) bool {
				return slices.Contains(files, file)
			})
		} else {
			selected = append(selected, files...)
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("no files matched context %q in source directory %q", context, sourceDir)
	}
	return selected, nil
}

// resolveContextPath expands a literal path or glob and collects files from any
// matching directories. Hidden entries are skipped during glob expansion,
// explicitly named hidden files or directories still work.
func resolveContextPath(sourceDir, selection string) ([]string, error) {
	path := filepath.Join(sourceDir, selection)
	matches := []string{path}
	if _, err := os.Stat(path); err != nil {
		if !errors.Is(err, os.ErrNotExist) || !strings.ContainsAny(selection, "*?[") {
			return nil, err
		}
		// Keep the source directory outside the glob so its name remains literal.
		base, pattern := doublestar.SplitPattern(filepath.ToSlash(selection))
		baseDir := filepath.Join(sourceDir, filepath.FromSlash(base))
		matches, err = doublestar.Glob(os.DirFS(baseDir), pattern,
			doublestar.WithFailOnIOErrors(), doublestar.WithNoHidden())
		if err != nil {
			return nil, err
		}
		for i, match := range matches {
			matches[i] = filepath.Join(baseDir, filepath.FromSlash(match))
		}
	}

	var files []string
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("context path %q is not a regular file", match)
			}
			files = append(files, match)
			continue
		}

		for _, pattern := range []string{"**/*.yaml", "**/*.yml"} {
			yamlFiles, err := doublestar.Glob(os.DirFS(match), pattern,
				doublestar.WithFilesOnly(), doublestar.WithFailOnIOErrors(), doublestar.WithNoHidden())
			if err != nil {
				return nil, err
			}
			for _, file := range yamlFiles {
				files = append(files, filepath.Join(match, file))
			}
		}
	}
	return files, nil
}
