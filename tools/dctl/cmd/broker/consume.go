package broker

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

func Consume(cmd *cobra.Command, args []string) error {
	fmt.Println(args)

	// connect to running catalyst process

	return errors.New("implement me")
}
