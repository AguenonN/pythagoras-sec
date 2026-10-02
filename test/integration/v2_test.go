package integration

// Scénarios v2, gate réel devant les vrais serveurs, avec l'annuaire de
// démonstration (identités fictives, clés neuves générées par TestMain) :
//
//   - week-end : lecture autorisée, action refusée, aux horodatages
//     limites (vendredi 23:59, samedi 00:00, dimanche 23:59, lundi 00:00,
//     heure de Paris), par l'horloge simulée du gate ;
//   - top management : attente par rôle, reprise avec le bon valideur,
//     refus d'un valideur sans le rôle ou identique au demandeur,
//     notification du juridique avant transmission.
//
// Chaque journal est vérifié et rejoué sans écart, par le paquet rejeu et
// par la commande journal.

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/internal/annuaire"
	"mcprogue/internal/journal"
	"mcprogue/internal/registre"
	"mcprogue/internal/registredemo"
)

// preparerAnnuaire génère l'annuaire de démo signé et ses clés dans dir.
func preparerAnnuaire(dir string) error {
	src, err := annuaire.LireSourceDemo(filepath.Join(racine, annuaire.FichierSourceDemo))
	if err != nil {
		return err
	}
	d, cles, err := annuaire.Generer(*src, time.Now(), nil)
	if err != nil {
		return err
	}
	b, err := annuaire.Encoder(d)
	if err != nil {
		return err
	}
	cheminAnnuaire = filepath.Join(dir, "annuaire.json")
	dirCles = filepath.Join(dir, "cles")
	cheminAutorite = filepath.Join(dirCles, annuaire.FichierAutoritePublic)
	if err := annuaire.EcrireCles(dirCles, cles); err != nil {
		return err
	}
	return os.WriteFile(cheminAnnuaire, b, 0o600)
}

