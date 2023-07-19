package config

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"reflect"
	"runtime"
	"strings"

	"github.com/kairos-io/enki/internal/version"
	"github.com/kairos-io/enki/pkg/constants"
	"github.com/kairos-io/enki/pkg/utils"
	"github.com/kairos-io/kairos-agent/v2/pkg/cloudinit"
	"github.com/kairos-io/kairos-agent/v2/pkg/http"
	v1 "github.com/kairos-io/kairos-agent/v2/pkg/types/v1"
	"github.com/mitchellh/mapstructure"
	"github.com/sanity-io/litter"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/twpayne/go-vfs"
	"k8s.io/mount-utils"
)

var decodeHook = viper.DecodeHook(
	mapstructure.ComposeDecodeHookFunc(
		UnmarshalerHook(),
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
	),
)

func WithFs(fs v1.FS) func(r *v1.Config) error {
	return func(r *v1.Config) error {
		r.Fs = fs
		return nil
	}
}

func WithLogger(logger v1.Logger) func(r *v1.Config) error {
	return func(r *v1.Config) error {
		r.Logger = logger
		return nil
	}
}

func WithSyscall(syscall v1.SyscallInterface) func(r *v1.Config) error {
	return func(r *v1.Config) error {
		r.Syscall = syscall
		return nil
	}
}

func WithMounter(mounter mount.Interface) func(r *v1.Config) error {
	return func(r *v1.Config) error {
		r.Mounter = mounter
		return nil
	}
}

func WithRunner(runner v1.Runner) func(r *v1.Config) error {
	return func(r *v1.Config) error {
		r.Runner = runner
		return nil
	}
}

func WithClient(client v1.HTTPClient) func(r *v1.Config) error {
	return func(r *v1.Config) error {
		r.Client = client
		return nil
	}
}

func WithCloudInitRunner(ci v1.CloudInitRunner) func(r *v1.Config) error {
	return func(r *v1.Config) error {
		r.CloudInitRunner = ci
		return nil
	}
}

func WithArch(arch string) func(r *v1.Config) error {
	return func(r *v1.Config) error {
		r.Arch = arch
		return nil
	}
}

func WithImageExtractor(extractor v1.ImageExtractor) func(r *v1.Config) error {
	return func(r *v1.Config) error {
		r.ImageExtractor = extractor
		return nil
	}
}

type GenericOptions func(a *v1.Config) error

func ReadConfigBuild(configDir string, flags *pflag.FlagSet, mounter mount.Interface) (*v1.BuildConfig, error) {
	logger := v1.NewLogger()
	if configDir == "" {
		configDir = "."
	}

	cfg := NewBuildConfig(
		WithLogger(logger),
		WithMounter(mounter),
	)

	configLogger(cfg.Logger, cfg.Fs)

	viper.AddConfigPath(configDir)
	viper.SetConfigType("yaml")
	viper.SetConfigName("manifest.yaml")
	// If a config file is found, read it in.
	_ = viper.MergeInConfig()

	// Bind buildconfig flags
	bindGivenFlags(viper.GetViper(), flags)
	// merge environment variables on top for rootCmd
	viperReadEnv(viper.GetViper(), "BUILD", constants.GetBuildKeyEnvMap())

	// unmarshal all the vars into the config object
	err := viper.Unmarshal(cfg, setDecoder, decodeHook)
	if err != nil {
		cfg.Logger.Warnf("error unmarshalling config: %s", err)
	}

	err = cfg.Sanitize()
	cfg.Logger.Debugf("Full config loaded: %s", litter.Sdump(cfg))
	return cfg, err
}

func ReadBuildISO(b *v1.BuildConfig, flags *pflag.FlagSet) (*v1.LiveISO, error) {
	iso := NewISO()
	vp := viper.Sub("iso")
	if vp == nil {
		vp = viper.New()
	}
	// Bind build-iso cmd flags
	bindGivenFlags(vp, flags)
	// Bind build-iso env vars
	viperReadEnv(vp, "ISO", constants.GetISOKeyEnvMap())

	err := vp.Unmarshal(iso, setDecoder, decodeHook)
	if err != nil {
		b.Logger.Warnf("error unmarshalling LiveISO: %s", err)
	}
	err = iso.Sanitize()
	b.Logger.Debugf("Loaded LiveISO: %s", litter.Sdump(iso))
	return iso, err
}

func NewISO() *v1.LiveISO {
	return &v1.LiveISO{
		Label:     constants.ISOLabel,
		GrubEntry: constants.GrubDefEntry,
		UEFI:      []*v1.ImageSource{},
		Image:     []*v1.ImageSource{},
	}
}

func NewBuildConfig(opts ...GenericOptions) *v1.BuildConfig {
	b := &v1.BuildConfig{
		Config: *NewConfig(opts...),
		Name:   constants.BuildImgName,
	}
	if len(b.Repos) == 0 {
		repo := constants.LuetDefaultRepoURI
		if b.Arch != constants.Archx86 {
			repo = fmt.Sprintf("%s-%s", constants.LuetDefaultRepoURI, b.Arch)
		}
		b.Repos = []v1.Repository{{
			Name:     "cos",
			Type:     "docker",
			URI:      repo,
			Arch:     b.Arch,
			Priority: constants.LuetDefaultRepoPrio,
		}}
	}
	return b
}

