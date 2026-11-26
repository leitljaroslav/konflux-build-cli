package tknbundle

import (
	"fmt"

	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	"github.com/spf13/cobra"
)

type TektonBundlePush struct {
	Params *TektonBundlePushParams
}

func (c *TektonBundlePush) Run() error {
	return fmt.Errorf("tkn-bundle push is not implemented")
}

func NewTektonBundlePush(cmd *cobra.Command) (*TektonBundlePush, error) {
	params := &TektonBundlePushParams{}
	if err := common.ParseParameters(cmd, TektonBundlePushParamsConfig, params); err != nil {
		return nil, err
	}
	return &TektonBundlePush{Params: params}, nil
}
