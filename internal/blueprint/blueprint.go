package blueprint

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/sploders101/amnesty/internal/config"
)

type Blueprint struct {
	Name            string
	Username        string
	Locale          string
	Timezone        string
	Packages        map[string]struct{}
	AURPackages     map[string]struct{}
	FlatpakPackages map[string]struct{}
	FlatpakRemotes  map[string]FlatpakRemote
	SystemServices  []string
	UserServices    []string
	Polkit          []PolkitEntry
	InstallFiles    []InstallFile
	AdminGroups     []string
	UserGroups      []string
	Postinstall     []Postinstall
}

type FlatpakRemote struct {
	Name string
	Url  string
}

type PolkitEntry struct {
	Id     string `json:"id"`
	Result string `json:"result"`
}

type InstallFile struct {
	Path     string
	Mode     os.FileMode
	Contents string
}

type Postinstall struct {
	Runas string
	Cmd   string
}

func NewBlueprint() *Blueprint {
	return &Blueprint{
		Packages:        make(map[string]struct{}),
		AURPackages:     make(map[string]struct{}),
		FlatpakPackages: make(map[string]struct{}),
		FlatpakRemotes:  make(map[string]FlatpakRemote),
	}
}

func (bp *Blueprint) AddModule(templateVars map[string]any, module config.SystemModule) error {
	// Packages
	for _, archPkgName := range module.Packages {
		pkgName, err := archPkgName.Render(templateVars)
		if err != nil {
			return fmt.Errorf("failed to render arch package: %w", err)
		}
		bp.Packages[pkgName] = struct{}{}
	}
	for _, aurPkgName := range module.AURPackages {
		pkgName, err := aurPkgName.Render(templateVars)
		if err != nil {
			return fmt.Errorf("failed to render aur package: %w", err)
		}
		bp.AURPackages[pkgName] = struct{}{}
	}
	for _, flatpakPkgName := range module.FlatpakPackages {
		pkgName, err := flatpakPkgName.Render(templateVars)
		if err != nil {
			return fmt.Errorf("failed to render flatpak package: %w", err)
		}
		bp.FlatpakPackages[pkgName] = struct{}{}
	}

	// Remotes
	for _, remote := range module.FlatpakRemotes {
		url, err := remote.Url.Render(templateVars)
		if err != nil {
			return fmt.Errorf("failed to render flatpak url: %w", err)
		}
		bp.FlatpakRemotes[remote.Name] = FlatpakRemote{
			Name: remote.Name,
			Url:  url,
		}
	}

	// Services
	for _, service := range module.Services.System {
		serviceName, err := service.Render(templateVars)
		if err != nil {
			return fmt.Errorf("failed to render system service name: %w", err)
		}
		bp.SystemServices = append(bp.SystemServices, serviceName)
	}
	for _, service := range module.Services.User {
		serviceName, err := service.Render(templateVars)
		if err != nil {
			return fmt.Errorf("failed to render user service name: %w", err)
		}
		bp.UserServices = append(bp.UserServices, serviceName)
	}

	// Polkit
	for _, entry := range module.Polkit {
		result, err := entry.Result.Render(templateVars)
		if err != nil {
			return fmt.Errorf("failed to render polkit result: %w", err)
		}
		bp.Polkit = append(bp.Polkit, PolkitEntry{
			Id:     entry.Id,
			Result: result,
		})
	}

	// InstallFiles
	for _, entry := range module.InstallFiles {
		path, err := entry.Path.Render(templateVars)
		if err != nil {
			return fmt.Errorf("failed to render install_files path: %w", err)
		}
		contents, err := entry.Contents.Render(templateVars)
		if err != nil {
			return fmt.Errorf("failed to render install_files contents: %w", err)
		}
		bp.InstallFiles = append(bp.InstallFiles, InstallFile{
			Path:     path,
			Mode:     entry.Mode,
			Contents: contents,
		})
	}

	// AdminGroups
	for _, entry := range module.AdminGroups {
		group, err := entry.Render(templateVars)
		if err != nil {
			return fmt.Errorf("failed to render admin group: %w", err)
		}
		bp.AdminGroups = append(bp.AdminGroups, group)
	}

	// UserGroups
	for _, entry := range module.UserGroups {
		group, err := entry.Render(templateVars)
		if err != nil {
			return fmt.Errorf("failed to render admin group: %w", err)
		}
		bp.UserGroups = append(bp.UserGroups, group)
	}

	// Postinstall
	for _, entry := range module.Postinstall {
		runas, err := entry.Runas.Render(templateVars)
		if err != nil {
			return fmt.Errorf("failed to render postinstall runas: %w", err)
		}
		cmd, err := entry.Cmd.Render(templateVars)
		if err != nil {
			return fmt.Errorf("failed to render postinstall cmd: %w", err)
		}
		bp.Postinstall = append(bp.Postinstall, Postinstall{
			Runas: runas,
			Cmd:   cmd,
		})
	}

	return nil
}

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

func AssembleBlueprint(systemplate *config.SystemTemplate, sysconfig string) (*Blueprint, error) {
	config, ok := systemplate.Configs[sysconfig]
	if !ok {
		return nil, fmt.Errorf("unknown sysconfig %q", sysconfig)
	}

	bp := NewBlueprint()
	bp.Name = config.Name
	bp.Username = config.Username
	bp.Locale = config.Locale
	bp.Timezone = config.Timezone

	templateVars := make(map[string]any)
	templateVars["name"] = bp.Name
	templateVars["hostname"] = bp.Name
	templateVars["username"] = bp.Username
	templateVars["locale"] = bp.Locale
	templateVars["timezone"] = bp.Timezone
	for _, moduleName := range config.Modules {
		module, ok := systemplate.Modules[moduleName]
		if !ok {
			return nil, fmt.Errorf("unknown module %q", moduleName)
		}
		if err := bp.AddModule(templateVars, module); err != nil {
			return nil, fmt.Errorf("failed to register module %q: %w", moduleName, err)
		}
	}

	return bp, nil
}
