package main

import (
	"encoding/json"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"
)

func lire(t *testing.T, chemin string) string {
	t.Helper()
	b, err := os.ReadFile(chemin)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func rapport(t *testing.T, out string) Rapport {
	t.Helper()
	var r Rapport
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("sortie non JSON : %v\n%s", err, out)
	}
	return r
}

// Les jeux v2 des scénarios sont acceptés et relus comme la relecture les
// attend, octet pour octet.
func TestV2_VerifierEtRelire(t *testing.T) {
	for _, nom := range []string{"credit", "weekend", "top-management", "rh", "demandeur", "complet"} {
		t.Run(nom, func(t *testing.T) {
			chemin := "../../internal/directive/testdata/v2/" + nom + ".json"
			jeu := lire(t, chemin)
			rc, out, _ := lancer(t, jeu, "verifier")
			if r := rapport(t, out); rc != 0 || !r.OK || !strings.HasPrefix(r.Empreinte, "sha256:") {
				t.Fatalf("rc=%d rapport=%+v", rc, r)
			}
			rc, out, errOut := lancer(t, jeu, "relire")
			if rc != 0 {
				t.Fatalf("rc=%d err=%s", rc, errOut)
			}
			if want := lire(t, "../../internal/relecture/testdata/v2/"+nom+".txt"); out != want {
				t.Fatalf("relecture différente de l'attendu :\n%s", out)
			}
		})
	}
}

// --exiger-brouillon vaut pour les directives v2 comme pour les v1.
func TestV2_ExigerBrouillon(t *testing.T) {
	jeu := lire(t, "../../internal/directive/testdata/v2/credit.json")
	rc, out, _ := lancer(t, jeu, "verifier", "--exiger-brouillon")
	r := rapport(t, out)
	if rc != 1 || r.OK || len(r.Erreurs) != 4 || r.Empreinte != "" {
		t.Fatalf("rc=%d rapport=%+v", rc, r)
	}
	if r.Erreurs[0] != `directive: directive C-001: statut "active", brouillon exigé` {
		t.Errorf("erreur 1 = %q", r.Erreurs[0])
	}
	brouillon := strings.ReplaceAll(jeu, `"statut": "active"`, `"statut": "brouillon"`)
	if rc, out, _ := lancer(t, brouillon, "verifier", "--exiger-brouillon"); rc != 0 {
		t.Fatalf("jeu v2 en brouillon refusé : %s", out)
	}
}

// Les erreurs v2 sont rendues en bloc, avec le chemin du nœud, et les
// statuts restent contrôlés quand le chargeur refuse.
func TestV2_ErreursEnBloc(t *testing.T) {
	doc := `{"jeu":{"id":"t","version":1,"date_effet":"2026-10-04T08:00:00Z","auteur":"j"},"directives":[{"id":"D-1","version":1,"statut":"active","date_effet":"2026-10-04T08:00:00Z","texte_fr":"t","fondements":[{"texte":"RGPD"}],` +
		`"declencheur":{"evenement":"appel_outil","cible":{"serveur":"*"}},` +
		`"condition":{"et":[{"temps":{"jour_semaine":["samedi"]}},{"argument":{"champ":"/montant","op":">","valeur":"beaucoup"}}]},` +
		`"effets":[{"effet":"refuser"},{"effet":"caviarder","champs_autorises":["/a"],"mode":"caviarder"}]}]}`
	rc, out, _ := lancer(t, doc, "verifier", "--exiger-brouillon")
	r := rapport(t, out)
	veut := []string{
		"condition.et[0].temps: fuseau obligatoire",
		"condition.et[1].argument: l'opérateur > exige une valeur numérique",
		"caviarder exige une cible nommée",
		"refuser ne se combine ni avec attendre_validation ni avec caviarder",
		`statut "active", brouillon exigé`,
	}
	if rc != 1 || len(r.Erreurs) != len(veut) {
		t.Fatalf("rc=%d erreurs=%q", rc, r.Erreurs)
	}
	for i, v := range veut {
		if !strings.Contains(r.Erreurs[i], v) {
			t.Errorf("erreur %d = %q, attendu %q", i, r.Erreurs[i], v)
		}
	}
	if rc, _, errOut := lancer(t, doc, "relire"); rc != 1 || !strings.Contains(errOut, "fuseau obligatoire") {
		t.Fatalf("relire d'un jeu v2 refusé : rc=%d err=%q", rc, errOut)
	}
}

// fuseaux rend la liste fermée du chargeur, triée.
func TestFuseaux(t *testing.T) {
	rc, out, _ := lancer(t, "", "fuseaux")
	var l []string
	if err := json.Unmarshal([]byte(out), &l); rc != 0 || err != nil {
		t.Fatalf("rc=%d err=%v sortie=%s", rc, err, out)
	}
	if len(l) == 0 || !sort.StringsAreSorted(l) || !slices.Contains(l, "Europe/Paris") || !slices.Contains(l, "UTC") {
		t.Fatalf("liste inattendue : %v", l)
	}
	if rc, _, errOut := lancer(t, "", "fuseaux", "x"); rc != 2 || !strings.Contains(errOut, "usage") {
		t.Fatalf("argument en trop accepté : rc=%d", rc)
	}
}

// relire --directives : seulement les directives ajoutées, jugées dans le
// jeu entier (version n+1 du jeu de démo).
func TestV2_RelireDirectivesAjoutees(t *testing.T) {
	var jeu map[string]any
	if err := json.Unmarshal([]byte(lire(t, "../../demo/directives/demo.json")), &jeu); err != nil {
		t.Fatal(err)
	}
	var w map[string]any
	if err := json.Unmarshal([]byte(lire(t, "../../internal/directive/testdata/v2/weekend.json")), &w); err != nil {
		t.Fatal(err)
	}
	d := w["directives"].([]any)[0].(map[string]any)
	d["id"], d["statut"] = "D-009", "brouillon"
	jeu["directives"] = append(jeu["directives"].([]any), d)
	jeu["jeu"].(map[string]any)["version"] = 2
	b, _ := json.Marshal(jeu)

	rc, out, errOut := lancer(t, string(b), "relire", "--directives", "D-009")
	if rc != 0 {
		t.Fatalf("rc=%d %s", rc, errOut)
	}
	if !strings.HasPrefix(out, "Ajout au jeu demo, version 2 : 1 directive ajoutée (D-009), évaluée après 8 directives existantes") ||
		strings.Contains(out, "D-001") || !strings.Contains(out, "Directive D-009, version 1, brouillon") {
		t.Fatalf("relecture :\n%s", out)
	}
	if rc, _, _ := lancer(t, string(b), "relire", "--directives", "D-010"); rc != 2 {
		t.Fatalf("identifiant absent : rc=%d", rc)
	}
}
