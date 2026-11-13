package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"

	ipcv1 "github.com/sploders101/amnesty/internal/gen/proto/com/shaunkeys/amnesty/ipc/v1"
	"google.golang.org/grpc"
)

type connListener struct {
	mu     sync.Mutex
	conn   net.Conn
	closed chan struct{}
	once   sync.Once
}

func newConnListener(conn net.Conn) *connListener {
	return &connListener{
		conn:   conn,
		closed: make(chan struct{}),
	}
}

func (l *connListener) Accept() (net.Conn, error) {
	l.mu.Lock()
	if l.conn != nil {
		conn := l.conn
		l.conn = nil
		l.mu.Unlock()
		return conn, nil
	}
	l.mu.Unlock()

	<-l.closed
	return nil, net.ErrClosed
}

func (l *connListener) Close() error {
	l.once.Do(func() {
		close(l.closed)
	})

	l.mu.Lock()
	conn := l.conn
	l.conn = nil
	l.mu.Unlock()

	if conn != nil {
		return conn.Close()
	}
	return nil
}

func (l *connListener) Addr() net.Addr {
	return &net.UnixAddr{Name: "anonymous", Net: "unix"}
}

func runBuildService(ipcFd int) error {
	ipcFile := os.NewFile(uintptr(ipcFd), "grpc-server")
	ipcConn, err := net.FileConn(ipcFile)
	if err := ipcFile.Close(); err != nil {
		return err
	}
	if err != nil {
		return err
	}
	ipcListener := newConnListener(ipcConn)

	server := grpc.NewServer()
	ipcv1.RegisterBuilderServiceServer(server, buildService{})

	return server.Serve(ipcListener)
}

type buildService struct {
	// This is fine because this is an IPC channel. If the protos are mismatched, we've got bigger problems.
	ipcv1.UnsafeBuilderServiceServer
}

func (buildService) Initialize(
	ctx context.Context,
	req *ipcv1.InitializeRequest,
) (*ipcv1.InitializeResponse, error) {
	rootDir := req.GetRootdir()

	// Remount private
	if err := syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, ""); err != nil {
		return nil, fmt.Errorf("failed to isolate container mounts: %w", err)
	}

	// Bind-mount root
	if err := syscall.Mount(rootDir, rootDir, "", syscall.MS_BIND, ""); err != nil {
		return nil, fmt.Errorf("failed to bind-mount root to itself: %w", err)
	}

	// Mount API filesystems
	if err := syscall.Mount(
		"/dev",
		filepath.Join(rootDir, "dev"),
		"",
		syscall.MS_BIND|syscall.MS_REC,
		"",
	); err != nil {
		return nil, fmt.Errorf("failed to bind mount /dev: %w", err)
	}
	if err := syscall.Mount("proc", filepath.Join(rootDir, "proc"), "proc", 0, ""); err != nil {
		return nil, fmt.Errorf("failed to mount /proc: %w", err)
	}
	if err := syscall.Mount("tmpfs", filepath.Join(rootDir, "tmp"), "tmpfs", 0, ""); err != nil {
		return nil, fmt.Errorf("failed to mount /tmp: %w", err)
	}
	if err := os.Chmod(filepath.Join(rootDir, "tmp"), 0o777); err != nil {
		return nil, fmt.Errorf("failed to chmod /tmp: %w", err)
	}
	if err := syscall.Mount("sysfs", filepath.Join(rootDir, "sys"), "sysfs", 0, ""); err != nil {
		return nil, fmt.Errorf("failed to mount /sys: %w", err)
	}

	// Pivot root
	if err := syscall.Chdir(rootDir); err != nil {
		return nil, fmt.Errorf("failed to chdir to newroot: %w", err)
	}
	if err := syscall.PivotRoot(".", "."); err != nil {
		return nil, fmt.Errorf("failed to pivot root: %w", err)
	}
	if err := syscall.Chdir("/"); err != nil {
		return nil, fmt.Errorf("failed to chdir to / after pivot: %w", err)
	}
	if err := syscall.Unmount(".", syscall.MNT_DETACH); err != nil {
		return nil, fmt.Errorf("failed to unmount oldroot: %w", err)
	}
	return ipcv1.InitializeResponse_builder{}.Build(), nil
}

