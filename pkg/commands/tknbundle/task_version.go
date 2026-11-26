package tknbundle

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"go.yaml.in/yaml/v3"
	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"
)

const LABEL_ACTUAL_TASK_VERSION = "app.kubernetes.io/version"
const TASK_KIND = "Task"

// readBundleVersion requires a version for Tasks while allowing other resource kinds.
func readBundleVersion(taskFile string) (string, error) {
	version, err := getTaskVersion(taskFile)
	if err != nil {
		return "", fmt.Errorf("reading task version: %w", err)
	}
	if version == "" {
		manifest, err := kyaml.ReadFile(taskFile)
		if err != nil {
			return "", fmt.Errorf("reading bundle file %q: %w", taskFile, err)
		}
		if manifest.GetKind() == TASK_KIND {
			return "", fmt.Errorf("label %q is not set in task file %q", LABEL_ACTUAL_TASK_VERSION, taskFile)
		}
	}
	return version, nil
}

func getTaskVersion(taskFile string) (string, error) {
	// #nosec G304 -- reading caller-selected task files is required by this command.
	task, err := os.ReadFile(taskFile)
	if err != nil {
		return "", fmt.Errorf("reading task file: %w", err)
	}

	var versions []string
	decoder := yaml.NewDecoder(bytes.NewReader(task))
	for {
		var manifest struct {
			Metadata struct {
				Labels map[string]string `yaml:"labels"`
			} `yaml:"metadata"`
		}
		if err := decoder.Decode(&manifest); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return "", fmt.Errorf("parsing task file: %w", err)
		}

		// Preserve yq document separators and the original sed '/null/d'
		// filtering before removing whitespace from its combined output.
		var lines []string
		for line := range strings.SplitSeq(manifest.Metadata.Labels[LABEL_ACTUAL_TASK_VERSION], "\n") {
			if !strings.Contains(line, "null") {
				lines = append(lines, line)
			}
		}
		versions = append(versions, strings.Join(lines, "\n"))
	}

	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, strings.Join(versions, "---")), nil
}
