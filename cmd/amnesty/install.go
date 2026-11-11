package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"charm.land/huh/v2"
	"github.com/sploders101/amnesty/internal/blueprint"
	"github.com/sploders101/amnesty/internal/install"
	"github.com/urfave/cli/v3"
)

func installCli() *cli.Command {
	var sysconfig string
	hostname, err := os.ReadFile("/etc/hostname")
	if err == nil {
		sysconfig = strings.TrimSpace(string(hostname))
	}
	modules := "/etc/amnesty/modules"

	return &cli.Command{
		Name:  "install",
		Usage: "Installs an Arch sysroot in the specified directory",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "sysconfig",
				Usage: "The SystemConfig profile to use for the installation",
				Value: sysconfig,
			},
			&cli.StringFlag{
				Name:  "modules",
				Usage: "Path to the modules file or directory",
				Value: modules,
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:      "rootfs",
				Required:  true,
				UsageText: "<rootfs>",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			sysconfig := cmd.String("sysconfig")
			modules := cmd.String("modules")
			rootfs := cmd.StringArg("rootfs")

			systemplate, err := blueprint.LoadSystemTemplate(ctx, modules)
			if err != nil {
				return err
			}

			blueprint, err := blueprint.AssembleBlueprint(systemplate, sysconfig)
			if err != nil {
				return err
			}

			var password string
			if err := huh.NewInput().
				EchoMode(huh.EchoModePassword).
				Title(fmt.Sprintf("Password for user %q", blueprint.Username)).
				Value(&password).
				Run(); err != nil {
				return fmt.Errorf("failed to get password: %w", err)
			}

			if err := install.InstallBlueprint(
				ctx,
				blueprint,
				rootfs,
				password,
			); err != nil {
				return err
			}

			return nil
		},
	}
}
