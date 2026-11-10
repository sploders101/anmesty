package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
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
		"devtmpfs",
		filepath.Join(rootDir, "dev"),
		"devtmpfs",
		0,
		"",
	); err != nil {
		return nil, fmt.Errorf("failed to mount /dev: %w", err)
	}
	if err := syscall.Mount("proc", filepath.Join(rootDir, "proc"), "proc", 0, ""); err != nil {
		return nil, fmt.Errorf("failed to mount /proc: %w", err)
	}
	if err := syscall.Mount("tmpfs", filepath.Join(rootDir, "tmp"), "tmpfs", 0, ""); err != nil {
		return nil, fmt.Errorf("failed to mount /tmp: %w", err)
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
