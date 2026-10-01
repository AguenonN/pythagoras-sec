// Package integration fait tourner le gate devant les vrais serveurs, de
// bout en bout, sans réseau ni modèle : un client MCP joue l'agent devant
// le binaire cmd/mcp-gate, lancé avec le registre de démo (généré par
// internal/registredemo depuis les binaires construits par le test), le
// jeu demo/directives/credit-demo.json, les serveurs de données en REJEU et
// le registre des gels synthétique.
//
// Quatre scénarios, chacun sur un gate neuf :
//
//  1. nominal : rechercher_entreprise avec des champs en trop (caviardés),
//     verifier_gel, get_taux_usure au 2026-10-04 (T4), puis attribuer_pret
//     de DOS-FICTIF-002 mis en attente par la provenance (D-001) ;
//  2. seuil : attribuer_pret à 900 000 € refusé par D-002 ;
//  3. attaque : registre détourné, l'imposteur n'est jamais lancé et
//     get_taux_usure est refusé par source_attestee (D-003) ;
//  4. base périmée : get_taux_interne rend le T3, et le journal consigne
//     la consultation.
//
// Pour chacun, le journal relu depuis le disque se vérifie (chaîne HMAC)
// et se rejoue sans écart.
package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/internal/directive"
	"mcprogue/internal/journal"
	"mcprogue/internal/registre"
	"mcprogue/internal/registredemo"
	"mcprogue/internal/rejeu"
)

var (
	racine string
	// Binaires construits par TestMain.
	binaireGate string
	// Registres générés depuis ces binaires.
	cheminRegistre, cheminAttaque string
	empreinteImposteur            string
	// Annuaire de démonstration, généré avec des clés neuves.
	cheminAnnuaire, cheminAutorite, dirCles string
	binaireJournal                          string
	// Clé de test, sans valeur hors de ce fichier.
	cleTest = []byte(strings.Repeat("integ-", 6))
)

const cheminDirectives = "demo/directives/credit-demo.json"

func TestMain(m *testing.M) {
	os.Exit(preparer(m))
}

