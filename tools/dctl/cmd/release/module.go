package release

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steady-bytes/draft/tools/dctl/config"
	"github.com/steady-bytes/draft/tools/dctl/execute"
	"github.com/steady-bytes/draft/tools/dctl/input"
	"github.com/steady-bytes/draft/tools/dctl/output"

	"github.com/Masterminds/semver/v3"
)

var (
	Path string
)

func Module(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()

	if Path == "" {
		return fmt.Errorf("module path is required")
	}

	// check current branch
	command := exec.Command("git", "branch", "--show-current")
	branchString, err := execute.ExecuteCommandReturnStdout(ctx, command)
	if err != nil {
		return err
	}

	if branchString != config.CurrentProject().TrunkBranch {
		output.Println("You are currently on branch %s, not the trunk branch %s. Would you like to proceed? (yes/NO)", branchString, config.CurrentProject().TrunkBranch)
		if !input.ConfirmDefaultDeny() {
			output.Println("Cancelled")
			return nil
		}
	}

	// git fetch to update all local tags
	command = exec.Command("git", "fetch", "--tags")
	err = execute.ExecuteCommand(ctx, "git", output.Blue, command)
	if err != nil {
		return err
	}

	// git tag -l
	command = exec.Command("git", "tag", "-l", "--sort=v:refname", fmt.Sprintf("%s*", Path))
	tagsString, err := execute.ExecuteCommandReturnStdout(ctx, command)
	if err != nil {
		return err
	}

	var newVersion semver.Version
	if tagsString == "" {
		newVersion, err = initialVersion()
		if err != nil {
			return err
		}
		empty := semver.Version{}
		if newVersion == empty {
			return nil
		}
	} else {
		newVersion, err = incrementVersion(tagsString)
		if err != nil {
			return err
		}
	}

	output.Println("New version will be: v%s", newVersion)
	output.Println("Would you like to proceed? (YES/no)")
	if !input.ConfirmDefaultAllow() {
		output.Println("Cancelled")
		return nil
	}

	// create tag
	newTag := fmt.Sprintf("%s/v%s", Path, newVersion)
	command = exec.Command("git", "tag", newTag)
	err = execute.ExecuteCommand(ctx, "git", output.Blue, command)
	if err != nil {
		return err
	}

	// push tag
	command = exec.Command("git", "push", "origin", newTag)
	err = execute.ExecuteCommand(ctx, "git", output.Blue, command)
	if err != nil {
		return err
	}

	return nil
}

func initialVersion() (semver.Version, error) {
	output.Println("Looks like this module doesn't yet have a version. Would you like to create an initial version? (YES/no)")
	if !input.ConfirmDefaultAllow() {
		output.Println("Cancelled")
		return semver.Version{}, nil
	}
	output.Println("Enter a new version in semver format (Major.Minor.Patch):")
	newVersionString := input.Get()
	newVersion, err := semver.NewVersion(newVersionString)
	if err != nil {
		return semver.Version{}, err
	}

	return *newVersion, nil
}

func incrementVersion(tagsString string) (semver.Version, error) {
	// find current tag (and semver)
	tags := strings.Split(tagsString, "\n")
	currentTag := strings.TrimPrefix(tags[len(tags)-1], fmt.Sprintf("%s/", Path))
	currentVersion, err := semver.NewVersion(currentTag)
	if err != nil {
		return semver.Version{}, err
	}

	// show current version and ask for upgrade increment
	output.Println("Most recent version: v%s", currentVersion)
	output.Println("Would you like to release a (M)ajor, (m)inor, or (p)atch release?")
	selection := input.Get()
	var newVersion semver.Version
	switch selection {
	case "M", "major":
		newVersion = currentVersion.IncMajor()
	case "m", "minor":
		newVersion = currentVersion.IncMinor()
	case "p", "patch":
		newVersion = currentVersion.IncPatch()
	}

	return newVersion, nil
}
