package tknbundle

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
)

func TestTektonBundlePushValidation(t *testing.T) {
	g := NewWithT(t)

	sourceDir := t.TempDir()

	sourceFile := filepath.Join(sourceDir, "task.yaml")
	g.Expect(os.WriteFile(sourceFile, []byte("test"), 0o600)).To(Succeed())

	missingDir := filepath.Join(sourceDir, "does-not-exist")

	tests := []struct {
		name        string
		params      TektonBundlePushParams
		wantErr     string
		expectError bool
	}{
		{
			name: "invalid image syntax",
			params: TektonBundlePushParams{
				Image:  "invalid image:v1",
				Source: sourceDir,
			},
			wantErr: "invalid image:",
		},
		{
			name: "untagged image is accepted",
			params: TektonBundlePushParams{
				Image:  "quay.io/mock/just-image",
				Source: sourceDir,
			},
		},
		{
			name: "source directory does not exist",
			params: TektonBundlePushParams{
				Image: "quay.io/mock/bundle:v1", Source: missingDir},
			wantErr: "checking source directory:",
		},
		{
			name: "source directory is empty",
			params: TektonBundlePushParams{
				Image: "quay.io/mock/bundle:v1"},
			wantErr: "checking source directory:",
		},
		{
			name: "source is a file",
			params: TektonBundlePushParams{
				Image: "quay.io/mock/bundle:v1", Source: sourceFile},
			wantErr: `source path "task.yaml" is not a directory`,
		},
		{
			name: "validation passed",
			params: TektonBundlePushParams{
				Image:  "quay.io/mock/bundle:v1",
				Source: sourceDir,
			},
		},
		{
			name: "replacement disabled",
			params: TektonBundlePushParams{
				Image:  "quay.io/mock/bundle:v1",
				Source: sourceDir,
			},
		},
		{
			name: "empty selector selects all steps",
			params: TektonBundlePushParams{
				Image:      "quay.io/mock/bundle:v1",
				Source:     sourceDir,
				StepsImage: "quay.io/mock/tools:v1",
			},
		},
		{
			name: "one included step",
			params: TektonBundlePushParams{
				Image:               "quay.io/mock/bundle:v1",
				Source:              sourceDir,
				StepsImage:          "quay.io/mock/tools:v1",
				StepsImageStepNames: "build",
			},
		},
		{
			name: "comma separated includes",
			params: TektonBundlePushParams{
				Image:               "quay.io/mock/bundle:v1",
				Source:              sourceDir,
				StepsImage:          "quay.io/mock/tools:v1",
				StepsImageStepNames: "build,test",
			},
		},
		{
			name: "space separated includes",
			params: TektonBundlePushParams{
				Image:               "quay.io/mock/bundle:v1",
				Source:              sourceDir,
				StepsImage:          "quay.io/mock/tools:v1",
				StepsImageStepNames: "build test",
			},
		},
		{
			name: "mixed separators",
			params: TektonBundlePushParams{
				Image:               "quay.io/mock/bundle:v1",
				Source:              sourceDir,
				StepsImage:          "quay.io/mock/tools:v1",
				StepsImageStepNames: " build,\t test ",
			},
		},
		{
			name: "one excluded step",
			params: TektonBundlePushParams{
				Image:               "quay.io/mock/bundle:v1",
				Source:              sourceDir,
				StepsImage:          "quay.io/mock/tools:v1",
				StepsImageStepNames: "!publish",
			},
		},
		{
			name: "comma separated excludes",
			params: TektonBundlePushParams{
				Image:               "quay.io/mock/bundle:v1",
				Source:              sourceDir,
				StepsImage:          "quay.io/mock/tools:v1",
				StepsImageStepNames: "!test,!publish",
			},
		},
		{
			name: "space separated excludes",
			params: TektonBundlePushParams{
				Image:               "quay.io/mock/bundle:v1",
				Source:              sourceDir,
				StepsImage:          "quay.io/mock/tools:v1",
				StepsImageStepNames: "!test !publish",
			},
		},
		{
			name: "include then exclude",
			params: TektonBundlePushParams{
				Image:               "quay.io/mock/bundle:v1",
				Source:              sourceDir,
				StepsImage:          "quay.io/mock/tools:v1",
				StepsImageStepNames: "build,!test",
			},
			expectError: true,
		},
		{
			name: "exclude then include",
			params: TektonBundlePushParams{
				Image:               "quay.io/mock/bundle:v1",
				Source:              sourceDir,
				StepsImage:          "quay.io/mock/tools:v1",
				StepsImageStepNames: "!test build",
			},
			expectError: true,
		},
		{
			name: "bare exclusion",
			params: TektonBundlePushParams{
				Image:               "quay.io/mock/bundle:v1",
				Source:              sourceDir,
				StepsImage:          "quay.io/mock/tools:v1",
				StepsImageStepNames: "!",
			},
		},
		{
			name: "bare exclusion among names",
			params: TektonBundlePushParams{
				Image:               "quay.io/mock/bundle:v1",
				Source:              sourceDir,
				StepsImage:          "quay.io/mock/tools:v1",
				StepsImageStepNames: "!test,!",
			},
		},
		{
			name: "selector without replacement image is ignored",
			params: TektonBundlePushParams{
				Image:               "quay.io/mock/bundle:v1",
				Source:              sourceDir,
				StepsImageStepNames: "build",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			command := &TektonBundlePush{Params: &tt.params}

			err := command.validateParams()

			if !tt.expectError && tt.wantErr == "" {
				g.Expect(err).NotTo(HaveOccurred())
			} else {
				g.Expect(err).To(HaveOccurred())
				if tt.wantErr != "" {
					g.Expect(err.Error()).To(ContainSubstring(tt.wantErr))
				}
			}
		})
	}
}
