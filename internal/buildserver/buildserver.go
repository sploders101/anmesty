package buildserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"sync"
	"syscall"

	ipcv1 "github.com/sploders101/amnesty/internal/gen/proto/com/shaunkeys/amnesty/ipc/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type BuildServer struct {
	GrpcConn *grpc.ClientConn
	Client   ipcv1.BuilderServiceClient
	Cmd      *exec.Cmd
}

// Re-execs the current executable in a new container and sets up a gRPC server
func StartBuildServer(ctx context.Context) (BuildServer, error) {
	executable, err := os.Executable()
	if err != nil {
		return BuildServer{}, err
	}

	// Create socketpair for IPC
	spair, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return BuildServer{}, err
	}
	oursockFd := spair[0]
	theirsockFd := spair[1]
	oursockFile := os.NewFile(uintptr(oursockFd), "parent-socket")
	theirsockFile := os.NewFile(uintptr(theirsockFd), "child-socket")
	defer func() {
		if err := theirsockFile.Close(); err != nil {
			slog.Error("Failed to close theirsock while forking buildserver", "error", err)
		}
	}()
	oursock, err := net.FileConn(oursockFile)
	closeErr := oursockFile.Close()
	if err != nil {
		if closeErr != nil {
			slog.Error("Failed to close oursock while forking buildserver", "error", closeErr)
		}
		return BuildServer{}, err
	}
	if closeErr != nil {
		return BuildServer{}, fmt.Errorf("failed to close oursockFile: %w", closeErr)
	}

	// Start build server
	cmd := exec.CommandContext(ctx, executable)
	cmd.Env = append(os.Environ(), "AMNESTY_IPC_FD=3")
	cmd.ExtraFiles = []*os.File{theirsockFile}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWNS |
			syscall.CLONE_NEWCGROUP |
			syscall.CLONE_NEWPID |
			syscall.CLONE_NEWIPC |
			syscall.CLONE_NEWUTS,
	}
	if err := cmd.Start(); err != nil {
		if err := oursock.Close(); err != nil {
			slog.Error("Failed to close oursock during error handling", "error", err)
		}
		return BuildServer{}, err
	}
	go func() {
		if err := cmd.Wait(); err != nil {
			slog.Error("Build server exited with error", "error", err)
		}
	}()

	// Wrap socket in gRPC client
	var mu sync.Mutex
	ipcDialer := func(ctx context.Context, addr string) (net.Conn, error) {
		mu.Lock()
		defer mu.Unlock()
		if oursock == nil {
			return nil, errors.New("socketpair connection already consumed")
		}
		c := oursock
		oursock = nil
		return c, nil
	}

	cc, err := grpc.NewClient(
		"passthrough:///socketpair",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(ipcDialer),
	)
	if err != nil {
		mu.Lock()
		defer mu.Unlock()
		if oursock != nil {
			if err := oursock.Close(); err != nil {
				slog.Error("Failed to close oursock during gRPC error handling", "error", err)
			}
		}
		return BuildServer{}, fmt.Errorf("failed to construct gRPC client for IPC: %w", err)
	}

	return BuildServer{
		GrpcConn: cc,
		Client:   ipcv1.NewBuilderServiceClient(cc),
		Cmd:      cmd,
	}, nil
}
