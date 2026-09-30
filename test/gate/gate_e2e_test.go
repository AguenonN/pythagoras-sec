// Package gatee2e est le point de contrôle de l'agrégateur : un gate devant
// deux backends réels de transports différents, taux-officiel en stdio (un
// binaire compilé pendant le test, empreinte vérifiée) et
// banque-synthetique en HTTP streamable derrière TLS (clé épinglée), avec
// une directive de provenance chargée.
//
// Scénario : l'agent attribue un premier prêt (routé en HTTP), consulte le
// taux (routé en stdio), puis tente un second prêt. La seconde décision sur
// la banque doit citer la provenance, et le journal relu depuis le disque
// doit se rejouer sans écart.
package gatee2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/internal/backendtest"
	"mcprogue/internal/directive"
	"mcprogue/internal/gate"
	"mcprogue/internal/journal"
	"mcprogue/internal/registre"
	"mcprogue/internal/rejeu"
	"mcprogue/internal/session"
)

var (
	serveurTest, empreinteServeur string
	binaireGate                   string
	// Clé de test, sans valeur hors de ce fichier.
	cleTest = []byte(strings.Repeat("e2e-", 8))
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "gate-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := 1
	defer func() { os.RemoveAll(dir); os.Exit(code) }()
	if serveurTest, empreinteServeur, err = backendtest.Construire(dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	binaireGate = filepath.Join(dir, "mcp-gate")
	cmd := exec.Command("go", "build", "-o", binaireGate, "mcprogue/cmd/mcp-gate")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "compiler mcp-gate:", err)
		return
	}
	code = m.Run()
}

const jeuProvenance = `{
  "jeu": {"id": "credit-pme-gate", "version": 1, "date_effet": "2026-01-01T00:00:00Z", "auteur": "juriste@cabinet.fr"},
  "directives": [
    {"id": "D-001", "version": 1, "statut": "active", "date_effet": "2026-01-01T00:00:00Z",
     "texte_fr": "Aucun prêt ne doit être attribué de manière autonome à partir des informations du serveur de taux officiel.",
     "fondement": {"texte": "RGPD", "article": "22", "url": "https://eur-lex.europa.eu/eli/reg/2016/679/oj"},
     "verrou": "provenance_session", "cible": {"serveur": "banque-synthetique", "outil": "attribuer_pret"},
     "provenance": {"serveurs": ["taux-officiel"]}, "effet": "refuser"},
    {"id": "D-003", "version": 1, "statut": "active", "date_effet": "2026-01-01T00:00:00Z",
     "texte_fr": "Toute source de taux non attestée est rejetée.",
     "fondement": {"texte": "RGPD", "article": "5"},
     "verrou": "source_attestee", "cible": {"serveur": "taux-officiel", "outil": "*"}, "effet": "refuser"}
  ]
}`

// Le même jeu sans la directive de provenance, pour la vue avant / après.
const jeuSansProvenance = `{
  "jeu": {"id": "credit-pme-gate", "version": 2, "date_effet": "2026-01-01T00:00:00Z", "auteur": "juriste@cabinet.fr"},
  "directives": [
    {"id": "D-003", "version": 1, "statut": "active", "date_effet": "2026-01-01T00:00:00Z",
     "texte_fr": "Toute source de taux non attestée est rejetée.",
     "fondement": {"texte": "RGPD", "article": "5"},
     "verrou": "source_attestee", "cible": {"serveur": "taux-officiel", "outil": "*"}, "effet": "refuser"}
  ]
}`

func charger(t *testing.T, doc string) directive.Jeu {
	t.Helper()
	j, err := directive.Charger([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	return *j
}

// banque sert le profil banque en HTTP streamable derrière TLS et rend
// l'entrée de registre qui l'épingle.
func banque(t *testing.T) registre.Serveur {
	t.Helper()
	srv := backendtest.Serveur(backendtest.Banque)
	ts := httptest.NewUnstartedServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil))
	ts.EnableHTTP2 = true
	ts.Config.ErrorLog = log.New(io.Discard, "", 0)
	ts.StartTLS()
	t.Cleanup(ts.Close)
	somme := sha256.Sum256(ts.Certificate().RawSubjectPublicKeyInfo)
	return backendtest.HTTP("banque-synthetique", backendtest.Banque, ts.URL+"/mcp",
		"sha256:"+hex.EncodeToString(somme[:]), "example.com")
}

func registreE2E(t *testing.T) *registre.Registre {
	t.Helper()
	b := banque(t)
	r := &registre.Registre{
		Meta: registre.MetaRegistre{ID: "lab-e2e", Version: 1, Auteur: "ops@cabinet.fr"},
		Serveurs: []registre.Serveur{
			backendtest.Stdio("taux-officiel", backendtest.Taux, serveurTest, empreinteServeur),
			b,
		},
	}
	if err := registre.Valider(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func texte(r *mcp.CallToolResult) string {
	var out []string
	for _, c := range r.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			out = append(out, tc.Text)
		}
	}
	return strings.Join(out, "\n")
}

