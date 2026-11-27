package tknbundle_test

import (
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	"github.com/konflux-ci/konflux-build-cli/pkg/commands/tknbundle"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"
)

const (
	compatibilityImage         = "registry.example.com/team/bundle:v1"
	compatibilityDigest        = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	compatibilityKustomization = `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - task.yaml
namePrefix: rendered-
commonLabels:
  app.kubernetes.io/version: "2.0"
`
)

func compatibilityTask(name, version string) string {
	return fmt.Sprintf(`apiVersion: tekton.dev/v1
kind: Task
metadata:
  name: %s
  labels:
    app.kubernetes.io/version: %q
spec:
  description: Build the project
  stepTemplate:
    image: template:original
  steps:
    - name: build
      image: build:original
    - name: test
      image: test:original
    - name: inherited
      script: echo inherited
    - name: referenced
      ref:
        name: reusable-step
  sidecars:
    - name: service
      image: sidecar:original
`, name, version)
}

func newCompatibilityCommand(t *testing.T, fixtures map[string]string, context string) (*tknbundle.TektonBundlePush, *compatibilityPublisher) {
	t.Helper()
	g := NewWithT(t)
	sourceDir := t.TempDir()
	for name, content := range fixtures {
		path := filepath.Join(sourceDir, name)
		g.Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
		g.Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())
	}
	resultDir := t.TempDir()
	publisher := &compatibilityPublisher{t: t}
	return &tknbundle.TektonBundlePush{
		Params: &tknbundle.TektonBundlePushParams{
			Image:                 compatibilityImage,
			Source:                sourceDir,
			Context:               context,
			URL:                   "https://example.com/repo",
			Revision:              "abc123",
			ResultPathImageURL:    filepath.Join(resultDir, "IMAGE_URL"),
			ResultPathImageDigest: filepath.Join(resultDir, "IMAGE_DIGEST"),
			ResultPathImageRef:    filepath.Join(resultDir, "IMAGE_REF"),
		},
		CliWrappers:   tknbundle.TektonBundlePushCliWrappers{TknCli: publisher},
		ResultsWriter: common.NewResultsWriter(),
	}, publisher
}

// Capture file contents during publication: Run removes temporary bundle files
// before returning. Tests observe the payload supplied to the external publisher.
type compatibilityPublisher struct {
	t       *testing.T
	bundles []compatibilityBundle
}

var _ cliwrappers.TknCliInterface = &compatibilityPublisher{}

func (p *compatibilityPublisher) Push(args *cliwrappers.TknPushArgs) (string, string, error) {
	p.t.Helper()
	g := NewWithT(p.t)
	bundle := compatibilityBundle{
		destination: args.DestinationImage,
		annotations: maps.Clone(args.Annotations),
	}
	for _, path := range args.Filenames {
		content, err := os.ReadFile(path)
		g.Expect(err).NotTo(HaveOccurred())
		bundle.manifests = append(bundle.manifests, string(content))
	}
	p.bundles = append(p.bundles, bundle)
	return "Pushed Tekton Bundle to " + args.DestinationImage + "@" + compatibilityDigest + "\n", "", nil
}

func (p *compatibilityPublisher) onlyBundle(t *testing.T) compatibilityBundle {
	t.Helper()
	NewWithT(t).Expect(p.bundles).To(HaveLen(1))
	return p.bundles[0]
}

type compatibilityBundle struct {
	destination string
	annotations map[string]string
	manifests   []string
}

type compatibilityResource struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Steps []struct {
			Name  string  `yaml:"name"`
			Image *string `yaml:"image"`
		} `yaml:"steps"`
		StepTemplate struct {
			Image string `yaml:"image"`
		} `yaml:"stepTemplate"`
		Sidecars []struct {
			Image string `yaml:"image"`
		} `yaml:"sidecars"`
	} `yaml:"spec"`
}

func (b compatibilityBundle) resources(t *testing.T) []compatibilityResource {
	t.Helper()
	g := NewWithT(t)
	var resources []compatibilityResource
	for _, content := range b.manifests {
		decoder := yaml.NewDecoder(strings.NewReader(content))
		for {
			var resource compatibilityResource
			err := decoder.Decode(&resource)
			if err == io.EOF {
				break
			}
			g.Expect(err).NotTo(HaveOccurred())
			resources = append(resources, resource)
		}
	}
	return resources
}

func (b compatibilityBundle) resourceNames(t *testing.T) []string {
	t.Helper()
	var names []string
	for _, resource := range b.resources(t) {
		names = append(names, resource.Metadata.Name)
	}
	return names
}

func expectNoCompatibilityResults(t *testing.T, command *tknbundle.TektonBundlePush) {
	t.Helper()
	g := NewWithT(t)
	for _, path := range []string{command.Params.ResultPathImageURL, command.Params.ResultPathImageDigest, command.Params.ResultPathImageRef} {
		_, err := os.Stat(path)
		g.Expect(os.IsNotExist(err)).To(BeTrue(), "a rejected bundle must not produce result files")
	}
}

func expectCompatibilityStepImages(t *testing.T, bundle compatibilityBundle, want map[string]string) {
	t.Helper()
	g := NewWithT(t)
	resources := bundle.resources(t)
	g.Expect(resources).To(HaveLen(1))
	resource := resources[0]
	g.Expect(resource.Kind).To(Equal("Task"))
	g.Expect(resource.Spec.Steps).To(HaveLen(4))
	images := make(map[string]string)
	for _, step := range resource.Spec.Steps {
		if step.Image != nil {
			images[step.Name] = *step.Image
		}
	}
	g.Expect(images).To(Equal(want), "only steps with existing image fields may be replaced")
	g.Expect(resource.Spec.StepTemplate.Image).To(Equal("template:original"))
	g.Expect(resource.Spec.Sidecars).To(HaveLen(1))
	g.Expect(resource.Spec.Sidecars[0].Image).To(Equal("sidecar:original"))
}
