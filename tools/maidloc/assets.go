package main

// Leitura e escrita do resources.assets (SerializedFile v22) sem reconstruir o arquivo.
//
// Estrutura confirmada por engenharia reversa:
//   - Cabeçalho: version u32 BE @8 (22), fileSize u32 BE @28, dataOffset u32 BE @36
//   - Object info table: entradas de 24 bytes [u64 offsetRelativo][u32 sizeTotal][u32 typeId][i64 pathId]
//   - offsetRelativo = posição do objeto no arquivo menos dataOffset
//   - TextAsset: [u32 nameLen][nome][pad4][u32 dataLen][payload][pad4]
//
// O patch grava cada objeto novo no FIM do arquivo (alinhado) e atualiza a entrada
// da tabela e o fileSize. O espaço antigo fica órfão; por isso o patch sempre parte
// do arquivo original (.bak), e o arquivo não cresce a cada aplicação.

import (
	"encoding/binary"
	"fmt"
	"os"
	"sort"
)

type AssetFile struct {
	Path       string
	Raw        []byte
	DataOffset int
	FileSize   int
	TableStart int
	Count      int
}

type TextAsset struct {
	Name    string
	TableAt int // posição da entrada na object table
	Payload []byte
}

func OpenAssets(path string) (*AssetFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) < 64 {
		return nil, fmt.Errorf("%s: arquivo pequeno demais", path)
	}
	be32 := func(off int) int { return int(binary.BigEndian.Uint32(raw[off : off+4])) }
	if v := be32(8); v != 22 {
		return nil, fmt.Errorf("%s: versão %d inesperada (esperava 22)", path, v)
	}
	a := &AssetFile{Path: path, Raw: raw, DataOffset: be32(36), FileSize: be32(28)}
	a.TableStart, a.Count = findObjectTable(raw, a.DataOffset)
	if a.TableStart < 0 || a.Count == 0 {
		return nil, fmt.Errorf("%s: object table não encontrada", path)
	}
	return a, nil
}

// TextAssets devolve os objetos cujo payload é JSON (primeira ocorrência de cada nome).
func (a *AssetFile) TextAssets() []TextAsset {
	raw := a.Raw
	seen := map[string]bool{}
	var out []TextAsset
	for e := 0; e < a.Count; e++ {
		at := a.TableStart + e*24
		off := int(binary.LittleEndian.Uint64(raw[at : at+8]))
		size := int(binary.LittleEndian.Uint32(raw[at+8 : at+12]))
		obj := off + a.DataOffset
		if obj < 0 || obj+8 > len(raw) || obj+size > len(raw) {
			continue
		}
		nameLen := int(int32(binary.LittleEndian.Uint32(raw[obj : obj+4])))
		if nameLen <= 0 || nameLen > 200 || 4+alignUp(nameLen, 4)+4 > size {
			continue
		}
		name := string(raw[obj+4 : obj+4+nameLen])
		lenAt := obj + 4 + alignUp(nameLen, 4)
		dataLen := int(int32(binary.LittleEndian.Uint32(raw[lenAt : lenAt+4])))
		start := lenAt + 4
		if dataLen < 2 || start+dataLen > obj+size {
			continue
		}
		payload := trimBOM(raw[start : start+dataLen])
		if len(payload) == 0 || (payload[0] != '{' && payload[0] != '[') {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, TextAsset{Name: name, TableAt: at, Payload: payload})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Patched devolve uma cópia do arquivo com os payloads substituídos (por nome de asset).
func (a *AssetFile) Patched(assets []TextAsset, payloads map[string][]byte) []byte {
	byName := map[string]TextAsset{}
	for _, t := range assets {
		byName[t.Name] = t
	}
	names := make([]string, 0, len(payloads))
	for n := range payloads {
		names = append(names, n)
	}
	sort.Strings(names)

	size := a.FileSize
	if size > len(a.Raw) {
		size = len(a.Raw)
	}
	extra := 0
	for _, n := range names {
		extra += 16 + 8 + alignUp(len(n), 4) + alignUp(len(payloads[n]), 4)
	}
	buf := make([]byte, size, size+extra)
	copy(buf, a.Raw[:size])

	for _, n := range names {
		t, ok := byName[n]
		if !ok {
			continue
		}
		payload := payloads[n]
		for len(buf)%16 != 0 {
			buf = append(buf, 0)
		}
		objAt := len(buf)
		buf = binary.LittleEndian.AppendUint32(buf, uint32(len(n)))
		buf = append(buf, n...)
		for len(buf)%4 != 0 {
			buf = append(buf, 0)
		}
		buf = binary.LittleEndian.AppendUint32(buf, uint32(len(payload)))
		buf = append(buf, payload...)
		for len(buf)%4 != 0 {
			buf = append(buf, 0)
		}
		binary.LittleEndian.PutUint64(buf[t.TableAt:t.TableAt+8], uint64(objAt-a.DataOffset))
		binary.LittleEndian.PutUint32(buf[t.TableAt+8:t.TableAt+12], uint32(len(buf)-objAt))
	}
	binary.BigEndian.PutUint32(buf[28:32], uint32(len(buf)))
	return buf
}

func findObjectTable(raw []byte, dataOffset int) (int, int) {
	bestStart, bestCount := -1, 0
	curStart, curCount := -1, 0
	for i := 100; i+24 <= len(raw); i += 4 {
		off := binary.LittleEndian.Uint64(raw[i : i+8])
		size := binary.LittleEndian.Uint32(raw[i+8 : i+12])
		typ := binary.LittleEndian.Uint32(raw[i+16 : i+20])
		valid := off < uint64(len(raw)) && size > 16 &&
			int(off)+dataOffset+int(size) <= len(raw) && typ <= 600
		if valid {
			if curStart < 0 {
				curStart = i
			}
			curCount++
			if curCount > bestCount {
				bestStart, bestCount = curStart, curCount
			}
		} else {
			curStart, curCount = -1, 0
		}
	}
	if bestStart < 0 {
		return -1, 0
	}
	// reconta a partir do melhor início, de 24 em 24
	count := 0
	for at := bestStart; at+24 <= len(raw); at += 24 {
		off := binary.LittleEndian.Uint64(raw[at : at+8])
		size := binary.LittleEndian.Uint32(raw[at+8 : at+12])
		if off >= uint64(len(raw)) || int(off)+dataOffset+int(size) > len(raw) {
			break
		}
		count++
	}
	return bestStart, count
}

func alignUp(n, a int) int { return (n + a - 1) &^ (a - 1) }

func trimBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}
