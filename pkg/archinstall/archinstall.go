//go:build linux

package archinstall

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sploders101/amnesty/internal/blueprint"
	"github.com/sploders101/amnesty/internal/buildserver"
	ipcv1 "github.com/sploders101/amnesty/internal/gen/proto/com/shaunkeys/amnesty/ipc/v1"
)

func Pacstrap(ctx context.Context, destdir string, packages []string) error {
	args := append([]string{"-cK", destdir, "--noconfirm"}, packages...)
	cmd := exec.CommandContext(ctx, "pacstrap", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

// Reads locale.gen and uncomments locales that match the requested value
func EnableLocales(locgenPath string, chosenLocale string) error {
	locgenFile, err := os.OpenFile(locgenPath, os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open locale.gen: %w", err)
	}
	defer func() {
		if err := locgenFile.Close(); err != nil {
			slog.Error("Failed to close locale.gen", "error", err)
		}
	}()
	oldLocale, err := io.ReadAll(locgenFile)
	if err != nil {
		return fmt.Errorf("failed to read locale.gen: %w", err)
	}
	if _, err := locgenFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek locale.gen: %w", err)
	}
	var foundLocale bool
	for line := range strings.Lines(string(oldLocale)) {
		if strings.HasPrefix(line, "#"+chosenLocale+" ") {
			foundLocale = true
			if _, err := locgenFile.Write([]byte(line[1:])); err != nil {
				return fmt.Errorf("failed to write new locale.gen: %w", err)
			}
		} else {
			if strings.HasPrefix(line, chosenLocale+" ") {
				foundLocale = true
			}
			if _, err := locgenFile.Write([]byte(line)); err != nil {
				return fmt.Errorf("failed to write new locale.gen: %w", err)
			}
		}
	}
	if !foundLocale {
		return errors.New("locale not found")
	}
	return nil
}

// TODO: Not finished
func InstallAurPackages(rootfsPath string, aurPackages []string) error {
	// Set up nopasswd sudo for primary user to install AUR packages
	// TODO: Once we're plumbed into the COW semantics of btrfs, we could fork the system to build AUR packages,
	// thus eliminating the need to do this in the user's actual system.
	sudoerNopassPath := filepath.Join(rootfsPath, "etc/sudoers.d/system_setup.conf")
	if err := os.WriteFile(
		sudoerNopassPath,
		[]byte("%sudo ALL=(ALL:ALL) NOPASSWD: ALL\n"),
		0o700,
	); err != nil {
		return fmt.Errorf("failed to set up nopasswd sudo for user: %w", err)
	}
	// TODO: Do AUR installs here.
	if err := os.Remove(sudoerNopassPath); err != nil {
		return fmt.Errorf("failed to clean up nopasswd sudo for user: %w", err)
	}
	return nil
}

func SetupLocales(
	ctx context.Context,
	rootfsPath string,
	blueprint *blueprint.Blueprint,
	container buildserver.BuildServer,
) error {
	if err := EnableLocales(
		filepath.Join(rootfsPath, "etc/locale.gen"),
		blueprint.Locale,
	); err != nil {
		return fmt.Errorf("failed to write locale.gen: %w", err)
	}
	locgenResp, err := container.Client.RunCommand(ctx, ipcv1.RunCommandRequest_builder{
		Command: "locale-gen",
	}.Build())
	if err != nil {
		return fmt.Errorf("failed to generate locales: %w", err)
	}
	if locgenResp.GetExitCode() != 0 {
		return fmt.Errorf("locale-gen failed: %v", locgenResp.GetOutput())
	}
	if err := os.WriteFile(
		filepath.Join(rootfsPath, "etc/locale.conf"),
		[]byte("LANG=\""+blueprint.Locale+"\"\n"),
		0o644,
	); err != nil {
		return fmt.Errorf("failed to configure locale: %w", err)
	}
	return nil
}

func SetupTimezone(rootfsPath string, blueprint *blueprint.Blueprint) error {
	localtimePath := filepath.Join(rootfsPath, "etc/localtime")
	if err := os.Remove(localtimePath); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("failed to remove existing /etc/localtime: %w", err)
	}
	if err := os.Symlink(
		filepath.Join("/usr/share/zoneinfo/", blueprint.Timezone),
		localtimePath,
	); err != nil {
		return fmt.Errorf("failed to link /etc/localtime: %w", err)
	}
	return nil
}
