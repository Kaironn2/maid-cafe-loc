package main

// macOS: o jogo só existe para Windows, então roda pelo CrossOver ou Whisky, com a Steam do
// Windows dentro de uma "garrafa". A seleção de pasta usa o osascript, que vem com o sistema.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func steamRoots() []string {
	home, _ := os.UserHomeDir()
	support := filepath.Join(home, "Library", "Application Support")
	roots := []string{filepath.Join(support, "Steam")}
	for _, pattern := range []string{
		filepath.Join(support, "CrossOver", "Bottles", "*", "drive_c", "Program Files (x86)", "Steam"),
		filepath.Join(home, "Library", "Containers", "com.isaacmarovitz.Whisky", "Bottles", "*", "drive_c", "Program Files (x86)", "Steam"),
	} {
		found, _ := filepath.Glob(pattern)
		roots = append(roots, found...)
	}
	return roots
}

// pickFolder abre a janela de seleção de pasta. Devolve "" se o usuário cancelar.
func pickFolder(title string) (string, error) {
	out, err := exec.Command("osascript", "-e", `POSIX path of (choose folder with prompt "`+title+`")`).Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
