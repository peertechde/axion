package config

import (
	"fmt"
	"os"
	"path/filepath"

	"peertech.de/axion/api/client"
)

const BackupEnvVar = "AXION_BACKUP_DIR"

type Config struct {
	EnableBackups bool
	BackupDir     string
	Concurrency   int

	Client *client.ConfigurationManagement
}

func DefaultBackupDir() string {
	if env := os.Getenv(BackupEnvVar); env != "" {
		return env
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/axionctl/backups"
	}
	return filepath.Join(home, ".config", "axion", "backups")
}

func ValidateBackupDir(path string) error {
	if path == "" {
		return fmt.Errorf("backup directory is empty")
	}

	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("backup directory %q does not exist and could not be created: %w", path, err)
		}
		return nil
	}

	if err != nil {
		return fmt.Errorf("cannot access backup directory %q: %w", path, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("backup path %q is not a directory", path)
	}

	// Try writing a test file
	testFile := filepath.Join(path, ".axionctl_write_test")
	f, err := os.Create(testFile)
	if err != nil {
		return fmt.Errorf("backup directory %q is not writable: %w", path, err)
	}
	f.Close()
	os.Remove(testFile)

	return nil
}
