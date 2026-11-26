package tknbundle

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
)

func TestAssembleAnnotations(t *testing.T) {
	g := NewWithT(t)
	url := "https://example.com/team/task"
	filePath := filepath.Join(t.TempDir(), "task.yaml")
	writeAnnotationTestFile(t, filePath, `apiVersion: tekton.dev/v1
kind: Task
spec:
  description: Build and test the project
`)

	source := sourceInfo{URL: url, Revision: "main", Context: "tasks/build"}
	bundle := preparedBundle{Files: []string{filePath}, TaskDir: filepath.Dir(filePath), Version: "1.2.3"}
	got, err := assembleAnnotations(source, bundle, &migrationImage{
		Tag:    "migration-1.2.3-abcd-1700000000",
		Digest: "sha256:abc123",
	})

	g.Expect(err).NotTo(HaveOccurred())
	imageURL := url + "/tree/main/tasks/build"
	g.Expect(got.values).To(Equal(map[string]string{
		"org.opencontainers.image.source":      url,
		"org.opencontainers.image.revision":    "main",
		"org.opencontainers.image.url":         imageURL,
		"org.opencontainers.image.version":     "1.2.3",
		"org.opencontainers.image.description": "Build and test the project",
		"dev.konflux-ci.task.migration.digest": "sha256:abc123",
		"dev.konflux-ci.task.migration.tag":    "migration-1.2.3-abcd-1700000000",
	}))
	g.Expect(got.keys).To(Equal([]string{
		"org.opencontainers.image.source",
		"org.opencontainers.image.revision",
		"org.opencontainers.image.url",
		"org.opencontainers.image.version",
		"org.opencontainers.image.description",
		"dev.konflux-ci.task.migration.digest",
		"dev.konflux-ci.task.migration.tag",
	}))
}

func TestAssembleAnnotationsAddsOptionalDocumentationLinks(t *testing.T) {
	g := NewWithT(t)
	repositoryDir := t.TempDir()
	url := "https://example.com/team/repository"
	for _, name := range []string{"README.md", "TROUBLESHOOTING.md", "USAGE.md"} {
		writeAnnotationTestFile(t, filepath.Join(repositoryDir, name), "documentation")
	}

	source := sourceInfo{URL: url, Revision: "release-1", Context: "."}
	got, err := assembleAnnotations(source, preparedBundle{TaskDir: repositoryDir}, nil)

	g.Expect(err).NotTo(HaveOccurred())
	imageURL := url + "/tree/release-1/."
	g.Expect(got.values).To(Equal(map[string]string{
		"org.opencontainers.image.source":        url,
		"org.opencontainers.image.revision":      "release-1",
		"org.opencontainers.image.url":           imageURL,
		"org.opencontainers.image.documentation": imageURL + "/README.md",
		"dev.tekton.docs.troubleshooting":        imageURL + "/TROUBLESHOOTING.md",
		"dev.tekton.docs.usage":                  imageURL + "/USAGE.md",
		"org.opencontainers.image.description":   "",
	}))
	g.Expect(got.values).NotTo(HaveKey("org.opencontainers.image.version"))
	g.Expect(got.values).NotTo(HaveKey("dev.konflux-ci.task.migration.digest"))
	g.Expect(got.values).NotTo(HaveKey("dev.konflux-ci.task.migration.tag"))
}

func TestAssembleAnnotationsReturnsDescriptionError(t *testing.T) {
	g := NewWithT(t)
	missingFile := filepath.Join(t.TempDir(), "missing.yaml")

	source := sourceInfo{URL: "https://example.com/task", Revision: "main", Context: "."}
	bundle := preparedBundle{Files: []string{missingFile}, TaskDir: filepath.Dir(missingFile), Version: "1.0"}
	got, err := assembleAnnotations(source, bundle, nil)

	g.Expect(err).To(MatchError(ContainSubstring(fmt.Sprintf("reading bundle file %q", missingFile))))
	g.Expect(got.values).To(BeNil())
}

func TestCollectDescriptions(t *testing.T) {
	g := NewWithT(t)
	firstFile := filepath.Join(t.TempDir(), "first.yaml")
	secondFile := filepath.Join(t.TempDir(), "second.yaml")
	writeAnnotationTestFile(t, firstFile, `apiVersion: tekton.dev/v1
kind: Task
spec:
  description: First task
---
apiVersion: tekton.dev/v1
kind: Pipeline
spec:
  description: Second resource
---
apiVersion: tekton.dev/v1
kind: Task
spec: {}
`)
	writeAnnotationTestFile(t, secondFile, `apiVersion: tekton.dev/v1
kind: Task
spec:
  description: Third task
`)

	got, err := collectDescriptions([]string{firstFile, secondFile})

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal("First task\n---\nSecond resource\n---\n---\nThird task"))
}

func TestCollectDescriptionsReturnsEmptyWhenNoDescriptionsExist(t *testing.T) {
	g := NewWithT(t)
	filePath := filepath.Join(t.TempDir(), "task.yaml")
	writeAnnotationTestFile(t, filePath, `apiVersion: tekton.dev/v1
kind: Task
spec:
  steps: []
`)

	got, err := collectDescriptions([]string{filePath})

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(BeEmpty())

	got, err = collectDescriptions(nil)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(BeEmpty())
}

func TestCollectDescriptionsReportsMalformedYAMLDocument(t *testing.T) {
	g := NewWithT(t)
	filePath := filepath.Join(t.TempDir(), "task.yaml")
	writeAnnotationTestFile(t, filePath, `apiVersion: tekton.dev/v1
kind: Task
spec:
  description: Valid first document
---
kind: [invalid
`)

	_, err := collectDescriptions([]string{filePath})

	g.Expect(err).To(MatchError(ContainSubstring(fmt.Sprintf("decoding YAML document 2 from %q", filePath))))
}

func writeAnnotationTestFile(t *testing.T, path, contents string) {
	t.Helper()
	g := NewWithT(t)
	g.Expect(os.WriteFile(path, []byte(contents), 0o600)).To(Succeed())
}