func cle(t *testing.T, identite string) ed25519.PrivateKey {
	t.Helper()
	k, err := annuaire.LireCle(filepath.Join(dirCles, annuaire.FichierCle(identite)))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// jetonIdentite : le jeton que le demandeur signe pour lui-même, émis à
// l'instant de départ de l'horloge simulée (ou maintenant).
func jetonIdentite(t *testing.T, identite, depart string) string {
	t.Helper()
	emis := time.Now()
	if depart != "" {
		d, err := time.Parse(time.RFC3339, depart)
		if err != nil {
			t.Fatal(err)
		}
		emis = d
	}
	j, err := annuaire.SignerIdentite(cle(t, identite), identite, identite, emis, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func jetonApprobation(t *testing.T, valideur, role, empreinte string) string {
	t.Helper()
	j, err := annuaire.SignerApprobation(cle(t, valideur), valideur, role, empreinte, time.Now(), 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func (b *banc) appelerMeta(t *testing.T, nom string, args map[string]any, meta mcp.Meta) *mcp.CallToolResult {
	t.Helper()
	ctx, annuler := context.WithTimeout(context.Background(), 30*time.Second)
	defer annuler()
	r, err := b.cs.CallTool(ctx, &mcp.CallToolParams{Name: nom, Arguments: args, Meta: meta})
	if err != nil {
		t.Fatalf("%s : %v\n%s", nom, err, b.stderr.String())
	}
	return r
}

// commandeJournal vérifie et rejoue le journal avec la commande journal,
// annuaire compris : aucune erreur, aucun écart.
func (b *banc) commandeJournal(t *testing.T) string {
	t.Helper()
	env := []string{"MCP_GATE_CLE_JOURNAL=" + hex.EncodeToString(cleTest)}
	var sortie bytes.Buffer
	for _, args := range [][]string{
		{"verifier", b.journal},
		{"rejouer", b.journal, "-directives", filepath.Join(racine, b.jeu), "-coffre", b.coffre,
			"-annuaire", cheminAnnuaire, "-annuaire-autorite", cheminAutorite},
	} {
		cmd := exec.Command(binaireJournal, args...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		sortie.Write(out)
		if err != nil {
			t.Fatalf("journal %s : %v\n%s", args[0], err, out)
		}
	}
	if !strings.Contains(sortie.String(), "Écarts   : aucun") {
		t.Fatalf("commande journal :\n%s", sortie.String())
	}
	return sortie.String()
}

var paris = func() *time.Location {
	l, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		panic(err)
	}
	return l
}()

const jeuWeekEnd = "test/integration/testdata/weekend.json"

func TestWeekEnd(t *testing.T) {
	pret := map[string]any{"dossier_id": "DOS-FICTIF-002", "siren": "382891752", "montant": 400000}
	lecture := map[string]any{"categorie": "decouvert-societes-non-financieres"}
	cas := []struct {
		nom, depart string
		weekend     bool
	}{
		{"vendredi 23:59 Paris", "2026-10-02T21:59:00Z", false},
		{"samedi 00:00 Paris", "2026-10-02T22:00:00Z", true},
		{"dimanche 23:59 Paris", "2026-10-04T21:59:00Z", true},
		{"lundi 00:00 Paris", "2026-10-04T22:00:00Z", false},
	}
	for _, c := range cas {
		t.Run(c.nom, func(t *testing.T) {
			b := demarrerAvec(t, lancement{registre: cheminRegistre, jeu: jeuWeekEnd, depart: c.depart})
			// La lecture passe toujours.
			if r := b.appeler(t, "banque-synthetique__get_taux_interne", lecture); r.IsError {
				t.Fatalf("lecture : %s", texte(r))
			}
			r := b.appeler(t, "banque-synthetique__attribuer_pret", pret)
			if c.weekend {
				sc := structure(t, r)
				if !r.IsError || sc["statut"] != "refuse" || sc["directive"] != "W-001" {
					t.Fatalf("action le week-end : %v %s", sc, texte(r))
				}
			} else if sc, _ := r.StructuredContent.(map[string]any); sc != nil && sc["statut"] == "refuse" {
				t.Fatalf("action en semaine refusée : %v", sc)
			}
			entrees, appels := b.verifier(t)
			if len(appels) != 2 {
				t.Fatalf("%d appels", len(appels))
			}
			jour := appels[1].TS.In(paris).Weekday()
			if (jour == time.Saturday || jour == time.Sunday) != c.weekend {
				t.Fatalf("horodatage %s (%s à Paris)", appels[1].TS, jour)
			}
			if appels[0].Acces != "read" || appels[1].Acces != "action" || appels[1].VersionRegistre == "" {
				t.Fatalf("accès %q %q, version %q", appels[0].Acces, appels[1].Acces, appels[1].VersionRegistre)
			}
			effet := "autoriser"
			if c.weekend {
				effet = "refuser"
			}
			if appels[0].Decision.Effet != "autoriser" || appels[1].Decision.Effet != effet {
				t.Fatalf("effets %s %s", appels[0].Decision.Effet, appels[1].Decision.Effet)
			}
			simulee := false
			for _, e := range entrees {
				simulee = simulee || (e.Type == journal.AlerteT && strings.Contains(e.Message, "horloge simulée"))
			}
			if !simulee {
				t.Fatal("l'horloge simulée n'est pas signalée au journal")
			}
			b.commandeJournal(t)
		})
	}
}

const jeuTopManagement = "test/integration/testdata/top-management.json"

const (
	claire = "claire.analyste@exemple.test"
	diane  = "diane.risques@exemple.test"
	julie  = "julie.juridique@exemple.test"
	marc   = "marc.ancien@exemple.test"
)

var grosPret = map[string]any{"dossier_id": "DOS-FICTIF-002", "siren": "382891752", "montant": 3000000}

func attente(t *testing.T, r *mcp.CallToolResult) map[string]any {
	t.Helper()
	sc := structure(t, r)
	if !r.IsError || sc["statut"] != "en_attente" {
		t.Fatalf("attente attendue : %v %s", sc, texte(r))
	}
	return sc
}

func TestTopManagement_RepriseBonValideur(t *testing.T) {
	b := demarrerAvec(t, lancement{registre: cheminRegistre, jeu: jeuTopManagement, demandeur: claire, ecran: true})
	sc := attente(t, b.appeler(t, "banque-synthetique__attribuer_pret", grosPret))
	roles, _ := sc["roles"].([]any)
	if sc["directive"] != "T-001" || len(roles) != 1 || roles[0] != "directeur-risques" || sc["quatre_yeux"] != true {
		t.Fatalf("attente %v", sc)
	}
	emp := sc["empreinte_transmise"].(string)
	r := b.appelerMeta(t, "banque-synthetique__attribuer_pret", grosPret,
		mcp.Meta{"approvalToken": jetonApprobation(t, diane, "directeur-risques", emp)})
	if sc, _ := r.StructuredContent.(map[string]any); sc != nil {
		t.Fatalf("reprise : %v %s", sc, texte(r))
	}
	entrees, appels := b.verifier(t)
	if len(appels) != 2 || appels[1].Approbation == nil || appels[1].Approbation.Valideurs[0].Identite != diane ||
		appels[1].Resultat.Statut == journal.StatutNonTransmis || appels[1].Demandeur == nil || appels[1].Demandeur.Identite != claire {
		t.Fatalf("appel repris %+v", appels[1])
	}
	if n := transmissions(entrees); n["banque-synthetique/attribuer_pret"] != 1 {
		t.Fatalf("transmissions %v", n)
	}
	// Le juridique a été notifié à chaque évaluation, avant transmission,
	// sur l'écran abonné.
	var notes []journal.Entree
	for _, e := range entrees {
		if e.Type == journal.NotificationT {
			notes = append(notes, e)
		}
	}
	if len(notes) != 2 || notes[1].Notification.Statut != journal.RemiseFaite || notes[1].Notification.DestinataireRole != "juridique" {
		t.Fatalf("notifications %+v", notes)
	}
	ecran, err := os.ReadFile(b.notifications)
	if err != nil || bytes.Count(ecran, []byte("\n")) != 2 || !bytes.Contains(ecran, []byte(`"destinataire_role":"juridique"`)) {
		t.Fatalf("écran : %v\n%s", err, ecran)
	}
	out := b.commandeJournal(t)
	contientTout(t, "commande journal", out, "Approbations : acceptee 1", "Notifications : remise 2", "demandeur "+claire)
}

func TestTopManagement_ValideursRefuses(t *testing.T) {
	cas := []struct {
		nom, demandeur, valideur, motif string
	}{
		{"valideur sans le rôle", claire, julie, "ne détenait pas le rôle"},
		{"rôle échu", claire, marc, "ne détenait pas le rôle"},
		{"valideur identique au demandeur", diane, diane, "est le demandeur"},
	}
	for _, c := range cas {
		t.Run(c.nom, func(t *testing.T) {
			b := demarrerAvec(t, lancement{registre: cheminRegistre, jeu: jeuTopManagement, demandeur: c.demandeur, ecran: true})
			sc := attente(t, b.appeler(t, "banque-synthetique__attribuer_pret", grosPret))
			r := b.appelerMeta(t, "banque-synthetique__attribuer_pret", grosPret,
				mcp.Meta{"approvalToken": jetonApprobation(t, c.valideur, "directeur-risques", sc["empreinte_transmise"].(string))})
			sc = attente(t, r)
			if m, _ := sc["motif_jeton"].(string); !strings.Contains(m, c.motif) {
				t.Fatalf("motif %q", m)
			}
			entrees, appels := b.verifier(t)
			if n := transmissions(entrees); len(n) != 0 {
				t.Fatalf("transmissions %v", n)
			}
			var ap []journal.Entree
			for _, e := range entrees {
				if e.Type == journal.ApprobationT {
					ap = append(ap, e)
				}
			}
			if len(ap) != 1 || ap[0].Approbation.Statut != journal.ApprobationRefusee || appels[1].Approbation != nil {
				t.Fatalf("approbations %+v", ap)
			}
			b.commandeJournal(t)
		})
	}
}

// Sans écran abonné, la notification du juridique (si_echec: refuser) ne
// peut être remise : l'appel est refusé, même avec un bon jeton.
func TestTopManagement_SansEcranRefus(t *testing.T) {
	b := demarrerAvec(t, lancement{registre: cheminRegistre, jeu: jeuTopManagement, demandeur: claire})
	r := b.appeler(t, "banque-synthetique__attribuer_pret", grosPret)
	sc := structure(t, r)
	if !r.IsError || sc["statut"] != "refuse" || sc["origine"] != "notification" || sc["directive"] != "T-001" {
		t.Fatalf("%v", sc)
	}
	b.verifier(t)
	b.commandeJournal(t)
}

const jeuRH = "test/integration/testdata/rh.json"

// RH : le gate résout les groupes des salariés désignés par
// personnes_concernees auprès de referentiel-rh (attesté) avant Decide.
func TestRH_PersonnesConcernees(t *testing.T) {
	cas := []struct {
		nom       string
		ids       []any
		statut    string // refuse | en_attente | "" (transmis)
		directive string
		resolus   int
	}{
		{"un salarié en contentieux dans la liste", []any{"SAL-FICTIF-001", "SAL-FICTIF-003"}, "refuse", "R-001", 2},
		{"un salarié protégé dans la liste", []any{"SAL-FICTIF-001", "SAL-FICTIF-002"}, "en_attente", "R-002", 2},
		{"un salarié introuvable", []any{"SAL-FICTIF-001", "SAL-FICTIF-999"}, "refuse", "R-001", 1},
		{"aucun groupe", []any{"SAL-FICTIF-001", "SAL-FICTIF-005"}, "", "", 2},
	}
	for _, c := range cas {
		t.Run(c.nom, func(t *testing.T) {
			b := demarrerAvec(t, lancement{registre: cheminRegistre, jeu: jeuRH})
			r := b.appeler(t, "rh-synthetique__decider_prime", map[string]any{"salarie_ids": c.ids, "montant": 500})
			sc, _ := r.StructuredContent.(map[string]any)
			if c.statut == "" {
				if sc != nil {
					t.Fatalf("transmis attendu : %v", sc)
				}
			} else if sc == nil || sc["statut"] != c.statut || sc["directive"] != c.directive {
				t.Fatalf("%v %s", sc, texte(r))
			}
			_, appels := b.verifier(t)
			a := appels[0]
			if len(a.Sujets) != c.resolus || len(a.Sujets)+len(a.SujetsNonResolus) != len(c.ids) {
				t.Fatalf("sujets %+v, non résolus %v", a.Sujets, a.SujetsNonResolus)
			}
			// Jamais l'identifiant en clair : seulement son empreinte.
			brut, _ := os.ReadFile(b.journal)
			if bytes.Contains(brut, []byte("SAL-FICTIF")) {
				t.Fatal("un identifiant de salarié est écrit en clair au journal")
			}
			b.commandeJournal(t)
		})
	}
	// Sanction : identifiant seul (pas une liste).
	b := demarrerAvec(t, lancement{registre: cheminRegistre, jeu: jeuRH})
	r := b.appeler(t, "rh-synthetique__proposer_sanction", map[string]any{"salarie_id": "SAL-FICTIF-002", "motif": "retards"})
	if sc := structure(t, r); sc["directive"] != "R-003" {
		t.Fatalf("%v", sc)
	}
	b.verifier(t)
	b.commandeJournal(t)
}

// cheminRapports est le registre de démo augmenté du serveur fictif
// rapports, dont l'outil remettre_rapport est étiqueté production_rapport
// par demo/etiquettes.json.
var cheminRapports string

func preparerRegistreRapports(dir string, demo *registre.Registre) error {
	bin := filepath.Join(dir, "rapports")
	ctx, annuler := context.WithTimeout(context.Background(), 30*time.Second)
	defer annuler()
	// Le catalogue épinglé est celui que le serveur annonce.
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "preparation", Version: "0"}, nil).
		Connect(ctx, &mcp.CommandTransport{Command: exec.Command(bin)}, nil)
	if err != nil {
		return err
	}
	liste, err := cs.ListTools(ctx, nil)
	cs.Close()
	if err != nil {
		return err
	}
	brut, err := json.Marshal(liste.Tools)
	if err != nil {
		return err
	}
	outils, err := registredemo.Convertir(brut)
	if err != nil {
		return err
	}
	h, err := empreinte(bin)
	if err != nil {
		return err
	}
	r := *demo
	r.Serveurs = append(append([]registre.Serveur{}, demo.Serveurs...), registre.Serveur{
		Nom: "rapports", Transport: registre.Stdio, Commande: &registre.Commande{Chemin: bin},
		Attestation: registre.Attestation{Mode: registre.ModeBinaire, Attendu: "sha256:" + h}, Outils: outils,
	})
	e, err := registredemo.LireEtiquettes(filepath.Join(racine, registredemo.FichierEtiquettes))
	if err != nil {
		return err
	}
	if err := e.Appliquer(&r); err != nil {
		return err
	}
	r.Meta.Empreinte = ""
	if err := registre.Valider(&r); err != nil {
		return err
	}
	b, err := registredemo.Encoder(&r)
	if err != nil {
		return err
	}
	cheminRapports = filepath.Join(dir, "registre-rapports.json")
	return os.WriteFile(cheminRapports, b, 0o600)
}

const jeuRapport = "test/integration/testdata/rapport.json"

// Demandeur du groupe prestataires-externes, données client reçues, puis
// rapport : refus, et le DPO est notifié.
func TestDemandeur_RapportSurDonneesClient(t *testing.T) {
	rechercher := func(b *banc) {
		if r := b.appeler(t, "recherche-entreprises__rechercher_entreprise", map[string]any{"siren": "382891752"}); r.IsError {
			t.Fatalf("recherche : %s", texte(r))
		}
	}
	rapport := map[string]any{"titre": "Synthèse", "contenu": "Analyse du dossier."}
	cas := []struct {
		nom, demandeur string
		consulter      bool
		refus          bool
	}{
		{"prestataire, données client reçues", "bruno.prestataire@exemple.test", true, true},
		{"prestataire, sans données client", "bruno.prestataire@exemple.test", false, false},
		{"analyste, données client reçues", "claire.analyste@exemple.test", true, false},
	}
	for _, c := range cas {
		t.Run(c.nom, func(t *testing.T) {
			b := demarrerAvec(t, lancement{registre: cheminRapports, jeu: jeuRapport, demandeur: c.demandeur, ecran: true})
			if c.consulter {
				rechercher(b)
			}
			r := b.appeler(t, "rapports__remettre_rapport", rapport)
			sc, _ := r.StructuredContent.(map[string]any)
			if c.refus != (sc != nil && sc["statut"] == "refuse" && sc["directive"] == "U-001") {
				t.Fatalf("%v %s", sc, texte(r))
			}
			entrees, appels := b.verifier(t)
			dernier := appels[len(appels)-1]
			if strings.Join(dernier.EtiquettesCible, ",") != "production_rapport" || dernier.Demandeur == nil {
				t.Fatalf("appel %+v", dernier)
			}
			notes := 0
			for _, e := range entrees {
				if e.Type == journal.NotificationT && e.Notification.DestinataireRole == "dpo" {
					notes++
					if e.Notification.Statut != journal.RemiseFaite {
						t.Fatalf("remise %+v", e.Notification)
					}
				}
			}
			if (notes == 1) != c.refus {
				t.Fatalf("%d notification(s) au DPO", notes)
			}
			b.commandeJournal(t)
		})
	}
}

// Deux processus mcp-gate successifs, comme dans la démo où chaque demande
// lance le sien : le jeton consommé par le premier est refusé par le
// second, qui relit le journal du premier.
func TestJetonConsommeParUnProcessusPrecedent(t *testing.T) {
	b1 := demarrerAvec(t, lancement{registre: cheminRegistre, jeu: jeuTopManagement, demandeur: claire, ecran: true})
	sc := attente(t, b1.appeler(t, "banque-synthetique__attribuer_pret", grosPret))
	jeton := jetonApprobation(t, diane, "directeur-risques", sc["empreinte_transmise"].(string))
	if r := b1.appelerMeta(t, "banque-synthetique__attribuer_pret", grosPret, mcp.Meta{"approvalToken": jeton}); r.StructuredContent != nil {
		t.Fatalf("premier processus : %v", r.StructuredContent)
	}
	b1.verifier(t)

	b2 := demarrerAvec(t, lancement{registre: cheminRegistre, jeu: jeuTopManagement, demandeur: claire, ecran: true,
		precedents: filepath.Join(filepath.Dir(b1.journal), "journal*.jsonl")})
	sc = attente(t, b2.appelerMeta(t, "banque-synthetique__attribuer_pret", grosPret, mcp.Meta{"approvalToken": jeton}))
	if m, _ := sc["motif_jeton"].(string); !strings.Contains(m, "déjà utilisé") {
		t.Fatalf("second processus : %v", sc)
	}
	entrees, _ := b2.verifier(t)
	if n := transmissions(entrees); len(n) != 0 {
		t.Fatalf("transmissions %v", n)
	}
	refusee := false
	for _, e := range entrees {
		refusee = refusee || (e.Type == journal.ApprobationT && e.Approbation.Statut == journal.ApprobationRefusee &&
			strings.Contains(e.Approbation.Motif, "jeton déjà utilisé"))
	}
	if !refusee {
		t.Fatal("pas d'entrée approbation refusée « jeton déjà utilisé »")
	}
	b2.commandeJournal(t)

	// Un journal précédent dont la chaîne est rompue empêche le démarrage.
	dir := t.TempDir()
	brut, err := os.ReadFile(b1.journal)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "rompu.jsonl"), bytes.Replace(brut, []byte(`"acceptee"`), []byte(`"refusee"`), 1), 0o600)
	cmd := exec.Command(binaireGate, "-registre", cheminRegistre, "-directives", filepath.Join(racine, jeuTopManagement),
		"-journal", filepath.Join(dir, "journal.jsonl"), "-coffre", filepath.Join(dir, "coffre.jsonl"),
		"-journaux-precedents", filepath.Join(dir, "rompu.jsonl"))
	cmd.Env = []string{"MCP_GATE_CLE_JOURNAL=" + hex.EncodeToString(cleTest), "DONNEES_MODE=REJEU"}
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "rompu.jsonl") || !strings.Contains(string(out), "chaîne rompue") ||
		!strings.Contains(string(out), "démarrage refusé") {
		t.Fatalf("démarrage sur journal rompu : %v\n%s", err, out)
	}
}

