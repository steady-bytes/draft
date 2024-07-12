package pipelines

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steady-bytes/draft/tools/dctl/config"
	"github.com/steady-bytes/draft/tools/dctl/execute"
	"github.com/steady-bytes/draft/tools/dctl/input"
	"github.com/steady-bytes/draft/tools/dctl/output"

	"github.com/spf13/cobra"
)

const (
	secretTemplate = `apiVersion: v1
kind: Secret
metadata:
  name: ssh-key
  annotations:
    tekton.dev/git-0: github.com
type: kubernetes.io/ssh-auth
stringData:
  ssh-privatekey: |
`
)

var (
	SshIdFile     string
	// NOTE: keep these in the order in which they should be applied
	manifestPaths = []string{
		// remote manifests
		"https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml",
		"https://storage.googleapis.com/tekton-releases/dashboard/latest/release-full.yaml",
		"https://raw.githubusercontent.com/tektoncd/catalog/main/task/git-clone/0.6/git-clone.yaml",
		// local manifests
		"secrets",
		"serviceaccounts",
		"caches",
		"tasks",
		"pipelines",
	}
)

type InitConfig struct {
	PrivateKey string
}

func Init(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	dctx := config.GetContext()

	if SshIdFile == "" {
		// get the home directory
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		SshIdFile = filepath.Join(home, ".ssh", "id_rsa")
	}

	// read the ssh private key file
	f, err := file(SshIdFile)
	if err != nil {
		return err
	}
	defer f.Close()

	// write key to secret template
	t := secretTemplate
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		t += fmt.Sprintf("    %s\n", scanner.Text())
	}

	// set the path to the pipelines secrets
	pipelinesPath := filepath.Join(dctx.Root, "pipelines")

	// create the directory if it doesn't exist
	secretsPath := filepath.Join(pipelinesPath, "secrets")
	if _, err := os.Stat(secretsPath); os.IsNotExist(err) {
		err := os.MkdirAll(secretsPath, os.ModePerm)
		if err != nil {
			return err
		}
	}

	// create the secret file
	secretFile := filepath.Join(secretsPath, "ssh-key.yaml")
	secret, err := os.Create(secretFile)
	if err != nil {
		return err
	}
	defer secret.Close()

	// write the secret to the file
	_, err = secret.WriteString(t)
	if err != nil {
		return err
	}

	// check current kube context and ask to proceed
	command := exec.Command("kubectl", "config", "current-context")
	kubeContext, err := execute.ExecuteCommandReturnStdout(ctx, command)
	if err != nil {
		return err
	}
	output.Print("Current kube context: %s", kubeContext)
	output.Print("The above context will be used to install required pipeline manifests. Would you like to proceed? (yes/NO)")
	if !input.ConfirmDefaultDeny() {
		output.Warn("Aborted")
		return nil
	}

	// apply all manifests except runs
	for index, path := range manifestPaths {
		if !strings.HasPrefix(path, "https") {
			path = filepath.Join(pipelinesPath, path)
		}
		err := apply(ctx, path)
		if err != nil {
			return err
		}
		// on initial tekton manifest install, watch for pods to be ready before continuing
		if index == 0 {
			output.Print("Waiting for up to 30 seconds for Tekton pods to be ready...")
			for i := 0; i < 30; i++ {
				time.Sleep(1 * time.Second)
				command := exec.Command("kubectl", "get", "pods", "--namespace", "tekton-pipelines", "--field-selector", "status.phase==Running")
				output, err := execute.ExecuteCommandReturnStdout(ctx, command)
				if err != nil {
					return err
				}
				if output == "No resources found in tekton-pipelines namespace." {
					break
				}
			}
		}
	}

	return nil
}

func file(filePath string) (*os.File, error) {
	path, err := filepath.Abs(filePath)
	if err != nil {
		return nil, err
	}
	inFile, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return inFile, nil
}

func apply(ctx context.Context, path string) error {
	// confirm with user
	output.Print("About to apply the manifest(s) located at: %s", path)
	output.Print("Would you like to proceed? (YES/no)")
	if !input.ConfirmDefaultAllow() {
		output.Warn("Skipped")
		return nil
	}
	// apply the manifest
	command := exec.Command("kubectl", "apply", "-f", path)
	return execute.ExecuteCommand(ctx, "kubectl", output.Cyan, command)
}
