package install

import (
	"context"
	"fmt"

	"github.com/sploders101/amnesty/internal/blueprint"
	"github.com/sploders101/amnesty/internal/buildserver"
	ipcv1 "github.com/sploders101/amnesty/internal/gen/proto/com/shaunkeys/amnesty/ipc/v1"
	"github.com/sploders101/amnesty/pkg/archinstall"
	"github.com/sploders101/amnesty/pkg/utils"
)

func InstallBlueprint(
	ctx context.Context,
	blueprint *blueprint.Blueprint,
	rootfsPath string,
	userPassword string,
) error {
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

	// Set up timezone
	if err := archinstall.SetupTimezone(rootfsPath, blueprint); err != nil {
		return err
	}

	// Set up clock
	hwclockResp, err := container.Client.RunCommand(ctx, ipcv1.RunCommandRequest_builder{
		Command: "hwclock",
		Args:    []string{"--systohc"},
	}.Build())
	if err != nil {
		return fmt.Errorf("failed to set hwclock: %w", err)
	}
	if hwclockResp.GetExitCode() != 0 {
		return fmt.Errorf("hwclock failed: %v", hwclockResp.GetOutput())
	}

	// Set up locales
	if err := archinstall.SetupLocales(ctx, rootfsPath, blueprint, container); err != nil {
		return err
	}

	// Set up user
	// Add user to admin groups
	// Add user to user groups

	// Install AUR packages (unfinished)
	if err := archinstall.InstallAurPackages(
		rootfsPath,
		utils.LinearizeStringSet(blueprint.AURPackages),
	); err != nil {
		return err
	}

	// Add flatpak remotes
	// Install flatpak packages
	// Enable system services
	// Enable user services (using --global)
	// Install polkit files
	// Install user files
	// Run postinstall scripts

	// Need to figure out:
	// * Disk management
	// * Filesystem creation
	// * Btrfs subvolume layout
	// * LVM
	// * UKI & synchronization between kernel & rootfs
	// * Boot management

	return nil
}
