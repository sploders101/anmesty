package install

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/sploders101/amnesty/internal/blueprint"
	"github.com/sploders101/amnesty/internal/buildserver"
	"github.com/sploders101/amnesty/internal/config"
	ipcv1 "github.com/sploders101/amnesty/internal/gen/proto/com/shaunkeys/amnesty/ipc/v1"
	"github.com/sploders101/amnesty/pkg/archinstall"
	"github.com/sploders101/amnesty/pkg/utils"
)

func LoadSystemTemplate(ctx context.Context, modulesPath string) (*config.SystemTemplate, error) {
	systemplate := config.NewSystemTemplate()

	modules, err := os.Stat(modulesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat modules directory: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !modules.IsDir() {
		if err := systemplate.LoadFile(modulesPath); err != nil {
			return nil, err
		}
	} else {
		if err := filepath.WalkDir(
			modulesPath,
			func(path string, entry fs.DirEntry, err error) error {
				if err := ctx.Err(); err != nil {
					return err
				}
				if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".yaml") {
					return nil
				}
				return systemplate.LoadFile(path)
			},
		); err != nil {
			return nil, err
		}
	}
	return systemplate, nil
}

func InstallBlueprint(
	ctx context.Context,
	sysconfig string,
	modulesPath string,
	rootfsPath string,
) error {
	systemplate, err := LoadSystemTemplate(ctx, modulesPath)
	if err != nil {
		return err
	}

	blueprint, err := blueprint.AssembleBlueprint(systemplate, sysconfig)
	if err != nil {
		return err
	}

	packages := utils.LinearizeStringSet(blueprint.Packages)
	if err := archinstall.Pacstrap(ctx, rootfsPath, packages); err != nil {
		return fmt.Errorf("failed to install packages into rootfs: %w", err)
	}

	container, err := buildserver.StartBuildServer(ctx)
	if err != nil {
		return err
	}
	if _, err := container.Client.Initialize(ctx, ipcv1.InitializeRequest_builder{
		Rootdir: rootfsPath,
	}.Build()); err != nil {
		return err
	}

	output, err := container.Client.RunCommand(ctx, ipcv1.RunCommandRequest_builder{
		Command: "mount",
	}.Build())
	if err != nil {
		return err
	}
	fmt.Printf("Ls output:\n%v\n", output.GetOutput())

	return nil
}
