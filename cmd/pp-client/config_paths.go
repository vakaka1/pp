package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func configDirs(existingOnly bool) []string {
	var dirs []string
	add := func(dir string) {
		if dir == "" {
			return
		}
		if existingOnly {
			if info, err := os.Stat(dir); err != nil || !info.IsDir() {
				return
			}
		}
		for _, existing := range dirs {
			if existing == dir {
				return
			}
		}
		dirs = append(dirs, dir)
	}

	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			add(filepath.Join(appData, "pp"))
			add(filepath.Join(appData, "pp-client"))
		}
		if confDir, err := os.UserConfigDir(); err == nil {
			add(filepath.Join(confDir, "pp"))
			add(filepath.Join(confDir, "pp-client"))
		}
		if exePath, err := os.Executable(); err == nil {
			add(filepath.Dir(exePath))
		}
	} else {
		if os.Geteuid() == 0 {
			add("/etc/pp-client")
			add("/etc/pp")
		}
		if confDir, err := os.UserConfigDir(); err == nil {
			add(filepath.Join(confDir, "pp-client"))
		}
		if os.Geteuid() != 0 {
			add("/etc/pp-client")
			add("/etc/pp")
		}
	}
	add("configs")

	if existingOnly && len(dirs) == 0 {
		add(".")
	}
	return dirs
}

func configSaveDir() string {
	for _, dir := range configDirs(false) {
		if err := os.MkdirAll(dir, 0700); err == nil {
			return dir
		}
	}
	return "."
}

func configSearchDirs() []string {
	return configDirs(false)
}

func configListDirs() []string {
	return configDirs(true)
}

func resolveConfigPath(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("config name or path is required")
	}

	if info, err := os.Stat(name); err == nil && !info.IsDir() {
		return name, nil
	}

	var candidates []string
	searchDirs := configSearchDirs()

	if !strings.HasSuffix(name, ".json") {
		nameExt := name + ".json"
		candidates = append(candidates, nameExt)
		for _, dir := range searchDirs {
			candidates = append(candidates, filepath.Join(dir, nameExt))
		}
	} else {
		for _, dir := range searchDirs {
			candidates = append(candidates, filepath.Join(dir, name))
		}
	}

	for _, cand := range candidates {
		if info, err := os.Stat(cand); err == nil && !info.IsDir() {
			return cand, nil
		}
	}

	return "", fmt.Errorf("config file not found for: %s", name)
}
