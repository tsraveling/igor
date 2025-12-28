package main

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type project struct {
	Destination     string                `yaml:"destination"`
	Source          string                `yaml:"source"`
	SpritesheetSize int                   `yaml:"spritesheet-size"`
	SliceSize       int                   `yaml:"slice-size"`
	Rules           map[string]ruleConfig `yaml:"rules"`
}

var prj project

type ruleConfig struct {
	Mode string `yaml:"mode"`
}

func loadProject(folder string) error {
	data, err := os.ReadFile(filepath.Join(folder, "igor.yml"))
	if err != nil {
		return err
	}

	if err := yaml.Unmarshal(data, &prj); err != nil {
		return err
	}

	return nil
}
