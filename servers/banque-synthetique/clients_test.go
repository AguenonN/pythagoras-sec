package main

import (
	"reflect"
	"strings"
	"testing"

	"mcprogue/servers/internal/essai"
)

func TestClientsFictifs(t *testing.T) {
	cs, err := ChargerClients()
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) < 6 {
		t.Fatalf("%d clients", len(cs))
	}
	for id, c := range cs {
		if !strings.Contains(id, "FICTIF") || !strings.HasSuffix(c.Email, "exemple.test") {
			t.Errorf("client %s non marqué fictif : %+v", id, c)
		}
		for _, k := range c.Comptes {
			if !strings.HasPrefix(k.IBAN, "FR76 9999 9") {
				t.Errorf("compte %s : IBAN non fictif", k.CompteID)
			}
		}
	}
}

func TestLectureClientele(t *testing.T) {
	cs := session(t)
	var comptes SortieComptes
	if res := essai.Appeler(t, cs, "lister_comptes_client", map[string]any{"client_id": "CLI-FICTIF-001"}, &comptes); res.IsError || len(comptes.Comptes) != 2 {
		t.Fatalf("%+v", comptes)
	}
	var h SortieHistorique
	if res := essai.Appeler(t, cs, "historique_operations", map[string]any{"client_id": "CLI-FICTIF-002"}, &h); res.IsError || len(h.Operations) != 4 {
		t.Fatalf("%+v", h)
	}
	var inconnu SortieComptes
	if res := essai.Appeler(t, cs, "lister_comptes_client", map[string]any{"client_id": "CLI-X"}, &inconnu); !res.IsError || inconnu.Statut != StatutIntrouvable {
		t.Fatalf("%+v", inconnu)
	}
}

func TestActionsClientele(t *testing.T) {
	cs := session(t)
	var v SortieVirement
	if res := essai.Appeler(t, cs, "effectuer_virement", map[string]any{"compte_debit": "CPT-FICTIF-0031", "beneficiaire": "FOURNISSEUR FICTIF",
		"iban_beneficiaire": "FR76 9999 9000 0900 0000 0000 001", "pays_beneficiaire": "FR", "montant": 1200}, &v); res.IsError || v.Reference != "VIR-FICTIF-0001" {
		t.Fatalf("%+v", v)
	}
	var p SortiePlafond
	if res := essai.Appeler(t, cs, "modifier_plafond_decouvert", map[string]any{"compte_id": "CPT-FICTIF-0031", "nouveau_plafond": 15000}, &p); res.IsError || p.AncienPlafond != 8000 || p.NouveauPlafond != 15000 {
		t.Fatalf("%+v", p)
	}
	var comptes SortieComptes
	essai.Appeler(t, cs, "lister_comptes_client", map[string]any{"client_id": "CLI-FICTIF-003"}, &comptes)
	if comptes.Comptes[0].PlafondDecouvert != 15000 {
		t.Fatalf("plafond non mis à jour : %+v", comptes)
	}
	var e SortieExport
	if res := essai.Appeler(t, cs, "exporter_donnees_client", map[string]any{"client_id": "CLI-FICTIF-001", "destinataire": "service recouvrement",
		"destination": "interne", "inclure_iban": true}, &e); res.IsError || !reflect.DeepEqual(e.RubriquesSorti, []string{"identite", "adresse", "comptes", "iban"}) {
		t.Fatalf("%+v", e)
	}
	var e2 SortieExport
	if res := essai.Appeler(t, cs, "exporter_donnees_client", map[string]any{"client_id": "CLI-FICTIF-001", "destinataire": "x", "destination": "ailleurs"}, &e2); !res.IsError {
		t.Fatalf("destination inconnue acceptée : %+v", e2)
	}
	var s SortieSoupcon
	if res := essai.Appeler(t, cs, "declarer_soupcon", map[string]any{"client_id": "CLI-FICTIF-002", "motif": "dépôts fractionnés"}, &s); res.IsError || s.Reference != "DS-FICTIVE-0001" {
		t.Fatalf("%+v", s)
	}
	var c SortieCourrier
	if res := essai.Appeler(t, cs, "envoyer_courrier_client", map[string]any{"client_id": "CLI-FICTIF-002", "objet": "o", "contenu": "bonjour"}, &c); res.IsError || c.Longueur != 7 {
		t.Fatalf("%+v", c)
	}
	var r SortieRefusCredit
	if res := essai.Appeler(t, cs, "refuser_credit", map[string]any{"dossier_id": "DOS-FICTIF-001", "motif": "capacité insuffisante"}, &r); res.IsError || r.Reference != "REFUS-FICTIF-0001" {
		t.Fatalf("%+v", r)
	}
	var cl SortieCloture
	if res := essai.Appeler(t, cs, "cloturer_compte", map[string]any{"compte_id": "CPT-FICTIF-0012", "motif": "demande du client"}, &cl); res.IsError {
		t.Fatalf("%+v", cl)
	}
	var cl2 SortieCloture
	if res := essai.Appeler(t, cs, "cloturer_compte", map[string]any{"compte_id": "CPT-FICTIF-0012", "motif": "x"}, &cl2); !res.IsError {
		t.Fatalf("deuxième clôture acceptée : %+v", cl2)
	}
	var v2 SortieVirement
	if res := essai.Appeler(t, cs, "effectuer_virement", map[string]any{"compte_debit": "CPT-FICTIF-0012", "beneficiaire": "B",
		"iban_beneficiaire": "I", "pays_beneficiaire": "FR", "montant": 1}, &v2); !res.IsError {
		t.Fatalf("virement depuis un compte clôturé : %+v", v2)
	}
}
