package main

// Textos fixos no código do jogo: literais de string do IL2CPP (global-metadata.dat, versão 27).
//
// Alguns diálogos não vêm de nenhum asset: o C# monta o texto direto, por exemplo
// new DialogueClass(..., "还没到下班的时候。", "まだ仕事が終わってない。", "Your work is not done.", ...).
// O IL2CPP guarda esses textos no global-metadata.dat:
//   - Cabeçalho: magic u32 @0 (0xFAB11BAF), versão i32 @4, depois pares (offset, tamanho) i32:
//     tabela de literais @8/@12 e dados dos literais @16/@20.
//   - Tabela: entradas de 8 bytes [u32 length][i32 dataIndex], dataIndex relativo ao início dos dados.
//
// Só os literais de literals.json são traduzidos. O metadata também tem nomes de cena, chaves,
// mensagens de bibliotecas... e cada texto existe uma vez só (o literal é compartilhado por todo
// o código), então a lista tem apenas textos exibidos que não são usados em comparações.
//
// O patch grava o texto novo no fim do arquivo e aponta a entrada da tabela para lá; o runtime
// lê o literal por (início dos dados + dataIndex, length), sem conferir limites.

import (
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const literalsAsset = "code_literals"

//go:embed literals.json
var literalsJSON []byte

type Metadata struct {
	Path     string
	Raw      []byte
	TableOff int
	Count    int
	DataOff  int
	index    map[string]int // texto -> índice do literal
}

func metadataPath(dataDir string) string {
	return filepath.Join(dataDir, "il2cpp_data", "Metadata", "global-metadata.dat")
}

func OpenMetadata(path string) (*Metadata, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) < 64 || binary.LittleEndian.Uint32(raw) != 0xFAB11BAF {
		return nil, fmt.Errorf("%s: não é um global-metadata.dat", path)
	}
	if v := int32(binary.LittleEndian.Uint32(raw[4:])); v != 27 {
		return nil, fmt.Errorf("%s: versão %d inesperada (esperava 27)", path, v)
	}
	i32 := func(off int) int { return int(int32(binary.LittleEndian.Uint32(raw[off:]))) }
	m := &Metadata{Path: path, Raw: raw, TableOff: i32(8), Count: i32(12) / 8, DataOff: i32(16), index: map[string]int{}}
	if m.TableOff+m.Count*8 > len(raw) || m.DataOff+i32(20) > len(raw) {
		return nil, fmt.Errorf("%s: tabela de literais fora do arquivo", path)
	}
	for i := 0; i < m.Count; i++ {
		if _, dup := m.index[m.Literal(i)]; !dup {
			m.index[m.Literal(i)] = i
		}
	}
	return m, nil
}

func (m *Metadata) Literal(i int) string {
	at := m.TableOff + i*8
	n := int(binary.LittleEndian.Uint32(m.Raw[at:]))
	start := m.DataOff + int(int32(binary.LittleEndian.Uint32(m.Raw[at+4:])))
	return string(m.Raw[start : start+n])
}

// LiteralsDoc devolve o asset "rows" com os literais traduzíveis (chave = texto original).
func (m *Metadata) LiteralsDoc() (*Object, error) {
	var texts []string
	if err := json.Unmarshal(literalsJSON, &texts); err != nil {
		return nil, fmt.Errorf("literals.json: %v", err)
	}
	doc := &Object{}
	for _, s := range texts {
		if _, ok := m.index[s]; !ok {
			warn("literal não encontrado no global-metadata.dat: %q", s)
			continue
		}
		doc.Pairs = append(doc.Pairs, Pair{s, &Object{Pairs: []Pair{{"EN", s}}}})
	}
	return doc, nil
}

// Patched devolve uma cópia do metadata com os literais trocados pelo EN do doc
// (onde o patch já gravou o PT), e quantos literais mudaram.
func (m *Metadata) Patched(doc *Object) ([]byte, int) {
	buf := append([]byte(nil), m.Raw...)
	n := 0
	for _, p := range doc.Pairs {
		i, ok := m.index[p.Key]
		row, isRow := p.Val.(*Object)
		if !ok || !isRow || row.GetString("EN") == p.Key {
			continue
		}
		text := row.GetString("EN")
		at := m.TableOff + i*8
		binary.LittleEndian.PutUint32(buf[at:], uint32(len(text)))
		binary.LittleEndian.PutUint32(buf[at+4:], uint32(len(buf)-m.DataOff))
		buf = append(buf, text...)
		n++
	}
	return buf, n
}
