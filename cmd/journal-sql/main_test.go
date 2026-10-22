package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mcprogue/internal/approval"
	"mcprogue/internal/directive"
	"mcprogue/internal/journal"
)

const jeuScenario4 = "../../demo/scenarios/jeux/avec-d009.json"

func TestLit(t *testing.T) {
	cas := map[string]string{
		"":                "''",
		"abc":             "'abc'",
		"l'usure":         "'l''usure'",
		"a\x00b":          "'ab'",
		"'; DROP TABLE x": "'''; DROP TABLE x'",
	}
	for entree, attendu := range cas {
		if got := Lit(entree); got != attendu {
			t.Errorf("Lit(%q) = %s, attendu %s", entree, got, attendu)
		}
	}
	if LitOuNull("") != "NULL" || EntierOuNull(0) != "NULL" || Instant(time.Time{}) != "NULL" || InstantTexte("pas une date") != "NULL" {
		t.Error("les valeurs vides doivent valoir NULL")
	}
}

func TestJeuAvecGels_SeChargeEtAjouteLaDirective(t *testing.T) {
	brut, err := os.ReadFile(jeuScenario4)
	if err != nil {
		t.Fatal(err)
	}
	origine, err := directive.Charger(brut)
	if err != nil {
		t.Fatal(err)
	}
	augmente, err := JeuAvecGels(brut, []string{"552100554", "405750753"})
	if err != nil {
		t.Fatal(err)
	}
	apres, err := directive.Charger(augmente)
	if err != nil {
		t.Fatalf("le jeu augmenté doit passer le chargeur strict : %v", err)
	}
	if apres.Meta.ID != origine.Meta.ID || apres.Meta.Version != origine.Meta.Version+1 {
		t.Fatalf("jeu %s v%d, attendu %s v%d", apres.Meta.ID, apres.Meta.Version, origine.Meta.ID, origine.Meta.Version+1)
	}
	if len(apres.Directives) != len(origine.Directives)+1 {
		t.Fatalf("%d directives, attendu %d", len(apres.Directives), len(origine.Directives)+1)
	}
	if apres.Meta.Empreinte == origine.Meta.Empreinte {
		t.Fatal("l'empreinte du jeu augmenté doit changer")
	}
	if !strings.Contains(string(augmente), `"405750753"`) || !strings.Contains(string(augmente), IDDirectiveGels) {
		t.Fatal("la directive des gels doit porter les SIREN")
	}
	if _, err := JeuAvecGels(augmente, []string{"1"}); err == nil {
		t.Fatal("ajouter deux fois la directive doit échouer")
	}
}