func appeler(t *testing.T, cs *mcp.ClientSession, nom string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	ctx, annuler := context.WithTimeout(context.Background(), 20*time.Second)
	defer annuler()
	r, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: nom, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", nom, err)
	}
	return r
}

// scenario joue le scénario du point de contrôle et vérifie les réponses.
func scenario(t *testing.T, cs *mcp.ClientSession) {
	t.Helper()
	outils, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(outils.Tools) != 2 {
		t.Fatalf("outils exposés %d", len(outils.Tools))
	}

	// 1. Action sur B, sans provenance : routée en HTTP.
	r := appeler(t, cs, "banque-synthetique__attribuer_pret", map[string]any{"montant": 1000, "siren": "552100554"})
	if r.IsError || !strings.Contains(texte(r), "prêt de 1000 accordé à 552100554") || !strings.Contains(texte(r), "<untrusted_data") {
		t.Fatalf("prêt 1 : %q", texte(r))
	}
	// 2. Lecture sur A : routée en stdio.
	r = appeler(t, cs, "taux-officiel__get_taux", map[string]any{"devise": "EUR"})
	if r.IsError || !strings.Contains(texte(r), "taux EUR 3,15 %") || !strings.Contains(texte(r), "<untrusted_data") {
		t.Fatalf("taux : %q", texte(r))
	}
	// 3. Action sur B après consultation de A : la décision cite la provenance.
	r = appeler(t, cs, "banque-synthetique__attribuer_pret", map[string]any{"montant": 2000, "siren": "552100554"})
	m := texte(r)
	for _, attendu := range []string{"D-001 v1", "provenance_session", "RGPD art. 22"} {
		if !r.IsError || !strings.Contains(m, attendu) {
			t.Fatalf("prêt 2 : %q ne cite pas %q", m, attendu)
		}
	}
	if strings.Contains(m, "accordé") {
		t.Fatalf("prêt 2 transmis : %q", m)
	}
}

// verifierJournal relit journal et coffre depuis le disque, vérifie la
// chaîne, et exige un rejeu sans écart.
func verifierJournal(t *testing.T, cheminJ, cheminC string, jeu directive.Jeu) []journal.Entree {
	t.Helper()
	brut, err := os.ReadFile(cheminJ)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Verifier(bytes.NewReader(brut), cleTest); err != nil {
		t.Fatalf("chaîne : %v", err)
	}
	entrees, err := journal.Lire(bytes.NewReader(brut))
	if err != nil {
		t.Fatal(err)
	}
	coffre, err := journal.OuvrirCoffre(cheminC)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { coffre.Fermer() })

	var appels []journal.Entree
	for _, e := range entrees {
		if e.Type == journal.AppelT {
			appels = append(appels, e)
		}
	}
	if len(appels) != 3 {
		t.Fatalf("%d entrées appel", len(appels))
	}
	statuts := []string{journal.StatutOK, journal.StatutOK, journal.StatutNonTransmis}
	serveurs := []string{"banque-synthetique", "taux-officiel", "banque-synthetique"}
	for i, a := range appels {
		if a.Resultat.Statut != statuts[i] || a.Serveur != serveurs[i] || a.Seq != i+1 || !a.Attestation.Valide {
			t.Fatalf("appel %d : %s %s seq %d attesté %v", i+1, a.Serveur, a.Resultat.Statut, a.Seq, a.Attestation.Valide)
		}
	}
	if appels[0].Attestation.Mode != "tls-pin" || appels[1].Attestation.Mode != "binaire" {
		t.Fatalf("modes %s %s", appels[0].Attestation.Mode, appels[1].Attestation.Mode)
	}
	// La provenance est tenue par l'engine : l'instantané de l'appel 3
	// porte les deux consultations, et D-001 a déclenché sur taux-officiel.
	a3 := appels[2]
	if len(a3.EtatSession) != 2 || a3.EtatSession[1].Serveur != "taux-officiel" || a3.EtatSession[1].Resultat != "ok" {
		t.Fatalf("état de session %+v", a3.EtatSession)
	}
	d001 := false
	for _, app := range a3.Decision.Applications {
		if app.ID == "D-001" && app.Declenchee && app.Appliquee && strings.Contains(app.Motif, "la session a consulté taux-officiel (ok)") {
			d001 = true
		}
	}
	if !d001 || a3.Decision.Effet != "refuser" {
		t.Fatalf("décision %+v", a3.Decision)
	}

	// Les deux appels transmis ont chacun leur entrée transmission, et
	// aucune n'est orpheline.
	nbTransmissions := 0
	for _, e := range entrees {
		if e.Type == journal.TransmissionT {
			nbTransmissions++
		}
	}
	if nbTransmissions != 2 || len(rejeu.TransmissionsSansAppel(entrees)) != 0 {
		t.Fatalf("%d transmission(s), %d orpheline(s)", nbTransmissions, len(rejeu.TransmissionsSansAppel(entrees)))
	}
	rs, err := rejeu.RejouerJournal(entrees, coffre, rejeu.Catalogue{jeu}, rejeu.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 3 {
		t.Fatalf("%d rejeux", len(rs))
	}
	for _, r := range rs {
		if !r.Rejouable || len(r.Ecarts) != 0 || r.Nature != rejeu.Violation || string(r.Journalise) != r.Effet() {
			t.Fatalf("rejeu seq %d : %+v", r.Seq, r)
		}
	}
	return entrees
}

