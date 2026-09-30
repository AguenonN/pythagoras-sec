package directive

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// La pureté de Decide est une propriété vérifiée, pas seulement documentée :
// aucun fichier non-test du paquet n'importe un paquet d'I/O ni ne lit
// l'horloge. Un jeu, un état, un appel : la même décision, aujourd'hui et
// au rejeu.
func TestPurete_AucuneIONiHorloge(t *testing.T) {
	// Étendue à tout internal/directive (sous-répertoires compris) et aux
	// sources d'indéterminisme : aléa, expressions régulières, fuseau du
	// système, horloge sous toutes ses formes.
	interdits := map[string]bool{
		"os": true, "net": true, "net/http": true, "log": true, "io": true,
		"io/fs": true, "os/exec": true, "syscall": true, "io/ioutil": true,
		"math/rand": true, "math/rand/v2": true, "crypto/rand": true,
		"regexp": true, "time/tzdata": true, "unsafe": true, "runtime": true,
		"plugin": true, "reflect": true, "context": true,
	}
	horloge := map[string]bool{
		"Now": true, "Since": true, "Until": true, "LoadLocation": true, "Local": true,
		"Sleep": true, "Tick": true, "After": true, "AfterFunc": true, "NewTimer": true, "NewTicker": true,
	}
	fset := token.NewFileSet()
	fichiers := 0
	err := filepath.WalkDir(".", func(chemin string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() || !strings.HasSuffix(chemin, ".go") || strings.HasSuffix(chemin, "_test.go") {
			return err
		}
		f, err := parser.ParseFile(fset, chemin, nil, 0)
		if err != nil {
			return err
		}
		fichiers++
		for _, imp := range f.Imports {
			if c := strings.Trim(imp.Path.Value, `"`); interdits[c] {
				t.Errorf("%s importe %s : le paquet directive doit rester pur", chemin, c)
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if x, ok := sel.X.(*ast.Ident); ok && x.Name == "time" && horloge[sel.Sel.Name] {
				t.Errorf("%s emploie time.%s : l'horodatage vient de l'appel, le fuseau de la base embarquée",
					fset.Position(sel.Pos()), sel.Sel.Name)
			}
			if x, ok := sel.X.(*ast.Ident); ok && x.Name == "json" && sel.Sel.Name == "Unmarshal" {
				// Le chargeur décode par directive_json.go en json.Number ;
				// un Unmarshal nu rendrait des float64.
				if !strings.HasSuffix(chemin, "directive_json.go") && !strings.HasSuffix(chemin, "charger.go") {
					t.Errorf("%s emploie json.Unmarshal hors du chargeur", fset.Position(sel.Pos()))
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if fichiers < 10 {
		t.Fatalf("%d fichiers inspectés, le garde ne voit pas le paquet", fichiers)
	}
}

// Decide ne lit aucune entrée hors de ses arguments : même décision avec un
// autre fuseau de machine (time.Local modifié en cours d'exécution).
func TestPurete_FuseauMachineIgnore(t *testing.T) {
	j := jeuV2(t, "weekend")
	a := appelWeekend("2026-10-02T22:00:00Z", AccesAction)
	ref := Decide(j, EtatSession{}, a)
	ancien := time.Local
	defer func() { time.Local = ancien }()
	time.Local = time.FixedZone("ailleurs", -11*3600)
	if d := Decide(j, EtatSession{}, a); d.Effet != ref.Effet || d.Motif != ref.Motif {
		t.Fatalf("la décision dépend de time.Local : %s puis %s", ref.Effet, d.Effet)
	}
}
