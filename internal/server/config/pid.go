package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// WritePIDFile writes the current process id to path.
func WritePIDFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir pid dir: %w", err)
		}
	}
	content := fmt.Sprintf("%d\n", os.Getpid())
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// RemovePIDFile deletes the pid file if present.
func RemovePIDFile(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	_ = os.Remove(path)
}

// ReadPIDFile returns the pid stored in path.
func ReadPIDFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read pid file %s: %w", path, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid pid in %s: %w", path, err)
	}
	return pid, nil
}

// SignalReload sends SIGHUP to the process in pidFile.
func SignalReload(pidFile string) error {
	pid, err := ReadPIDFile(pidFile)
	if err != nil {
		return err
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(syscall.SIGHUP)
}