func TestExecuter_RefuseUnJournalRompu(t *testing.T) {
	dir := t.TempDir()
	chemin := filepath.Join(dir, "journal.jsonl")
	cle := bytes.Repeat([]byte{7}, 32)
	ecrireJournal(t, chemin, cle)

	// Intègre : du SQL transactionnel, les consultations comprises.
	var out, errw bytes.Buffer
	env := func(string) string { return strings.Repeat("07", 32) }
	if code := executer([]string{"exporter", chemin}, &out, &errw, env); code != 0 {
		t.Fatalf("code %d : %s", code, errw.String())
	}
	sql := out.String()
	for _, attendu := range []string{"BEGIN;", "INSERT INTO journaux", "INSERT INTO entrees", "INSERT INTO consultations", "'gels-avoirs'", "COMMIT;"} {
		if !strings.Contains(sql, attendu) {
			t.Errorf("SQL sans %q", attendu)
		}
	}

	// Altéré d'un octet : rien n'est écrit.
	b, _ := os.ReadFile(chemin)
	b = bytes.Replace(b, []byte("verifier_gel"), []byte("verifier_gal"), 1)
	if err := os.WriteFile(chemin, b, 0o600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errw.Reset()
	if code := executer([]string{"exporter", chemin}, &out, &errw, env); code != 1 || out.Len() != 0 {
		t.Fatalf("journal altéré : code %d, %d octets de SQL (attendu 1 et rien)", code, out.Len())
	}
}

func TestSchema(t *testing.T) {
	var out bytes.Buffer
	if code := executer([]string{"schema"}, &out, &out, func(string) string { return "" }); code != 0 {
		t.Fatal("schema doit réussir sans clé")
	}
	for _, table := range []string{"entrees", "consultations", "applications", "impacts", "VIEW detective"} {
		if !strings.Contains(out.String(), table) {
			t.Errorf("schéma sans %s", table)
		}
	}
}

func ecrireJournal(t *testing.T, chemin string, cle []byte) {
	t.Helper()
	e, err := journal.Ouvrir(chemin, cle)
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Date(2026, 10, 3, 9, 19, 0, 0, time.UTC)
	e.Horloge(func() time.Time { return ts })
	entrees := []journal.Entree{
		{Type: journal.SessionOuverte, Session: "s1"},
		{
			Type: journal.AppelT, Session: "s1", Seq: 2, Serveur: "banque-synthetique", Outil: "attribuer_pret",
			SousAgentDeclare: "credit-historique",
			EtatSession: []journal.Consultation{{
				Seq: 1, Serveur: "gels-avoirs", Outil: "verifier_gel", TS: ts, Resultat: journal.StatutOK,
				Source: &journal.Source{Origine: "rejeu", Date: "2026-09-29T08:00:00Z"},
			}},
			Decision: &journal.Decision{Effet: "autoriser", Motif: "aucune directive déclenchée"},
			Resultat: &journal.Resultat{Statut: journal.StatutOK},
		},
	}
	for _, en := range entrees {
		if _, err := e.Ecrire(en); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.Fermer(); err != nil {
		t.Fatal(err)
	}
}

// TestImpacts_DeBoutEnBout : deux prêts décidés sous le jeu du scénario 4,
// journalisés et déposés au coffre comme par l'engine ; le registre des
// gels ajoute l'un des deux SIREN. Seule la décision de ce prêt change.
func TestImpacts_DeBoutEnBout(t *testing.T) {
	brut, err := os.ReadFile(jeuScenario4)
	if err != nil {
		t.Fatal(err)
	}
	origine, err := directive.Charger(brut)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	cheminJ, cheminC, cheminJeu := filepath.Join(dir, "journal.jsonl"), filepath.Join(dir, "coffre.jsonl"), filepath.Join(dir, "jeu.json")
	if err := os.WriteFile(cheminJeu, brut, 0o600); err != nil {
		t.Fatal(err)
	}
	cle := bytes.Repeat([]byte{9}, 32)
	ecr, err := journal.Ouvrir(cheminJ, cle)
	if err != nil {
		t.Fatal(err)
	}
	coffre, err := journal.OuvrirCoffre(cheminC)
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Date(2026, 10, 3, 9, 19, 0, 0, time.UTC)
	ecr.Horloge(func() time.Time { return ts })
	att := directive.Attestation{Mode: "spiffe", Attendu: "spiffe://lab/banque", Observe: "spiffe://lab/banque", Schemas: true}
	gels := []journal.Consultation{{
		Seq: 1, Serveur: "gels-avoirs", Outil: "verifier_gel", TS: ts, Resultat: journal.StatutOK, Empreinte: "e1",
		Source: &journal.Source{Origine: "rejeu", Date: "2026-09-29T08:00:00Z"},
	}}
	for i, siren := range []string{"405750753", "552100554"} {
		args := map[string]any{"dossier_id": "DOS-FICTIF-00" + string(rune('1'+i)), "siren": siren, "montant": float64(200000)}
		appel := directive.Appel{Serveur: "banque-synthetique", Outil: "attribuer_pret", Arguments: args, Attestation: att, Horodatage: ts,
			VersionRegistre: "reg-1", EtiquettesCible: []string{"credit"}, Acces: directive.Acces("action")}
		dec := directive.Decide(*origine, directive.EtatSession{ID: "s1", Consultations: journal.VersConsultations(gels)}, appel)
		if dec.Effet != directive.Autoriser {
			t.Fatalf("sous le jeu d'origine, le prêt %s devrait être autorisé : %s (%s)", siren, dec.Effet, dec.Motif)
		}
		emp := approvalDigest("attribuer_pret", args)
		if err := coffre.Deposer(journal.Depot{TS: ts, Empreinte: emp, Outil: "attribuer_pret", ArgumentsOrigine: args}); err != nil {
			t.Fatal(err)
		}
		if _, err := ecr.Ecrire(journal.Entree{
			Type: journal.AppelT, Session: "s1", Seq: 2 + i, Serveur: "banque-synthetique", Outil: "attribuer_pret",
			Jeu: journal.DepuisJeu(*origine), Attestation: journal.DepuisAttestation(att),
			VersionRegistre: "reg-1", EtiquettesCible: []string{"credit"}, Acces: "action",
			EtatSession: gels, Decision: journal.DepuisDecision(dec),
			Appel:    &journal.Appel{EmpreinteOrigine: emp, EmpreinteTransmise: dec.EmpreinteTransmise},
			Resultat: &journal.Resultat{Statut: journal.StatutOK},
		}); err != nil {
			t.Fatal(err)
		}
	}
	ecr.Fermer()
	coffre.Fermer()

	var out, errw bytes.Buffer
	env := func(string) string { return strings.Repeat("09", 32) }
	code := executer([]string{"impacts", cheminJ, "-directives", cheminJeu, "-gels", "552100554", "-evenement", "gels 2026-10-22"}, &out, &errw, env)
	if code != 0 {
		t.Fatalf("code %d : %s", code, errw.String())
	}
	sql := out.String()
	if strings.Count(sql, "INSERT INTO impacts") != 2 {
		t.Fatalf("deux décisions rejouées attendues :\n%s", sql)
	}
	if !strings.Contains(sql, "'autoriser', 'refuser', true") || !strings.Contains(sql, "'autoriser', 'autoriser', false") {
		t.Fatalf("une décision invalidée, une inchangée attendues :\n%s", sql)
	}
	if !strings.Contains(errw.String(), "1 invalidée(s)") {
		t.Fatalf("résumé : %s", errw.String())
	}
}

func approvalDigest(outil string, args map[string]any) string { return approval.Digest(outil, args) }
