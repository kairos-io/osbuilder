package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enki",
		Short: "enki",
	}
	cmd.PersistentFlags().Bool("debug", false, "Enable debug output")
	cmd.PersistentFlags().String("config-dir", "/etc/elemental", "Set config dir (default is /etc/elemental)")
	cmd.PersistentFlags().String("logfile", "", "Set logfile")
	cmd.PersistentFlags().Bool("quiet", false, "Do not output to stdout")
	_ = viper.BindPFlag("debug", cmd.PersistentFlags().Lookup("debug"))
	_ = viper.BindPFlag("config-dir", cmd.PersistentFlags().Lookup("config-dir"))
	_ = viper.BindPFlag("logfile", cmd.PersistentFlags().Lookup("logfile"))
	_ = viper.BindPFlag("quiet", cmd.PersistentFlags().Lookup("quiet"))

	if viper.GetBool("debug") {
		logrus.SetLevel(logrus.DebugLevel)
	}
	return cmd
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = NewRootCmd()

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

// CheckRoot is a helper to return on PreRunE, so we can add it to commands that require root
func CheckRoot() error {
	if os.Geteuid() != 0 {
		return errors.New("this command requires root privileges")
	}
	return nil
}

type enum struct {
	Allowed []string
	Value   string
}

func (a enum) String() string {
	return a.Value
}

func (a *enum) Set(p string) error {
	isIncluded := func(opts []string, val string) bool {
		for _, opt := range opts {
			if val == opt {
				return true
			}
		}
		return false
	}
	if !isIncluded(a.Allowed, p) {
		return fmt.Errorf("%s is not included in %s", p, strings.Join(a.Allowed, ","))
	}
	a.Value = p
	return nil
}

func (a *enum) Type() string {
	return "string"
}

// newEnum give a list of allowed flag parameters, where the second argument is the default
func newEnumFlag(allowed []string, d string) *enum {
	return &enum{
		Allowed: allowed,
		Value:   d,
	}
}
