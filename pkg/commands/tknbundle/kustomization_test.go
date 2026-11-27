package tknbundle

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/kustomize/kyaml/kio"
)

func TestIsTaskCustomized(t *testing.T) {
	tests := []struct {
		name        string
		files       []string
		directories []string
		missing     []string
		want        bool
	}{
		{name: "nil files"},
		{name: "empty files", files: []string{}},
		{name: "ordinary task", files: []string{"task.yaml", "README.md"}},
		{name: "yaml kustomization", files: []string{"kustomization.yaml"}, want: true},
		{name: "yml kustomization", files: []string{"kustomization.yml"}, want: true},
		{name: "nested kustomization", files: []string{"task/kustomization.yaml"}, want: true},
		{name: "kustomization after other files", files: []string{"task.yaml", "kustomization.yml", "README.md"}, want: true},
		{name: "kustomization in later directory is ignored", files: []string{"task/task.yaml", "other/kustomization.yaml"}},
		{name: "kustomizations in multiple directories use first", files: []string{"task/kustomization.yaml", "other/kustomization.yaml"}, want: true},
		{name: "filename suffix is not a kustomization", files: []string{"ustomization.yaml"}},
		{name: "prefixed filename", files: []string{"my-kustomization.yaml"}},
		{name: "backup file", files: []string{"kustomization.yaml.bak"}},
		{name: "directory named kustomization", files: []string{"kustomization.yaml/task.yaml"}},
		{name: "kustomization directory", directories: []string{"kustomization.yaml", "kustomization.yml"}},
		{name: "missing kustomization", missing: []string{"kustomization.yaml", "kustomization.yml"}},
		{name: "missing file before kustomization", missing: []string{"missing.yaml"}, files: []string{"kustomization.yaml"}, want: true},
		{name: "directory before kustomization", directories: []string{"kustomization.yaml"}, files: []string{"kustomization.yml"}, want: true},
		{name: "empty filename", files: []string{""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			t.Chdir(t.TempDir())
			var files []string
			if tt.files != nil {
				files = make([]string, 0, len(tt.files))
			}
			files = append(files, tt.missing...)
			for _, directory := range tt.directories {
				g.Expect(os.MkdirAll(directory, 0o700)).To(Succeed())
				files = append(files, directory)
			}
			for _, file := range tt.files {
				if file != "" {
					g.Expect(os.MkdirAll(filepath.Dir(file), 0o700)).To(Succeed())
					g.Expect(os.WriteFile(file, nil, 0o600)).To(Succeed())
				}
				files = append(files, file)
			}
			g.Expect(isTaskKustomized(files)).To(Equal(tt.want))
		})
	}
}

func TestBuildKustomization(t *testing.T) {
	const task = `apiVersion: tekton.dev/v1
kind: Task
metadata:
  name: build
  labels:
    app: example
  annotations:
    description: Build the image
spec:
  steps:
    - name: build
      image: quay.io/example/tools:v1
      script: echo build

`
	const pipeline = `apiVersion: tekton.dev/v1
kind: Pipeline
metadata:
  name: build-pipeline
spec:
  tasks:
    - name: build
      taskRef:
        name: build

`

	tests := []struct {
		name  string
		files []string
		want  []string
	}{
		{name: "single resource", files: []string{task}, want: []string{task}},
		{name: "multiple files", files: []string{task, pipeline}, want: []string{task, pipeline}},
		{name: "multiple documents in one file", files: []string{task + "---\n" + pipeline}, want: []string{task, pipeline}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			// Source files must be loadable from outside the working directory.
			sourceDir := filepath.Join(t.TempDir(), "source with spaces")
			g.Expect(os.Mkdir(sourceDir, 0o700)).To(Succeed())
			workDir := t.TempDir()
			var files []string
			for i, contents := range tt.files {
				path := filepath.Join(sourceDir, fmt.Sprintf("resource-%d.yaml", i))
				g.Expect(os.WriteFile(path, []byte(contents), 0o600)).To(Succeed())
				files = append(files, path)
			}
			// Files outside the selection must not be included or parsed.
			g.Expect(os.WriteFile(filepath.Join(sourceDir, "unselected.yaml"), []byte("invalid: ["), 0o600)).To(Succeed())

			bundlePath, err := buildKustomization(files, workDir)

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(bundlePath).To(Equal(filepath.Join(workDir, "bundle.yaml")))
			bundle, err := os.ReadFile(bundlePath)
			g.Expect(err).NotTo(HaveOccurred())
			resources, err := kio.FromBytes(bundle)
			g.Expect(err).NotTo(HaveOccurred())
			var documents []string
			for _, resource := range resources {
				document, err := resource.String()
				g.Expect(err).NotTo(HaveOccurred())
				documents = append(documents, document)
			}
			want := make([]any, len(tt.want))
			for i, document := range tt.want {
				want[i] = MatchYAML(document)
			}
			g.Expect(documents).To(ConsistOf(want...))
			for i, path := range files {
				contents, err := os.ReadFile(path)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(string(contents)).To(Equal(tt.files[i]))
			}
		})
	}
}

