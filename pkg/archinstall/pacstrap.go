//go:build linux

package archinstall

import (
	"context"
	"os"
	"os/exec"
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
