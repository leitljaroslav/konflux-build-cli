package tknbundle

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

type sourceInfo struct {
	URL      string
	Revision string
	Context  string // Original selection, used verbatim in annotation URLs.
}

type annotations struct {
	keys   []string
	values map[string]string
}

func assembleAnnotations(source sourceInfo, bundle preparedBundle, migration *migrationImage) (annotations, error) {
	annots := annotations{values: make(map[string]string)}
	annots.add("org.opencontainers.image.source", source.URL)
	annots.add("org.opencontainers.image.revision", source.Revision)
	imageURL := fmt.Sprintf("%s/tree/%s/%s", source.URL, source.Revision, source.Context)
	annots.add("org.opencontainers.image.url", imageURL)

	if bundle.Version != "" {
		annots.add("org.opencontainers.image.version", bundle.Version)
	}
	if info, err := os.Stat(filepath.Join(bundle.TaskDir, "README.md")); err == nil && info.Mode().IsRegular() {
		annots.add("org.opencontainers.image.documentation", imageURL+"/README.md")
	}
	if info, err := os.Stat(filepath.Join(bundle.TaskDir, "TROUBLESHOOTING.md")); err == nil && info.Mode().IsRegular() {
		annots.add("dev.tekton.docs.troubleshooting", imageURL+"/TROUBLESHOOTING.md")
	}
	if info, err := os.Stat(filepath.Join(bundle.TaskDir, "USAGE.md")); err == nil && info.Mode().IsRegular() {
		annots.add("dev.tekton.docs.usage", imageURL+"/USAGE.md")
	}

	description, err := collectDescriptions(bundle.Files)
	if err != nil {
		return annotations{}, err
	}
	annots.add("org.opencontainers.image.description", description)

	if migration != nil && migration.Digest != "" {
		annots.add("dev.konflux-ci.task.migration.digest", migration.Digest)
		annots.add("dev.konflux-ci.task.migration.tag", migration.Tag)
	}

	return annots, nil
}

func collectDescriptions(allFiles []string) (string, error) {
	descriptions := make([]string, 0)
	for _, filePath := range allFiles {
		// #nosec G304 -- bundle inputs are caller-selected local manifest files.
		content, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("reading bundle file %q: %w", filePath, err)
		}

		decoder := yaml.NewDecoder(bytes.NewReader(content))
		for document := 1; ; document++ {
			var resource struct {
				Spec struct {
					Description *string `yaml:"description"`
				} `yaml:"spec"`
			}
			err := decoder.Decode(&resource)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return "", fmt.Errorf("decoding YAML document %d from %q: %w", document, filePath, err)
			}
			description := "null"
			if resource.Spec.Description != nil {
				description = *resource.Spec.Description
			}
			descriptions = append(descriptions, description)
		}
	}

	// Match yq's document separators, sed '/null/d', and Bash command
	// substitution, which strips trailing newlines from the complete output.
	lines := strings.Split(strings.Join(descriptions, "\n---\n"), "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if !strings.Contains(line, "null") {
			filtered = append(filtered, line)
		}
	}
	return strings.TrimRight(strings.Join(filtered, "\n"), "\n"), nil
}

func (a *annotations) add(key, val string) {
	if a.values == nil {
		a.values = make(map[string]string)
	}
	if _, exists := a.values[key]; !exists {
		a.keys = append(a.keys, key)
	}
	a.values[key] = val
}
