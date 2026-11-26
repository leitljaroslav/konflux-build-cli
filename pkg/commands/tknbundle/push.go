package tknbundle

import (
	"fmt"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	"github.com/spf13/cobra"
)

type TektonBundlePushCliWrappers struct {
	TknCli cliwrappers.TknCliInterface
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
