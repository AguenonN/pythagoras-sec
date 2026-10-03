package main

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"mcprogue/servers/internal/essai"
)

// Chaque salarié est au référentiel RH (résolveur des personnes concernées
// du gate) : un salarié absent rendrait toute action le visant indéterminée.
func TestTousAuReferentiel(t *testing.T) {
	b, err := os.ReadFile("../referentiel-rh/groupes_fictifs.json")
	if err != nil {
		t.Fatal(err)
	}
	var r struct {
		Appartenances map[string][]string `json:"appartenances"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	_, idx, _ := Charger()
	for id := range idx {
		if _, ok := r.Appartenances[id]; !ok {
			t.Errorf("%s absent du référentiel", id)
		}
	}
}

func TestListeEtRecherche(t *testing.T) {
	cs := session(t)
	var l SortieListe
	if res := essai.Appeler(t, cs, "lister_salaries", map[string]any{}, &l); res.IsError || l.Nombre != 80 || l.Salaries[0].SalaireBrutAnnuel != nil {
		t.Fatalf("%+v", l)
	}
	var ls SortieListe
	essai.Appeler(t, cs, "lister_salaries", map[string]any{"service": "Finance", "inclure_salaires": true}, &ls)
	if ls.Nombre == 0 || ls.Salaries[0].SalaireBrutAnnuel == nil {
		t.Fatalf("%+v", ls)
	}
	var r SortieListe
	essai.Appeler(t, cs, "rechercher_salaries", map[string]any{"criteres": map[string]any{"syndique": true}}, &r)
	if r.Nombre != 9 {
		t.Fatalf("syndiqués : %+v", r)
	}
	var r2 SortieListe
	essai.Appeler(t, cs, "rechercher_salaries", map[string]any{"criteres": map[string]any{"service": "Production", "contrat": "CDI"}}, &r2)
	if r2.Nombre == 0 {
		t.Fatalf("%+v", r2)
	}
}

func TestFicheRubriques(t *testing.T) {
	cs := session(t)
	var f SortieFiche
	essai.Appeler(t, cs, "consulter_fiche", map[string]any{"salarie_id": "SAL-FICTIF-012"}, &f)
	if f.Fiche == nil || f.Fiche.Sante != nil || f.Fiche.Syndical != nil || f.Fiche.SalaireBrutAnnuel != nil {
		t.Fatalf("rubriques sensibles sans demande : %+v", f.Fiche)
	}
	var g SortieFiche
	essai.Appeler(t, cs, "consulter_fiche", map[string]any{"salarie_id": "SAL-FICTIF-012", "inclure_sante": true, "inclure_syndical": true}, &g)
	if g.Fiche.Sante == nil || !g.Fiche.Sante.ArretEnCours || g.Fiche.Syndical == nil || !g.Fiche.Syndical.Adherent ||
		!reflect.DeepEqual(g.Rubriques, []string{"identite", "poste", "contrat", "evaluation", "sante", "syndical"}) {
		t.Fatalf("%+v", g)
	}
}

func TestActionsGestion(t *testing.T) {
	cs := session(t)
	ok := []struct {
		outil string
		args  map[string]any
		ref   string
	}{
		{"convoquer_entretien_prealable", map[string]any{"salarie_id": "SAL-FICTIF-006", "date_entretien": "2026-10-12", "motif": "retards"}, "CONVOCATION-FICTIVE-0001"},
		{"sanctionner", map[string]any{"salarie_id": "SAL-FICTIF-006", "sanction": "avertissement", "motif": "retards"}, "SANCTION-NOTIFIEE-FICTIVE-0001"},
		{"licencier", map[string]any{"salarie_id": "SAL-FICTIF-008", "motif": "abandon de poste"}, "LICENCIEMENT-FICTIF-0001"},
		{"attribuer_prime", map[string]any{"salarie_id": "SAL-FICTIF-013", "montant": 800}, "PRIME-INDIVIDUELLE-FICTIVE-0001"},
		{"exporter_paie", map[string]any{"destinataire": "cabinet", "pays": "FR", "periode": "2026-09"}, "EXPORT-PAIE-FICTIF-0001"},
		{"envoyer_message_salarie", map[string]any{"salarie_id": "SAL-FICTIF-013", "objet": "o", "contenu": "c"}, "MESSAGE-FICTIF-0001"},
	}
	for _, c := range ok {
		var o SortieAction
		if res := essai.Appeler(t, cs, c.outil, c.args, &o); res.IsError || o.Reference != c.ref {
			t.Errorf("%s : %+v", c.outil, o)
		}
	}
	ko := []struct {
		outil string
		args  map[string]any
	}{
		{"sanctionner", map[string]any{"salarie_id": "SAL-FICTIF-006", "sanction": "amende", "motif": "x"}},
		{"licencier", map[string]any{"salarie_id": "SAL-X", "motif": "x"}},
		{"attribuer_prime", map[string]any{"salarie_id": "SAL-FICTIF-013", "montant": 9000}},
		{"exporter_paie", map[string]any{"destinataire": "c", "pays": "France", "periode": "2026-09"}},
		{"convoquer_entretien_prealable", map[string]any{"salarie_id": "SAL-FICTIF-006", "date_entretien": "lundi", "motif": "x"}},
	}
	for _, c := range ko {
		var o SortieAction
		if res := essai.Appeler(t, cs, c.outil, c.args, &o); !res.IsError || o.Statut != StatutRefuse {
			t.Errorf("%s %v : %+v", c.outil, c.args, o)
		}
	}
}