// preparer construit les neuf serveurs et le gate dans un répertoire
// temporaire, écrit le manifeste qui les décrit et génère les deux
// registres, comme scripts/lancer-demo.sh, mais sans toucher à bin/ ni à
// demo/.
func preparer(m *testing.M) int {
	var err error
	if racine, err = filepath.Abs("../.."); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	dir, err := os.MkdirTemp("", "integration-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)

	noms := append(append([]string{}, registredemo.Serveurs...), registredemo.Imposteur)
	paquets := []string{"./cmd/mcp-gate", "./cmd/journal", "./test/integration/rapports"}
	for _, n := range noms {
		paquets = append(paquets, "./servers/"+n)
	}
	cmd := exec.Command("go", append([]string{"build", "-trimpath", "-o", dir + string(filepath.Separator)}, paquets...)...)
	cmd.Dir = racine
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "construire les binaires :", err)
		return 1
	}
	binaireGate = filepath.Join(dir, "mcp-gate")
	binaireJournal = filepath.Join(dir, "journal")
	if err := preparerAnnuaire(dir); err != nil {
		fmt.Fprintln(os.Stderr, "annuaire :", err)
		return 1
	}

	man := &registredemo.Manifeste{}
	for _, n := range noms {
		bin := filepath.Join(dir, n)
		schema := filepath.Join("servers", n, "schema.json")
		e := registredemo.EntreeManifeste{Nom: n, Binaire: bin, Schema: schema}
		if e.SHA256, err = empreinte(bin); err == nil {
			e.SchemaSHA256, err = empreinte(filepath.Join(racine, schema))
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		man.Serveurs = append(man.Serveurs, e)
		if n == registredemo.Imposteur {
			empreinteImposteur = "sha256:" + e.SHA256
		}
	}
	demo, attaque, err := registredemo.Generer(man, registredemo.Options{Racine: racine, Gels: registredemo.DossierGelsSynthetique})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	cheminRegistre, cheminAttaque = filepath.Join(dir, "registre.json"), filepath.Join(dir, "registre-attaque.json")
	for chemin, r := range map[string]*registre.Registre{cheminRegistre: demo, cheminAttaque: attaque} {
		b, err := registredemo.Encoder(r)
		if err == nil {
			err = os.WriteFile(chemin, b, 0o600)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	if err := preparerRegistreRapports(dir, demo); err != nil {
		fmt.Fprintln(os.Stderr, "registre avec rapports :", err)
		return 1
	}
	return m.Run()
}

func empreinte(chemin string) (string, error) {
	f, err := os.Open(chemin)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// tampon est un bytes.Buffer sûr entre le processus qui écrit et le test
// qui lit.
type tampon struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (t *tampon) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.b.Write(p)
}

func (t *tampon) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.b.String()
}

// banc est un gate lancé et l'agent qui lui parle.
type banc struct {
	cs              *mcp.ClientSession
	stderr          *tampon
	journal, coffre string
	ferme           bool
	jeu             string // chemin du jeu, relatif à la racine
	notifications   string // écran des notifications, s'il y en a un
}

// lancement décrit un gate à démarrer.
type lancement struct {
	registre string
	jeu      string // relatif à la racine ; défaut cheminDirectives
	// depart, s'il est posé, est l'instant de départ de l'horloge simulée.
	depart string
	// annuaire charge l'annuaire de démo ; demandeur, s'il est posé, est
	// l'identité dont le jeton (signé par elle-même) est remis au gate.
	annuaire  bool
	demandeur string
	// ecran abonne un écran de notifications au journal.
	ecran bool
	// precedents est le motif des journaux de processus précédents.
	precedents string
}

// demarrer lance cmd/mcp-gate comme un hôte MCP le lancerait, avec un
// environnement réduit : la clé du journal et DONNEES_MODE=REJEU, que le
// registre transmet par nom aux serveurs. Aucun secret PISTE : legifrance
// ne sert que ses enregistrements.
func demarrer(t *testing.T, cheminReg string) *banc {
	return demarrerAvec(t, lancement{registre: cheminReg})
}

func demarrerAvec(t *testing.T, l lancement) *banc {
	t.Helper()
	dir := t.TempDir()
	if l.jeu == "" {
		l.jeu = cheminDirectives
	}
	b := &banc{stderr: &tampon{}, journal: filepath.Join(dir, "journal.jsonl"), coffre: filepath.Join(dir, "coffre.jsonl"), jeu: l.jeu}
	args := []string{"-registre", l.registre, "-directives", filepath.Join(racine, l.jeu), "-journal", b.journal, "-coffre", b.coffre}
	env := []string{"MCP_GATE_CLE_JOURNAL=" + hex.EncodeToString(cleTest), "DONNEES_MODE=REJEU"}
	if l.depart != "" {
		args = append(args, "-horloge-depart", l.depart)
	}
	if l.annuaire || l.demandeur != "" {
		args = append(args, "-annuaire", cheminAnnuaire, "-annuaire-autorite", cheminAutorite)
	}
	if l.demandeur != "" {
		env = append(env, "MCP_GATE_JETON_DEMANDEUR="+jetonIdentite(t, l.demandeur, l.depart))
	}
	if l.precedents != "" {
		args = append(args, "-journaux-precedents", l.precedents)
	}
	if l.ecran {
		b.notifications = filepath.Join(dir, "notifications.jsonl")
		args = append(args, "-notifications", b.notifications)
	}
	cmd := exec.Command(binaireGate, args...)
	cmd.Env = env
	cmd.Stderr = b.stderr
	ctx, annuler := context.WithTimeout(context.Background(), 60*time.Second)
	defer annuler()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "agent-integration", Version: "0"}, nil).
		Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("démarrage du gate : %v\n%s", err, b.stderr.String())
	}
	b.cs = cs
	t.Cleanup(func() { b.fermer(t) })
	return b
}

// fermer ferme la session de l'agent ; le gate s'arrête et ferme son
// journal.
func (b *banc) fermer(t *testing.T) {
	t.Helper()
	if b.ferme {
		return
	}
	b.ferme = true
	if err := b.cs.Close(); err != nil {
		t.Errorf("arrêt du gate : %v\n%s", err, b.stderr.String())
	}
}

