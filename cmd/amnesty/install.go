package main

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/adrg/xdg"
	"github.com/sploders101/amnesty/internal/install"
	"github.com/urfave/cli/v3"
)

func installCli() *cli.Command {
	sysconfig := "shaunkeys-pc"
	modulesPath := filepath.Join(xdg.ConfigHome, "amnesty", "modules")

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
				Value: modulesPath,
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

			if err := install.InstallBlueprint(ctx, sysconfig, modules, rootfs); err != nil {
				return err
			}

			return errors.New("success???")
		},
	}
}
