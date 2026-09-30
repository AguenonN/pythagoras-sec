package console

import (
	"embed"
	"io/fs"
	"sort"
)

// Tous les fichiers de l'interface sont embarqués : aucune ressource n'est
// chargée depuis le réseau.
//
//go:embed web
var webFS embed.FS

// exemples : jeux de directives fournis avec la console, pour que l'écran
// avant / après ait toujours deux jeux à comparer. credit-demo-v1.json est
// une copie de demo/directives/credit-demo.json.
//
//go:embed exemples
var exemples embed.FS

func nomsExemples() []string {
	ents, _ := fs.ReadDir(exemples, "exemples")
	var out []string
	for _, e := range ents {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}
