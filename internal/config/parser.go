package config

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"gopkg.in/yaml.v3"
)

var ErrInvalidConfig error = errors.New("invalid config")

type parsedDoc struct {
	config *SystemConfig
	module *SystemModule
}

func (doc *parsedDoc) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return ErrInvalidConfig
	}

	if len(node.Content)%2 == 1 {
		return ErrInvalidConfig
	}
	for i := range len(node.Content) / 2 {
		key := node.Content[i*2]
		value := node.Content[i*2+1]

		if key.Kind != yaml.ScalarNode || key.Value != "kind" {
			continue
		}
		if value.Kind != yaml.ScalarNode {
			return ErrInvalidConfig
		}
		var err error
		switch value.Value {
		case "SystemConfig":
			doc.config = &SystemConfig{}
			err = node.Decode(doc.config)
		case "SystemModule":
			doc.module = &SystemModule{}
			err = node.Decode(doc.module)
		}
		if err != nil {
			return err
		}
		return nil
	}
	return ErrInvalidConfig
}

type SystemTemplate struct {
	Configs map[string]SystemConfig
	Modules map[string]SystemModule
}

func NewSystemTemplate() *SystemTemplate {
	return &SystemTemplate{
		Configs: make(map[string]SystemConfig),
		Modules: make(map[string]SystemModule),
	}
}

func (bp *SystemTemplate) LoadFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open config file %q: %w", path, err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Error("Failed to close file", "file", path, "error", err)
		}
	}()

	decoder := yaml.NewDecoder(file)
	for i := 0; true; i += 1 {
		var doc parsedDoc
		if err := decoder.Decode(&doc); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("failed to load file %q[%v]: %w", path, i, err)
		}

		switch {
		case doc.config != nil:
			bp.Configs[doc.config.Name] = *doc.config
		case doc.module != nil:
			bp.Modules[doc.module.Name] = *doc.module
		}
	}

	return nil
}
