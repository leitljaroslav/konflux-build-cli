package tknbundle

import (
	"fmt"
	"os"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	"github.com/spf13/cobra"
	"go.podman.io/image/v5/docker/reference"
)

type TektonBundlePushCliWrappers struct {
	TknCli  cliwrappers.TknCliInterface
	OrasCli cliwrappers.OrasCliInterface
}

type TektonBundlePush struct {
	Params        *TektonBundlePushParams
	CliWrappers   TektonBundlePushCliWrappers
	ResultsWriter common.ResultsWriterInterface
}

func (c *TektonBundlePush) Run() error {
	return fmt.Errorf("tkn-bundle push is not implemented")
}

func NewTektonBundlePush(cmd *cobra.Command) (*TektonBundlePush, error) {
	pushCmd := &TektonBundlePush{}
	params := &TektonBundlePushParams{}
	if err := common.ParseParameters(cmd, TektonBundlePushParamsConfig, params); err != nil {
		return nil, err
	}
	pushCmd.Params = params
	if err := pushCmd.initCliWrappers(); err != nil {
		return nil, err
	}
	pushCmd.ResultsWriter = common.NewResultsWriter()
	return pushCmd, nil
}

func (c *TektonBundlePush) initCliWrappers() error {
	executor := cliwrappers.NewCliExecutor()
	tknCli, err := cliwrappers.NewTknCli(executor)
	if err != nil {
		return err
	}
	c.CliWrappers.TknCli = tknCli
	return nil
}

func (c *TektonBundlePush) validateParams() error {
	// validate the image
	_, err := reference.ParseNormalizedNamed(c.Params.Image)
	if err != nil {
		return fmt.Errorf("invalid image: %w", err)
	}
	// validate the source
	info, err := os.Stat(c.Params.Source)
	if err != nil {
		return fmt.Errorf("checking source directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("source path %q is not a directory", info.Name())
	}

	if _, err := newStepImageOverride(c.Params.StepsImage, c.Params.StepsImageStepNames); err != nil {
		return err
	}

	return nil
}