func TestPointDeControle(t *testing.T) {
	dir := t.TempDir()
	cheminJ, cheminC := filepath.Join(dir, "journal.jsonl"), filepath.Join(dir, "coffre.jsonl")
	jeu := charger(t, jeuProvenance)
	reg := registreE2E(t)

	j, err := journal.Ouvrir(cheminJ, cleTest)
	if err != nil {
		t.Fatal(err)
	}
	coffre, err := journal.OuvrirCoffre(cheminC)
	if err != nil {
		t.Fatal(err)
	}
	ctx, annuler := context.WithTimeout(context.Background(), 30*time.Second)
	defer annuler()
	g, err := gate.Demarrer(ctx, gate.Config{
		Registre: reg, Jeu: jeu, Journal: j, Coffre: coffre, Appelant: gate.AppelantStdio,
		Session: session.Config{DelaiBackend: 10 * time.Second},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, nom := range []string{"taux-officiel", "banque-synthetique"} {
		if a := g.Backend(nom).Attestation(); !a.Valide() {
			t.Fatalf("%s non attesté : %+v", nom, a)
		}
	}

	tc, ts := mcp.NewInMemoryTransports()
	ss, err := g.Serveur().Connect(ctx, ts, nil)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "0"}, nil).Connect(ctx, tc, nil)
	if err != nil {
		t.Fatal(err)
	}
	scenario(t, cs)
	cs.Close()
	ss.Wait()
	// Le départ du client ferme la session (session_fermee) ; on attend
	// qu'elle soit écrite avant de fermer le journal.
	for i := 0; i < 100 && g.Sessions().Session(ss) != nil; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	g.Fermer()
	j.Fermer()
	coffre.Fermer()

	entrees := verifierJournal(t, cheminJ, cheminC, jeu)
	ferme := false
	for _, e := range entrees {
		if e.Type == journal.SessionFermee && e.NbConsultations == 2 && e.DernierSeq == 3 {
			ferme = true
		}
	}
	if !ferme {
		t.Fatal("pas d'entrée session_fermee complète")
	}

	// Vue avant / après : sans D-001, le second prêt aurait été autorisé.
	c2, err := journal.OuvrirCoffre(cheminC)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Fermer()
	sans := charger(t, jeuSansProvenance)
	cmp, err := rejeu.Comparer(entrees, c2, rejeu.Catalogue{jeu, sans}, rejeu.Options{}, rejeu.Options{Jeu: &sans})
	if err != nil {
		t.Fatal(err)
	}
	if l := cmp.Lignes[2]; l.Avant != "refuser" || l.Apres != "autoriser" {
		t.Fatalf("ligne 3 %+v", l)
	}
}

// Le même scénario à travers le binaire cmd/mcp-gate, lancé comme un hôte
// MCP le lancerait : l'agent parle au gate en stdio.
func TestPointDeControle_Binaire(t *testing.T) {
	dir := t.TempDir()
	cheminJ, cheminC := filepath.Join(dir, "journal.jsonl"), filepath.Join(dir, "coffre.jsonl")
	cheminR, cheminD := filepath.Join(dir, "registre.json"), filepath.Join(dir, "directives.json")
	reg := registreE2E(t)
	b, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cheminR, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cheminD, []byte(jeuProvenance), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(binaireGate, "-registre", cheminR, "-directives", cheminD, "-journal", cheminJ, "-coffre", cheminC)
	cmd.Env = []string{"MCP_GATE_CLE_JOURNAL=" + hex.EncodeToString(cleTest)}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	ctx, annuler := context.WithTimeout(context.Background(), 60*time.Second)
	defer annuler()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "0"}, nil).Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("%v\n%s", err, stderr.String())
	}
	scenario(t, cs)
	if err := cs.Close(); err != nil {
		t.Fatalf("fermeture du gate : %v\n%s", err, stderr.String())
	}
	if !strings.Contains(stderr.String(), "attesté=true") {
		t.Fatalf("traces du gate :\n%s", stderr.String())
	}
	verifierJournal(t, cheminJ, cheminC, charger(t, jeuProvenance))
}
