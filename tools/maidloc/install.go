package main

// Instalador para jogadores: o binário de release é este mesmo programa, com translations/
// embutido. Aberto sem argumentos (duplo clique), procura o jogo na Steam, ou pede a pasta
// numa janela do sistema, e aplica o patch com as traduções embutidas.

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"maidcafeloc"
)

// version é definida no build da release: -ldflags "-X main.version=v1.0.0"
var version = "dev"

const gameFolder = "Maid Cafe on Electric Street"

// markSuffix: arquivo ao lado do arquivo do jogo com o hash do que o patch gravou.
const markSuffix = ".ptbr"

var stdin = bufio.NewReader(os.Stdin)

func cmdInstall(args []string) error {
	fl := flag.NewFlagSet("install", flag.ExitOnError)
	game := fl.String("game", "", "pasta do jogo ou pasta *_Data (sem isso, procura na Steam)")
	restoreFlag := fl.Bool("restore", false, "restaurar os arquivos originais em vez de instalar")
	fl.Parse(args)

	var dataDir string
	if *game != "" {
		d, ok := resolveDataDir(*game)
		if !ok {
			return fmt.Errorf("%s não é a pasta do jogo (não achei resources.assets e global-metadata.dat)", *game)
		}
		dataDir = d
	} else if dataDir = findGame(); dataDir == "" {
		return errors.New("jogo não encontrado na Steam; informe a pasta com -game")
	}
	fmt.Println("Pasta do jogo:", dataDir)
	if *restoreFlag {
		return restore(dataDir)
	}
	_, err := patch(dataDir, maidcafeloc.Translations, "translations", true, "", "")
	return err
}

func runInstaller() {
	if err := install(); err != nil {
		fmt.Fprintln(os.Stderr, "\nERRO:", err)
		fmt.Fprintln(os.Stderr, "Se o jogo estiver aberto, feche-o e tente de novo.")
	}
	fmt.Print("\nPressione Enter para sair.")
	stdin.ReadString('\n')
}

func install() error {
	fmt.Printf("Tradução PT-BR de %s (%s)\n\n", gameFolder, version)
	dataDir := locateGame()
	if dataDir == "" {
		return errors.New("nenhuma pasta escolhida; nada foi alterado")
	}
	fmt.Println("Pasta do jogo:", dataDir)

	for _, f := range gameFiles(dataDir) {
		if err := dropStaleBackup(f); err != nil {
			return err
		}
	}
	if _, err := os.Stat(gameFiles(dataDir)[0] + ".bak"); err == nil {
		fmt.Print("\nA tradução já está instalada.\n" +
			"  1) Reinstalar / atualizar a tradução\n" +
			"  2) Restaurar o jogo original (em inglês)\n" +
			"  3) Sair\n")
		switch ask("Escolha [1]: ") {
		case "", "1":
		case "2":
			return restore(dataDir)
		default:
			return nil
		}
	} else if a := strings.ToLower(ask("\nInstalar a tradução? [S/n] ")); a != "" && a != "s" && a != "sim" {
		return nil
	}

	fmt.Println()
	warnings, err := patch(dataDir, maidcafeloc.Translations, "translations", true, "", "")
	if err != nil {
		return err
	}
	if warnings > 0 {
		fmt.Printf("\n%d avisos acima: esses textos podem aparecer com defeito no jogo.\n", warnings)
	}
	fmt.Println("\nTradução instalada! Para voltar ao inglês, abra este programa de novo e escolha \"Restaurar\".")
	return nil
}

// locateGame procura o jogo na Steam e, se não achar, pede a pasta ao usuário.
// Devolve "" se o usuário desistir.
func locateGame() string {
	fmt.Println("Procurando o jogo na Steam...")
	if d := findGame(); d != "" {
		return d
	}
	fmt.Printf("Não encontrei o jogo automaticamente.\n"+
		"Escolha a pasta do jogo (\"%s\", dentro de steamapps/common) na janela que vai abrir.\n", gameFolder)
	for {
		dir, err := pickFolder("Escolha a pasta do jogo " + gameFolder)
		if err != nil {
			fmt.Printf("Não deu para abrir a janela de seleção (%v).\n", err)
			dir = cleanPath(ask("Digite ou arraste para cá a pasta do jogo e aperte Enter: "))
		}
		if dir == "" {
			return ""
		}
		if d, ok := resolveDataDir(dir); ok {
			return d
		}
		fmt.Printf("%s não é a pasta do jogo (não achei resources.assets). Tente de novo.\n", dir)
	}
}

