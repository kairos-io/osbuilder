package cmd

import (
	"fmt"
	"os/exec"

	"github.com/kairos-io/enki/pkg/action"
	"github.com/kairos-io/enki/pkg/config"
	"github.com/kairos-io/enki/pkg/utils"
	v1 "github.com/kairos-io/kairos-agent/v2/pkg/types/v1"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/mount-utils"
)

// NewBuildISOCmd returns a new instance of the build-iso subcommand and appends it to
// the root command.
func NewBuildISOCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "build-iso SOURCE",
		Short: "Build bootable installation media ISOs",
		Long: "Build bootable installation media ISOs\n\n" +
			"SOURCE - should be provided as uri in following format <sourceType>:<sourceName>\n" +
			"    * <sourceType> - might be [\"dir\", \"file\", \"oci\", \"docker\"], as default is \"docker\"\n" +
			"    * <sourceName> - is path to file or directory, image name with tag version",
		Args: cobra.MaximumNArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return CheckRoot()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := exec.LookPath("mount")
			if err != nil {
				return err
			}
			mounter := mount.New(path)

			cfg, err := config.ReadConfigBuild(viper.GetString("config-dir"), cmd.Flags(), mounter)
			if err != nil {
				cfg.Logger.Errorf("Error reading config: %s\n", err)
			}

			flags := cmd.Flags()

			// Set this after parsing of the flags, so it fails on parsing and prints usage properly
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true // Do not propagate errors down the line, we control them
			spec, err := config.ReadBuildISO(cfg, flags)
			if err != nil {
				cfg.Logger.Errorf("invalid install command setup %v", err)
				return err
			}

			if len(args) == 1 {
				imgSource, err := v1.NewSrcFromURI(args[0])
				if err != nil {
					cfg.Logger.Errorf("not a valid rootfs source image argument: %s", args[0])
					return err
				}
				spec.RootFS = []*v1.ImageSource{imgSource}
			} else if len(spec.RootFS) == 0 {
				errmsg := "rootfs source image for building ISO was not provided"
				cfg.Logger.Errorf(errmsg)
				return fmt.Errorf(errmsg)
			}

			// Repos and overlays can't be unmarshaled directly as they require
			// to be merged on top and flags do not match any config value key
			oRootfs, _ := flags.GetString("overlay-rootfs")
			oUEFI, _ := flags.GetString("overlay-uefi")
			oISO, _ := flags.GetString("overlay-iso")

			if oRootfs != "" {
				if ok, err := utils.Exists(cfg.Fs, oRootfs); ok {
					spec.RootFS = append(spec.RootFS, v1.NewDirSrc(oRootfs))
				} else {
					cfg.Logger.Errorf("Invalid value for overlay-rootfs")
					return fmt.Errorf("Invalid path '%s': %v", oRootfs, err)
				}
			}
			if oUEFI != "" {
				if ok, err := utils.Exists(cfg.Fs, oUEFI); ok {
					spec.UEFI = append(spec.UEFI, v1.NewDirSrc(oUEFI))
				} else {
					cfg.Logger.Errorf("Invalid value for overlay-uefi")
					return fmt.Errorf("Invalid path '%s': %v", oUEFI, err)
				}
			}
			if oISO != "" {
				if ok, err := utils.Exists(cfg.Fs, oISO); ok {
					spec.Image = append(spec.Image, v1.NewDirSrc(oISO))
				} else {
					cfg.Logger.Errorf("Invalid value for overlay-iso")
					return fmt.Errorf("Invalid path '%s': %v", oISO, err)
				}
			}

			buildISO := action.NewBuildISOAction(cfg, spec)
			err = buildISO.ISORun()
			if err != nil {
				cfg.Logger.Errorf(err.Error())
				return err
			}

			return nil
		},
	}
	c.Flags().StringP("name", "n", "", "Basename of the generated ISO file")
	c.Flags().StringP("output", "o", "", "Output directory (defaults to current directory)")
	c.Flags().Bool("date", false, "Adds a date suffix into the generated ISO file")
	c.Flags().String("overlay-rootfs", "", "Path of the overlayed rootfs data")
	c.Flags().String("overlay-uefi", "", "Path of the overlayed uefi data")
	c.Flags().String("overlay-iso", "", "Path of the overlayed iso data")
	c.Flags().String("label", "", "Label of the ISO volume")
	archType := newEnumFlag([]string{"x86_64", "arm64"}, "x86_64")
	c.Flags().Bool("squash-no-compression", true, "Disable squashfs compression.")
	c.Flags().VarP(archType, "arch", "a", "Arch to build the image for")
	return c
}

func init() {
	rootCmd.AddCommand(NewBuildISOCmd())
}
