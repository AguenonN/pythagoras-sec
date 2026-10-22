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

// journalDeuxPrets : deux prêts autorisés sous le jeu du scénario 4 (SIREN
// 405750753 pour 200 000 €, 552100554 pour 50 000 €), journalisés et
// déposés au coffre comme par l'engine. Rend le journal et le jeu.
func journalDeuxPrets(t *testing.T) (string, string) {
	t.Helper()
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
	ecr, err := journal.Ouvrir(cheminJ, bytes.Repeat([]byte{9}, 32))
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
	prets := []struct {
		siren   string
		montant float64
	}{{"405750753", 200000}, {"552100554", 50000}}
	for i, p := range prets {
		args := map[string]any{"dossier_id": "DOS-FICTIF-00" + string(rune('1'+i)), "siren": p.siren, "montant": p.montant}
		appel := directive.Appel{Serveur: "banque-synthetique", Outil: "attribuer_pret", Arguments: args, Attestation: att, Horodatage: ts,
			VersionRegistre: "reg-1", EtiquettesCible: []string{"credit"}, Acces: directive.Acces("action")}
		dec := directive.Decide(*origine, directive.EtatSession{ID: "s1", Consultations: journal.VersConsultations(gels)}, appel)
		if dec.Effet != directive.Autoriser {
			t.Fatalf("sous le jeu d'origine, le prêt %s devrait être autorisé : %s (%s)", p.siren, dec.Effet, dec.Motif)
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
	return cheminJ, cheminJeu
}

func lancerImpacts(t *testing.T, args ...string) (string, string) {
	t.Helper()
	var out, errw bytes.Buffer
	env := func(string) string { return strings.Repeat("09", 32) }
	if code := executer(append([]string{"impacts"}, args...), &out, &errw, env); code != 0 {
		t.Fatalf("code %d : %s", code, errw.String())
	}
	return out.String(), errw.String()
}

// Le registre des gels ajoute l'un des deux SIREN : seule la décision de
// ce prêt change.
func TestImpacts_Gels(t *testing.T) {
	cheminJ, cheminJeu := journalDeuxPrets(t)
	sql, resume := lancerImpacts(t, cheminJ, "-directives", cheminJeu, "-gels", "552100554", "-evenement", "gels 2026-10-22")
	if strings.Count(sql, "INSERT INTO impacts") != 2 {
		t.Fatalf("deux décisions rejouées attendues :\n%s", sql)
	}
	if !strings.Contains(sql, "'autoriser', 'refuser', true") || !strings.Contains(sql, "'autoriser', 'autoriser', false") {
		t.Fatalf("une décision changée, une inchangée attendues :\n%s", sql)
	}
	if !strings.Contains(resume, "1 changée(s)") {
		t.Fatalf("résumé : %s", resume)
	}
}

// « Et si cette règle avait existé ? » : une directive candidate en
// brouillon, datée d'aujourd'hui, est évaluée comme active depuis la date
// du jeu. Seul le prêt de 200 000 € dépasse le seuil.
func TestImpacts_Hypothese(t *testing.T) {
	cheminJ, cheminJeu := journalDeuxPrets(t)
	candidate := `{"id": "D-100", "version": 1, "statut": "brouillon", "date_effet": "2026-10-22T00:00:00Z",
	  "texte_fr": "Au-delà de 100 000 euros, aucun prêt n'est attribué sans consultation du taux officiel dans la demande.",
	  "fondements": [{"texte": "Politique de crédit — à valider par le juriste", "article": "7.2"}],
	  "declencheur": {"evenement": "appel_outil", "cible": {"serveur": "banque-synthetique", "outil": "attribuer_pret"}},
	  "condition": {"et": [{"argument": {"champ": "/montant", "op": ">", "valeur": 100000}},
	                       {"non": {"session.a_consulte": {"serveur": "taux-officiel", "critere": "donnees_recues"}}}]},
	  "effets": [{"effet": "refuser"}]}`
	chemin := filepath.Join(filepath.Dir(cheminJ), "candidate.json")
	if err := os.WriteFile(chemin, []byte(candidate), 0o600); err != nil {
		t.Fatal(err)
	}
	sql, resume := lancerImpacts(t, cheminJ, "-directives", cheminJeu, "-apres", chemin)
	if !strings.Contains(sql, "'hypothèse D-100'") {
		t.Fatalf("évènement par défaut attendu :\n%s", sql)
	}
	if !strings.Contains(sql, "'autoriser', 'refuser', true") || !strings.Contains(sql, "'autoriser', 'autoriser', false") {
		t.Fatalf("le prêt de 200 000 € seul doit changer :\n%s", sql)
	}
	if !strings.Contains(resume, "1 changée(s)") {
		t.Fatalf("résumé : %s", resume)
	}
}

func TestLireCandidates(t *testing.T) {
	for _, brut := range []string{
		`{"id": "D-1"}`,
		`[{"id": "D-1"}]`,
		`{"directives": [{"id": "D-1"}]}`,
		`{"jeu": {"id": "x", "version": 1}, "directives": [{"id": "D-1"}]}`,
	} {
		cs, err := LireCandidates([]byte(brut))
		if err != nil || len(cs) != 1 || cs[0]["id"] != "D-1" {
			t.Errorf("%s : %v %v", brut, cs, err)
		}
	}
	if _, err := LireCandidates([]byte(`[]`)); err == nil {
		t.Error("une liste vide doit être refusée")
	}
}

func approvalDigest(outil string, args map[string]any) string { return approval.Digest(outil, args) }
