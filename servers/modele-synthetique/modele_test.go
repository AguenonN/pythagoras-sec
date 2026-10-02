package modele

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mcprogue/servers/internal/essai"
)

func domaine(t *testing.T, nom string) (outils, donnees []byte) {
	t.Helper()
	var err error
	if outils, err = os.ReadFile(filepath.Join("domaines", nom, "outils.json")); err != nil {
		t.Fatal(err)
	}
	if donnees, err = os.ReadFile(filepath.Join("domaines", nom, "donnees.json")); err != nil {
		t.Fatal(err)
	}
	return
}

// Les deux domaines prêts à l'emploi sont valides et cohérents.
func TestDomainesFournis(t *testing.T) {
	for _, nom := range []string{"assurance", "contentieux"} {
		o, d := domaine(t, nom)
		if _, err := Serveur("essai-"+nom, o, d); err != nil {
			t.Errorf("%s : %v", nom, err)
		}
	}
}

func TestAssurance(t *testing.T) {
	o, d := domaine(t, "assurance")
	srv, err := Serveur("assurance-synthetique", o, d)
	if err != nil {
		t.Fatal(err)
	}
	cs := essai.Connecter(t, srv)
	var out Resultat

	if res := essai.Appeler(t, cs, "lire_sinistre", map[string]any{"sinistre_id": "SIN-FICTIF-002"}, &out); res.IsError || out.Statut != StatutOK || out.Enregistrement == nil {
		t.Fatalf("lire : %+v", out)
	}
	if !strings.Contains(strings.ToLower(out.DonneesFictives), "fictives") {
		t.Fatalf("avertissement absent : %+v", out)
	}
	if res := essai.Appeler(t, cs, "lire_sinistre", map[string]any{"sinistre_id": "SIN-INCONNU"}, &out); !res.IsError || out.Statut != StatutIntrouvable || !strings.Contains(out.Message, "SIN-FICTIF-001") {
		t.Fatalf("introuvable : %+v", out)
	}
	// Filtre : une seule pièce médicale… deux pour SIN-FICTIF-002.
	essai.Appeler(t, cs, "lister_pieces", map[string]any{"sinistre_id": "SIN-FICTIF-002"}, &out)
	if out.Nombre == nil || *out.Nombre != 2 {
		t.Fatalf("pièces de SIN-FICTIF-002 : %+v", out)
	}
	essai.Appeler(t, cs, "lister_sinistres", map[string]any{}, &out)
	if out.Nombre == nil || *out.Nombre != 4 {
		t.Fatalf("sinistres : %+v", out)
	}
	essai.Appeler(t, cs, "lister_sinistres", map[string]any{"statut": "ouvert"}, &out)
	if out.Nombre == nil || *out.Nombre != 2 {
		t.Fatalf("sinistres ouverts : %+v", out)
	}

	// Action : référence croissante, sinistre vérifié.
	for _, ref := range []string{"INDEMNISATION-FICTIVE-0001", "INDEMNISATION-FICTIVE-0002"} {
		res := essai.Appeler(t, cs, "valider_indemnisation", map[string]any{"sinistre_id": "SIN-FICTIF-001", "montant": 3200.5}, &out)
		if res.IsError || out.Statut != StatutEnregistre || out.Reference != ref {
			t.Fatalf("action : %+v", out)
		}
	}
	if res := essai.Appeler(t, cs, "valider_indemnisation", map[string]any{"sinistre_id": "SIN-X", "montant": 1}, &out); !res.IsError || out.Statut != StatutIntrouvable {
		t.Fatalf("action sur sinistre inconnu : %+v", out)
	}
}

func TestArgumentsRefuses(t *testing.T) {
	o, d := domaine(t, "contentieux")
	srv, err := Serveur("cabinet-synthetique", o, d)
	if err != nil {
		t.Fatal(err)
	}
	cs := essai.Connecter(t, srv)
	var out Resultat
	for nom, args := range map[string]map[string]any{
		"absent":  {},
		"inconnu": {"dossier_id": "DOSSIER-FICTIF-001", "autre": 1},
		"type":    {"dossier_id": 42},
	} {
		if res := essai.Appeler(t, cs, "lire_dossier", args, &out); !res.IsError || out.Statut != StatutRefuse {
			t.Errorf("%s : %+v", nom, out)
		}
	}
	if res := essai.Appeler(t, cs, "deposer_conclusions", map[string]any{"dossier_id": "DOSSIER-FICTIF-001", "titre": "Conclusions en réponse", "pieces_jointes": []string{"PCLIENT-FICTIVE-001"}}, &out); res.IsError || out.Reference != "CONCLUSIONS-FICTIVES-0001" {
		t.Fatalf("dépôt : %+v", out)
	}
}

