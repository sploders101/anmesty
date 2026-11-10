package config

import (
	"fmt"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

type TemplatedString struct {
	IsTemplate bool
	Value      string
}

func (tstring *TemplatedString) UnmarshalYAML(value *yaml.Node) error {
	tstring.Value = value.Value
	tstring.IsTemplate = value.ShortTag() == "!template"
	return nil
}

func (tstring TemplatedString) Render(data map[string]any) (string, error) {
	if tstring.IsTemplate {
		tpl, err := template.New("<inline>").Parse(tstring.Value)
		if err != nil {
			return "", fmt.Errorf("failed to parse template: %w", err)
		}
		var str strings.Builder
		if err := tpl.Execute(&str, data); err != nil {
			return "", err
		}
		return str.String(), nil
	}
	return tstring.Value, nil
}
