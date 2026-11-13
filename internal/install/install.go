package install

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sploders101/amnesty/internal/blueprint"
	"github.com/sploders101/amnesty/internal/buildserver"
	ipcv1 "github.com/sploders101/amnesty/internal/gen/proto/com/shaunkeys/amnesty/ipc/v1"
	"github.com/sploders101/amnesty/pkg/archinstall"
	"github.com/sploders101/amnesty/pkg/utils"
)

//go:embed polkitrules.rules
var polkitTemplate []byte

func InstallBlueprint(
	ctx context.Context,
	blueprint *blueprint.Blueprint,
	rootfsPath string,
	userPassword string,
) error {
	slog.Info("Installing Arch packages...")
	packages := utils.LinearizeStringSet(blueprint.Packages)
	if err := archinstall.Pacstrap(ctx, rootfsPath, packages); err != nil {
		return fmt.Errorf("failed to install packages into rootfs: %w", err)
	}

	slog.Info("Copying resolv.conf...")
	if err := copyFile(
		"/etc/resolv.conf",
		filepath.Join(rootfsPath, "/etc/resolv.conf"),
	); err != nil {
		return fmt.Errorf("failed to copy resolv.conf: %w", err)
	}

	// Set up timezone
	slog.Info("Setting up timezone...")
	if err := archinstall.SetupTimezone(rootfsPath, blueprint); err != nil {
		return err
	}

	slog.Info("Starting containerized build server...")
	container, err := buildserver.StartBuildServer(ctx)
	if err != nil {
		return err
	}
	if _, err := container.Client.Initialize(ctx, ipcv1.InitializeRequest_builder{
		Rootdir: rootfsPath,
	}.Build()); err != nil {
		return err
	}

	// Set up clock
	slog.Info("Syncing hwclock...")
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
	slog.Info("Adding administrator...")
	useraddResp, err := container.Client.RunCommand(ctx, ipcv1.RunCommandRequest_builder{
		Command: "useradd",
		Args: []string{
			"-m",
			"-G",
			strings.Join(slices.Concat(blueprint.AdminGroups, blueprint.UserGroups), ","),
			blueprint.Username,
		},
	}.Build())
	if err != nil {
		return fmt.Errorf("failed to create user %q: %w", blueprint.Username, err)
	}
	if useraddResp.GetExitCode() != 0 {
		return fmt.Errorf(
			"failed to create user %q: %v",
			blueprint.Username,
			useraddResp.GetOutput(),
		)
	}
	userpassResp, err := container.Client.RunCommand(ctx, ipcv1.RunCommandRequest_builder{
		Command: "bash",
		Args:    []string{"-c", "echo \"$USERNAME:$PASSWORD\" | chpasswd"},
		Env:     []string{"USERNAME=" + blueprint.Username, "PASSWORD=" + userPassword},
	}.Build())
	if err != nil {
		return fmt.Errorf("failed to change password for %q: %w", blueprint.Username, err)
	}
	if userpassResp.GetExitCode() != 0 {
		return fmt.Errorf(
			"failed to change password for %q: %v",
			blueprint.Username,
			userpassResp.GetOutput(),
		)
	}

	// Install AUR packages (unfinished)
	slog.Info("Installing AUR packages...")
	if err := archinstall.InstallAurPackages(
		ctx,
		container,
		blueprint.Username,
		rootfsPath,
		utils.LinearizeStringSet(blueprint.AURPackages),
	); err != nil {
		return err
	}

	// Add flatpak remotes
	for _, remote := range blueprint.FlatpakRemotes {
		slog.Info("Setting up flatpak remote", "remote", remote.Name)
		fpremoteResp, err := container.Client.RunCommand(ctx, ipcv1.RunCommandRequest_builder{
			Command: "flatpak",
			Args: []string{
				"remote-add",
				"--if-not-exists",
				remote.Name,
				remote.Url,
			},
		}.Build())
		if err != nil {
			return fmt.Errorf("failed to add flatpak remote %q: %w", remote.Name, err)
		}
		if fpremoteResp.GetExitCode() != 0 {
			return fmt.Errorf(
				"failed to add flatpak remote %q: %v",
				remote.Name,
				fpremoteResp.GetOutput(),
			)
		}
	}

	// Install flatpak packages
	slog.Info("Installing flatpak packages...")
	fpinstallResp, err := container.Client.RunCommand(ctx, ipcv1.RunCommandRequest_builder{
		Command: "flatpak",
		Args: append(
			[]string{"install", "-y"},
			utils.LinearizeStringSet(blueprint.FlatpakPackages)...),
	}.Build())
	if err != nil {
		return fmt.Errorf("failed to install flatpaks: %w", err)
	}
	if fpinstallResp.GetExitCode() != 0 {
		return fmt.Errorf("failed to install flatpaks: %v", fpinstallResp.GetOutput())
	}

	// Enable system services
	slog.Info("Enabling system services...")
	sysserviceResp, err := container.Client.RunCommand(ctx, ipcv1.RunCommandRequest_builder{
		Command: "systemctl",
		Args:    append([]string{"enable"}, blueprint.SystemServices...),
	}.Build())
	if err != nil {
		return fmt.Errorf("failed to enable services: %w", err)
	}
	if sysserviceResp.GetExitCode() != 0 {
		return fmt.Errorf("failed to enable services: %w", err)
	}

	// Enable user services (using --global)
	slog.Info("Enabling user services...")
	usrserviceResp, err := container.Client.RunCommand(ctx, ipcv1.RunCommandRequest_builder{
		Command: "systemctl",
		Args:    append([]string{"enable", "--global"}, blueprint.UserServices...),
	}.Build())
	if err != nil {
		return fmt.Errorf("failed to enable user services: %w", err)
	}
	if usrserviceResp.GetExitCode() != 0 {
		return fmt.Errorf("failed to enable user services: %v", usrserviceResp.GetOutput())
	}

	// Install polkit files
	polkitRules, err := json.Marshal(blueprint.Polkit)
	if err != nil {
		return fmt.Errorf("failed to marshal polkit rules: %w", err)
	}
	polkitRules = append(polkitRules, '\n', '\n')
	polkitRules = append(polkitRules, polkitTemplate...)
	if err := os.WriteFile(
		filepath.Join(rootfsPath, "/etc/polkit-1/rules.d/userrules.conf"),
		polkitRules,
		0o644,
	); err != nil {
		return fmt.Errorf("failed to write polkit rules: %w", err)
	}

	// Install user files
	for _, fileMeta := range blueprint.InstallFiles {
		slog.Info("Installing user file", "file", fileMeta.Path)
		if err := os.WriteFile(
			fileMeta.Path,
			[]byte(fileMeta.Contents),
			os.FileMode(fileMeta.Mode),
		); err != nil {
			return fmt.Errorf("failed to create %q: %w", fileMeta.Path, err)
		}
	}

	// Run postinstall scripts
	for i, postinstall := range blueprint.Postinstall {
		slog.Info("Running postinstalls...", "current", i, "total", len(blueprint.Postinstall))
		runas := postinstall.Runas
		runResp, err := container.Client.RunCommand(ctx, ipcv1.RunCommandRequest_builder{
			Command: "bash",
			Args:    []string{"-c", postinstall.Cmd},
			Runas:   &runas,
		}.Build())
		if err != nil {
			return fmt.Errorf("failed to run postinstall %q: %w", postinstall.Cmd, err)
		}
		if runResp.GetExitCode() != 0 {
			return fmt.Errorf(
				"failed to run postinstall %q: %v",
				postinstall.Cmd,
				runResp.GetOutput(),
			)
		}
	}

	// Need to figure out:
	// * Disk management
	// * Filesystem creation
	// * Btrfs subvolume layout
	// * LVM
	// * UKI & synchronization between kernel & rootfs
	// * Boot management

	return nil
}

func copyFile(srcPath string, dstPath string) error {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open %q: %w", dstPath, err)
	}
	dstFile, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("failed to open %q: %w", dstPath, err)
	}
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}
	return nil
}
