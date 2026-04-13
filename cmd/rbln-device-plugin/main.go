package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/urfave/cli/v2"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"

	"github.com/RBLN-SW/rbln-k8s-device-plugin/pkg/flags"
)

var version = "dev"

type Flags struct {
	loggingConfig *flags.LoggingConfig

	cdiRoot                 string
	kubeletDevicePluginPath string
	healthcheckPort         int
	useGenericResourceName  bool
	deviceScanInterval      time.Duration
}

type Config struct {
	flags *Flags
}

func (c Config) KubeletSocketPath() string {
	return filepath.Join(c.flags.kubeletDevicePluginPath, filepath.Base(pluginapi.KubeletSocket))
}

func main() {
	if err := newApp().Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func newApp() *cli.App {
	flags := &Flags{
		loggingConfig: flags.NewLoggingConfig(),
	}

	cliFlags := []cli.Flag{
		&cli.StringFlag{
			Name:        "cdi-root",
			Usage:       "Absolute path to the directory where CDI files will be generated.",
			Value:       "/var/run/cdi",
			Destination: &flags.cdiRoot,
			EnvVars:     []string{"CDI_ROOT"},
		},
		&cli.StringFlag{
			Name:        "kubelet-device-plugin-path",
			Usage:       "Absolute path to the kubelet device-plugin directory.",
			Value:       pluginapi.DevicePluginPath,
			Destination: &flags.kubeletDevicePluginPath,
			EnvVars:     []string{"KUBELET_DEVICE_PLUGIN_PATH"},
		},
		&cli.IntFlag{
			Name:        "healthcheck-port",
			Usage:       "Port to start a gRPC healthcheck service. Use a negative value to disable it.",
			Value:       51515,
			Destination: &flags.healthcheckPort,
			EnvVars:     []string{"HEALTHCHECK_PORT"},
		},
		&cli.BoolFlag{
			Name:        "use-generic-resource-name",
			Usage:       "Expose devices as rebellions.ai/npu instead of legacy rebellions.ai/ATOM or rebellions.ai/REBEL resources.",
			Destination: &flags.useGenericResourceName,
			EnvVars:     []string{"USE_GENERIC_RESOURCE_NAME"},
		},
		&cli.DurationFlag{
			Name:        "device-scan-interval",
			Usage:       "Polling interval used to refresh the device inventory.",
			Value:       time.Minute,
			Destination: &flags.deviceScanInterval,
			EnvVars:     []string{"DEVICE_SCAN_INTERVAL"},
		},
	}
	cliFlags = append(cliFlags, flags.loggingConfig.Flags()...)

	app := &cli.App{
		Name:            "rbln-device-plugin",
		Version:         version,
		Usage:           "rbln-device-plugin exposes Rebellions NPUs through the Kubernetes device plugin API.",
		ArgsUsage:       " ",
		HideHelpCommand: true,
		Flags:           cliFlags,
		Before: func(c *cli.Context) error {
			if c.Args().Len() > 0 {
				return fmt.Errorf("arguments not supported: %v", c.Args().Slice())
			}
			return flags.loggingConfig.Apply()
		},
		Action: func(c *cli.Context) error {
			return Run(c.Context, &Config{flags: flags})
		},
	}

	return app
}

func Run(ctx context.Context, config *Config) error {
	if err := os.MkdirAll(config.flags.kubeletDevicePluginPath, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(config.flags.cdiRoot, 0o755); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer stop()

	manager, err := NewManager(ctx, config)
	if err != nil {
		return err
	}
	defer manager.Stop()

	if err := manager.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}
