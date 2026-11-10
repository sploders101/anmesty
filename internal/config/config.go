package config

type SystemConfig struct {
	Version  string   `yaml:"version"`
	Kind     string   `yaml:"kind"`
	Name     string   `yaml:"name"`
	Modules  []string `yaml:"modules"`
	Hostname string   `yaml:"hostname"`
	Username string   `yaml:"username"`
	Locale   string   `yaml:"locale"`
	Timezone string   `yaml:"timezone"`
}

type SystemModule struct {
	Version         string            `yaml:"version"`
	Kind            string            `yaml:"kind"`
	Name            string            `yaml:"name"`
	Description     string            `yaml:"description"`
	Packages        []TemplatedString `yaml:"packages"`
	AURPackages     []TemplatedString `yaml:"aur_packages"`
	FlatpakPackages []TemplatedString `yaml:"flatpak_packages"`
	FlatpakRemotes  []FlatpakRemote   `yaml:"flatpak_remotes"`
	Services        Services          `yaml:"services"`
	Polkit          []PolkitEntry     `yaml:"polkit"`
	InstallFiles    []InstallFile     `yaml:"install_files"`
	AdminGroups     []TemplatedString `yaml:"admin_groups"`
	UserGroups      []TemplatedString `yaml:"user_groups"`
	Postinstall     []Postinstall     `yaml:"postinstall"`
}

type FlatpakRemote struct {
	Name string          `yaml:"name"`
	Url  TemplatedString `yaml:"url"`
}

type Services struct {
	System []TemplatedString `yaml:"system"`
	User   []TemplatedString `yaml:"user"`
}

type PolkitEntry struct {
	Id     string          `yaml:"id"`
	Result TemplatedString `yaml:"result"`
}

type InstallFile struct {
	Path     TemplatedString `yaml:"path"`
	Mode     uint32          `yaml:"mode"`
	Contents TemplatedString `yaml:"contents"`
}

type Postinstall struct {
	Runas TemplatedString `yaml:"runas"`
	Cmd   TemplatedString `yaml:"cmd"`
}
