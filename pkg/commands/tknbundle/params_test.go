package tknbundle

import (
	"path/filepath"
	"testing"

	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
)

func TestTektonBundlePushParams(t *testing.T) {
	// Every subtest needs fresh flags and an isolated environment. t.Setenv
	// restores the previous environment when that subtest finishes.
	newCommand := func(t *testing.T) *cobra.Command {
		t.Helper()
		for _, param := range TektonBundlePushParamsConfig {
			if param.EnvVarName != "" {
				t.Setenv(param.EnvVarName, "")
			}
		}

		cmd := &cobra.Command{Use: "push"}
		common.RegisterParameters(cmd, TektonBundlePushParamsConfig)
		return cmd
	}

	sourceDir := t.TempDir()
	digestPath := filepath.Join(sourceDir, "IMAGE_DIGEST")
	urlPath := filepath.Join(sourceDir, "IMAGE_URL")
	refPath := filepath.Join(sourceDir, "IMAGE_REF")

	fromFlags := TektonBundlePushParams{
		Image:                 "quay.io/mock/bundle:v1",
		Source:                sourceDir,
		Context:               "tasks,shared",
		URL:                   "https://github.com/mock/repo",
		Revision:              "abc123",
		StepsImage:            "quay.io/mock/tools:v1",
		StepsImageStepNames:   "build,test",
		ResultPathImageDigest: digestPath,
		ResultPathImageURL:    urlPath,
		ResultPathImageRef:    refPath,
	}
	requiredFlags := []string{
		"--image", fromFlags.Image,
		"--source", sourceDir,
		"--url", fromFlags.URL,
		"--revision", fromFlags.Revision,
	}
	allFlags := []string{
		"--image", fromFlags.Image,
		"--source", sourceDir,
		"--context", fromFlags.Context,
		"--url", fromFlags.URL,
		"--revision", fromFlags.Revision,
		"--steps-image", fromFlags.StepsImage,
		"--steps-image-step-names", fromFlags.StepsImageStepNames,
		"--result-path-image-digest", digestPath,
		"--result-path-image-url", urlPath,
		"--result-path-image-ref", refPath,
	}

	// Use different values from the flags so the precedence test can detect
	// an environment value incorrectly winning over a flag.
	environment := map[string]string{
		"KBC_TEKTON_BUNDLE_PUSH_IMAGE":                    "quay.io/mock/env-bundle:v2",
		"KBC_TEKTON_BUNDLE_PUSH_SOURCE":                   filepath.Join(sourceDir, "env-source"),
		"KBC_TEKTON_BUNDLE_PUSH_CONTEXT":                  "env-tasks",
		"KBC_TEKTON_BUNDLE_PUSH_URL":                      "https://github.com/mock/env-repo",
		"KBC_TEKTON_BUNDLE_PUSH_REVISION":                 "def456",
		"KBC_TEKTON_BUNDLE_PUSH_STEPS_IMAGE":              "quay.io/mock/env-tools:v2",
		"KBC_TEKTON_BUNDLE_PUSH_STEPS_IMAGE_STEP_NAMES":   "!publish",
		"KBC_TEKTON_BUNDLE_PUSH_RESULT_PATH_IMAGE_DIGEST": filepath.Join(sourceDir, "ENV_DIGEST"),
		"KBC_TEKTON_BUNDLE_PUSH_RESULT_PATH_IMAGE_URL":    filepath.Join(sourceDir, "ENV_URL"),
		"KBC_TEKTON_BUNDLE_PUSH_RESULT_PATH_IMAGE_REF":    filepath.Join(sourceDir, "ENV_REF"),
	}
	fromEnvironment := TektonBundlePushParams{
		Image:                 "quay.io/mock/env-bundle:v2",
		Source:                filepath.Join(sourceDir, "env-source"),
		Context:               "env-tasks",
		URL:                   "https://github.com/mock/env-repo",
		Revision:              "def456",
		StepsImage:            "quay.io/mock/env-tools:v2",
		StepsImageStepNames:   "!publish",
		ResultPathImageDigest: filepath.Join(sourceDir, "ENV_DIGEST"),
		ResultPathImageURL:    filepath.Join(sourceDir, "ENV_URL"),
		ResultPathImageRef:    filepath.Join(sourceDir, "ENV_REF"),
	}

	tests := []struct {
		name  string
		flags []string
		env   map[string]string
		want  TektonBundlePushParams
	}{
		{
			name:  "values from flags",
			flags: allFlags,
			want:  fromFlags,
		},
		{
			name: "values from environment",
			env:  environment,
			want: fromEnvironment,
		},
		{
			name:  "flags override environment",
			flags: allFlags,
			env:   environment,
			want:  fromFlags,
		},
		{
			name:  "optional parameters omitted",
			flags: requiredFlags,
			// Unspecified fields in this expected struct are empty strings.
			want: TektonBundlePushParams{
				Image:    "quay.io/mock/bundle:v1",
				Source:   sourceDir,
				Context:  ".",
				URL:      "https://github.com/mock/repo",
				Revision: "abc123",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			cmd := newCommand(t)
			for name, value := range tt.env {
				t.Setenv(name, value)
			}

			// Cobra parses the flags; the project helper populates our struct.
			g.Expect(cmd.ParseFlags(tt.flags)).To(Succeed())
			params := &TektonBundlePushParams{}
			err := common.ParseParameters(cmd, TektonBundlePushParamsConfig, params)

			g.Expect(err).NotTo(HaveOccurred())
			// Comparing the whole struct checks all three result paths too.
			g.Expect(*params).To(Equal(tt.want))
		})
	}

	t.Run("missing required parameter", func(t *testing.T) {
		for _, missing := range []string{"image", "source", "url", "revision"} {
			t.Run(missing, func(t *testing.T) {
				g := NewWithT(t)
				cmd := newCommand(t)

				// Omit exactly one flag/value pair. Keeping the other required
				// inputs valid makes the expected error unambiguous.
				var flags []string
				for i := 0; i < len(requiredFlags); i += 2 {
					if requiredFlags[i] != "--"+missing {
						flags = append(flags, requiredFlags[i], requiredFlags[i+1])
					}
				}
				g.Expect(cmd.ParseFlags(flags)).To(Succeed())
				params := &TektonBundlePushParams{}
				err := common.ParseParameters(cmd, TektonBundlePushParamsConfig, params)

				g.Expect(err).To(MatchError("required parameter '" + missing + "' is not set"))
			})
		}
	})
}
