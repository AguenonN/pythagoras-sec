package main

import (
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/internal/nommage"
	"mcprogue/servers/internal/essai"
)

func rh(t *testing.T) *RH {
	t.Helper()
	r, err := NouveauRH(func() time.Time { return time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC) })
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func session(t *testing.T) *mcp.ClientSession { return essai.Connecter(t, Serveur(rh(t))) }

func TestConsulterSalarie(t *testing.T) {
	cs := session(t)
	var out SortieSalarie
	res := essai.Appeler(t, cs, "consulter_salarie", map[string]any{"salarie_id": "SAL-FICTIF-002"}, &out)
	if res.IsError || out.Statut != StatutOK || out.Salarie == nil || out.Salarie.Libelle != "Salarié fictif B" || out.DonneesFictives == "" {
		t.Fatalf("%+v", out)
	}
	var inconnu SortieSalarie
	if res := essai.Appeler(t, cs, "consulter_salarie", map[string]any{"salarie_id": "SAL-X"}, &inconnu); !res.IsError || inconnu.Statut != StatutIntrouvable {
		t.Fatalf("%+v", inconnu)
	}
}

func TestDeciderPrime(t *testing.T) {
	cs := session(t)
	var out SortiePrime
	res := essai.Appeler(t, cs, "decider_prime", map[string]any{"salarie_ids": []string{"SAL-FICTIF-001", "SAL-FICTIF-004"}, "montant": 800}, &out)
	if res.IsError || out.Statut != StatutDecidee || out.Reference != "PRIME-FICTIVE-0001" || out.MontantTotal != 1600 || len(out.SalarieIDs) != 2 || out.Horodatage != "2026-10-04T09:00:00Z" {
		t.Fatalf("%+v", out)
	}
	for _, args := range []map[string]any{
		{"salarie_ids": []string{}, "montant": 100},
		{"salarie_ids": []string{"SAL-X"}, "montant": 100},
		{"salarie_ids": []string{"SAL-FICTIF-001", "SAL-FICTIF-001"}, "montant": 100},
		{"salarie_ids": []string{"SAL-FICTIF-001"}, "montant": 0},
		{"salarie_ids": []string{"SAL-FICTIF-001"}, "montant": 5000.01},
	} {
		var o SortiePrime
		if res := essai.Appeler(t, cs, "decider_prime", args, &o); !res.IsError || o.Statut != StatutRefuse {
			t.Errorf("%v: %+v", args, o)
		}
	}
}

func TestProposerSanction(t *testing.T) {
	cs := session(t)
	var out SortieSanction
	res := essai.Appeler(t, cs, "proposer_sanction", map[string]any{"salarie_id": "SAL-FICTIF-002", "motif": "  retards répétés  "}, &out)
	if res.IsError || out.Statut != StatutProposee || out.Reference != "SANCTION-FICTIVE-0001" || out.Motif != "retards répétés" {
		t.Fatalf("%+v", out)
	}
	for _, args := range []map[string]any{
		{"salarie_id": "SAL-X", "motif": "m"},
		{"salarie_id": "SAL-FICTIF-002", "motif": "   "},
	} {
		var o SortieSanction
		if res := essai.Appeler(t, cs, "proposer_sanction", args, &o); !res.IsError || o.Statut != StatutRefuse {
			t.Errorf("%v: %+v", args, o)
		}
	}
}

func TestAnnotationsEtNoms(t *testing.T) {
	res, err := session(t).ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	lecture := map[string]bool{"consulter_salarie": true, "decider_prime": false, "proposer_sanction": false,
		"lister_salaries": true, "rechercher_salaries": true, "consulter_fiche": true,
		"convoquer_entretien_prealable": false, "sanctionner": false, "licencier": false, "attribuer_prime": false,
		"exporter_paie": false, "envoyer_message_salarie": false}
	if len(res.Tools) != len(lecture) {
		t.Fatalf("%d outils", len(res.Tools))
	}
	for _, o := range res.Tools {
		if a, ok := lecture[o.Name]; !ok || o.Annotations == nil || o.Annotations.ReadOnlyHint != a {
			t.Errorf("%s: %+v", o.Name, o.Annotations)
		}
		if _, err := nommage.Exposer(Nom, o.Name); err != nil {
			t.Error(err)
		}
	}
}

func TestDonneesFictivesMarquees(t *testing.T) {
	d, idx, err := Charger()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d.Avertissement, "FICTIVES") || !strings.Contains(d.Entreprise, "fictive") || len(idx) != 30 {
		t.Fatalf("%+v", d)
	}
	for id, s := range idx {
		if !strings.HasPrefix(id, "SAL-FICTIF-") || !strings.HasPrefix(s.Libelle, "Salarié fictif ") || !strings.Contains(s.Poste, "(fictif)") {
			t.Errorf("%s non marqué fictif: %+v", id, s)
		}
	}
}

func TestTransportHTTPTLS(t *testing.T) {
	cs := essai.ConnecterTLS(t, Serveur(rh(t)))
	var out SortieSalarie
	essai.Appeler(t, cs, "consulter_salarie", map[string]any{"salarie_id": "SAL-FICTIF-005"}, &out)
	if out.Statut != StatutOK || out.Salarie.Contrat != "CDD" {
		t.Fatalf("%+v", out)
	}
}

func TestSchema(t *testing.T) {
	essai.VerifierSchema(t, session(t), "schema.json")
}
