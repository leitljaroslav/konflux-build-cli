package tknbundle_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
)

// These tests preserve the observable contract of both scripts in
// container-build-catalog/task/tkn-bundle/tkn-bundle.yaml at
// ec561d5e2b6188bac9d68715aacbaf6ba19d55a8. Known compatibility gaps are
// intentionally failing assertions, not skips or expectations of the new behavior.
// Publishers are faked; manifests, Kustomize, annotations, and result files use
// the real implementation through TektonBundlePush.Run.

func TestTektonBundlePushOriginalResults(t *testing.T) {
	g := NewWithT(t)
	command, publisher := newCompatibilityCommand(t, map[string]string{
		"task/task.yaml": compatibilityTask("build", "1.0"),
	}, "task")

	g.Expect(command.Run()).To(Succeed())
	bundle := publisher.onlyBundle(t)
	g.Expect(bundle.destination).To(Equal(compatibilityImage))
	g.Expect(bundle.resourceNames(t)).To(Equal([]string{"build"}))
	g.Expect(bundle.annotations).To(Equal(map[string]string{
		"org.opencontainers.image.source":      "https://example.com/repo",
		"org.opencontainers.image.revision":    "abc123",
		"org.opencontainers.image.url":         "https://example.com/repo/tree/abc123/task",
		"org.opencontainers.image.version":     "1.0",
		"org.opencontainers.image.description": "Build the project",
	}))
	for path, want := range map[string]string{
		command.Params.ResultPathImageURL:    compatibilityImage,
		command.Params.ResultPathImageDigest: compatibilityDigest,
		command.Params.ResultPathImageRef:    compatibilityImage + "@" + compatibilityDigest,
	} {
		contents, err := os.ReadFile(path)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(string(contents)).To(Equal(want), "result files must not contain a trailing newline")
	}
}

func TestTektonBundlePushOriginalTaskVersionRequired(t *testing.T) {
	const versionLabel = "    app.kubernetes.io/version: \"1.0\"\n"
	for _, tt := range []struct {
		name  string
		label string
	}{
		{name: "missing label"},
		{name: "empty label", label: "    app.kubernetes.io/version: \"\"\n"},
		{name: "null label", label: "    app.kubernetes.io/version: null\n"},
		{name: "whitespace label", label: "    app.kubernetes.io/version: \" \\t\\n \"\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			manifest := strings.Replace(compatibilityTask("build", "1.0"), versionLabel, tt.label, 1)
			command, publisher := newCompatibilityCommand(t, map[string]string{"task.yaml": manifest}, "task.yaml")

			err := command.Run()

			g.Expect(err).To(MatchError(And(
				ContainSubstring("app.kubernetes.io/version"),
				ContainSubstring("is not set"),
			)))
			g.Expect(publisher.bundles).To(BeEmpty(), "an unversioned Task must not be published")
			expectNoCompatibilityResults(t, command)
		})
	}
}

func TestTektonBundlePushOriginalPipelineVersionOptional(t *testing.T) {
	g := NewWithT(t)
	command, publisher := newCompatibilityCommand(t, map[string]string{
		"pipeline.yaml": "apiVersion: tekton.dev/v1\nkind: Pipeline\nmetadata:\n  name: pipeline\nspec:\n  tasks: []\n",
	}, "pipeline.yaml")

	g.Expect(command.Run()).To(Succeed())
	bundle := publisher.onlyBundle(t)
	g.Expect(bundle.resourceNames(t)).To(Equal([]string{"pipeline"}))
	g.Expect(bundle.annotations).NotTo(HaveKey("org.opencontainers.image.version"))
}