// findGame procura a pasta *_Data do jogo em todas as bibliotecas da Steam conhecidas.
func findGame() string {
	for _, root := range steamRoots() {
		for _, lib := range steamLibraries(root) {
			if d, ok := resolveDataDir(filepath.Join(lib, "steamapps", "common", gameFolder)); ok {
				return d
			}
		}
	}
	return ""
}

var vdfPathRe = regexp.MustCompile(`"path"\s+"((?:[^"\\]|\\.)*)"`)

// steamLibraries devolve root e as outras bibliotecas listadas em libraryfolders.vdf
// (jogos instalados em outro disco).
func steamLibraries(root string) []string {
	libs := []string{root}
	data, err := os.ReadFile(filepath.Join(root, "steamapps", "libraryfolders.vdf"))
	if err != nil {
		return libs
	}
	for _, m := range vdfPathRe.FindAllStringSubmatch(string(data), -1) {
		libs = append(libs, strings.ReplaceAll(m[1], `\\`, `\`))
	}
	return libs
}

// resolveDataDir aceita a pasta *_Data ou a pasta do jogo que a contém.
func resolveDataDir(dir string) (string, bool) {
	candidates := []string{dir, filepath.Join(dir, gameFolder+"_Data")}
	more, _ := filepath.Glob(filepath.Join(dir, "*_Data"))
	for _, d := range append(candidates, more...) {
		if isFile(filepath.Join(d, "resources.assets")) && isFile(metadataPath(d)) {
			return d, true
		}
	}
	return "", false
}

func gameFiles(dataDir string) []string {
	return []string{filepath.Join(dataDir, "resources.assets"), metadataPath(dataDir)}
}

// restore volta os arquivos do jogo para os originais guardados em .bak.
func restore(dataDir string) error {
	n := 0
	for _, f := range gameFiles(dataDir) {
		if !isFile(f + ".bak") {
			continue
		}
		if err := os.Rename(f+".bak", f); err != nil {
			return err
		}
		os.Remove(f + markSuffix)
		fmt.Println("Restaurado: " + f)
		n++
	}
	if n == 0 {
		fmt.Println("Não há backup: o jogo já está com os arquivos originais.")
	} else {
		fmt.Println("\nJogo restaurado para o original.")
	}
	return nil
}

// dropStaleBackup apaga o .bak quando o arquivo do jogo não é mais o que o patch gravou:
// a Steam atualizou o jogo (ou verificou os arquivos), e o arquivo atual é o novo original.
func dropStaleBackup(gameFile string) error {
	mark, err := os.ReadFile(gameFile + markSuffix)
	if err != nil {
		return nil // sem marca: o patch não gravou este arquivo
	}
	data, err := os.ReadFile(gameFile)
	if err != nil || fileHash(data) == strings.TrimSpace(string(mark)) {
		return nil
	}
	fmt.Printf("%s mudou desde a última instalação (atualização do jogo?); o backup antigo foi descartado.\n", filepath.Base(gameFile))
	for _, p := range []string{gameFile + ".bak", gameFile + markSuffix} {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func fileHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func isFile(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular()
}

func ask(prompt string) string {
	fmt.Print(prompt)
	line, _ := stdin.ReadString('\n')
	return strings.TrimSpace(line)
}

// cleanPath limpa um caminho digitado ou arrastado para o terminal: aspas em volta
// (Windows) e espaços escapados com \ (macOS, Linux).
func cleanPath(s string) string {
	s = strings.Trim(strings.TrimSpace(s), `"'`)
	if runtime.GOOS != "windows" {
		s = strings.ReplaceAll(s, `\ `, " ")
	}
	return s
}
