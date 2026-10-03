package extension

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// instantane liste les fichiers de dir avec leur taille : deux instantanés
// égaux, rien n'a été écrit.
func instantane(t *testing.T, dir string) string {
	t.Helper()
	var l []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		i, err := d.Info()
		if err != nil {
			return err
		}
		l = append(l, p[len(dir):]+" "+i.ModTime().String()+" "+string(rune(i.Size()%1000+0x100)))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(l)
	return strings.Join(l, "\n")
}

// Un nom non conforme à internal/nommage, un fichier invalide ou une
// étiquette inconnue sont refusés avant toute écriture : le dépôt ne change
// pas.
func TestServeurRefus(t *testing.T) {
	if imbrique() {
		t.Skip()
	}
	dir := copierDepot(t)
	d := "servers/modele-synthetique/domaines/assurance"
	ecrire := func(nom, contenu string) string {
		p := filepath.Join(t.TempDir(), nom)
		if err := os.WriteFile(p, []byte(contenu), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	donnees, outils := d+"/donnees.json", d+"/outils.json"
	cas := []struct{ nom, script, message string }{
		{"majuscules", "scripts/nouveau-serveur.sh Assurance " + donnees + " " + outils, "non conforme"},
		{"double soulignement", "scripts/nouveau-serveur.sh assurance__x " + donnees + " " + outils, "non conforme"},
		{"tiret final", "scripts/nouveau-serveur.sh assurance- " + donnees + " " + outils, "non conforme"},
		{"espace", "scripts/nouveau-serveur.sh 'mon serveur' " + donnees + " " + outils, "non conforme"},
		{"déjà existant", "scripts/nouveau-serveur.sh banque-synthetique " + donnees + " " + outils, "existe déjà"},
		{"nom réservé", "scripts/nouveau-serveur.sh modele-synthetique " + donnees + " " + outils, "réservé"},
		{"données réelles", "scripts/nouveau-serveur.sh ok-serveur " + ecrire("d.json", `{"avertissement":"clients de la banque","collections":{"c":[{"id":"x"}]}}`) + " " + outils, "fictives"},
		{"outil sans readOnlyHint", "scripts/nouveau-serveur.sh ok-serveur " + donnees + " " + ecrire("o.json", `{"outils":[{"nom":"a","description":"d","genre":"lister","collection":"sinistres"}]}`), "readOnlyHint"},
		{"collection inconnue", "scripts/nouveau-serveur.sh ok-serveur " + donnees + " " + ecrire("o2.json", `{"outils":[{"nom":"a","description":"d","genre":"lister","readOnlyHint":true,"collection":"nulle"}]}`), "collection"},
		{"étiquette sur un outil inconnu", "scripts/nouveau-serveur.sh ok-serveur " + donnees + " " + outils + " " + ecrire("e.json", `{"outils":{"fantome":["x"]}}`), "fantome"},
		{"nom d'étiquette invalide", "scripts/nouveau-serveur.sh ok-serveur " + donnees + " " + outils + " " + ecrire("e2.json", `{"serveur":["Mauvaise Etiquette"]}`), "étiquette"},
		{"fichier absent", "scripts/nouveau-serveur.sh ok-serveur absent.json " + outils, "introuvable"},
		{"arguments manquants", "scripts/nouveau-serveur.sh ok-serveur", "usage"},
		{"régénérer un serveur absent", "scripts/nouveau-serveur.sh --regenerer nexiste-pas", "n'existe pas"},
	}
	avant := instantane(t, dir)
	for _, c := range cas {
		t.Run(c.nom, func(t *testing.T) {
			sortie, err := essayer(dir, c.script)
			if err == nil {
				t.Fatalf("accepté :\n%s", sortie)
			}
			if !strings.Contains(sortie, c.message) {
				t.Errorf("message attendu %q :\n%s", c.message, sortie)
			}
			if instantane(t, dir) != avant {
				t.Error("le dépôt a changé")
			}
		})
	}
}

func TestScenarioRefus(t *testing.T) {
	if imbrique() {
		t.Skip()
	}
	dir := copierDepot(t)
	cas := []struct{ nom, script, message string }{
		{"nom invalide", "scripts/nouveau-scenario.sh Mon_Scenario", "non conforme"},
		{"sans nom", "scripts/nouveau-scenario.sh", "scripts/nouveau-scenario.sh <nom>"},
		{"demandeur inconnu", "scripts/nouveau-scenario.sh x --demandeur inconnu@exemple.test", "inconnu"},
		{"sous-agent absent", "scripts/nouveau-scenario.sh x --sous-agents fantome", "fantome"},
		{"effet inconnu", "scripts/nouveau-scenario.sh x --decision banque-synthetique/attribuer_pret=supprimer", "effet"},
		{"statut inconnu", "scripts/nouveau-scenario.sh x --statut fini", "statut"},
		{"directive mal formée", "scripts/nouveau-scenario.sh x --directives D9", "D-009"},
		{"option inconnue", "scripts/nouveau-scenario.sh x --truc 1", "inconnue"},
		{"scénario existant", "scripts/nouveau-scenario.sh nominal", "existe déjà"},
	}
	avant := instantane(t, dir)
	for _, c := range cas {
		t.Run(c.nom, func(t *testing.T) {
			sortie, err := essayer(dir, c.script)
			if err == nil {
				t.Fatalf("accepté :\n%s", sortie)
			}
			if !strings.Contains(sortie, c.message) {
				t.Errorf("message attendu %q :\n%s", c.message, sortie)
			}
			if instantane(t, dir) != avant {
				t.Error("le dépôt a changé")
			}
		})
	}
}

// Un scénario sans option est un gabarit à compléter ; son test est ignoré
// tant qu'il n'est pas enregistré.
func TestScenarioGabarit(t *testing.T) {
	if imbrique() {
		t.Skip()
	}
	dir := copierDepot(t)
	bash(t, dir, "scripts/nouveau-scenario.sh mon-scenario")
	if s := lire(t, filepath.Join(dir, "demo", "scenarios", "mon-scenario.json")); !strings.Contains(s, "À REMPLACER") || !strings.Contains(s, `"directives": null`) {
		t.Errorf("gabarit :\n%s", s)
	}
	sortie := bash(t, dir, "go test ./test/extension -run Scenario_mon_scenario -v -count=1", "EXTENSION_IMBRIQUE=1")
	if !strings.Contains(sortie, "SKIP") {
		t.Errorf("le test du gabarit devait être ignoré :\n%s", sortie)
	}
}

// Le second domaine fourni (contentieux) crée lui aussi un serveur complet,
// avec ses étiquettes, sans toucher aux fichiers de la démo figée.
func TestDomaineContentieux(t *testing.T) {
	if imbrique() {
		t.Skip()
	}
	dir := copierDepot(t)
	etiquettes := lire(t, filepath.Join(dir, "demo", "etiquettes.json"))
	bash(t, dir, `D=servers/modele-synthetique/domaines/contentieux
scripts/nouveau-serveur.sh cabinet-contentieux $D/donnees.json $D/outils.json $D/etiquettes.json`)
	supp := lire(t, filepath.Join(dir, "demo", "serveurs-supplementaires.json"))
	if !strings.Contains(supp, `"cabinet-contentieux/deposer_conclusions": ["externe"]`) {
		t.Errorf("étiquettes :\n%s", supp)
	}
	if lire(t, filepath.Join(dir, "demo", "etiquettes.json")) != etiquettes {
		t.Error("demo/etiquettes.json a changé")
	}
	// Relancé avec --regenerer, rien ne change.
	avant := lire(t, filepath.Join(dir, "servers", "cabinet-contentieux", "schema.json"))
	bash(t, dir, "scripts/nouveau-serveur.sh --regenerer cabinet-contentieux")
	if lire(t, filepath.Join(dir, "servers", "cabinet-contentieux", "schema.json")) != avant {
		t.Error("schema.json non reproductible")
	}
}
