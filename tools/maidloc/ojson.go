package main

// JSON que preserva a ordem das chaves (os arquivos do jogo e os de tradução
// precisam manter a ordem original para continuarem legíveis e comparáveis).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"unicode/utf8"
)

type Pair struct {
	Key string
	Val any
}

type Object struct {
	Pairs []Pair
}

func (o *Object) Get(key string) (any, bool) {
	for _, p := range o.Pairs {
		if p.Key == key {
			return p.Val, true
		}
	}
	return nil, false
}

func (o *Object) GetString(key string) string {
	v, _ := o.Get(key)
	s, _ := v.(string)
	return s
}

// Set altera todas as ocorrências da chave, ou acrescenta no fim.
func (o *Object) Set(key string, val any) {
	found := false
	for i := range o.Pairs {
		if o.Pairs[i].Key == key {
			o.Pairs[i].Val = val
			found = true
		}
	}
	if !found {
		o.Pairs = append(o.Pairs, Pair{key, val})
	}
}

func ParseJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(trimBOM(data)))
	dec.UseNumber()
	v, err := parseValue(dec)
	if err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, fmt.Errorf("conteúdo extra depois do JSON")
	}
	return v, nil
}

func parseValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			o := &Object{}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				v, err := parseValue(dec)
				if err != nil {
					return nil, err
				}
				o.Pairs = append(o.Pairs, Pair{kt.(string), v})
			}
			_, err := dec.Token()
			return o, err
		case '[':
			var arr []any
			for dec.More() {
				v, err := parseValue(dec)
				if err != nil {
					return nil, err
				}
				arr = append(arr, v)
			}
			_, err := dec.Token()
			if arr == nil {
				arr = []any{}
			}
			return arr, err
		}
		return nil, fmt.Errorf("delimitador inesperado %v", t)
	default:
		return tok, nil // string, json.Number, bool, nil
	}
}

// EncodeJSON serializa em UTF-8 puro (sem \uXXXX). indent=false gera JSON compacto.
func EncodeJSON(v any, indent bool) []byte {
	var b bytes.Buffer
	encodeValue(&b, v, indent, 0)
	if indent {
		b.WriteByte('\n')
	}
	return b.Bytes()
}

func encodeValue(b *bytes.Buffer, v any, indent bool, depth int) {
	newline := func(d int) {
		if indent {
			b.WriteByte('\n')
			for i := 0; i < d; i++ {
				b.WriteString("  ")
			}
		}
	}
	switch t := v.(type) {
	case *Object:
		if len(t.Pairs) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteByte('{')
		for i, p := range t.Pairs {
			if i > 0 {
				b.WriteByte(',')
			}
			newline(depth + 1)
			encodeString(b, p.Key)
			b.WriteByte(':')
			if indent {
				b.WriteByte(' ')
			}
			encodeValue(b, p.Val, indent, depth+1)
		}
		newline(depth)
		b.WriteByte('}')
	case []any:
		if len(t) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				b.WriteByte(',')
			}
			newline(depth + 1)
			encodeValue(b, e, indent, depth+1)
		}
		newline(depth)
		b.WriteByte(']')
	case string:
		encodeString(b, t)
	case json.Number:
		b.WriteString(t.String())
	case bool:
		b.WriteString(strconv.FormatBool(t))
	case int:
		b.WriteString(strconv.Itoa(t))
	case nil:
		b.WriteString("null")
	default:
		panic(fmt.Sprintf("tipo JSON não suportado: %T", v))
	}
}

func encodeString(b *bytes.Buffer, s string) {
	b.WriteByte('"')
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			switch c {
			case '"':
				b.WriteString(`\"`)
			case '\\':
				b.WriteString(`\\`)
			case '\n':
				b.WriteString(`\n`)
			case '\r':
				b.WriteString(`\r`)
			case '\t':
				b.WriteString(`\t`)
			default:
				if c < 0x20 {
					fmt.Fprintf(b, `\u%04x`, c)
				} else {
					b.WriteByte(c)
				}
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			b.WriteString(`�`)
		} else {
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	b.WriteByte('"')
}
