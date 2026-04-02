package cmd

import (
	"os"
	"path/filepath"

	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

type readFilePathConfig struct {
	globalSkillsDir  string
	builtinSkillsDir string
	dataDir          string
	managedSkillDirs []string
}

func allowReadFileSkillPaths(reg *tools.Registry, cfg readFilePathConfig) {
	readTool, ok := reg.Get("read_file")
	if !ok {
		return
	}
	pa, ok := readTool.(tools.PathAllowable)
	if !ok {
		return
	}

	if cfg.globalSkillsDir != "" {
		pa.AllowPaths(cfg.globalSkillsDir)
	}
	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		pa.AllowPaths(filepath.Join(homeDir, ".agents", "skills"))
	}
	if cfg.dataDir != "" {
		pa.AllowPaths(filepath.Join(cfg.dataDir, "cli-workspaces"))
		pa.AllowPaths(filepath.Join(cfg.dataDir, "tenants"))
	}
	if len(cfg.managedSkillDirs) > 0 {
		pa.AllowPaths(cfg.managedSkillDirs...)
	}
	if cfg.builtinSkillsDir != "" {
		pa.AllowPaths(cfg.builtinSkillsDir)
	}
}
