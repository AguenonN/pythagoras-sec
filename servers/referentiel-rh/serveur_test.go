package main

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/internal/nommage"
	"mcprogue/servers/internal/essai"
)

func session(t *testing.T) *mcp.ClientSession {
	t.Helper()
	r, err := Charger()
	if err != nil {
		t.Fatal(err)
	}
	return essai.Connecter(t, Serveur(r))
}

func groupes(t *testing.T, cs *mcp.ClientSession, id string) SortieGroupes {
	t.Helper()
	var out SortieGroupes
	res := essai.Appeler(t, cs, "groupes_salarie", map[string]any{"salarie_id": id}, &out)
	if res.IsError != (out.Statut != StatutOK) {
		t.Fatalf("%s: isError %v, statut %s", id, res.IsError, out.Statut)
	}
	return out
}

func noms(out SortieGroupes) []string {
	var n []string
	for _, g := range *out.Groupes {
		n = append(n, g.Groupe)
	}
	return n
}

func TestGroupesSalarie(t *testing.T) {
	cs := session(t)
	attendu := map[string]string{
		"SAL-FICTIF-001": "",
		"SAL-FICTIF-002": "salaries-proteges",
		"SAL-FICTIF-003": "contentieux-en-cours",
		"SAL-FICTIF-004": "contentieux-en-cours,salaries-proteges",
		"SAL-FICTIF-005": "",
	}
	for id, a := range attendu {
		out := groupes(t, cs, id)
		if out.Statut != StatutOK || out.Groupes == nil || strings.Join(noms(out), ",") != a || out.DonneesFictives == "" {
			t.Errorf("%s: %+v", id, out)
		}
		for _, g := range *out.Groupes {
			if g.Description == "" {
				t.Errorf("%s: groupe %s sans description", id, g.Groupe)
			}
		}
	}
}

// Aucun groupe : liste vide explicite. Salarié inconnu : introuvable, sans
// champ groupes, jamais « aucun groupe ».
func TestInconnuNEstPasAucunGroupe(t *testing.T) {
	cs := session(t)
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "groupes_salarie", Arguments: map[string]any{"salarie_id": "SAL-FICTIF-001"}})
	if err != nil || !strings.Contains(string(essai.JSON(t, res.StructuredContent)), `"groupes":[]`) {
		t.Fatalf("aucun groupe doit être une liste vide explicite: %v %s", err, essai.JSON(t, res.StructuredContent))
	}
	res, err = cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "groupes_salarie", Arguments: map[string]any{"salarie_id": "SAL-FICTIF-999"}})
	if err != nil {
		t.Fatal(err)
	}
	brut := string(essai.JSON(t, res.StructuredContent))
	if !res.IsError || strings.Contains(brut, `"groupes"`) || !strings.Contains(brut, `"statut":"introuvable"`) {
		t.Fatalf("salarié inconnu: %s", brut)
	}
}

// Le référentiel couvre exactement les salariés fictifs de rh-synthetique
// et, pour les personnes politiquement exposées (session secteurs), les
// clients et comptes fictifs de banque-synthetique : toute personne
// concernée que le gate peut chercher y figure.
func TestCoherenceAvecRhSynthetique(t *testing.T) {
	var a, c []string
	for _, f := range []struct{ chemin, liste, id string }{
		{"../rh-synthetique/salaries_fictifs.json", "salaries", "salarie_id"},
		{"../rh-synthetique/salaries_population.json", "salaries", "salarie_id"},
		{"../banque-synthetique/clients_fictifs.json", "clients", "client_id"},
		{"../banque-synthetique/clients_population.json", "clients", "client_id"},
	} {
		b, err := os.ReadFile(f.chemin)
		if err != nil {
			t.Fatal(err)
		}
		var brut map[string]json.RawMessage
		var liste []map[string]any
		if err := json.Unmarshal(b, &brut); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(brut[f.liste], &liste); err != nil {
			t.Fatal(err)
		}
		for _, x := range liste {
			a = append(a, x[f.id].(string))
			comptes, _ := x["comptes"].([]any)
			for _, k := range comptes {
				a = append(a, k.(map[string]any)["compte_id"].(string))
			}
		}
	}
	r, _ := Charger()
	for id := range r.Appartenances {
		c = append(c, id)
	}
	sort.Strings(a)
	sort.Strings(c)
	if strings.Join(a, ",") != strings.Join(c, ",") {
		t.Fatalf("sources %v, referentiel-rh %v", a, c)
	}
	if len(r.Groupes) != 3 || r.Groupes["salaries-proteges"] == "" || r.Groupes["contentieux-en-cours"] == "" ||
		r.Groupes["personne-politiquement-exposee"] == "" {
		t.Fatalf("groupes: %v", r.Groupes)
	}
	if !strings.Contains(r.Avertissement, "FICTIVES") {
		t.Fatal("données non marquées fictives")
	}
}

func TestAnnotationsEtNom(t *testing.T) {
	res, err := session(t).ListTools(t.Context(), nil)
	if err != nil || len(res.Tools) != 1 || res.Tools[0].Annotations == nil || !res.Tools[0].Annotations.ReadOnlyHint {
		t.Fatalf("%v %+v", err, res)
	}
	if _, err := nommage.Exposer(Nom, res.Tools[0].Name); err != nil {
		t.Fatal(err)
	}
}

func TestTransportHTTPTLS(t *testing.T) {
	r, _ := Charger()
	cs := essai.ConnecterTLS(t, Serveur(r))
	if out := groupes(t, cs, "SAL-FICTIF-002"); out.Statut != StatutOK || strings.Join(noms(out), ",") != "salaries-proteges" {
		t.Fatalf("%+v", out)
	}
}

func TestSchema(t *testing.T) {
	essai.VerifierSchema(t, session(t), "schema.json")
}
