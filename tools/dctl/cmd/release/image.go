package release

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steady-bytes/draft/tools/dctl/docker"
)

var (
	ImageName string
	SourceTag    string
	TargetTag    string
)

func Image(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()

	dockerCtl, err := docker.NewDockerController()
	if err != nil {
		return err
	}

	if ImageName == "" {
		return fmt.Errorf("image name is required")
	}

	if TargetTag == "" {
		return fmt.Errorf("tag is required")
	}

	err = dockerCtl.PullImage(ctx, fmt.Sprintf("%s:%s", ImageName, SourceTag))
	if err != nil {
		return err
	}

	err = dockerCtl.TagImage(ctx, ImageName, SourceTag, TargetTag)
	if err != nil {
		return err
	}

	err = dockerCtl.PushImage(ctx, fmt.Sprintf("%s:%s", ImageName, TargetTag))
	if err != nil {
		return err
	}

	return nil
}