func TestTektonBundlePushOriginalDocumentationLinks(t *testing.T) {
	for _, mode := range []string{"source task", "step image override", "kustomized task"} {
		t.Run(mode, func(t *testing.T) {
			g := NewWithT(t)
			fixtures := map[string]string{
				"task/task.yaml":          compatibilityTask("build", "1.0"),
				"task/README.md":          "Readme",
				"task/TROUBLESHOOTING.md": "Troubleshooting",
				"task/USAGE.md":           "Usage",
			}
			if mode == "kustomized task" {
				fixtures["task/kustomization.yaml"] = compatibilityKustomization
			}
			command, publisher := newCompatibilityCommand(t, fixtures, "task")
			if mode == "step image override" {
				command.Params.StepsImage = "replacement:v1"
			}

			g.Expect(command.Run()).To(Succeed())
			annotations := publisher.onlyBundle(t).annotations
			for key, want := range map[string]string{
				"org.opencontainers.image.documentation": "https://example.com/repo/tree/abc123/task/README.md",
				"dev.tekton.docs.troubleshooting":        "https://example.com/repo/tree/abc123/task/TROUBLESHOOTING.md",
				"dev.tekton.docs.usage":                  "https://example.com/repo/tree/abc123/task/USAGE.md",
			} {
				g.Expect(annotations).To(HaveKeyWithValue(key, want))
			}
		})
	}
}

func TestTektonBundlePushOriginalDocumentationMustBeTaskFiles(t *testing.T) {
	for _, location := range []string{"missing", "source root", "directories beside task"} {
		t.Run(location, func(t *testing.T) {
			g := NewWithT(t)
			command, publisher := newCompatibilityCommand(t, map[string]string{
				"task/task.yaml": compatibilityTask("build", "1.0"),
			}, "task")
			for _, name := range []string{"README.md", "TROUBLESHOOTING.md", "USAGE.md"} {
				switch location {
				case "source root":
					g.Expect(os.WriteFile(filepath.Join(command.Params.Source, name), []byte("root documentation"), 0o600)).To(Succeed())
				case "directories beside task":
					g.Expect(os.Mkdir(filepath.Join(command.Params.Source, "task", name), 0o755)).To(Succeed())
				}
			}

			g.Expect(command.Run()).To(Succeed())
			annotations := publisher.onlyBundle(t).annotations
			for _, key := range []string{"org.opencontainers.image.documentation", "dev.tekton.docs.troubleshooting", "dev.tekton.docs.usage"} {
				g.Expect(annotations).NotTo(HaveKey(key))
			}
		})
	}
}

func TestTektonBundlePushOriginalFileOrder(t *testing.T) {
	for _, tt := range []struct {
		name     string
		fixtures map[string]string
		context  string
		want     []string
	}{
		{
			name: "explicit context order determines version",
			fixtures: map[string]string{
				"a/task.yaml": compatibilityTask("a", "1.0"),
				"z/task.yaml": compatibilityTask("z", "2.0"),
			},
			context: "z/task.yaml,a/task.yaml", want: []string{"z", "a"},
		},
		{
			name: "directory collects yaml before yml",
			fixtures: map[string]string{
				"a.yml":  compatibilityTask("a", "1.0"),
				"z.yaml": compatibilityTask("z", "2.0"),
			},
			context: ".", want: []string{"z", "a"},
		},
		{
			name:     "repeated file is retained",
			fixtures: map[string]string{"task.yaml": compatibilityTask("z", "2.0")},
			context:  "task.yaml,task.yaml", want: []string{"z", "z"},
		},
		{
			name:     "overlapping selections are retained",
			fixtures: map[string]string{"task/task.yaml": compatibilityTask("z", "2.0")},
			context:  "task,task/task.yaml", want: []string{"z", "z"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			command, publisher := newCompatibilityCommand(t, tt.fixtures, tt.context)

			g.Expect(command.Run()).To(Succeed())
			bundle := publisher.onlyBundle(t)
			g.Expect(bundle.resourceNames(t)).To(Equal(tt.want))
			g.Expect(bundle.annotations).To(HaveKeyWithValue("org.opencontainers.image.version", "2.0"))
		})
	}
}

func TestTektonBundlePushOriginalContextSelections(t *testing.T) {
	for _, tt := range []struct {
		context string
		want    []string
	}{
		{context: ".", want: []string{"root", "yml", "nested"}},
		{context: "task.yml", want: []string{"yml"}},
		{context: "sub", want: []string{"nested"}},
		{context: "task.yml,sub", want: []string{"yml", "nested"}},
		{context: "task.yml sub", want: []string{"yml", "nested"}},
		{context: "!sub", want: []string{"root", "yml"}},
		{context: "sub,!sub/task.yaml,task.yml", want: []string{"yml"}},
	} {
		t.Run(tt.context, func(t *testing.T) {
			g := NewWithT(t)
			command, publisher := newCompatibilityCommand(t, map[string]string{
				"task.yaml":           compatibilityTask("root", "1.0"),
				"task.yml":            compatibilityTask("yml", "1.0"),
				"sub/task.yaml":       compatibilityTask("nested", "1.0"),
				"README.md":           "not a manifest",
				".hidden.yaml":        "not: [valid YAML",
				".hidden/secret.yaml": "not: [valid YAML",
			}, tt.context)

			g.Expect(command.Run()).To(Succeed())
			g.Expect(publisher.onlyBundle(t).resourceNames(t)).To(ConsistOf(tt.want))
		})
	}
}

