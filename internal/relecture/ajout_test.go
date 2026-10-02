package relecture

import (
	"strings"
	"testing"

	"mcprogue/internal/directive"
)

const jeuAjout = `{"jeu": {"id": "demo", "version": 2, "date_effet": "2026-09-01T00:00:00Z", "auteur": "j@exemple.test"},
 "directives": [
  {"id": "D-001", "version": 1, "statut": "active", "date_effet": "2026-09-01T00:00:00Z", "texte_fr": "Existante.",
   "fondements": [{"texte": "RGPD", "article": "5"}],
   "declencheur": {"evenement": "appel_outil", "cible": {"serveur": "banque", "outil": "attribuer_pret"}},
   "effets": [{"effet": "refuser"}]},
  {"id": "D-002", "version": 1, "statut": "brouillon", "date_effet": "2026-10-04T09:05:00Z", "texte_fr": "Ajoutée.",
   "fondements": [{"texte": "Règlement intérieur", "article": "12"}],
   "declencheur": {"evenement": "appel_outil", "cible": {"serveur": "*", "outil": "*", "acces": "action"}},
   "condition": {"temps": {"jour_semaine": ["samedi", "dimanche"], "fuseau": "Europe/Paris"}},
   "effets": [{"effet": "refuser"}]}
 ]}`

func TestAjoutNeRelitQueLesDirectivesAjoutees(t *testing.T) {
	j, err := directive.Charger([]byte(jeuAjout))
	if err != nil {
		t.Fatal(err)
	}
	out, err := Ajout(j, []string{"D-002"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "Ajout au jeu demo, version 2 : 1 directive ajoutée (D-002), évaluée après 1 directive existante,") {
		t.Fatalf("en-tête : %q", strings.SplitN(out, "\n", 2)[0])
	}
	if strings.Contains(out, "D-001") || strings.Contains(out, "Existante") {
		t.Fatalf("la directive existante est relue :\n%s", out)
	}
	if !strings.Contains(out, "\n"+Directive(j.Directives[1])) {
		t.Fatalf("la directive ajoutée n'est pas relue comme dans le jeu entier :\n%s", out)
	}
	again, _ := Ajout(j, []string{"D-002"})
	if again != out {
		t.Fatal("relecture instable")
	}
}

func TestAjoutIdentifiantsInvalides(t *testing.T) {
	j, err := directive.Charger([]byte(jeuAjout))
	if err != nil {
		t.Fatal(err)
	}
	for _, ids := range [][]string{nil, {"D-009"}, {"D-002", "D-002"}} {
		if _, err := Ajout(j, ids); err == nil {
			t.Errorf("%v accepté", ids)
		}
	}
}
