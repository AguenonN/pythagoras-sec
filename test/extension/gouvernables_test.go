package extension

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// La section « Concevoir des outils gouvernables » est en tête de la recette
// serveur, avec ses six règles, chacune avec son mauvais et son bon exemple ;
// la documentation de docs/equipe renvoie vers etendre.md.
func TestDocOutilsGouvernables(t *testing.T) {
	r := racine(t)
	doc := lire(t, filepath.Join(r, "docs", "demo", "etendre.md"))
	i := strings.Index(doc, "### Concevoir des outils gouvernables")
	j := strings.Index(doc, "Un serveur = deux fichiers JSON")
	k := strings.Index(doc, "```sh recette-serveur")
	if i < 0 || j < i || k < j {
		t.Fatalf("la section doit précéder la recette serveur (positions %d %d %d)", i, j, k)
	}
	section := doc[i:j]
	for _, regle := range []string{"préparer et agir", "Un outil par effet irréversible", "Arguments explicites", "Étiqueter les champs sensibles", "`interne` ou `externe`", "`readOnlyHint` juste"} {
		if !strings.Contains(section, regle) {
			t.Errorf("règle absente : %s", regle)
		}
	}
	if n := strings.Count(section, "Mauvais :"); n < 6 {
		t.Errorf("%d exemples « Mauvais », 6 attendus", n)
	}
	if n := strings.Count(section, "Bon :"); n < 6 {
		t.Errorf("%d exemples « Bon », 6 attendus", n)
	}
	equipe := lire(t, filepath.Join(r, "docs", "equipe", "ajouter-un-serveur.md"))
	if !strings.Contains(equipe, "docs/demo/etendre.md") || strings.Contains(equipe, "lire_fiche") || strings.Contains(equipe, "registredemo.go") {
		t.Error("docs/equipe/ajouter-un-serveur.md doit renvoyer vers etendre.md et ne plus décrire l'ancienne procédure")
	}
	entrees, _ := os.ReadDir(filepath.Join(r, "docs", "equipe"))
	for _, e := range entrees {
		if b := lire(t, filepath.Join(r, "docs", "equipe", e.Name())); strings.Contains(b, "nouveau-serveur.sh <nom>        #") {
			t.Errorf("%s décrit l'ancien nouveau-serveur.sh", e.Name())
		}
	}
}

// Un outil d'action sans readOnlyHint explicite est refusé ; un argument
// nommé instructions, texte ou commande sur un outil d'action est signalé
// sans bloquer ; sur une lecture, il ne l'est pas.
func TestServeurOutilsGouvernables(t *testing.T) {
	if imbrique() {
		t.Skip()
	}
	dir := copierDepot(t)
	donnees := "servers/modele-synthetique/domaines/assurance/donnees.json"
	ecrire := func(nom, contenu string) string {
		p := filepath.Join(t.TempDir(), nom)
		if err := os.WriteFile(p, []byte(contenu), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	sansAnnotation := ecrire("a.json", `{"outils":[{"nom":"payer","description":"d","genre":"action","reference_prefixe":"PAIEMENT-FICTIF"}]}`)
	sortie, err := essayer(dir, "scripts/nouveau-serveur.sh gouv-refus "+donnees+" "+sansAnnotation)
	if err == nil || !strings.Contains(sortie, "readOnlyHint obligatoire") {
		t.Fatalf("action sans readOnlyHint acceptée :\n%s", sortie)
	}
	if _, err := os.Stat(filepath.Join(dir, "servers", "gouv-refus")); err == nil {
		t.Error("servers/gouv-refus a été créé")
	}

	faible := ecrire("b.json", `{"outils":[
	 {"nom":"envoyer","description":"d","genre":"action","readOnlyHint":false,"reference_prefixe":"ENVOI-FICTIF","arguments":[
	  {"nom":"instructions","type":"string","description":"d"},{"nom":"commande","type":"string","description":"d"},{"nom":"destinataire","type":"string","description":"d"}]},
	 {"nom":"chercher","description":"d","genre":"lister","readOnlyHint":true,"collection":"sinistres","arguments":[{"nom":"statut","type":"string","description":"d"}]}]}`)
	sortie = bash(t, dir, "scripts/nouveau-serveur.sh gouv-avertit "+donnees+" "+faible)
	for _, a := range []string{`l'argument "instructions"`, `l'argument "commande"`} {
		if !strings.Contains(sortie, "AVERTISSEMENT") || !strings.Contains(sortie, a) {
			t.Errorf("avertissement attendu pour %s :\n%s", a, sortie)
		}
	}
	if strings.Contains(sortie, `l'argument "destinataire"`) || strings.Contains(sortie, `outil d'action "chercher"`) {
		t.Errorf("avertissement à tort :\n%s", sortie)
	}
	if _, err := os.Stat(filepath.Join(dir, "servers", "gouv-avertit", "schema.json")); err != nil {
		t.Errorf("le serveur averti doit être créé : %v", err)
	}
}
