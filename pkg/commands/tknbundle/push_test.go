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
