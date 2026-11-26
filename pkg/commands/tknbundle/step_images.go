package tknbundle

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

type stepImageOverride struct {
	image    string
	selector stepSelector
}

type stepSelector struct {
	names   []string
	exclude bool
	all     bool
}

// newStepImageOverride ignores the selector when no replacement image is set.
func newStepImageOverride(image, selector string) (stepImageOverride, error) {
	if image == "" {
		return stepImageOverride{}, nil
	}
	selection, err := parseStepImageSelector(selector)
	if err != nil {
		return stepImageOverride{}, err
	}
	return stepImageOverride{image: image, selector: selection}, nil
}

// rewriteYAML transforms the documents without reading or writing files.
func (o stepImageOverride) rewriteYAML(content []byte, file string) ([]byte, error) {
	resources, err := (&kio.ByteReader{
		Reader:                bytes.NewReader(content),
		OmitReaderAnnotations: true,
	}).Read()
	if err != nil {
		return nil, fmt.Errorf("decoding task file %q: %w", file, err)
	}

	changed := false
	matchedNames := make(map[string]bool)
	for _, resource := range resources {
		stepsNode, err := resource.Pipe(yaml.Lookup("spec", "steps"))
		if err != nil {
			return nil, fmt.Errorf("reading steps in %q: %w", file, err)
		}
		if stepsNode == nil {
			continue
		}
		steps, err := stepsNode.Elements()
		if err != nil {
			return nil, fmt.Errorf("reading steps in %q: %w", file, err)
		}

		for _, step := range steps {
			stepImage, err := step.Pipe(yaml.Lookup("image"))
			if err != nil {
				return nil, fmt.Errorf("reading step image in %q: %w", file, err)
			}
			if stepImage == nil {
				continue
			}

			if !o.selector.all {
				nameNode, err := step.Pipe(yaml.Lookup("name"))
				if err != nil {
					return nil, fmt.Errorf("reading step name in %q: %w", file, err)
				}
				name := ""
				if nameNode != nil {
					name = nameNode.YNode().Value
				}
				matchedNames[name] = true
				if !o.selector.matches(name) {
					continue
				}
			}

			if err := step.PipeE(yaml.SetField("image", yaml.NewStringRNode(o.image))); err != nil {
				return nil, fmt.Errorf("updating step image in %q: %w", file, err)
			}
			changed = true
		}
	}
	if !o.selector.all && !o.selector.exclude {
		for _, name := range o.selector.names {
			if !matchedNames[name] {
				l.Logger.Warnf("step '%s' does not exist in task file '%s'", name, filepath.Base(file))
			}
		}
	}

	if changed {
		var updated bytes.Buffer
		if err := (kio.ByteWriter{Writer: &updated}).Write(resources); err != nil {
			return nil, fmt.Errorf("encoding task file %q: %w", file, err)
		}
		content = updated.Bytes()
	}

	return content, nil
}

// applyStepImageOverrides stages transformed files in workDir, leaving the inputs untouched.
// A disabled override returns the original paths without creating any files.
func applyStepImageOverrides(files []string, override stepImageOverride, workDir string) ([]string, error) {
	if override.image == "" {
		return files, nil
	}

	updatedFiles := make([]string, 0, len(files))
	for _, file := range files {
		// These paths come from the resolved task inputs or generated Kustomize output.
		// #nosec G304 -- reading caller-selected task files is required by this command.
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("reading task file %q: %w", file, err)
		}

		content, err = override.rewriteYAML(content, file)
		if err != nil {
			return nil, err
		}

		destination, err := os.CreateTemp(workDir, "step-images-*.yaml")
		if err != nil {
			return nil, fmt.Errorf("creating temporary task file: %w", err)
		}
		if _, err := destination.Write(content); err != nil {
			_ = destination.Close()
			return nil, fmt.Errorf("writing temporary task file %q: %w", destination.Name(), err)
		}
		if err := destination.Close(); err != nil {
			return nil, fmt.Errorf("closing temporary task file %q: %w", destination.Name(), err)
		}
		updatedFiles = append(updatedFiles, destination.Name())
	}

	return updatedFiles, nil
}

// applyStepImageOverridesInPlace persists overrides in the source files before
// bundling or Kustomize rendering, matching the task script's processing order.
func applyStepImageOverridesInPlace(files []string, override stepImageOverride, workDir string) error {
	updatedFiles, err := applyStepImageOverrides(files, override, workDir)
	if err != nil {
		return err
	}
	if override.image == "" {
		return nil
	}

	for i, file := range files {
		content, err := os.ReadFile(updatedFiles[i])
		if err != nil {
			return fmt.Errorf("reading overridden task file %q: %w", updatedFiles[i], err)
		}
		info, err := os.Stat(file)
		if err != nil {
			return fmt.Errorf("checking task file %q: %w", file, err)
		}
		// #nosec G703 -- overrides intentionally persist in caller-selected source files.
		if err := os.WriteFile(file, content, info.Mode().Perm()); err != nil {
			return fmt.Errorf("writing overridden task file %q: %w", file, err)
		}
	}

	return nil
}

func parseStepImageSelector(selector string) (stepSelector, error) {
	entries := strings.FieldsFunc(selector, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})
	selectedNames := make([]string, 0, len(entries))
	var hasIncludes, hasExcludes bool
	for _, entry := range entries {
		if strings.HasPrefix(entry, "!") {
			hasExcludes = true
			selectedNames = append(selectedNames, strings.TrimPrefix(entry, "!"))
		} else {
			hasIncludes = true
			selectedNames = append(selectedNames, entry)
		}
		if hasIncludes && hasExcludes {
			return stepSelector{}, fmt.Errorf("STEPS_IMAGE_STEP_NAMES cannot mix include and exclude (!) entries")
		}
	}
	return stepSelector{names: selectedNames, exclude: hasExcludes, all: selector == ""}, nil
}

func (s stepSelector) matches(name string) bool {
	if s.all {
		return true
	}
	selected := slices.Contains(s.names, name)
	if s.exclude {
		return !selected
	}
	return selected
}