func TestTektonBundlePushOriginalKustomizationDiscovery(t *testing.T) {
	for _, tt := range []struct {
		name            string
		context         string
		kustomization   string
		otherKustomized bool
	}{
		{name: "directory context", context: "task", kustomization: "kustomization.yaml"},
		{name: "explicit task file", context: "task/task.yaml", kustomization: "kustomization.yaml"},
		{name: "unselected yml kustomization", context: "task/task.yaml", kustomization: "kustomization.yml"},
		{name: "excluded kustomization still applies", context: "task,!task/kustomization.yaml", kustomization: "kustomization.yaml"},
		{name: "other selected files are ignored", context: "task/task.yaml,z-other/task.yaml", kustomization: "kustomization.yaml"},
		{name: "only first directory is rendered", context: "task,z-other", kustomization: "kustomization.yaml", otherKustomized: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fixtures := map[string]string{
				"task/task.yaml":           compatibilityTask("build", "1.0"),
				"task/" + tt.kustomization: compatibilityKustomization,
				"z-other/task.yaml":        compatibilityTask("other", "3.0"),
			}
			if tt.otherKustomized {
				fixtures["z-other/kustomization.yaml"] = compatibilityKustomization
			}
			command, publisher := newCompatibilityCommand(t, fixtures, tt.context)

			g.Expect(command.Run()).To(Succeed())
			bundle := publisher.onlyBundle(t)
			g.Expect(bundle.resourceNames(t)).To(Equal([]string{"rendered-build"}))
			g.Expect(bundle.annotations).To(HaveKeyWithValue("org.opencontainers.image.version", "2.0"))
		})
	}
}

func TestTektonBundlePushOriginalRenderedTaskVersionRequired(t *testing.T) {
	g := NewWithT(t)
	command, publisher := newCompatibilityCommand(t, map[string]string{
		"task/task.yaml": compatibilityTask("build", "1.0"),
		"task/kustomization.yaml": `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - task.yaml
patches:
  - target:
      kind: Task
    patch: |-
      - op: remove
        path: /metadata/labels/app.kubernetes.io~1version
`,
	}, "task")

	err := command.Run()

	g.Expect(err).To(MatchError(And(ContainSubstring("app.kubernetes.io/version"), ContainSubstring("is not set"))))
	g.Expect(publisher.bundles).To(BeEmpty())
	expectNoCompatibilityResults(t, command)
}

func TestTektonBundlePushOriginalStepImageSelection(t *testing.T) {
	for _, tt := range []struct {
		name     string
		image    string
		selector string
		want     map[string]string
	}{
		{name: "disabled", want: map[string]string{"build": "build:original", "test": "test:original"}},
		{name: "all steps", image: "replacement:v1", want: map[string]string{"build": "replacement:v1", "test": "replacement:v1"}},
		{name: "one included step", image: "replacement:v1", selector: "build", want: map[string]string{"build": "replacement:v1", "test": "test:original"}},
		{name: "comma separated includes", image: "replacement:v1", selector: "build,test", want: map[string]string{"build": "replacement:v1", "test": "replacement:v1"}},
		{name: "space separated includes", image: "replacement:v1", selector: "build test", want: map[string]string{"build": "replacement:v1", "test": "replacement:v1"}},
		{name: "one excluded step", image: "replacement:v1", selector: "!build", want: map[string]string{"build": "build:original", "test": "replacement:v1"}},
		{name: "comma separated excludes", image: "replacement:v1", selector: "!build,!test", want: map[string]string{"build": "build:original", "test": "test:original"}},
		{name: "space separated excludes", image: "replacement:v1", selector: "!build !test", want: map[string]string{"build": "build:original", "test": "test:original"}},
		{name: "unknown step leaves images unchanged", image: "replacement:v1", selector: "missing", want: map[string]string{"build": "build:original", "test": "test:original"}},
		{name: "selector without image is ignored", selector: "build", want: map[string]string{"build": "build:original", "test": "test:original"}},
		{name: "mixed selector without image is ignored", selector: "build,!test", want: map[string]string{"build": "build:original", "test": "test:original"}},
		{name: "bare exclusion excludes no named step", image: "replacement:v1", selector: "!", want: map[string]string{"build": "replacement:v1", "test": "replacement:v1"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			command, publisher := newCompatibilityCommand(t, map[string]string{
				"task.yaml": compatibilityTask("build", "1.0"),
			}, "task.yaml")
			command.Params.StepsImage = tt.image
			command.Params.StepsImageStepNames = tt.selector

			g.Expect(command.Run()).To(Succeed())
			expectCompatibilityStepImages(t, publisher.onlyBundle(t), tt.want)
		})
	}
}

