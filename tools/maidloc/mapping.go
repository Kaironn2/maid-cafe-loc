package main

// Mapeamento dos assets: em quais arquivos, idiomas e campos está o texto traduzível.
//
// Dois formatos de texto existem no jogo:
//   - "dict": um asset por idioma (ui_en, ui_jp...), cada um {chave: texto}.
//     Traduzimos o asset *_en.
//   - "rows": um asset com todas as línguas em cada linha, {id: {..., "EN": texto}}.
//     Traduzimos o campo inglês (EN, EnText ou EnName).

import (
	"regexp"
	"sort"
	"strings"
)

const (
	KindDict = "dict"
	KindRows = "rows"
)

type Unit struct {
	Asset    string   `json:"asset"`
	Category string   `json:"category"`
	Kind     string   `json:"format,omitempty"`
	Field    string   `json:"field,omitempty"`
	Langs    []string `json:"languages,omitempty"`
	Keys     int      `json:"keys"`
	Texts    int      `json:"translatable,omitempty"`
	Chars    int      `json:"en_chars,omitempty"`
	File     string   `json:"translation_file,omitempty"`
}

// Famílias de campos de idioma usadas nas linhas (rows), em ordem cn/jp/en/tc.
var rowLangFields = [][4]string{
	{"StoryText", "JP", "EN", "TC"},
	{"CnText", "JpText", "EnText", "TcText"},
	{"LKName", "JpName", "EnName", ""},
}
var langCodes = [4]string{"cn", "jp", "en", "tc"}

var dictLangRe = regexp.MustCompile(`(?i)^(.+)_(cn|jp|en|tc)$`)

// MapAssets classifica todos os assets JSON e marca os que têm texto traduzível.
func MapAssets(docs map[string]any) []Unit {
	var units []Unit
	for name, root := range docs {
		u := Unit{Asset: name, Category: "data", Keys: countKeys(root)}
		if m := dictLangRe.FindStringSubmatch(name); m != nil && isStringDict(root) {
			u.Category = "languages"
			for _, code := range langCodes {
				if _, ok := docs[m[1]+"_"+code]; ok {
					u.Langs = append(u.Langs, code)
				}
			}
			if strings.EqualFold(m[2], "en") {
				u.Kind = KindDict
				u.File = "texts/" + name + ".json"
			}
		} else if obj, ok := root.(*Object); ok {
			if hasField(obj, "ChatNum") {
				u.Category = "scenes"
			}
			if fam := rowFamily(obj); fam >= 0 {
				u.Kind = KindRows
				u.Field = rowLangFields[fam][2]
				for i, f := range rowLangFields[fam] {
					if f != "" && hasField(obj, f) {
						u.Langs = append(u.Langs, langCodes[i])
					}
				}
				if u.Category == "scenes" {
					u.File = "scenes/" + name + ".json"
				} else {
					u.File = "extras/" + name + ".json"
				}
			}
		}
		if u.Kind != "" {
			for _, e := range SourceEntries(u, root) {
				u.Texts++
				u.Chars += len([]rune(e.EN))
			}
			if u.Texts == 0 {
				u.Kind, u.Field, u.File = "", "", ""
			}
		}
		units = append(units, u)
	}
	sort.Slice(units, func(i, j int) bool {
		if units[i].Category != units[j].Category {
			return units[i].Category < units[j].Category
		}
		return units[i].Asset < units[j].Asset
	})
	return units
}

// SourceEntry é um texto inglês traduzível dentro de um asset.
type SourceEntry struct {
	Key       string
	EN        string
	Character string // só nas linhas que têm "Character"
}

func SourceEntries(u Unit, root any) []SourceEntry {
	obj, ok := root.(*Object)
	if !ok {
		return nil
	}
	var out []SourceEntry
	for _, p := range obj.Pairs {
		switch u.Kind {
		case KindDict:
			if s, _ := p.Val.(string); strings.TrimSpace(s) != "" {
				out = append(out, SourceEntry{Key: p.Key, EN: s})
			}
		case KindRows:
			row, ok := p.Val.(*Object)
			if !ok {
				continue
			}
			if s := row.GetString(u.Field); strings.TrimSpace(s) != "" {
				out = append(out, SourceEntry{Key: p.Key, EN: s, Character: row.GetString("Character")})
			}
		}
	}
	return out
}

// ApplyPT grava as traduções (chave -> PT) no asset original.
func ApplyPT(u Unit, root any, pt map[string]string) int {
	obj, ok := root.(*Object)
	if !ok {
		return 0
	}
	n := 0
	for i, p := range obj.Pairs {
		t, ok := pt[p.Key]
		if !ok {
			continue
		}
		switch u.Kind {
		case KindDict:
			obj.Pairs[i].Val = t
			n++
		case KindRows:
			if row, ok := p.Val.(*Object); ok {
				row.Set(u.Field, t)
				n++
			}
		}
	}
	return n
}

// CharacterNames resolve a chave do personagem (ex.: "xinyou") para o nome exibido em inglês.
func CharacterNames(docs map[string]any) map[string]string {
	names := map[string]string{}
	if o, ok := docs["charactername_en"].(*Object); ok {
		for _, p := range o.Pairs {
			if s, _ := p.Val.(string); s != "" {
				names[p.Key] = s
			}
		}
	}
	if o, ok := docs["textdata_en"].(*Object); ok {
		for _, p := range o.Pairs {
			if k, found := strings.CutPrefix(p.Key, "CharacterName_"); found {
				if s, _ := p.Val.(string); s != "" {
					names[k] = s
				}
			}
		}
	}
	return names
}

func isStringDict(root any) bool {
	obj, ok := root.(*Object)
	if !ok || len(obj.Pairs) == 0 {
		return false
	}
	for _, p := range obj.Pairs {
		if _, ok := p.Val.(string); !ok {
			return false
		}
	}
	return true
}

// hasField diz se alguma linha do objeto tem o campo.
func hasField(obj *Object, field string) bool {
	for _, p := range obj.Pairs {
		if row, ok := p.Val.(*Object); ok {
			if _, ok := row.Get(field); ok {
				return true
			}
		}
	}
	return false
}

func rowFamily(obj *Object) int {
	for i, fam := range rowLangFields {
		if hasField(obj, fam[2]) {
			return i
		}
	}
	return -1
}

func countKeys(root any) int {
	switch t := root.(type) {
	case *Object:
		return len(t.Pairs)
	case []any:
		return len(t)
	}
	return 0
}