// Un gate par demande (contrats de la démo, §2) : chaque demande a son
// dossier sessions/<id>/ et son journal ; le motif sessions/*/journal.jsonl
// désigne tous les journaux, y compris celui de la demande qui démarre
// (absent à ce moment). Le jeton consommé dans la première demande est
// refusé dans la deuxième, et la troisième relit les deux chaînes.
func TestJetonConsommeUnGateParDemande(t *testing.T) {
	sessions := t.TempDir()
	motif := filepath.Join(sessions, "*", "journal.jsonl")
	demande := func(id string) *banc {
		d := filepath.Join(sessions, id)
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
		return demarrerAvec(t, lancement{registre: cheminRegistre, jeu: jeuTopManagement, demandeur: claire, ecran: true,
			dossier: d, precedents: motif})
	}
	b1 := demande("20261004-100000-a")
	sc := attente(t, b1.appeler(t, "banque-synthetique__attribuer_pret", grosPret))
	jeton := jetonApprobation(t, diane, "directeur-risques", sc["empreinte_transmise"].(string))
	if r := b1.appelerMeta(t, "banque-synthetique__attribuer_pret", grosPret, mcp.Meta{"approvalToken": jeton}); r.StructuredContent != nil {
		t.Fatalf("première demande : %v", r.StructuredContent)
	}
	b1.verifier(t)

	for _, id := range []string{"20261004-100100-b", "20261004-100200-c"} {
		b := demande(id)
		sc = attente(t, b.appelerMeta(t, "banque-synthetique__attribuer_pret", grosPret, mcp.Meta{"approvalToken": jeton}))
		if m, _ := sc["motif_jeton"].(string); !strings.Contains(m, "déjà utilisé") {
			t.Fatalf("demande %s : %v", id, sc)
		}
		if entrees, _ := b.verifier(t); len(transmissions(entrees)) != 0 {
			t.Fatalf("demande %s : transmission", id)
		}
	}
}
