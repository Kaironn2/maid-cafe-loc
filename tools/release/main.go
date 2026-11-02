// release: gera os pacotes da release em dist/, um .zip por sistema com o instalador
// (maidloc com translations/ embutido) e o LEIA-ME, mais o SHA256SUMS.txt.
//
//	go run ./tools/release v1.0.0
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"
)

const name = "MaidCafe-PTBR"

var targets = []struct{ goos, goarch, label string }{
	{"windows", "amd64", "windows-x64"},
	{"linux", "amd64", "linux-x64"},
	{"darwin", "arm64", "macos-arm64"},
	{"darwin", "amd64", "macos-x64"},
}

func main() {
	if len(os.Args) != 2 || !regexp.MustCompile(`^v\d+\.\d+\.\d+`).MatchString(os.Args[1]) {
		fmt.Fprintln(os.Stderr, "uso: go run ./tools/release <versão, ex.: v1.0.0>")
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "ERRO:", err)
		os.Exit(1)
	}
}

func run(version string) error {
	readme, err := os.ReadFile(filepath.Join("tools", "release", "LEIA-ME.txt"))
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "maidloc-release")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := os.RemoveAll("dist"); err != nil {
		return err
	}
	if err := os.MkdirAll("dist", 0o755); err != nil {
		return err
	}

	var sums []byte
	for _, t := range targets {
		bin := name
		if t.goos == "windows" {
			bin += ".exe"
		}
		binPath := filepath.Join(tmp, t.label, bin)
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w -X main.version="+version, "-o", binPath, "./tools/maidloc")
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+t.goos, "GOARCH="+t.goarch)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("build %s: %v", t.label, err)
		}
		exe, err := os.ReadFile(binPath)
		if err != nil {
			return err
		}

		zipName := fmt.Sprintf("%s-%s-%s.zip", name, version, t.label)
		data, err := makeZip([]zipFile{
			{name + "/" + bin, exe, 0o755},
			{name + "/LEIA-ME.txt", readme, 0o644},
		})
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join("dist", zipName), data, 0o644); err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		sums = append(sums, fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), zipName)...)
		fmt.Printf("dist/%s (%d KB)\n", zipName, len(data)/1024)
	}
	return os.WriteFile(filepath.Join("dist", "SHA256SUMS.txt"), sums, 0o644)
}

type zipFile struct {
	path string
	data []byte
	mode os.FileMode
}

// makeZip grava as permissões Unix, para o binário continuar executável no macOS e no Linux.
func makeZip(files []zipFile) ([]byte, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, f := range files {
		h := &zip.FileHeader{Name: f.path, Method: zip.Deflate, Modified: time.Now()}
		h.SetMode(f.mode)
		fw, err := w.CreateHeader(h)
		if err != nil {
			return nil, err
		}
		if _, err := fw.Write(f.data); err != nil {
			return nil, err
		}
	}
	err := w.Close()
	return buf.Bytes(), err
}
