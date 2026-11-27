package tknbundle

import (
	tektonbundlecmd "github.com/konflux-ci/konflux-build-cli/pkg/commands/tknbundle"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
	"github.com/spf13/cobra"
)

var TektonBundlePushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push a Tekton bundle to an OCI registry",
	Long: `Select Tekton YAML files from a source directory and push them as a bundle
to an OCI registry. By default, all YAML files under the source directory are
included. Use --context to select specific files or directories.

The command adds source Git metadata to the bundle and prints the pushed image
URL, digest, and reference as JSON. Use the result-path flags to write those
values to files for a Tekton task.`,
	Example: `  # Push all Tekton YAML files in the source directory
  konflux-build-cli tkn-bundle push \
    --image quay.io/myorg/mytask:v1.0.0 \
    --source ./source \
    --url https://github.com/myorg/mytask.git \
    --revision main

  # Push YAML files from a specific directory and override Task step images
  konflux-build-cli tkn-bundle push \
    --image quay.io/myorg/mytask:v1.0.0 \
    --source ./source --context tasks \
    --url https://github.com/myorg/mytask.git --revision main \
    --steps-image quay.io/myorg/task-steps:v1.0.0

  # Write image results to Tekton result files
  konflux-build-cli tkn-bundle push \
    --image quay.io/myorg/mytask:v1.0.0 \
    --source ./source \
    --url https://github.com/myorg/mytask.git --revision main \
    --result-path-image-url /tekton/results/IMAGE_URL \
    --result-path-image-digest /tekton/results/IMAGE_DIGEST \
    --result-path-image-ref /tekton/results/IMAGE_REF`,
	Run: func(cmd *cobra.Command, arg []string) {
		pushTektonBundle, err := tektonbundlecmd.NewTektonBundlePush(cmd)
		if err != nil {
			l.Logger.Fatal(err)
		}
		if err := pushTektonBundle.Run(); err != nil {
			l.Logger.Fatal(err)
		}
	},
}

func init() {
	common.RegisterParameters(TektonBundlePushCmd, tektonbundlecmd.TektonBundlePushParamsConfig)
}
