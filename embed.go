// Package maidcafeloc embute os arquivos de tradução no binário do instalador.
package maidcafeloc

import (
	"embed"
	"io/fs"
)

//go:embed translations/*/*.json
var files embed.FS

// Translations é a pasta translations/, com caminhos como "texts/ui_en.json".
var Translations, _ = fs.Sub(files, "translations")