func TestBuildKustomizationAppliesSourceKustomization(t *testing.T) {
	g := NewWithT(t)
	sourceDir := t.TempDir()
	workDir := t.TempDir()

	taskPath := filepath.Join(sourceDir, "task.yaml")
	task := `apiVersion: tekton.dev/v1
kind: Task
metadata:
  name: build
  labels:
    app.kubernetes.io/version: "0.1"
spec:
  description: Original description
  steps:
    - name: build
      image: alpine
`
	patchPath := filepath.Join(sourceDir, "task-patch.yaml")
	patch := `apiVersion: tekton.dev/v1
kind: Task
metadata:
  name: build
  labels:
    app.kubernetes.io/version: "2.0"
spec:
  description: Rendered description
`
	kustomizationPath := filepath.Join(sourceDir, "kustomization.yaml")
	kustomization := `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - task.yaml
patches:
  - path: task-patch.yaml
    target:
      group: tekton.dev
      version: v1
      kind: Task
      name: build
`
	for path, content := range map[string]string{
		taskPath: task, patchPath: patch, kustomizationPath: kustomization,
	} {
		g.Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())
	}

	bundlePath, err := buildKustomization([]string{kustomizationPath, patchPath, taskPath}, workDir)
	g.Expect(err).NotTo(HaveOccurred())
	bundle, err := os.ReadFile(bundlePath)
	g.Expect(err).NotTo(HaveOccurred())
	resources, err := kio.FromBytes(bundle)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(resources).To(HaveLen(1))
	document, err := resources[0].String()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(document).To(MatchYAML(`apiVersion: tekton.dev/v1
kind: Task
metadata:
  name: build
  labels:
    app.kubernetes.io/version: "2.0"
spec:
  description: Rendered description
  steps:
    - name: build
      image: alpine
`))
}

func TestBuildKustomizationErrors(t *testing.T) {
	g := NewWithT(t)
	sourceDir := t.TempDir()
	validFile := filepath.Join(sourceDir, "valid.yaml")
	g.Expect(os.WriteFile(validFile, []byte("apiVersion: tekton.dev/v1\nkind: Task\nmetadata:\n  name: build\n"), 0o600)).To(Succeed())
	invalidYAML := filepath.Join(sourceDir, "invalid.yaml")
	g.Expect(os.WriteFile(invalidYAML, []byte("invalid: ["), 0o600)).To(Succeed())
	missingMetadata := filepath.Join(sourceDir, "missing-metadata.yaml")
	g.Expect(os.WriteFile(missingMetadata, []byte("apiVersion: tekton.dev/v1\nkind: Task\n"), 0o600)).To(Succeed())

	tests := []struct {
		name           string
		files          []string
		missingWorkDir bool
		blockedOutput  string
		wantErr        string
	}{
		{
			name:    "missing source file",
			files:   []string{filepath.Join(sourceDir, "missing.yaml")},
			wantErr: "building bundle kustomization:",
		},
		{name: "invalid YAML", files: []string{invalidYAML}, wantErr: "building bundle kustomization:"},
		{name: "missing resource name", files: []string{missingMetadata}, wantErr: "building bundle kustomization:"},
		{name: "duplicate resource", files: []string{validFile, validFile}, wantErr: "building bundle kustomization:"},
		{
			name:           "missing working directory",
			files:          []string{validFile},
			missingWorkDir: true,
			wantErr:        "writing bundle kustomization:",
		},
		{
			name:          "cannot write kustomization",
			files:         []string{validFile},
			blockedOutput: "kustomization.yaml",
			wantErr:       "writing bundle kustomization:",
		},
		{
			name:          "cannot write rendered bundle",
			files:         []string{validFile},
			blockedOutput: "bundle.yaml",
			wantErr:       "writing bundle resources:",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			workDir := t.TempDir()
			if tt.missingWorkDir {
				workDir = filepath.Join(workDir, "missing")
			}
			if tt.blockedOutput != "" {
				// A directory at the output path fails even when tests run as root.
				g.Expect(os.Mkdir(filepath.Join(workDir, tt.blockedOutput), 0o700)).To(Succeed())
			}

			bundlePath, err := buildKustomization(tt.files, workDir)

			g.Expect(err).To(MatchError(ContainSubstring(tt.wantErr)))
			g.Expect(bundlePath).To(BeEmpty())
		})
	}
}