func TestTektonBundlePushOriginalMixedStepSelectionRejected(t *testing.T) {
	for _, selector := range []string{"build,!test", "!test build"} {
		t.Run(selector, func(t *testing.T) {
			g := NewWithT(t)
			command, publisher := newCompatibilityCommand(t, map[string]string{
				"task.yaml": compatibilityTask("build", "1.0"),
			}, "task.yaml")
			command.Params.StepsImage = "replacement:v1"
			command.Params.StepsImageStepNames = selector

			g.Expect(command.Run()).To(MatchError(ContainSubstring("cannot mix include and exclude")))
			g.Expect(publisher.bundles).To(BeEmpty())
			expectNoCompatibilityResults(t, command)
		})
	}
}

func TestTektonBundlePushOriginalUnknownStepWarning(t *testing.T) {
	for _, step := range []string{"missing", "inherited"} {
		t.Run(step, func(t *testing.T) {
			g := NewWithT(t)
			var logs bytes.Buffer
			previousOutput, previousLevel := l.Logger.Out, l.Logger.GetLevel()
			l.Logger.SetOutput(&logs)
			l.Logger.SetLevel(logrus.WarnLevel)
			t.Cleanup(func() {
				l.Logger.SetOutput(previousOutput)
				l.Logger.SetLevel(previousLevel)
			})
			command, publisher := newCompatibilityCommand(t, map[string]string{
				"task.yaml": compatibilityTask("build", "1.0"),
			}, "task.yaml")
			command.Params.StepsImage = "replacement:v1"
			command.Params.StepsImageStepNames = step

			g.Expect(command.Run()).To(Succeed())
			expectCompatibilityStepImages(t, publisher.onlyBundle(t), map[string]string{"build": "build:original", "test": "test:original"})
			g.Expect(logs.String()).To(And(ContainSubstring(step), ContainSubstring("does not exist"), ContainSubstring("task.yaml")))
		})
	}
}

func TestTektonBundlePushOriginalStepImagesPersistInSource(t *testing.T) {
	for _, mode := range []string{"ordinary task", "kustomized task"} {
		t.Run(mode, func(t *testing.T) {
			g := NewWithT(t)
			fixtures := map[string]string{"task/task.yaml": compatibilityTask("build", "1.0")}
			if mode == "kustomized task" {
				fixtures["task/kustomization.yaml"] = compatibilityKustomization
			}
			command, publisher := newCompatibilityCommand(t, fixtures, "task")
			command.Params.StepsImage = "replacement:v1"
			// Exclude mode leaves the selected Kustomization file untouched in
			// the original yq script, while replacing the build step's image.
			command.Params.StepsImageStepNames = "!test"

			g.Expect(command.Run()).To(Succeed())
			want := map[string]string{"build": "replacement:v1", "test": "test:original"}
			expectCompatibilityStepImages(t, publisher.onlyBundle(t), want)
			contents, err := os.ReadFile(filepath.Join(command.Params.Source, "task", "task.yaml"))
			g.Expect(err).NotTo(HaveOccurred())
			expectCompatibilityStepImages(t, compatibilityBundle{manifests: []string{string(contents)}}, want)
		})
	}
}

