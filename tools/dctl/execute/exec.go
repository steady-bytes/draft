package execute

import (
	"bufio"
	"context"
	"os"
	"os/exec"

	"github.com/steady-bytes/draft/tools/dctl/output"
)

func ExecuteCommand(ctx context.Context, name string, c output.Color, cmd *exec.Cmd) error {
	// create a pipe for the output
	cmdReader, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	// create a pipe for the error output
	cmdErrReader, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	// scanner for output
	scanner := bufio.NewScanner(cmdReader)
	go func() {
		for scanner.Scan() {
			output.PrintlnWithNameAndColor(name, scanner.Text(), c)
		}
	}()

	// scanner for error output
	scannerErr := bufio.NewScanner(cmdErrReader)
	go func() {
		for scannerErr.Scan() {
			output.PrintlnWithNameAndColor(name, scannerErr.Text(), c)
		}
	}()

	// watch for done signal and kill process if received
	go func() {
		<-ctx.Done()
		err := cmd.Process.Signal(os.Interrupt)
		if err != nil {
			// only error if not closed by user
			if err.Error() != "signal: killed" && err.Error() != "os: process already finished" {
				output.Error(err)
			}
		}
	}()

	// start the command
	err = cmd.Start()
	if err != nil {
		return err
	}

	// wait for completion
	err = cmd.Wait()
	if err != nil {
		// only error if not closed by user
		if err.Error() != "signal: killed" && err.Error() != "os: process already finished" {
			return err
		}
	}

	return nil
}

// ExecuteCommandReturnStdout executes a command and returns the output of stdout as a string.
// It does not print the output to the console. This can be used to get the output of a command.
// It will not return until the command has completed or the context is cancelled.
func ExecuteCommandReturnStdout(ctx context.Context, cmd *exec.Cmd) (string, error) {
	// create a pipe for the output
	cmdReader, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}

	// scanner for output
	scanner := bufio.NewScanner(cmdReader)
	var output string
	go func() {
		for scanner.Scan() {
			if output != "" {
				output += "\n"
			}
			output += scanner.Text()
		}
	}()

	// start the command
	err = cmd.Start()
	if err != nil {
		return "", err
	}

	// watch for done signal and kill process if received
	go func() {
		<-ctx.Done()
		_ = cmd.Process.Kill()
	}()

	// wait for completion
	err = cmd.Wait()
	if err != nil {
		// only error if not closed by user
		if err.Error() != "signal: killed" && err.Error() != "os: process already finished" {
			return output, err
		}
	}

	return output, nil
}