func (b *banc) appeler(t *testing.T, nom string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	ctx, annuler := context.WithTimeout(context.Background(), 30*time.Second)
	defer annuler()
	r, err := b.cs.CallTool(ctx, &mcp.CallToolParams{Name: nom, Arguments: args})
	if err != nil {
		t.Fatalf("%s : %v\n%s", nom, err, b.stderr.String())
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

func structure(t *testing.T, r *mcp.CallToolResult) map[string]any {
	t.Helper()
	sc, ok := r.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent absent ou illisible : %#v (%s)", r.StructuredContent, texte(r))
	}
	return sc
}

// verifier ferme le gate, relit journal et coffre depuis le disque,
// vérifie la chaîne et exige un rejeu sans écart de chaque appel. Rend
// toutes les entrées et les seules entrées appel.
func (b *banc) verifier(t *testing.T) (entrees, appels []journal.Entree) {
	t.Helper()
	b.fermer(t)
	brut, err := os.ReadFile(b.journal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Verifier(bytes.NewReader(brut), cleTest); err != nil {
		t.Fatalf("chaîne du journal : %v", err)
	}
	if entrees, err = journal.Lire(bytes.NewReader(brut)); err != nil {
		t.Fatal(err)
	}
	coffre, err := journal.OuvrirCoffre(b.coffre)
	if err != nil {
		t.Fatal(err)
	}
	defer coffre.Fermer()
	jeu := chargerJeuChemin(t, b.jeu)
	rs, err := rejeu.RejouerJournal(entrees, coffre, rejeu.Catalogue{jeu}, rejeu.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entrees {
		if e.Type == journal.AppelT {
			appels = append(appels, e)
		}
	}
	if len(rs) != len(appels) || len(rs) == 0 {
		t.Fatalf("%d rejeux pour %d appels", len(rs), len(appels))
	}
	for _, r := range rs {
		if !r.Rejouable || len(r.Ecarts) != 0 || string(r.Journalise) != r.Effet() {
			t.Fatalf("rejeu seq %d (%s/%s) : rejouable %v, journalisé %s, rejoué %s, écarts %+v",
				r.Seq, r.Serveur, r.Outil, r.Rejouable, r.Journalise, r.Effet(), r.Ecarts)
		}
	}
	if o := rejeu.TransmissionsSansAppel(entrees); len(o) != 0 {
		t.Fatalf("%d transmission(s) sans appel", len(o))
	}
	return entrees, appels
}

func chargerJeuChemin(t *testing.T, chemin string) directive.Jeu {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(racine, chemin))
	if err != nil {
		t.Fatal(err)
	}
	j, err := directive.Charger(b)
	if err != nil {
		t.Fatal(err)
	}
	return *j
}

// transmissions compte les entrées transmission par (serveur, outil).
func transmissions(entrees []journal.Entree) map[string]int {
	n := map[string]int{}
	for _, e := range entrees {
		if e.Type == journal.TransmissionT {
			n[e.Serveur+"/"+e.Outil]++
		}
	}
	return n
}

// attestation rend la dernière attestation journalisée d'un serveur.
func attestation(entrees []journal.Entree, serveur string) *journal.Attestation {
	var a *journal.Attestation
	for _, e := range entrees {
		if e.Type == journal.AttestationT && e.Serveur == serveur {
			a = e.Attestation
		}
	}
	return a
}

func contientTout(t *testing.T, quoi, s string, attendus ...string) {
	t.Helper()
	for _, a := range attendus {
		if !strings.Contains(s, a) {
			t.Fatalf("%s ne contient pas %q :\n%s", quoi, a, s)
		}
	}
}

// Tous les serveurs du registre de démo sont attestés, et le gate expose
// leurs outils sous serveur__outil.
func TestDemarrage(t *testing.T) {
	b := demarrer(t, cheminRegistre)
	outils, err := b.cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(outils.Tools) != 13 {
		t.Fatalf("%d outils exposés", len(outils.Tools))
	}
	entrees, _ := func() ([]journal.Entree, []journal.Entree) {
		b.fermer(t)
		brut, err := os.ReadFile(b.journal)
		if err != nil {
			t.Fatal(err)
		}
		e, err := journal.Lire(bytes.NewReader(brut))
		if err != nil {
			t.Fatal(err)
		}
		return e, nil
	}()
	for _, s := range registredemo.Serveurs {
		if a := attestation(entrees, s); a == nil || !a.Valide || !a.Schemas || a.Mode != registre.ModeBinaire {
			t.Fatalf("%s non attesté : %+v\n%s", s, a, b.stderr.String())
		}
	}
}

// 1. Nominal.
func TestNominal(t *testing.T) {
	b := demarrer(t, cheminRegistre)

	// Des champs en trop vers recherche-entreprises : D-004 les caviarde,
	// seul le SIREN part.
	r := b.appeler(t, "recherche-entreprises__rechercher_entreprise", map[string]any{
		"siren": "382891752", "raison_sociale": "BIOCOOP", "dirigeant": "Personne Fictive", "note": "client fidèle",
	})
	if r.IsError {
		t.Fatalf("rechercher_entreprise : %s", texte(r))
	}
	contientTout(t, "rechercher_entreprise", texte(r), "Note de l'engine", "<untrusted_data", "BIOCOOP")
	if r.StructuredContent != nil {
		t.Fatal("structuredContent en clair dans une réponse normale")
	}

	r = b.appeler(t, "gels-avoirs__verifier_gel", map[string]any{"nom": "BIOCOOP"})
	if r.IsError {
		t.Fatalf("verifier_gel : %s", texte(r))
	}
	contientTout(t, "verifier_gel", texte(r), "<untrusted_data", "synthetique")

	r = b.appeler(t, "taux-officiel__get_taux_usure", map[string]any{
		"categorie": "decouvert-societes-non-financieres", "date_reference": "2026-10-04",
	})
	if r.IsError {
		t.Fatalf("get_taux_usure : %s", texte(r))
	}
	contientTout(t, "get_taux_usure", texte(r), `"trimestre":"2026-Q4"`, `"origine":"rejeu"`)

	// Le taux officiel a été consulté : D-001 met l'attribution en attente.
	r = b.appeler(t, "banque-synthetique__attribuer_pret", map[string]any{
		"dossier_id": "DOS-FICTIF-002", "siren": "382891752", "montant": 400000,
	})
	sc := structure(t, r)
	if !r.IsError || sc["statut"] != "en_attente" || sc["directive"] != "D-001" || sc["version"] != float64(1) {
		t.Fatalf("attribuer_pret : %v %s", sc, texte(r))
	}
	empreinteAttente, _ := sc["empreinte_transmise"].(string)
	if _, err := hex.DecodeString(empreinteAttente); err != nil || len(empreinteAttente) != 64 {
		t.Fatalf("empreinte_transmise %q", empreinteAttente)
	}

	entrees, appels := b.verifier(t)
	if len(appels) != 4 {
		t.Fatalf("%d appels", len(appels))
	}
	outils := []string{"rechercher_entreprise", "verifier_gel", "get_taux_usure", "attribuer_pret"}
	statuts := []string{journal.StatutOK, journal.StatutOK, journal.StatutOK, journal.StatutNonTransmis}
	effets := []string{"caviarder", "autoriser", "autoriser", "attendre_validation"}
	for i, a := range appels {
		if a.Outil != outils[i] || a.Resultat.Statut != statuts[i] || a.Decision.Effet != effets[i] || a.Seq != i+1 || !a.Attestation.Valide {
			t.Fatalf("appel %d : %s %s %s seq %d attesté %v", i+1, a.Outil, a.Resultat.Statut, a.Decision.Effet, a.Seq, a.Attestation.Valide)
		}
	}
	// Caviardage : empreintes d'origine et transmise distinctes, champs
	// retirés nommés, et l'origine complète au coffre.
	cav := appels[0].Appel
	if cav.EmpreinteOrigine == cav.EmpreinteTransmise || strings.Join(cav.ChampsRetires, ",") != "/dirigeant,/note,/raison_sociale" {
		t.Fatalf("caviardage : %+v", cav)
	}
	for i := 1; i < 4; i++ {
		if appels[i].Appel.EmpreinteOrigine != appels[i].Appel.EmpreinteTransmise {
			t.Fatalf("appel %d : empreintes distinctes sans caviardage", i+1)
		}
	}
	// Provenance déclarée par les serveurs de données : rejeu.
	for i := 0; i < 3; i++ {
		if s := appels[i].Resultat.Source; s == nil || s.Origine != "rejeu" {
			t.Fatalf("appel %d : source %+v", i+1, s)
		}
	}
	// L'attente vise l'empreinte transmise journalisée, sans transmission.
	if appels[3].Appel.EmpreinteTransmise != empreinteAttente {
		t.Fatal("l'empreinte rendue à l'agent n'est pas celle du journal")
	}
	if n := transmissions(entrees); n["recherche-entreprises/rechercher_entreprise"] != 1 || n["gels-avoirs/verifier_gel"] != 1 ||
		n["taux-officiel/get_taux_usure"] != 1 || n["banque-synthetique/attribuer_pret"] != 0 {
		t.Fatalf("transmissions %v", n)
	}
	// La décision cite la consultation de taux-officiel.
	d001 := false
	for _, app := range appels[3].Decision.Applications {
		if app.ID == "D-001" && app.Appliquee && strings.Contains(app.Motif, "taux-officiel") {
			d001 = true
		}
	}
	if !d001 || len(appels[3].EtatSession) != 3 {
		t.Fatalf("décision %+v, état %+v", appels[3].Decision, appels[3].EtatSession)
	}
}

// 2. Seuil.
func TestSeuil(t *testing.T) {
	b := demarrer(t, cheminRegistre)
	r := b.appeler(t, "banque-synthetique__attribuer_pret", map[string]any{
		"dossier_id": "DOS-FICTIF-002", "siren": "382891752", "montant": 900000,
	})
	sc := structure(t, r)
	f, _ := sc["fondement"].(map[string]any)
	if !r.IsError || sc["statut"] != "refuse" || sc["origine"] != "directive" || sc["directive"] != "D-002" ||
		sc["version"] != float64(1) || f == nil || f["article"] != "22" {
		t.Fatalf("attribuer_pret 900 000 : %v", sc)
	}
	contientTout(t, "fondement", fmt.Sprint(f["texte"]), "RGPD", "à valider par le juriste")
	contientTout(t, "refus", texte(r), "D-002 v1", "seuil_argument")
	if strings.Contains(texte(r), "900000") {
		t.Fatal("le motif cite la valeur d'un argument")
	}

	entrees, appels := b.verifier(t)
	if len(appels) != 1 || appels[0].Decision.Effet != "refuser" || appels[0].Resultat.Statut != journal.StatutNonTransmis {
		t.Fatalf("appels %+v", appels)
	}
	if n := transmissions(entrees); len(n) != 0 {
		t.Fatalf("transmissions %v", n)
	}
}

// 3. Attaque.
func TestAttaque(t *testing.T) {
	b := demarrer(t, cheminAttaque)
	// L'agent voit le catalogue épinglé, pas celui de l'imposteur.
	outils, err := b.cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	vu := false
	for _, o := range outils.Tools {
		vu = vu || o.Name == "taux-officiel__get_taux_usure"
	}
	if !vu {
		t.Fatal("outil épinglé de taux-officiel non exposé")
	}

	r := b.appeler(t, "taux-officiel__get_taux_usure", map[string]any{
		"categorie": "decouvert-societes-non-financieres", "date_reference": "2026-10-04",
	})
	sc := structure(t, r)
	f, _ := sc["fondement"].(map[string]any)
	if !r.IsError || sc["statut"] != "refuse" || sc["origine"] != "directive" || sc["directive"] != "D-003" || f == nil {
		t.Fatalf("get_taux_usure : %v", sc)
	}
	contientTout(t, "fondement", fmt.Sprint(f["texte"]), "RGPD", "à valider par le juriste")
	contientTout(t, "refus", texte(r), "D-003 v1", "source_attestee")

	// Les autres serveurs restent joignables.
	r = b.appeler(t, "banque-synthetique__get_taux_interne", map[string]any{"categorie": "decouvert-societes-non-financieres"})
	if r.IsError {
		t.Fatalf("get_taux_interne : %s", texte(r))
	}

	entrees, appels := b.verifier(t)
	// L'imposteur n'a jamais été lancé : son empreinte a été lue, pas
	// exécutée, et il n'a rien écrit (chaque serveur s'annonce sur stderr
	// au démarrage).
	a := attestation(entrees, "taux-officiel")
	if a == nil || a.Valide || a.Observe != empreinteImposteur || !strings.Contains(a.Detail, "processus non lancé") {
		t.Fatalf("attestation de taux-officiel : %+v", a)
	}
	if strings.Contains(b.stderr.String(), registredemo.Imposteur+":") || strings.Contains(b.stderr.String(), "taux-officiel: mode") {
		t.Fatalf("l'imposteur a tourné :\n%s", b.stderr.String())
	}
	if !strings.Contains(b.stderr.String(), "banque-synthetique: mode REJEU") {
		t.Fatalf("traces du gate inattendues :\n%s", b.stderr.String())
	}
	if len(appels) != 2 || appels[0].Decision.Effet != "refuser" || appels[0].Decision.Origine != "" ||
		appels[0].Resultat.Statut != journal.StatutNonTransmis || appels[0].Attestation.Valide {
		t.Fatalf("appel 1 : %+v", appels[0])
	}
	d003 := false
	for _, app := range appels[0].Decision.Applications {
		d003 = d003 || (app.ID == "D-003" && app.Appliquee && app.Effet == "refuser")
	}
	if !d003 {
		t.Fatalf("décision %+v", appels[0].Decision)
	}
	if n := transmissions(entrees); n["taux-officiel/get_taux_usure"] != 0 || n["banque-synthetique/get_taux_interne"] != 1 {
		t.Fatalf("transmissions %v", n)
	}
}

// 4. Base périmée.
func TestBasePerimee(t *testing.T) {
	b := demarrer(t, cheminRegistre)
	r := b.appeler(t, "banque-synthetique__get_taux_interne", map[string]any{"categorie": "decouvert-societes-non-financieres"})
	if r.IsError {
		t.Fatalf("get_taux_interne : %s", texte(r))
	}
	// La base interne sert le trimestre précédent.
	contientTout(t, "get_taux_interne", texte(r), `"trimestre":"2026-Q3"`, "<untrusted_data")

	// Aucune directive ne porte sur la base interne : l'attribution qui
	// suit n'est pas retenue, mais son instantané de session montre la
	// consultation.
	r = b.appeler(t, "banque-synthetique__attribuer_pret", map[string]any{
		"dossier_id": "DOS-FICTIF-002", "siren": "382891752", "montant": 400000,
	})
	if r.IsError {
		t.Fatalf("attribuer_pret : %s", texte(r))
	}

	entrees, appels := b.verifier(t)
	if len(appels) != 2 {
		t.Fatalf("%d appels", len(appels))
	}
	c := appels[0]
	if c.Serveur != "banque-synthetique" || c.Outil != "get_taux_interne" || c.Resultat.Statut != journal.StatutOK ||
		c.Decision.Effet != "autoriser" || c.Resultat.EmpreinteReponse == "" {
		t.Fatalf("consultation : %+v", c)
	}
	if n := transmissions(entrees); n["banque-synthetique/get_taux_interne"] != 1 {
		t.Fatalf("transmissions %v", n)
	}
	etat := appels[1].EtatSession
	if len(etat) != 1 || etat[0].Serveur != "banque-synthetique" || etat[0].Outil != "get_taux_interne" || etat[0].Resultat != "ok" || etat[0].Seq != 1 {
		t.Fatalf("état de session de l'appel 2 : %+v", etat)
	}
	if appels[1].Decision.Effet != "autoriser" || appels[1].Resultat.Statut != journal.StatutOK {
		t.Fatalf("appel 2 : %+v", appels[1])
	}
}
