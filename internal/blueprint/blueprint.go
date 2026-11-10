package blueprint

import (
	"fmt"
	"maps"

	"github.com/sploders101/amnesty/internal/config"
	ipcv1 "github.com/sploders101/amnesty/internal/gen/proto/com/shaunkeys/amnesty/ipc/v1"
	"github.com/sploders101/amnesty/pkg/utils"
)

type Blueprint struct {
	Name            string
	Hostname        string
	Username        string
	Locale          string
	Timezone        string
	Packages        map[string]struct{}
	AURPackages     map[string]struct{}
	FlatpakPackages map[string]struct{}
	FlatpakRemotes  map[string]*ipcv1.FlatpakRemote
	SystemServices  []string
	UserServices    []string
	Polkit          []*ipcv1.PolkitEntry
	InstallFiles    []*ipcv1.InstallFile
	AdminGroups     []string
	UserGroups      []string
	Postinstall     []*ipcv1.Postinstall
}

func NewBlueprint() *Blueprint {
	return &Blueprint{
		Packages:        make(map[string]struct{}),
		AURPackages:     make(map[string]struct{}),
		FlatpakPackages: make(map[string]struct{}),
		FlatpakRemotes:  make(map[string]*ipcv1.FlatpakRemote),
	}
}

func (bp Blueprint) IntoProto() *ipcv1.Blueprint {
	return ipcv1.Blueprint_builder{
		Name:            bp.Name,
		Hostname:        bp.Hostname,
		Username:        bp.Username,
		Locale:          bp.Locale,
		Timezone:        bp.Timezone,
		Packages:        utils.LinearizeStringSet(bp.Packages),
		AurPackages:     utils.LinearizeStringSet(bp.AURPackages),
		FlatpakPackages: utils.LinearizeStringSet(bp.FlatpakPackages),
		FlatpakRemotes:  utils.LinearizeIter(maps.Values(bp.FlatpakRemotes)),
		SystemServices:  bp.SystemServices,
		UserServices:    bp.UserServices,
		Polkit:          bp.Polkit,
		InstallFiles:    bp.InstallFiles,
		AdminGroups:     bp.AdminGroups,
		UserGroups:      bp.UserGroups,
		Postinstall:     bp.Postinstall,
	}.Build()
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
		bp.FlatpakRemotes[remote.Name] = ipcv1.FlatpakRemote_builder{
			Name: remote.Name,
			Url:  url,
		}.Build()
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
		bp.Polkit = append(bp.Polkit, ipcv1.PolkitEntry_builder{
			Id:     entry.Id,
			Result: result,
		}.Build())
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
		bp.InstallFiles = append(bp.InstallFiles, ipcv1.InstallFile_builder{
			Path:     path,
			Mode:     entry.Mode,
			Contents: contents,
		}.Build())
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
		bp.Postinstall = append(bp.Postinstall, ipcv1.Postinstall_builder{
			Runas: runas,
			Cmd:   cmd,
		}.Build())
	}

	return nil
}

func AssembleBlueprint(systemplate *config.SystemTemplate, sysconfig string) (*Blueprint, error) {
	config, ok := systemplate.Configs[sysconfig]
	if !ok {
		return nil, fmt.Errorf("unknown sysconfig %q", sysconfig)
	}

	bp := NewBlueprint()
	bp.Name = config.Name
	bp.Hostname = config.Hostname
	bp.Username = config.Username
	bp.Locale = config.Locale
	bp.Timezone = config.Timezone

	templateVars := make(map[string]any)
	templateVars["name"] = bp.Name
	templateVars["hostname"] = bp.Hostname
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