func (buildService) RunCommand(
	ctx context.Context,
	req *ipcv1.RunCommandRequest,
) (*ipcv1.RunCommandResponse, error) {
	cmd := exec.CommandContext(ctx, req.GetCommand(), req.GetArgs()...)
	cmd.Env = append(os.Environ(), req.GetEnv()...)
	if req.HasRunas() {
		cred, err := getCredential(req.GetRunas())
		if err != nil {
			return nil, err
		}
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Credential: cred,
		}
	}
	output, err := cmd.CombinedOutput()
	var exitCode int32
	if err != nil {
		if err, ok := errors.AsType[*exec.ExitError](err); ok {
			exitCode = int32(err.ExitCode())
		} else {
			return nil, fmt.Errorf("failed to execute command: %w", err)
		}
	}

	return ipcv1.RunCommandResponse_builder{
		ExitCode: exitCode,
		Output:   string(output),
	}.Build(), nil
}

func (buildService) InstallAur(
	ctx context.Context,
	req *ipcv1.InstallAurRequest,
) (*ipcv1.InstallAurResponse, error) {
	userCredential, err := getCredential(req.GetAsUser())
	if err != nil {
		return nil, fmt.Errorf("failed to get user credential: %w", err)
	}
	userEnv := buildUserEnv(req.GetAsUser())

	if err := os.Mkdir("/var/tmp/aurs", 0o777); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("failed to mkdir /var/tmp/aurs: %w", err)
		}
	} else {
		if err := os.Chown(
			"/var/tmp/aurs",
			int(userCredential.Uid),
			int(userCredential.Gid),
		); err != nil {
			return nil, fmt.Errorf("failed to chown dir: %w", err)
		}
	}

	pkgDir := filepath.Join("/var/tmp/aurs", req.GetPackage())
	defer func() {
		if err := os.RemoveAll(pkgDir); err != nil {
			slog.Error("Failed to clean up aur install", "package", req.GetPackage(), "error", err)
		}
	}()

	cloneCmd := exec.CommandContext(
		ctx,
		"git",
		"clone",
		fmt.Sprintf("https://aur.archlinux.org/%v.git", req.GetPackage()),
		req.GetPackage(),
	)
	cloneCmd.Env = userEnv
	cloneCmd.Dir = "/var/tmp/aurs"
	cloneCmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: userCredential,
	}
	cloneOutput, err := cloneCmd.CombinedOutput()
	if err != nil {
		if err, ok := errors.AsType[*exec.ExitError](err); ok {
			return nil, fmt.Errorf("failed to clone repository: %w: %v", err, string(cloneOutput))
		}
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}
	fmt.Println(string(cloneOutput))

	pkgCmd := exec.CommandContext(ctx, "makepkg", "-si", "--noconfirm")
	cloneCmd.Env = userEnv
	pkgCmd.Dir = pkgDir
	pkgCmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: userCredential,
	}
	pkgOutput, err := pkgCmd.CombinedOutput()
	if err != nil {
		if err, ok := errors.AsType[*exec.ExitError](err); ok {
			return nil, fmt.Errorf("failed to build package: %w: %v", err, string(pkgOutput))
		}
		return nil, fmt.Errorf("failed to build package: %w", err)
	}

	return ipcv1.InstallAurResponse_builder{}.Build(), nil
}

func getCredential(username string) (*syscall.Credential, error) {
	usr, err := user.Lookup(username)
	if err != nil {
		return nil, fmt.Errorf("failed to look up user %q: %w", username, err)
	}
	uid, err := strconv.ParseUint(usr.Uid, 10, 32)
	if err != nil {
		return nil, err
	}
	gid, err := strconv.ParseUint(usr.Gid, 10, 32)
	if err != nil {
		return nil, err
	}
	sgidStrings, err := usr.GroupIds()
	if err != nil {
		return nil, err
	}
	var sgids []uint32
	for _, sgidString := range sgidStrings {
		sgid, err := strconv.ParseUint(sgidString, 10, 32)
		if err != nil {
			return nil, err
		}
		if sgid != gid {
			sgids = append(sgids, uint32(sgid))
		}
	}
	return &syscall.Credential{
		Uid:    uint32(uid),
		Gid:    uint32(gid),
		Groups: sgids,
	}, nil
}

func buildUserEnv(username string) []string {
	home := filepath.Join("/home", username)
	return []string{
		"HOME=" + home,
		"USER=" + username,
		"LOGNAME=" + username,
		"PATH=/usr/local/bin:/usr/bin",
		"SHELL=/bin/bash",
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"XDG_DATA_HOME=" + filepath.Join(home, ".local/share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local/state"),
	}
}
