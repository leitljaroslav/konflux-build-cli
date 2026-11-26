package tknbundle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func TestApplyStepImageOverrides(t *testing.T) {
	const original = `apiVersion: tekton.dev/v1
kind: Task
metadata:
  name: example
spec:
  steps:
    - name: build
      image: ubuntu
    - name: test
      image: alpine
    - name: script
      script: echo no image
---
apiVersion: tekton.dev/v1
kind: Task
metadata:
  name: second
spec:
  steps:
    - name: publish
      image: busybox
`
	tests := []struct {
		name     string
		selector string
		want     map[string]string
	}{
		{
			name: "empty selector replaces all images",
			want: map[string]string{"build": "replacement", "test": "replacement", "publish": "replacement"},
		},
		{
			name:     "include selector replaces matching steps",
			selector: "build,publish",
			want:     map[string]string{"build": "replacement", "test": "alpine", "publish": "replacement"},
		},
		{
			name:     "exclude selector replaces other steps",
			selector: "!build !publish",
			want:     map[string]string{"build": "ubuntu", "test": "replacement", "publish": "busybox"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			inputDir := t.TempDir()
			inputPath := filepath.Join(inputDir, "task.yaml")
			g.Expect(os.WriteFile(inputPath, []byte(original), 0o600)).To(Succeed())
			workDir := t.TempDir()

			override, err := newStepImageOverride("replacement", tt.selector)
			g.Expect(err).NotTo(HaveOccurred())
			files, err := applyStepImageOverrides([]string{inputPath}, override, workDir)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(files).To(HaveLen(1))
			g.Expect(files[0]).NotTo(Equal(inputPath))

			updated, err := os.ReadFile(files[0])
			g.Expect(err).NotTo(HaveOccurred())
			resources, err := (&kio.ByteReader{Reader: strings.NewReader(string(updated))}).Read()
			g.Expect(err).NotTo(HaveOccurred())
			got := make(map[string]string)
			for _, resource := range resources {
				steps, err := resource.Pipe(yaml.Lookup("spec", "steps"))
				g.Expect(err).NotTo(HaveOccurred())
				elements, err := steps.Elements()
				g.Expect(err).NotTo(HaveOccurred())
				for _, step := range elements {
					name, err := step.Pipe(yaml.Lookup("name"))
					g.Expect(err).NotTo(HaveOccurred())
					image, err := step.Pipe(yaml.Lookup("image"))
					g.Expect(err).NotTo(HaveOccurred())
					if image != nil {
						got[name.YNode().Value] = image.YNode().Value
					}
				}
			}
			g.Expect(got).To(Equal(tt.want))

			originalAfter, err := os.ReadFile(inputPath)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(string(originalAfter)).To(Equal(original))
		})
	}
}

func TestApplyStepImageOverridesInPlaceStagesAllFiles(t *testing.T) {
	g := NewWithT(t)
	sourceDir := t.TempDir()
	firstPath := filepath.Join(sourceDir, "first.yaml")
	secondPath := filepath.Join(sourceDir, "second.yaml")
	const original = "kind: Task\nspec:\n  steps:\n    - name: build\n      image: original:v1\n"
	g.Expect(os.WriteFile(firstPath, []byte(original), 0o600)).To(Succeed())
	g.Expect(os.WriteFile(secondPath, []byte("kind: [invalid\n"), 0o600)).To(Succeed())
	override, err := newStepImageOverride("replacement:v1", "")
	g.Expect(err).NotTo(HaveOccurred())

	err = applyStepImageOverridesInPlace([]string{firstPath, secondPath}, override, t.TempDir())

	g.Expect(err).To(MatchError(ContainSubstring("decoding task file")))
	contents, err := os.ReadFile(firstPath)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(contents)).To(Equal(original))
}