func TestTektonBundlePushOriginalKustomizePatchOverridesStepImage(t *testing.T) {
	g := NewWithT(t)
	command, publisher := newCompatibilityCommand(t, map[string]string{
		"task/task.yaml": compatibilityTask("build", "1.0"),
		"task/kustomization.yaml": `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - task.yaml
patches:
  - target:
      kind: Task
    patch: |-
      - op: replace
        path: /spec/steps/0/image
        value: patched:v2
`,
	}, "task")
	command.Params.StepsImage = "replacement:v1"
	command.Params.StepsImageStepNames = "!test"

	g.Expect(command.Run()).To(Succeed())
	expectCompatibilityStepImages(t, publisher.onlyBundle(t), map[string]string{"build": "patched:v2", "test": "test:original"})
}

func TestTektonBundlePushOriginalDescriptionBytes(t *testing.T) {
	// Literal expected values come from yq '.spec.description' | sed '/null/d'
	// followed by Bash command substitution, as used by the original build script.
	// In particular, command substitution strips trailing newlines and yq emits
	// document separators. Do not replace these expectations with helper output.
	first := compatibilityTask("first", "1.0")
	second := strings.Replace(compatibilityTask("second", "2.0"), "Build the project", "Test the project", 1)
	for _, tt := range []struct {
		name     string
		fixtures map[string]string
		want     string
	}{
		{
			name:     "one description",
			fixtures: map[string]string{"task.yaml": first},
			want:     "Build the project",
		},
		{
			name:     "missing description",
			fixtures: map[string]string{"task.yaml": strings.Replace(first, "  description: Build the project\n", "", 1)},
		},
		{
			name:     "commas quotes and internal newline",
			fixtures: map[string]string{"task.yaml": strings.Replace(first, "Build the project", `"Build, then \"test\"\nPublish"`, 1)},
			want:     "Build, then \"test\"\nPublish",
		},
		{
			name:     "trailing newlines are stripped",
			fixtures: map[string]string{"task.yaml": strings.Replace(first, "Build the project", `"Build the project\n\n"`, 1)},
			want:     "Build the project",
		},
		{
			name:     "lines containing null are removed",
			fixtures: map[string]string{"task.yaml": strings.Replace(first, "Build the project", `"Keep\nnullable value\nLast\n"`, 1)},
			want:     "Keep\nLast",
		},
		{
			name:     "multiple files retain document separators",
			fixtures: map[string]string{"a.yaml": first, "b.yml": second},
			want:     "Build the project\n---\nTest the project",
		},
		{
			name:     "multiple documents retain document separators",
			fixtures: map[string]string{"task.yaml": first + "---\n" + second},
			want:     "Build the project\n---\nTest the project",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			command, publisher := newCompatibilityCommand(t, tt.fixtures, ".")

			g.Expect(command.Run()).To(Succeed())
			g.Expect(publisher.onlyBundle(t).annotations).To(HaveKeyWithValue("org.opencontainers.image.description", tt.want))
		})
	}
}

func TestTektonBundlePushOriginalMultiDocumentVersion(t *testing.T) {
	g := NewWithT(t)
	command, publisher := newCompatibilityCommand(t, map[string]string{
		"task.yaml": compatibilityTask("first", "1.0") + "---\n" + compatibilityTask("second", "2.0"),
	}, "task.yaml")

	g.Expect(command.Run()).To(Succeed())
	// The original yq/sed/tr pipeline processes every document in FILES[0].
	g.Expect(publisher.onlyBundle(t).annotations).To(HaveKeyWithValue("org.opencontainers.image.version", "1.0---2.0"))
}

func TestTektonBundlePushOriginalEmptySelectionRejected(t *testing.T) {
	for _, context := range []string{"empty", "task.yaml,!task.yaml"} {
		t.Run(context, func(t *testing.T) {
			g := NewWithT(t)
			command, publisher := newCompatibilityCommand(t, map[string]string{
				"task.yaml": compatibilityTask("build", "1.0"),
			}, context)
			g.Expect(os.Mkdir(filepath.Join(command.Params.Source, "empty"), 0o755)).To(Succeed())

			g.Expect(command.Run()).To(HaveOccurred())
			g.Expect(publisher.bundles).To(BeEmpty())
			expectNoCompatibilityResults(t, command)
		})
	}
}
