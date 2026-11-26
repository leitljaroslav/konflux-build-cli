package tknbundle

import "fmt"

func (c *TektonBundlePush) writeResults(result TektonBundlePushResult) error {
	resultJSON, err := c.ResultsWriter.CreateResultJson(result)
	if err != nil {
		return fmt.Errorf("creating bundle push result JSON: %w", err)
	}

	if err := c.ResultsWriter.WriteResultString(result.ImageURL, c.Params.ResultPathImageURL); err != nil {
		return fmt.Errorf("writing bundle image URL result: %w", err)
	}
	if err := c.ResultsWriter.WriteResultString(result.ImageDigest, c.Params.ResultPathImageDigest); err != nil {
		return fmt.Errorf("writing bundle image digest result: %w", err)
	}
	if err := c.ResultsWriter.WriteResultString(result.ImageRef, c.Params.ResultPathImageRef); err != nil {
		return fmt.Errorf("writing bundle image reference result: %w", err)
	}

	fmt.Print(resultJSON)
	return nil
}
