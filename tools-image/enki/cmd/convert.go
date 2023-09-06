package cmd

import (
	"github.com/kairos-io/enki/pkg/action"
	v1 "github.com/kairos-io/kairos-agent/v2/pkg/types/v1"
	"github.com/spf13/cobra"
)

// NewConvertCmd returns a new instance of the build-iso subcommand and appends it to
// the root command.
func NewConvertCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "convert rootfs",
		Short: "Convert a base image to a Kairos image",
		Long: "Convert a base image to a Kairos image\n\n" +
			"This is best effort. Enki will try to detect the distribution and add\n" +
			"the necessary bits to convert it to a Kairos image",
		Args: cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return CheckRoot() // TODO: Do we need root?
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Set this after parsing of the flags, so it fails on parsing and prints usage properly
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true // Do not propagate errors down the line, we control them

			// TODO: Check if this is really an existing dir (not a file)
			rootfsDir := args[0]
			resultPath := args[1]
			imageName := args[2]

			logger := v1.NewLogger()

			convertAction := action.NewConverterAction(rootfsDir, resultPath, imageName)
			err := convertAction.Run()
			if err != nil {
				logger.Errorf(err.Error())
				return err
			}

			return nil
		},
	}

	return c
}

func init() {
	rootCmd.AddCommand(NewConvertCmd())
}
