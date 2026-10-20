package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	"gopkg.in/yaml.v3"
)

func exportConfig(path string, output io.Writer) error {
	raw, errRead := os.ReadFile(path)
	if errRead != nil {
		return fmt.Errorf("read configuration: %w", errRead)
	}
	canonical, _, errNormalize := config.NormalizeConfigLayout(raw, true)
	if errNormalize != nil {
		return fmt.Errorf("normalize configuration: %w", errNormalize)
	}
	var document yaml.Node
	if errDecode := yaml.Unmarshal(canonical, &document); errDecode != nil {
		return fmt.Errorf("decode configuration: %w", errDecode)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("configuration must be a mapping")
	}
	var value map[string]any
	if errDecode := document.Content[0].Decode(&value); errDecode != nil {
		return fmt.Errorf("decode configuration mapping: %w", errDecode)
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if errWrite := encoder.Encode(value); errWrite != nil {
		return fmt.Errorf("write configuration: %w", errWrite)
	}
	return nil
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: config-export <config-path>")
		os.Exit(2)
	}
	if errExport := exportConfig(os.Args[1], os.Stdout); errExport != nil {
		fmt.Fprintln(os.Stderr, errExport)
		os.Exit(1)
	}
}