func TestDeclarationsInvalides(t *testing.T) {
	_, donnees := domaine(t, "assurance")
	cas := map[string]string{
		"sans readOnlyHint":       `{"outils":[{"nom":"a","description":"d","genre":"lister","collection":"sinistres"}]}`,
		"action en lecture seule": `{"outils":[{"nom":"a","description":"d","genre":"action","readOnlyHint":true,"reference_prefixe":"X-FICTIF"}]}`,
		"lecture non lecture":     `{"outils":[{"nom":"a","description":"d","genre":"lister","readOnlyHint":false,"collection":"sinistres"}]}`,
		"genre inconnu":           `{"outils":[{"nom":"a","description":"d","genre":"supprimer","readOnlyHint":false}]}`,
		"collection absente":      `{"outils":[{"nom":"a","description":"d","genre":"lister","readOnlyHint":true,"collection":"nulle"}]}`,
		"champ absent":            `{"outils":[{"nom":"a","description":"d","genre":"lire","readOnlyHint":true,"collection":"sinistres","arguments":[{"nom":"x","type":"string","description":"d","obligatoire":true}]}]}`,
		"nom d'outil invalide":    `{"outils":[{"nom":"A__b","description":"d","genre":"lister","readOnlyHint":true,"collection":"sinistres"}]}`,
		"sans description":        `{"outils":[{"nom":"a","genre":"lister","readOnlyHint":true,"collection":"sinistres"}]}`,
		"type inconnu":            `{"outils":[{"nom":"a","description":"d","genre":"lister","readOnlyHint":true,"collection":"sinistres","arguments":[{"nom":"statut","type":"date","description":"d"}]}]}`,
		"champ inconnu du JSON":   `{"outils":[],"autre":1}`,
		"aucun outil":             `{"outils":[]}`,
		"action sans préfixe":     `{"outils":[{"nom":"a","description":"d","genre":"action","readOnlyHint":false}]}`,
	}
	for nom, o := range cas {
		if _, err := Serveur("essai", []byte(o), donnees); err == nil {
			t.Errorf("%s : accepté", nom)
		}
	}
	o, _ := domaine(t, "assurance")
	for nom, d := range map[string]string{
		"sans avertissement": `{"collections":{"sinistres":[{"id":"x"}]}}`,
		"avertissement réel": `{"avertissement":"clients de la banque","collections":{"sinistres":[{"id":"x"}]}}`,
		"sans collection":    `{"avertissement":"données fictives"}`,
		"collection vide":    `{"avertissement":"données fictives","collections":{"sinistres":[]}}`,
	} {
		if _, err := Serveur("essai", o, []byte(d)); err == nil {
			t.Errorf("%s : accepté", nom)
		}
	}
	if _, err := Serveur("Nom__invalide", o, donnees); err == nil {
		t.Error("nom de serveur invalide accepté")
	}
}

func TestAvertissements(t *testing.T) {
	_, donnees := domaine(t, "assurance")
	o := `{"outils":[
	 {"nom":"envoyer","description":"d","genre":"action","readOnlyHint":false,"reference_prefixe":"X-FICTIF","arguments":[{"nom":"texte","type":"string","description":"d"},{"nom":"destinataire","type":"string","description":"d"}]},
	 {"nom":"chercher","description":"d","genre":"lister","readOnlyHint":true,"collection":"sinistres","arguments":[{"nom":"statut","type":"string","description":"d"}]}]}`
	d, err := LireDonnees(donnees)
	if err != nil {
		t.Fatal(err)
	}
	outils, err := LireOutils([]byte(o))
	if err != nil {
		t.Fatal(err)
	}
	if err := Verifier(outils, d); err != nil {
		t.Fatal(err)
	}
	if av := Avertissements(outils); len(av) != 1 || !strings.Contains(av[0], `"texte"`) {
		t.Fatalf("%v", av)
	}
	// Un argument nommé « texte » sur une lecture n'avertit pas.
	for _, f := range []string{"assurance", "contentieux"} {
		ob, _ := domaine(t, f)
		x, _ := LireOutils(ob)
		if av := Avertissements(x); len(av) != 0 {
			t.Errorf("%s : %v", f, av)
		}
	}
}
