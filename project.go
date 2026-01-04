package main

import (
	"fmt"
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
	fmt.Printf("%s\n", folder)
	fmt.Printf("%s\n", prj.Source)
	fmt.Printf("%s\n", prj.Destination)

	prj.Source = filepath.Join(folder, prj.Source)
	prj.Destination = filepath.Join(folder, prj.Destination)
	fmt.Printf("%s\n", prj.Source)
	fmt.Printf("%s\n", prj.Destination)

	return nil
}
