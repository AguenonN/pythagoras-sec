package directive

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"
)

// La pureté de Decide est une propriété vérifiée, pas seulement documentée :
// aucun fichier non-test du paquet n'importe un paquet d'I/O ni ne lit
// l'horloge. Un jeu, un état, un appel : la même décision, aujourd'hui et
// au rejeu.
func TestPurete_AucuneIONiHorloge(t *testing.T) {
	interdits := map[string]bool{
		"os": true, "net": true, "net/http": true, "log": true, "io": true,
		"io/fs": true, "os/exec": true, "syscall": true,
	}
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) == 0 {
		t.Fatal("aucun fichier source trouvé")
	}
	fichiers := 0
	for _, pkg := range pkgs {
		for nom, f := range pkg.Files {
			fichiers++
			for _, imp := range f.Imports {
				chemin := strings.Trim(imp.Path.Value, `"`)
				if interdits[chemin] {
					t.Errorf("%s importe %s : le paquet directive doit rester sans I/O", nom, chemin)
				}
			}
			ast.Inspect(f, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if x, ok := sel.X.(*ast.Ident); ok && x.Name == "time" && sel.Sel.Name == "Now" {
					t.Errorf("%s appelle time.Now : l'horodatage vient de l'appel, jamais de l'horloge",
						fset.Position(sel.Pos()))
				}
				return true
			})
		}
	}
	if fichiers < 4 {
		t.Fatalf("%d fichiers inspectés, le garde ne voit pas le paquet", fichiers)
	}
}
