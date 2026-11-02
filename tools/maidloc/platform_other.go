//go:build !windows && !darwin

package main

// Linux (e Steam Deck): o jogo roda pelo Proton, com a mesma pasta do Windows dentro de
// steamapps/common. A seleção de pasta usa o zenity (GNOME), kdialog (KDE, Steam Deck) ou yad.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func steamRoots() []string {
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, ".steam", "steam"),
		filepath.Join(home, ".steam", "root"),
		filepath.Join(home, ".local", "share", "Steam"),
		filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", ".local", "share", "Steam"), // Flatpak
		filepath.Join(home, "snap", "steam", "common", ".local", "share", "Steam"),
	}
}

// pickFolder abre a janela de seleção de pasta. Devolve "" se o usuário cancelar.
func pickFolder(title string) (string, error) {
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return "", errors.New("sem interface gráfica")
	}
	home, _ := os.UserHomeDir()
	for _, cmd := range [][]string{
		{"zenity", "--file-selection", "--directory", "--title=" + title},
		{"kdialog", "--getexistingdirectory", home, "--title", title},
		{"yad", "--file", "--directory", "--title=" + title},
	} {
		if _, err := exec.LookPath(cmd[0]); err != nil {
			continue
		}
		out, err := exec.Command(cmd[0], cmd[1:]...).Output()
		if err != nil {
			return "", nil // cancelado
		}
		return strings.TrimSpace(string(out)), nil
	}
	return "", errors.New("zenity, kdialog e yad não estão instalados")
}
