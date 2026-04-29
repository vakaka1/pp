package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vakaka1/pp/internal/config"
)

type ConfigInfo struct {
	Name string             `json:"name"`
	Path string             `json:"path"`
	Meta *config.ConfigMeta `json:"meta,omitempty"`
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List imported clients",
	Run: func(cmd *cobra.Command, args []string) {
		outputJson, _ := cmd.Flags().GetBool("json")

		var searchDirs []string
		if info, err := os.Stat("/etc/pp"); err == nil && info.IsDir() {
			searchDirs = append(searchDirs, "/etc/pp")
		} else if info, err := os.Stat("configs"); err == nil && info.IsDir() {
			searchDirs = append(searchDirs, "configs")
		} else {
			searchDirs = append(searchDirs, ".")
		}

		var results []ConfigInfo

		for _, dir := range searchDirs {
			files, err := os.ReadDir(dir)
			if err != nil {
				continue
			}

			for _, f := range files {
				if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
					continue
				}
				// exclude example configs
				if strings.Contains(f.Name(), "example") {
					continue
				}

				fullPath := filepath.Join(dir, f.Name())
				cfg, err := config.LoadConfig(fullPath)
				if err != nil {
					continue
				}

				// Check if it's actually a client config
				if cfg.Client == nil {
					continue
				}

				name := strings.TrimSuffix(f.Name(), ".json")

				results = append(results, ConfigInfo{
					Name: name,
					Path: fullPath,
					Meta: cfg.Meta,
				})
			}
		}

		if outputJson {
			data, _ := json.MarshalIndent(results, "", "  ")
			fmt.Println(string(data))
			return
		}

		if len(results) == 0 {
			fmt.Println("No clients found.")
			return
		}

		fmt.Printf("%-20s %-15s %-25s %s\n", "NAME", "PROTOCOL", "GENERATED AT", "PATH")
		fmt.Println(strings.Repeat("-", 80))
		for _, r := range results {
			proto := "-"
			genAt := "-"
			if r.Meta != nil {
				if r.Meta.Protocol != "" {
					proto = r.Meta.Protocol
				}
				if r.Meta.GeneratedAt != "" {
					genAt = r.Meta.GeneratedAt
				}
			}
			fmt.Printf("%-20s %-15s %-25s %s\n", r.Name, proto, genAt, r.Path)
		}
	},
}

func init() {
	listCmd.Flags().Bool("json", false, "Output in JSON format")
}
