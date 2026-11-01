package main

// Planilhas do jogo: ScriptableObjects gerados de Assets/Resources/GameData/Excel/*.xls
// (diálogos do prólogo e de algumas cenas do primeiro dia).
//
// Não são TextAssets, são MonoBehaviours. Layout confirmado (string = [u32 len][bytes][pad4]):
//   [12 m_GameObject][4 m_Enabled][12 m_Script][m_Name][caminho do .xls][nome da aba]
//   [u32 linhas] e cada linha com N strings. Nas falas N = 11:
//   id, personagem, cn, jp, en, tc, e mais 5 colunas (expressão, parte, voz...).
//
// Cada planilha aparece duas vezes no resources.assets; o patch grava as duas cópias.
// Para o resto da ferramenta, cada planilha vira um asset "rows" chamado excel_<nome>,
// com o campo EN como texto traduzível.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
)

const excelPrefix = "excel_"

var excelMarker = []byte("Assets/Resources/GameData/Excel/")

// Colunas das planilhas de falas (11 colunas); a coluna 0 (id) vira a chave.
var excelColumns = [11]string{"Id", "Character", "StoryText", "JP", "EN", "TC", "Col6", "Col7", "Col8", "Col9", "Col10"}

const excelENColumn = 4

type ExcelSheet struct {
	Name    string
	Copy    int // 1 na primeira cópia da planilha, 2 na segunda...
	TableAt int
	Header  []byte // bytes do objeto até o início das linhas (inclui a contagem)
	Rows    [][]string
}

// ExcelSheets lê as planilhas, na ordem em que aparecem na object table.
func (a *AssetFile) ExcelSheets() []ExcelSheet {
	var out []ExcelSheet
	copies := map[string]int{}
	for e := 0; e < a.Count; e++ {
		at, obj, size, ok := a.object(e)
		if !ok {
			continue
		}
		s, ok := parseExcelSheet(a.Raw[obj : obj+size])
		if !ok {
			continue
		}
		copies[s.Name]++
		s.Copy = copies[s.Name]
		s.TableAt = at
		out = append(out, s)
	}
	return out
}

func parseExcelSheet(b []byte) (ExcelSheet, bool) {
	var s ExcelSheet
	if len(b) < 64 || !bytes.Contains(b[:min(len(b), 512)], excelMarker) {
		return s, false
	}
	p := 28
	readStr := func() (string, bool) {
		if p+4 > len(b) {
			return "", false
		}
		n := int(int32(binary.LittleEndian.Uint32(b[p : p+4])))
		if n < 0 || p+4+n > len(b) {
			return "", false
		}
		str := string(b[p+4 : p+4+n])
		p += 4 + alignUp(n, 4)
		return str, true
	}
	name, ok1 := readStr()
	path, ok2 := readStr()
	_, ok3 := readStr()
	if !ok1 || !ok2 || !ok3 || !strings.HasPrefix(path, string(excelMarker)) || p+4 > len(b) {
		return s, false
	}
	count := int(int32(binary.LittleEndian.Uint32(b[p : p+4])))
	p += 4
	start := p
	// o número de colunas não está no arquivo: é o que consome o objeto inteiro
	for cols := 1; cols <= 40; cols++ {
		p = start
		rows := make([][]string, 0, count)
		ok := count >= 0
		for r := 0; r < count && ok; r++ {
			row := make([]string, cols)
			for c := 0; c < cols && ok; c++ {
				row[c], ok = readStr()
			}
			rows = append(rows, row)
		}
		if ok && p == len(b) {
			s.Name = name
			s.Header = append([]byte(nil), b[:start]...)
			s.Rows = rows
			return s, true
		}
	}
	return s, false
}

// excelRowKeys devolve a chave de cada linha: o id, com "#2", "#3"... quando o id
// se repete na planilha. Linhas sem id ficam com chave vazia (não entram no doc).
func excelRowKeys(rows [][]string) []string {
	keys := make([]string, len(rows))
	seen := map[string]int{}
	for i, r := range rows {
		if len(r) == 0 || r[0] == "" {
			continue
		}
		seen[r[0]]++
		keys[i] = r[0]
		if n := seen[r[0]]; n > 1 {
			keys[i] = fmt.Sprintf("%s#%d", r[0], n)
		}
	}
	return keys
}

// excelCopyKey é a chave de uma linha da cópia n quando o texto dela difere da cópia 1.
func excelCopyKey(key string, copy int) string {
	return fmt.Sprintf("%s@%d", key, copy)
}

// ExcelDocs converte as planilhas em docs "rows" (excel_<nome>). Linhas das outras
// cópias só viram chaves próprias (id@2) quando o inglês é diferente da cópia 1.
func ExcelDocs(sheets []ExcelSheet) map[string]any {
	docs := map[string]any{}
	for _, s := range sheets {
		name := excelPrefix + s.Name
		doc, _ := docs[name].(*Object)
		if doc == nil {
			doc = &Object{}
			docs[name] = doc
		}
		for i, key := range excelRowKeys(s.Rows) {
			if key == "" {
				continue
			}
			row := excelRowObject(s.Rows[i])
			if s.Copy > 1 {
				if prev, ok := doc.Get(key); ok && prev.(*Object).GetString("EN") == row.GetString("EN") {
					continue
				}
				key = excelCopyKey(key, s.Copy)
			}
			doc.Pairs = append(doc.Pairs, Pair{key, row})
		}
	}
	return docs
}

func excelRowObject(cols []string) *Object {
	row := &Object{}
	for c := 1; c < len(cols); c++ {
		field := fmt.Sprintf("Col%d", c)
		if len(cols) == len(excelColumns) {
			field = excelColumns[c]
		}
		row.Pairs = append(row.Pairs, Pair{field, cols[c]})
	}
	return row
}

// Encode devolve o objeto da planilha com a coluna EN trocada pelo EN do doc
// (onde o patch já gravou o PT).
func (s ExcelSheet) Encode(doc *Object) []byte {
	buf := append([]byte(nil), s.Header...)
	keys := excelRowKeys(s.Rows)
	for i, cols := range s.Rows {
		for c, val := range cols {
			if c == excelENColumn && len(cols) == len(excelColumns) && keys[i] != "" {
				key := keys[i]
				if _, ok := doc.Get(excelCopyKey(key, s.Copy)); ok {
					key = excelCopyKey(key, s.Copy)
				}
				if v, ok := doc.Get(key); ok {
					val = v.(*Object).GetString("EN")
				}
			}
			buf = binary.LittleEndian.AppendUint32(buf, uint32(len(val)))
			buf = append(buf, val...)
			for len(buf)%4 != 0 {
				buf = append(buf, 0)
			}
		}
	}
	return buf
}
