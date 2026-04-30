package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vakaka1/pp/internal/config"
)

type ConfigInfo struct {
	Name string             `json:"name"`
	Path string             `json:"path"`
	Meta *config.ConfigMeta `json:"meta,omitempty"`
}

func configListDirs() []string {
	var dirs []string
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData != "" {
			ppDir := filepath.Join(appData, "pp")
			if info, err := os.Stat(ppDir); err == nil && info.IsDir() {
				dirs = append(dirs, ppDir)
			}
		}
		exePath, err := os.Executable()
		if err == nil {
			dirs = append(dirs, filepath.Dir(exePath))
		}
	} else {
		if info, err := os.Stat("/etc/pp"); err == nil && info.IsDir() {
			dirs = append(dirs, "/etc/pp")
		}
	}
	if info, err := os.Stat("configs"); err == nil && info.IsDir() {
		dirs = append(dirs, "configs")
	}
	if len(dirs) == 0 {
		dirs = append(dirs, ".")
	}
	return dirs
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List imported clients",
	Run: func(cmd *cobra.Command, args []string) {
		outputJson, _ := cmd.Flags().GetBool("json")

		searchDirs := configListDirs()
		var results []ConfigInfo
		seen := make(map[string]bool)

		for _, dir := range searchDirs {
			files, err := os.ReadDir(dir)
			if err != nil {
				continue
			}

			for _, f := range files {
				if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
					continue
				}
				if strings.Contains(f.Name(), "example") {
					continue
				}

				fullPath := filepath.Join(dir, f.Name())
				absPath, _ := filepath.Abs(fullPath)
				if seen[absPath] {
					continue
				}

				cfg, err := config.LoadConfig(fullPath)
				if err != nil {
					continue
				}

				if cfg.Client == nil {
					continue
				}

				name := strings.TrimSuffix(f.Name(), ".json")
				seen[absPath] = true

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
