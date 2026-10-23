// maidloc: extração, mapeamento e patch dos textos de Maid Cafe on Electric Street.
//
//	go run ./tools/maidloc extract   jogo -> resources/extracted + resources/map.json
//	go run ./tools/maidloc sync      resources/extracted -> translations/ (preserva o PT já feito)
//	go run ./tools/maidloc status    progresso da tradução
//	go run ./tools/maidloc patch     translations/ (campo PT) -> resources.assets do jogo
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const defaultDataDir = `C:\Program Files (x86)\Steam\steamapps\common\Maid Cafe on Electric Street\Maid Cafe on Electric Street_Data`

const usage = `maidloc — ferramentas de tradução PT-BR

Comandos:
  extract   lê o jogo e grava resources/extracted/ e resources/map.json
  sync      gera/atualiza os arquivos de tradução em translations/ (não apaga o PT já feito)
  status    mostra o progresso da tradução
  patch     grava o campo PT de translations/ no resources.assets do jogo (-apply para gravar)

Use "maidloc <comando> -h" para ver as opções.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	var err error
	switch cmd {
	case "extract":
		err = cmdExtract(args)
	case "sync":
		err = cmdSync(args)
	case "status":
		err = cmdStatus(args)
	case "patch":
		err = cmdPatch(args)
	case "-h", "--help", "help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "comando desconhecido: %s\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERRO:", err)
		os.Exit(1)
	}
}

// ---------------------------------------------------------------- jogo

type gameData struct {
	assets *AssetFile
	texts  []TextAsset
	docs   map[string]any
}

// originalAssets devolve o resources.assets original: o .bak, se existir (o
// arquivo principal pode já estar com patch), senão o próprio resources.assets.
func originalAssets(dataDir string) string {
	main := filepath.Join(dataDir, "resources.assets")
	if _, err := os.Stat(main + ".bak"); err == nil {
		return main + ".bak"
	}
	return main
}

func loadGame(dataDir string) (*gameData, error) {
	path := originalAssets(dataDir)
	a, err := OpenAssets(path)
	if err != nil {
		return nil, err
	}
	g := &gameData{assets: a, texts: a.TextAssets(), docs: map[string]any{}}
	for _, t := range g.texts {
		doc, err := ParseJSON(t.Payload)
		if err != nil {
			warn("asset %s não é JSON válido: %v", t.Name, err)
			continue
		}
		g.docs[t.Name] = doc
	}
	fmt.Printf("Lido: %s (%d assets JSON)\n", path, len(g.docs))
	return g, nil
}

// ---------------------------------------------------------------- extract

func cmdExtract(args []string) error {
	fl := flag.NewFlagSet("extract", flag.ExitOnError)
	dataDir := fl.String("game", defaultDataDir, "pasta *_Data do jogo")
	out := fl.String("out", filepath.Join("resources", "extracted"), "pasta de saída")
	mapFile := fl.String("map", filepath.Join("resources", "map.json"), "arquivo do mapa de assets")
	fl.Parse(args)

	g, err := loadGame(*dataDir)
	if err != nil {
		return err
	}
	units := MapAssets(g.docs)
	if err := os.RemoveAll(*out); err != nil {
		return err
	}
	for _, u := range units {
		p := filepath.Join(*out, u.Category, u.Asset+".json")
		if err := writeFile(p, EncodeJSON(g.docs[u.Asset], true)); err != nil {
			return err
		}
	}
	mapJSON, _ := json.MarshalIndent(units, "", "  ")
	if err := writeFile(*mapFile, append(mapJSON, '\n')); err != nil {
		return err
	}

	byCat := map[string]int{}
	for _, u := range units {
		byCat[u.Category]++
	}
	fmt.Printf("\n%d assets -> %s (languages: %d, scenes: %d, data: %d)\n", len(units), *out,
		byCat["languages"], byCat["scenes"], byCat["data"])
	fmt.Printf("Mapa -> %s\n\nAssets com texto traduzível:\n", *mapFile)
	total, chars := 0, 0
	for _, u := range units {
		if u.File == "" {
			continue
		}
		fmt.Printf("  %-40s %-5s %-8s %-14s %5d textos\n", u.Asset, u.Kind, u.Field, strings.Join(u.Langs, ","), u.Texts)
		total += u.Texts
		chars += u.Chars
	}
	fmt.Printf("\nTotal: %d textos, %d caracteres em inglês.\n", total, chars)
	return nil
}

// ---------------------------------------------------------------- sync

func loadExtracted(dir string) (map[string]any, error) {
	docs := map[string]any{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".json" {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		doc, err := ParseJSON(data)
		if err != nil {
			return fmt.Errorf("%s: %v", p, err)
		}
		docs[strings.TrimSuffix(d.Name(), ".json")] = doc
		return nil
	})
	if err == nil && len(docs) == 0 {
		err = fmt.Errorf("nada em %s; rode \"maidloc extract\" antes", dir)
	}
	return docs, err
}

func cmdSync(args []string) error {
	fl := flag.NewFlagSet("sync", flag.ExitOnError)
	src := fl.String("src", filepath.Join("resources", "extracted"), "pasta gerada pelo extract")
	dir := fl.String("dir", "translations", "pasta dos arquivos de tradução")
	fl.Parse(args)

	docs, err := loadExtracted(*src)
	if err != nil {
		return err
	}
	names := CharacterNames(docs)
	created, updated, changedEN, orphans := 0, 0, 0, 0
	for _, u := range MapAssets(docs) {
		if u.File == "" {
			continue
		}
		path := filepath.Join(*dir, filepath.FromSlash(u.File))
		old := map[string]WorkEntry{}
		if _, err := os.Stat(path); err == nil {
			if old, _, err = readWorkFile(path); err != nil {
				return err
			}
			updated++
		} else {
			created++
		}

		out := &Object{}
		seen := map[string]bool{}
		for _, e := range SourceEntries(u, docs[u.Asset]) {
			seen[e.Key] = true
			row := &Object{}
			if e.Character != "" {
				who := names[e.Character]
				if who == "" {
					// variações de roupa/estado: "2_Miyu_Suit1" -> "2_Miyu", "fafuna1" -> "fafuna"
					who = names[variantSuffixRe.ReplaceAllString(e.Character, "")]
				}
				if who == "" {
					who = speakerLabels[e.Character]
				}
				if who == "" {
					who = e.Character
				}
				row.Set("Personagem", who)
			}
			prev, had := old[e.Key]
			row.Set("PT", prev.PT)
			row.Set("EN", e.EN)
			if had && prev.PT != "" && prev.EN != e.EN {
				changedEN++
				fmt.Printf("REVISAR %s [%s]: o inglês mudou\n    antes: %q\n    agora: %q\n", u.File, e.Key, prev.EN, e.EN)
			}
			out.Pairs = append(out.Pairs, Pair{e.Key, row})
		}
		for k, prev := range old {
			if !seen[k] && prev.PT != "" {
				orphans++
				fmt.Printf("ÓRFÃ %s [%s]: a chave sumiu do jogo; tradução descartada: %q\n", u.File, k, prev.PT)
			}
		}
		if err := writeFile(path, EncodeJSON(out, true)); err != nil {
			return err
		}
	}
	fmt.Printf("\ntranslations/: %d arquivos criados, %d atualizados", created, updated)
	if changedEN+orphans > 0 {
		fmt.Printf(", %d para revisar, %d órfãs", changedEN, orphans)
	}
	fmt.Println(".")
	return nil
}

// ---------------------------------------------------------------- arquivos de tradução

// speakerLabels nomeia os personagens que o jogo deixa sem nome exibido.
var speakerLabels = map[string]string{
	"system":  "(narração)",
	"pangbai": "(narração)",
	"zhujue":  "(protagonista)",
}

var variantSuffixRe = regexp.MustCompile(`(_[A-Za-z]+\d+|_sp|\d+)$`)

type WorkEntry struct {
	PT, EN string
}

// readWorkFile lê um arquivo de translations/ ({chave: {"PT": ..., "EN": ...}}), mantendo a ordem.
func readWorkFile(path string) (map[string]WorkEntry, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	root, err := ParseJSON(data)
	if err != nil {
		var se *json.SyntaxError
		if errors.As(err, &se) {
			return nil, nil, fmt.Errorf("%s:%d: JSON inválido: %v", path, lineOf(data, se.Offset), err)
		}
		return nil, nil, fmt.Errorf("%s: JSON inválido: %v", path, err)
	}
	obj, ok := root.(*Object)
	if !ok {
		return nil, nil, fmt.Errorf("%s: esperava um objeto JSON", path)
	}
	entries := map[string]WorkEntry{}
	var order []string
	for _, p := range obj.Pairs {
		row, ok := p.Val.(*Object)
		if !ok {
			return nil, nil, fmt.Errorf("%s: a chave %q deveria ser {\"PT\": ..., \"EN\": ...}", path, p.Key)
		}
		entries[p.Key] = WorkEntry{PT: row.GetString("PT"), EN: row.GetString("EN")}
		order = append(order, p.Key)
	}
	return entries, order, nil
}

func lineOf(data []byte, off int64) int {
	if off > int64(len(data)) {
		off = int64(len(data))
	}
	return strings.Count(string(data[:off]), "\n") + 1
}

func listWorkFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".json" {
			return err
		}
		files = append(files, p)
		return nil
	})
	sort.Strings(files)
	return files, err
}

// ---------------------------------------------------------------- status

func cmdStatus(args []string) error {
	fl := flag.NewFlagSet("status", flag.ExitOnError)
	dir := fl.String("dir", "translations", "pasta dos arquivos de tradução")
	verbose := fl.Bool("v", false, "mostrar todos os arquivos (não só os começados)")
	fl.Parse(args)

	files, err := listWorkFiles(*dir)
	if err != nil {
		return err
	}
	type acc struct{ done, total, doneChars, totalChars int }
	groups := map[string]*acc{}
	var all acc
	fmt.Printf("%-48s %13s %7s\n", "arquivo", "traduzidos", "%")
	for _, f := range files {
		entries, _, err := readWorkFile(f)
		if err != nil {
			return err
		}
		var a acc
		for _, e := range entries {
			n := len([]rune(e.EN))
			a.total++
			a.totalChars += n
			if strings.TrimSpace(e.PT) != "" {
				a.done++
				a.doneChars += n
			}
		}
		rel, _ := filepath.Rel(*dir, f)
		group := filepath.Dir(rel)
		if groups[group] == nil {
			groups[group] = &acc{}
		}
		for _, t := range []*acc{groups[group], &all} {
			t.done += a.done
			t.total += a.total
			t.doneChars += a.doneChars
			t.totalChars += a.totalChars
		}
		if *verbose || a.done > 0 {
			fmt.Printf("%-48s %6d/%-6d %6.1f%%\n", filepath.ToSlash(rel), a.done, a.total, pct(a.done, a.total))
		}
	}
	fmt.Println()
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		a := groups[k]
		fmt.Printf("%-12s %6d/%-6d textos  %6.1f%% dos caracteres\n", k+"/", a.done, a.total, pct(a.doneChars, a.totalChars))
	}
	fmt.Printf("%-12s %6d/%-6d textos  %6.1f%% dos caracteres\n", "TOTAL", all.done, all.total, pct(all.doneChars, all.totalChars))
	return nil
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return 100 * float64(a) / float64(b)
}

// ---------------------------------------------------------------- patch

var (
	placeholderRe = regexp.MustCompile(`\{\d+\}`)
	tagRe         = regexp.MustCompile(`</?[a-zA-Z][^<>]*>`)
)

func cmdPatch(args []string) error {
	fl := flag.NewFlagSet("patch", flag.ExitOnError)
	dataDir := fl.String("game", defaultDataDir, "pasta *_Data do jogo")
	dir := fl.String("dir", "translations", "pasta dos arquivos de tradução")
	apply := fl.Bool("apply", false, "grava no jogo (sem isso é só um teste)")
	out := fl.String("out", "", "gravar em outro arquivo em vez do resources.assets do jogo")
	fl.Parse(args)

	g, err := loadGame(*dataDir)
	if err != nil {
		return err
	}
	payloads := map[string][]byte{}
	totalPT, warnings := 0, 0
	for _, u := range MapAssets(g.docs) {
		if u.File == "" {
			continue
		}
		path := filepath.Join(*dir, filepath.FromSlash(u.File))
		if _, err := os.Stat(path); err != nil {
			continue
		}
		entries, order, err := readWorkFile(path)
		if err != nil {
			return err
		}
		source := map[string]string{}
		for _, e := range SourceEntries(u, g.docs[u.Asset]) {
			source[e.Key] = e.EN
		}
		pt := map[string]string{}
		for _, k := range order {
			e := entries[k]
			if strings.TrimSpace(e.PT) == "" {
				continue
			}
			en, ok := source[k]
			if !ok {
				warn("%s [%s]: chave não existe no jogo (ignorada)", u.File, k)
				warnings++
				continue
			}
			if en != e.EN {
				warn("%s [%s]: o inglês do jogo é diferente do arquivo; rode extract + sync", u.File, k)
				warnings++
			}
			for _, msg := range checkMarkup(en, e.PT) {
				warn("%s [%s]: %s", u.File, k, msg)
				warnings++
			}
			pt[k] = e.PT
		}
		if len(pt) == 0 {
			continue
		}
		ApplyPT(u, g.docs[u.Asset], pt)
		payloads[u.Asset] = EncodeJSON(g.docs[u.Asset], false)
		totalPT += len(pt)
		fmt.Printf("  %-40s %5d/%-5d traduzidos\n", u.Asset, len(pt), u.Texts)
	}

	result := g.assets.Patched(g.texts, payloads)
	fmt.Printf("\n%d textos em %d assets; arquivo final: %d bytes", totalPT, len(payloads), len(result))
	if warnings > 0 {
		fmt.Printf("; %d avisos", warnings)
	}
	fmt.Println(".")
	if !*apply {
		fmt.Println("[teste] nada foi gravado. Use -apply para gravar.")
		return nil
	}

	target := *out
	if target == "" {
		target = filepath.Join(*dataDir, "resources.assets")
		backup := target + ".bak"
		if _, err := os.Stat(backup); os.IsNotExist(err) {
			// sem .bak, o original foi lido do próprio resources.assets
			if err := os.WriteFile(backup, g.assets.Raw, 0o644); err != nil {
				return fmt.Errorf("falha no backup: %v", err)
			}
			fmt.Println("backup do original: " + backup)
		}
	}
	if err := os.WriteFile(target, result, 0o644); err != nil {
		return err
	}
	fmt.Println("Gravado: " + target)
	return nil
}

// checkMarkup confere se o PT mantém os {0} e as tags <color> do inglês.
func checkMarkup(en, pt string) []string {
	var msgs []string
	for _, c := range []struct {
		re   *regexp.Regexp
		what string
	}{{placeholderRe, "marcadores {n}"}, {tagRe, "tags <...>"}} {
		a, b := c.re.FindAllString(en, -1), c.re.FindAllString(pt, -1)
		sort.Strings(a)
		sort.Strings(b)
		if strings.Join(a, " ") != strings.Join(b, " ") {
			msgs = append(msgs, fmt.Sprintf("%s diferentes do inglês (EN: %v, PT: %v)", c.what, a, b))
		}
	}
	return msgs
}

// ---------------------------------------------------------------- util

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func warn(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "AVISO: "+format+"\n", args...)
}
