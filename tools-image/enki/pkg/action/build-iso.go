package action

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/kairos-io/enki/pkg/constants"
	"github.com/kairos-io/enki/pkg/utils"
	"github.com/kairos-io/kairos-agent/v2/pkg/elemental"
	v1 "github.com/kairos-io/kairos-agent/v2/pkg/types/v1"
)

type BuildISOAction struct {
	cfg  *v1.BuildConfig
	spec *v1.LiveISO
	e    *elemental.Elemental
}

type BuildISOActionOption func(a *BuildISOAction)

func NewBuildISOAction(cfg *v1.BuildConfig, spec *v1.LiveISO, opts ...BuildISOActionOption) *BuildISOAction {
	b := &BuildISOAction{
		cfg:  cfg,
		e:    elemental.NewElemental(&cfg.Config),
		spec: spec,
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// ISORun will install the system from a given configuration
func (b *BuildISOAction) ISORun() (err error) {
	cleanup := utils.NewCleanStack()
	defer func() { err = cleanup.Cleanup(err) }()

	isoTmpDir, err := utils.TempDir(b.cfg.Fs, "", "enki-iso")
	if err != nil {
		return err
	}
	cleanup.Push(func() error { return b.cfg.Fs.RemoveAll(isoTmpDir) })

	rootDir := filepath.Join(isoTmpDir, "rootfs")
	err = utils.MkdirAll(b.cfg.Fs, rootDir, constants.DirPerm)
	if err != nil {
		return err
	}

	uefiDir := filepath.Join(isoTmpDir, "uefi")
	err = utils.MkdirAll(b.cfg.Fs, uefiDir, constants.DirPerm)
	if err != nil {
		return err
	}

	isoDir := filepath.Join(isoTmpDir, "iso")
	err = utils.MkdirAll(b.cfg.Fs, isoDir, constants.DirPerm)
	if err != nil {
		return err
	}

	if b.cfg.OutDir != "" {
		err = utils.MkdirAll(b.cfg.Fs, b.cfg.OutDir, constants.DirPerm)
		if err != nil {
			b.cfg.Logger.Errorf("Failed creating output folder: %s", b.cfg.OutDir)
			return err
		}
	}

	b.cfg.Logger.Infof("Preparing squashfs root...")
	err = b.applySources(rootDir, b.spec.RootFS...)
	if err != nil {
		b.cfg.Logger.Errorf("Failed installing OS packages: %v", err)
		return err
	}
	err = utils.CreateDirStructure(b.cfg.Fs, rootDir)
	if err != nil {
		b.cfg.Logger.Errorf("Failed creating root directory structure: %v", err)
		return err
	}

	b.cfg.Logger.Infof("Preparing EFI image...")
	err = b.applySources(uefiDir, b.spec.UEFI...)
	if err != nil {
		b.cfg.Logger.Errorf("Failed installing EFI packages: %v", err)
		return err
	}

	b.cfg.Logger.Infof("Preparing ISO image root tree...")
	err = b.applySources(isoDir, b.spec.Image...)
	if err != nil {
		b.cfg.Logger.Errorf("Failed installing ISO image packages: %v", err)
		return err
	}

	err = b.prepareISORoot(isoDir, rootDir, uefiDir)
	if err != nil {
		b.cfg.Logger.Errorf("Failed preparing ISO's root tree: %v", err)
		return err
	}

	b.cfg.Logger.Infof("Creating ISO image...")
	err = b.burnISO(isoDir)
	if err != nil {
		b.cfg.Logger.Errorf("Failed preparing ISO's root tree: %v", err)
		return err
	}

	return err
}

func (b BuildISOAction) prepareISORoot(isoDir string, rootDir string, uefiDir string) error {
	kernel, initrd, err := b.e.FindKernelInitrd(rootDir)
	if err != nil {
		b.cfg.Logger.Error("Could not find kernel and/or initrd")
		return err
	}
	err = utils.MkdirAll(b.cfg.Fs, filepath.Join(isoDir, "boot"), constants.DirPerm)
	if err != nil {
		return err
	}
	//TODO document boot/kernel and boot/initrd expectation in bootloader config
	b.cfg.Logger.Debugf("Copying Kernel file %s to iso root tree", kernel)
	err = utils.CopyFile(b.cfg.Fs, kernel, filepath.Join(isoDir, constants.IsoKernelPath))
	if err != nil {
		return err
	}

	b.cfg.Logger.Debugf("Copying initrd file %s to iso root tree", initrd)
	err = utils.CopyFile(b.cfg.Fs, initrd, filepath.Join(isoDir, constants.IsoInitrdPath))
	if err != nil {
		return err
	}

	b.cfg.Logger.Info("Creating squashfs...")
	err = utils.CreateSquashFS(b.cfg.Runner, b.cfg.Logger, rootDir, filepath.Join(isoDir, constants.IsoRootFile), constants.GetDefaultSquashfsOptions())
	if err != nil {
		return err
	}

	b.cfg.Logger.Info("Creating EFI image...")
	err = b.createEFI(uefiDir, filepath.Join(isoDir, constants.IsoEFIPath))
	if err != nil {
		return err
	}
	return nil
}

func (b BuildISOAction) createEFI(root string, img string) error {
	efiSize, err := utils.DirSize(b.cfg.Fs, root)
	if err != nil {
		return err
	}

	// align efiSize to the next 4MB slot
	align := int64(4 * 1024 * 1024)
	efiSizeMB := (efiSize/align*align + align) / (1024 * 1024)

	err = b.e.CreateFileSystemImage(&v1.Image{
		File:  img,
		Size:  uint(efiSizeMB),
		FS:    constants.EfiFs,
		Label: constants.EfiLabel,
	})
	if err != nil {
		return err
	}

	files, err := b.cfg.Fs.ReadDir(root)
	if err != nil {
		return err
	}

	for _, f := range files {
		_, err = b.cfg.Runner.Run("mcopy", "-s", "-i", img, filepath.Join(root, f.Name()), "::")
		if err != nil {
			return err
		}
	}

	return nil
}

func (b BuildISOAction) burnISO(root string) error {
	cmd := "xorriso"
	var outputFile string
	var isoFileName string

	if b.cfg.Date {
		currTime := time.Now()
		isoFileName = fmt.Sprintf("%s.%s.iso", b.cfg.Name, currTime.Format("20060102"))
	} else {
		isoFileName = fmt.Sprintf("%s.iso", b.cfg.Name)
	}

	outputFile = isoFileName
	if b.cfg.OutDir != "" {
		outputFile = filepath.Join(b.cfg.OutDir, outputFile)
	}

	if exists, _ := utils.Exists(b.cfg.Fs, outputFile); exists {
		b.cfg.Logger.Warnf("Overwriting already existing %s", outputFile)
		err := b.cfg.Fs.Remove(outputFile)
		if err != nil {
			return err
		}
	}

	args := []string{
		"-volid", b.spec.Label, "-joliet", "on", "-padding", "0",
		"-outdev", outputFile, "-map", root, "/", "-chmod", "0755", "--",
	}
	args = append(args, constants.GetXorrisoBooloaderArgs(root)...)

	out, err := b.cfg.Runner.Run(cmd, args...)
	b.cfg.Logger.Debugf("Xorriso: %s", string(out))
	if err != nil {
		return err
	}

	checksum, err := utils.CalcFileChecksum(b.cfg.Fs, outputFile)
	if err != nil {
		return fmt.Errorf("checksum computation failed: %w", err)
	}
	err = b.cfg.Fs.WriteFile(fmt.Sprintf("%s.sha256", outputFile), []byte(fmt.Sprintf("%s %s\n", checksum, isoFileName)), 0644)
	if err != nil {
		return fmt.Errorf("cannot write checksum file: %w", err)
	}

	return nil
}

func (b BuildISOAction) applySources(target string, sources ...*v1.ImageSource) error {
	for _, src := range sources {
		_, err := b.e.DumpSource(target, src)
		if err != nil {
			return err
		}
	}
	return nil
}

func (g *BuildISOAction) PrepareEFI(rootDir, uefiDir string) error {
	err := utils.MkdirAll(g.cfg.Fs, filepath.Join(uefiDir, constants.EfiBootPath), constants.DirPerm)
	if err != nil {
		return err
	}

	switch g.cfg.Arch {
	case constants.ArchAmd64, constants.Archx86:
		err = utils.CopyFile(
			g.cfg.Fs,
			filepath.Join(rootDir, constants.GrubEfiImagex86),
			filepath.Join(uefiDir, constants.GrubEfiImagex86Dest),
		)
	case constants.ArchArm64:
		err = utils.CopyFile(
			g.cfg.Fs,
			filepath.Join(rootDir, constants.GrubEfiImageArm64),
			filepath.Join(uefiDir, constants.GrubEfiImageArm64Dest),
		)
	default:
		err = fmt.Errorf("Not supported architecture: %v", g.cfg.Arch)
	}
	if err != nil {
		return err
	}

	return g.cfg.Fs.WriteFile(filepath.Join(uefiDir, constants.EfiBootPath, constants.GrubCfg), []byte(constants.GrubEfiCfg), constants.FilePerm)
}

func (g *BuildISOAction) PrepareISO(rootDir, imageDir string) error {

	err := utils.MkdirAll(g.cfg.Fs, filepath.Join(imageDir, constants.GrubPrefixDir), constants.DirPerm)
	if err != nil {
		return err
	}

	switch g.cfg.Arch {
	case constants.ArchAmd64, constants.Archx86:
		// Create eltorito image
		eltorito, err := g.BuildEltoritoImg(rootDir)
		if err != nil {
			return err
		}

		// Inlude loaders in expected paths
		loaderDir := filepath.Join(imageDir, constants.IsoLoaderPath)
		err = utils.MkdirAll(g.cfg.Fs, loaderDir, constants.DirPerm)
		if err != nil {
			return err
		}
		loaderFiles := []string{eltorito, constants.GrubBootHybridImg}
		loaderFiles = append(loaderFiles, strings.Split(constants.SyslinuxFiles, " ")...)
		for _, f := range loaderFiles {
			err = utils.CopyFile(g.cfg.Fs, filepath.Join(rootDir, f), loaderDir)
			if err != nil {
				return err
			}
		}
		fontsDir := filepath.Join(loaderDir, "/grub2/fonts")
		err = utils.MkdirAll(g.cfg.Fs, fontsDir, constants.DirPerm)
		if err != nil {
			return err
		}
		err = utils.CopyFile(g.cfg.Fs, filepath.Join(rootDir, constants.GrubFont), fontsDir)
		if err != nil {
			return err
		}
	case constants.ArchArm64:
		// TBC
	default:
		return fmt.Errorf("Not supported architecture: %v", g.cfg.Arch)
	}

	// Write grub.cfg file
	err = g.cfg.Fs.WriteFile(
		filepath.Join(imageDir, constants.GrubPrefixDir, constants.GrubCfg),
		[]byte(fmt.Sprintf(constants.GrubCfgTemplate, g.spec.GrubEntry, g.spec.Label)),
		constants.FilePerm,
	)
	if err != nil {
		return err
	}

	// Include EFI contents in iso root too
	return g.PrepareEFI(rootDir, imageDir)
}

func (g *BuildISOAction) BuildEltoritoImg(rootDir string) (string, error) {
	var args []string
	args = append(args, "-O", constants.GrubBiosTarget)
	args = append(args, "-o", constants.GrubBiosImg)
	args = append(args, "-p", constants.GrubPrefixDir)
	args = append(args, "-d", constants.GrubI386BinDir)
	args = append(args, strings.Split(constants.GrubModules, " ")...)

	chRoot := utils.NewChroot(rootDir, &g.cfg.Config)
	out, err := chRoot.Run("grub2-mkimage", args...)
	if err != nil {
		g.cfg.Logger.Errorf("grub2-mkimage failed: %s", string(out))
		g.cfg.Logger.Errorf("Error: %v", err)
		return "", err
	}

	concatFiles := func() error {
		return utils.ConcatFiles(
			g.cfg.Fs, []string{constants.GrubBiosCDBoot, constants.GrubBiosImg},
			constants.GrubEltoritoImg,
		)
	}
	err = chRoot.RunCallback(concatFiles)
	if err != nil {
		return "", err
	}
	return constants.GrubEltoritoImg, nil
}
