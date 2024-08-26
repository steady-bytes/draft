package cmd

import (
	"github.com/steady-bytes/draft/tools/dctl/cmd/release"

	"github.com/spf13/cobra"
)

var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Commands for releasing Draft components",
}

var releaseModule = &cobra.Command{
	Use:     "module",
	Aliases: []string{"mod"},
	Short:   "Release a Go module",
	Long: `Release a Go module using a git tag. This will check the latest tag for the given module
and will ask how you would like to increment the semantic version (major.minor.patch). It will create
a git tag with the new version and push it to the git origin.`,
	Example: "dctl release module --path pkg/chassis",
	PreRunE: requireWorkspace,
	RunE:    release.Module,
}

var releaseImage = &cobra.Command{
	Use:     "image",
	Aliases: []string{"img"},
	Short:   "Release a Docker image",
	Long: `Release a Docker image by adding a tag to an existing image (e.g. latest, edge). This will pull the existing image,
apply the new tag, and then push the image. You must set the env variables CONTAINER_REGISTRY_USERNAME and CONTAINER_REGISTRY_PASSWORD
in order to authenticate to the registry.`,
	Example: "dctl release image --image registry.com/hello/world --source latest --target v1.0.0",
	RunE:    release.Image,
}

func init() {
	// add parent
	rootCmd.AddCommand(releaseCmd)
	// add children
	releaseCmd.AddCommand(releaseModule)
	releaseModule.Flags().StringVarP(&release.Path, "path", "p", "", "path of Go module to release (e.g. pkg/chassis or tools/dctl)")
	releaseCmd.AddCommand(releaseImage)
	releaseImage.Flags().StringVarP(&release.ImageName, "image", "i", "", "the Docker image to release/tag")
	releaseImage.Flags().StringVarP(&release.SourceTag, "source", "s", "", "the source image tag to pull")
	releaseImage.Flags().StringVarP(&release.TargetTag, "target", "t", "", "the target image tag to apply and push")
}