func NewConfig(opts ...GenericOptions) *v1.Config {
	log := v1.NewLogger()
	arch, err := utils.GolangArchToArch(runtime.GOARCH)
	if err != nil {
		log.Errorf("invalid arch: %s", err.Error())
		return nil
	}

	c := &v1.Config{
		Fs:                    vfs.OSFS,
		Logger:                log,
		Syscall:               &v1.RealSyscall{},
		Client:                http.NewClient(),
		Repos:                 []v1.Repository{},
		Arch:                  arch,
		SquashFsNoCompression: true,
	}
	for _, o := range opts {
		err := o(c)
		if err != nil {
			log.Errorf("error applying config option: %s", err.Error())
			return nil
		}
	}

	// delay runner creation after we have run over the options in case we use WithRunner
	if c.Runner == nil {
		c.Runner = &v1.RealRunner{Logger: c.Logger}
	}

	// Now check if the runner has a logger inside, otherwise point our logger into it
	// This can happen if we set the WithRunner option as that doesn't set a logger
	if c.Runner.GetLogger() == nil {
		c.Runner.SetLogger(c.Logger)
	}

	// Delay the yip runner creation, so we set the proper logger instead of blindly setting it to the logger we create
	// at the start of NewRunConfig, as WithLogger can be passed on init, and that would result in 2 different logger
	// instances, on the config.Logger and the other on config.CloudInitRunner
	if c.CloudInitRunner == nil {
		c.CloudInitRunner = cloudinit.NewYipCloudInitRunner(c.Logger, c.Runner, vfs.OSFS)
	}

	if c.Mounter == nil {
		c.Mounter = mount.New(constants.MountBinary)
	}

	return c
}

func configLogger(log v1.Logger, vfs v1.FS) {
	// Set debug level
	if viper.GetBool("debug") {
		log.SetLevel(v1.DebugLevel())
	}

	// Set formatter so both file and stdout format are equal
	log.SetFormatter(&logrus.TextFormatter{
		ForceColors:      true,
		DisableColors:    false,
		DisableTimestamp: false,
		FullTimestamp:    true,
	})

	// Logfile
	logfile := viper.GetString("logfile")
	if logfile != "" {
		o, err := vfs.OpenFile(logfile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, fs.ModePerm)

		if err != nil {
			log.Errorf("Could not open %s for logging to file: %s", logfile, err.Error())
		}

		// else set it to both stdout and the file
		mw := io.MultiWriter(os.Stdout, o)
		log.SetOutput(mw)
	} else { // no logfile
		if viper.GetBool("quiet") { // quiet is enabled so discard all logging
			log.SetOutput(io.Discard)
		} else { // default to stdout
			log.SetOutput(os.Stdout)
		}
	}

	v := version.Get()
	if log.GetLevel() == logrus.DebugLevel {
		log.Debugf("Starting enki version %s on commit %s", v.Version, v.GitCommit)
	} else {
		log.Infof("Starting enki version %s", v.Version)
	}
}

// BindGivenFlags binds to viper only passed flags, ignoring any non provided flag
func bindGivenFlags(vp *viper.Viper, flagSet *pflag.FlagSet) {
	if flagSet != nil {
		flagSet.VisitAll(func(f *pflag.Flag) {
			if f.Changed {
				_ = vp.BindPFlag(f.Name, f)
			}
		})
	}
}

// setDecoder sets ZeroFields mastructure attribute to true
func setDecoder(config *mapstructure.DecoderConfig) {
	// Make sure we zero fields before applying them, this is relevant for slices
	// so we do not merge with any already present value and directly apply whatever
	// we got form configs.
	config.ZeroFields = true
}

type Unmarshaler interface {
	CustomUnmarshal(interface{}) (bool, error)
}

func UnmarshalerHook() mapstructure.DecodeHookFunc {
	return func(from reflect.Value, to reflect.Value) (interface{}, error) {
		// get the destination object address if it is not passed by reference
		if to.CanAddr() {
			to = to.Addr()
		}
		// If the destination implements the unmarshaling interface
		u, ok := to.Interface().(Unmarshaler)
		if !ok {
			return from.Interface(), nil
		}
		// If it is nil and a pointer, create and assign the target value first
		if to.IsNil() && to.Type().Kind() == reflect.Ptr {
			to.Set(reflect.New(to.Type().Elem()))
			u = to.Interface().(Unmarshaler)
		}
		// Call the custom unmarshaling method
		cont, err := u.CustomUnmarshal(from.Interface())
		if cont {
			// Continue with the decoding stack
			return from.Interface(), err
		}
		// Decoding finalized
		return to.Interface(), err
	}
}

func viperReadEnv(vp *viper.Viper, prefix string, keyMap map[string]string) {
	// If we expect to override complex keys in the config, i.e. configs
	// that are nested, we probably need to manually do the env stuff
	// ourselves, as this will only match keys in the config root
	replacer := strings.NewReplacer("-", "_")
	vp.SetEnvKeyReplacer(replacer)

	if prefix == "" {
		prefix = "ELEMENTAL"
	} else {
		prefix = fmt.Sprintf("ELEMENTAL_%s", prefix)
	}

	// Manually bind keys to env variable if custom names are needed.
	for k, v := range keyMap {
		_ = vp.BindEnv(k, fmt.Sprintf("%s_%s", prefix, v))
	}
}
