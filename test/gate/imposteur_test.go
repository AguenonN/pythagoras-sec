package gatee2e

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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

// Le jeu du point de contrôle, plus une directive source_attestee sur la
// banque.
const jeuImposteur = `{
  "jeu": {"id": "credit-pme-gate", "version": 3, "date_effet": "2026-01-01T00:00:00Z", "auteur": "juriste@cabinet.fr"},
  "directives": [
    {"id": "D-001", "version": 1, "statut": "active", "date_effet": "2026-01-01T00:00:00Z",
     "texte_fr": "Aucun prêt ne doit être attribué de manière autonome à partir des informations du serveur de taux officiel.",
     "fondement": {"texte": "RGPD", "article": "22"},
     "verrou": "provenance_session", "cible": {"serveur": "banque-synthetique", "outil": "attribuer_pret"},
     "provenance": {"serveurs": ["taux-officiel"]}, "effet": "refuser"},
    {"id": "D-003", "version": 1, "statut": "active", "date_effet": "2026-01-01T00:00:00Z",
     "texte_fr": "Toute source de taux non attestée est rejetée.",
     "fondement": {"texte": "RGPD", "article": "5"},
     "verrou": "source_attestee", "cible": {"serveur": "taux-officiel", "outil": "*"}, "effet": "refuser"},
    {"id": "D-005", "version": 1, "statut": "active", "date_effet": "2026-01-01T00:00:00Z",
     "texte_fr": "Aucune demande de prêt ne part vers une banque dont l'identité n'est pas prouvée.",
     "fondement": {"texte": "DORA", "article": "28"},
     "verrou": "source_attestee", "cible": {"serveur": "banque-synthetique", "outil": "*"}, "effet": "refuser"}
  ]
}`

// configSession est la configuration de session des bancs ; un test peut
// la remplacer (demarrerAvec).
var configSession = session.Config{DelaiBackend: 10 * time.Second}

type banc struct {
	g        *gate.Gate
	cs       *mcp.ClientSession
	jeu      directive.Jeu
	cheminJ  string
	cheminC  string
	journalE *journal.Ecrivain
	coffre   *journal.Coffre
}

func demarrer(t *testing.T, reg *registre.Registre, doc string) *banc {
	t.Helper()
	if err := registre.Valider(reg); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	b := &banc{jeu: charger(t, doc), cheminJ: filepath.Join(dir, "journal.jsonl"), cheminC: filepath.Join(dir, "coffre.jsonl")}
	var err error
	if b.journalE, err = journal.Ouvrir(b.cheminJ, cleTest); err != nil {
		t.Fatal(err)
	}
	if b.coffre, err = journal.OuvrirCoffre(b.cheminC); err != nil {
		t.Fatal(err)
	}
	ctx, annuler := context.WithTimeout(context.Background(), 30*time.Second)
	defer annuler()
	b.g, err = gate.Demarrer(ctx, gate.Config{
		Registre: reg, Jeu: b.jeu, Journal: b.journalE, Coffre: b.coffre, Appelant: gate.AppelantStdio,
		Session: configSession,
	})
	if err != nil {
		t.Fatal(err)
	}
	tc, ts := mcp.NewInMemoryTransports()
	ss, err := b.g.Serveur().Connect(context.Background(), ts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if b.cs, err = mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "0"}, nil).Connect(context.Background(), tc, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		b.cs.Close()
		ss.Wait()
		b.g.Fermer()
		b.journalE.Fermer()
		b.coffre.Fermer()
	})
	return b
}

// descriptions rend la description exposée de chaque outil.
func (b *banc) descriptions(t *testing.T) map[string]string {
	t.Helper()
	res, err := b.cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, o := range res.Tools {
		out[o.Name] = o.Description
	}
	return out
}

// rejouer relit le journal depuis le disque et exige un rejeu sans écart.
func (b *banc) rejouer(t *testing.T) []journal.Entree {
	t.Helper()
	brut, err := os.ReadFile(b.cheminJ)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Verifier(bytes.NewReader(brut), cleTest); err != nil {
		t.Fatal(err)
	}
	entrees, err := journal.Lire(bytes.NewReader(brut))
	if err != nil {
		t.Fatal(err)
	}
	rs, err := rejeu.RejouerJournal(entrees, b.coffre, rejeu.Catalogue{b.jeu}, rejeu.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rs {
		if !r.Rejouable || len(r.Ecarts) != 0 {
			t.Errorf("rejeu seq %d : %+v", r.Seq, r.Ecarts)
		}
	}
	if strings.Contains(string(brut), "IMPORTANT") || strings.Contains(string(brut), "sans validation") {
		t.Fatal("le journal contient du texte fourni par l'imposteur")
	}
	return entrees
}

func attestationJournalisee(t *testing.T, entrees []journal.Entree, serveur string) *journal.Attestation {
	t.Helper()
	for _, e := range entrees {
		if e.Type == journal.AttestationT && e.Serveur == serveur {
			return e.Attestation
		}
	}
	t.Fatalf("pas d'entrée attestation pour %s", serveur)
	return nil
}

// imposteurBinaire copie le serveur de test en un binaire différent
// (quelques octets de plus) qui, s'il tournait, servirait le profil
// imposteur et créerait son témoin.
func imposteurBinaire(t *testing.T) (chemin, empreinte, temoin string) {
	t.Helper()
	dir := t.TempDir()
	b, err := os.ReadFile(serveurTest)
	if err != nil {
		t.Fatal(err)
	}
	chemin = filepath.Join(dir, "taux-officiel")
	if err := os.WriteFile(chemin, append(b, []byte("imposteur")...), 0o700); err != nil {
		t.Fatal(err)
	}
	empreinte, err = backendtest.Empreinte(chemin)
	if err != nil {
		t.Fatal(err)
	}
	return chemin, empreinte, filepath.Join(dir, "a-tourne")
}

// Le binaire au chemin du registre n'est pas celui épinglé : il n'est pas
// lancé, l'agent voit la description épinglée, chaque appel est refusé par
// source_attestee, qui cite sa directive et son fondement. La banque, elle,
// reste joignable.
func TestImposteur_EmpreinteBinaire(t *testing.T) {
	chemin, observe, temoin := imposteurBinaire(t)
	taux := backendtest.Stdio("taux-officiel", backendtest.Imposteur, chemin, empreinteServeur)
	taux.Commande.Args = append(taux.Commande.Args, "-temoin", temoin)
	reg := &registre.Registre{
		Meta:     registre.MetaRegistre{ID: "lab-imposteur", Version: 1, Auteur: "ops@cabinet.fr"},
		Serveurs: []registre.Serveur{taux, banque(t)},
	}
	b := demarrer(t, reg, jeuImposteur)

	if _, err := os.Stat(temoin); !os.IsNotExist(err) {
		t.Fatal("le binaire imposteur a été lancé")
	}
	d := b.descriptions(t)
	if d["taux-officiel__get_taux"] != backendtest.Outils(backendtest.Taux)[0].Description {
		t.Fatalf("description exposée %q", d["taux-officiel__get_taux"])
	}

	r := appeler(t, b.cs, "taux-officiel__get_taux", map[string]any{"devise": "EUR"})
	m := texte(r)
	for _, attendu := range []string{"D-003 v1", "source_attestee", "RGPD art. 5"} {
		if !r.IsError || !strings.Contains(m, attendu) {
			t.Fatalf("refus %q ne cite pas %q", m, attendu)
		}
	}
	// La banque attestée est toujours routée, et comme rien n'a été
	// consulté sur taux-officiel, la provenance ne déclenche pas.
	r = appeler(t, b.cs, "banque-synthetique__attribuer_pret", map[string]any{"montant": 1000, "siren": "552100554"})
	if r.IsError || !strings.Contains(texte(r), "prêt de 1000 accordé") {
		t.Fatalf("banque : %q", texte(r))
	}

	entrees := b.rejouer(t)
	a := attestationJournalisee(t, entrees, "taux-officiel")
	if a.Valide || a.Observe != observe || a.Attendu != empreinteServeur || !strings.Contains(a.Detail, "non lancé") {
		t.Fatalf("attestation journalisée %+v", a)
	}
	var appels []journal.Entree
	for _, e := range entrees {
		if e.Type == journal.AppelT {
			appels = append(appels, e)
		}
	}
	if len(appels) != 2 || appels[0].Resultat.Statut != journal.StatutNonTransmis || appels[0].Decision.Effet != "refuser" ||
		appels[1].Resultat.Statut != journal.StatutOK || len(appels[1].EtatSession) != 0 {
		t.Fatalf("appels %+v", appels)
	}
}

// Le bon binaire, lancé avec un profil qui annonce une autre description :
// lancé (le témoin le prouve), mais sa description n'est jamais exposée et
// ses appels sont refusés.
func TestImposteur_CatalogueEmpoisonne(t *testing.T) {
	temoin := filepath.Join(t.TempDir(), "a-tourne")
	taux := backendtest.Stdio("taux-officiel", backendtest.Imposteur, serveurTest, empreinteServeur)
	taux.Commande.Args = append(taux.Commande.Args, "-temoin", temoin)
	reg := &registre.Registre{
		Meta:     registre.MetaRegistre{ID: "lab-imposteur", Version: 1, Auteur: "ops@cabinet.fr"},
		Serveurs: []registre.Serveur{taux, banque(t)},
	}
	b := demarrer(t, reg, jeuImposteur)

	if _, err := os.Stat(temoin); err != nil {
		t.Fatal("contrôle : le binaire conforme doit avoir été lancé")
	}
	if a := b.g.Backend("taux-officiel").Attestation(); a.Valide() || a.Schemas || a.Observe != empreinteServeur {
		t.Fatalf("attestation %+v", a)
	}
	for nom, desc := range b.descriptions(t) {
		if strings.Contains(desc, "IMPORTANT") {
			t.Fatalf("%s expose la description de l'imposteur", nom)
		}
	}
	r := appeler(t, b.cs, "taux-officiel__get_taux", map[string]any{"devise": "EUR"})
	if !r.IsError || !strings.Contains(texte(r), "D-003 v1") || strings.Contains(texte(r), "sans validation") {
		t.Fatalf("refus %q", texte(r))
	}
	entrees := b.rejouer(t)
	if a := attestationJournalisee(t, entrees, "taux-officiel"); a.Valide || a.Schemas || !strings.Contains(a.Detail, "modifié(s) : get_taux") {
		t.Fatalf("attestation journalisée %+v", a)
	}
}

// Un serveur HTTP qui n'a pas la clé épinglée : aucune requête ne lui
// parvient, ses appels sont refusés par source_attestee (D-005).
func TestImposteur_ClefTLS(t *testing.T) {
	legitime := banque(t)
	var requetes atomic.Int64
	srv := backendtest.Serveur(backendtest.Banque)
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requetes.Add(1)
		h.ServeHTTP(w, r)
	}))
	ts.Config.ErrorLog = log.New(io.Discard, "", 0)
	// Sa propre clé, pour le même nom : ce qu'obtiendrait quiconque se fait
	// délivrer un certificat pour example.com.
	ts.TLS = &tls.Config{Certificates: []tls.Certificate{certificat(t, "example.com")}}
	ts.StartTLS()
	t.Cleanup(ts.Close)
	somme := sha256.Sum256(ts.Certificate().RawSubjectPublicKeyInfo)
	cleImposteur := "sha256:" + hex.EncodeToString(somme[:])
	if cleImposteur == legitime.Attestation.Attendu {
		t.Fatal("l'imposteur doit avoir sa propre clé")
	}
	// Le registre attend la clé du serveur légitime ; l'url mène à l'imposteur.
	imposteur := legitime
	imposteur.URL = ts.URL + "/mcp"
	reg := &registre.Registre{
		Meta: registre.MetaRegistre{ID: "lab-imposteur", Version: 1, Auteur: "ops@cabinet.fr"},
		Serveurs: []registre.Serveur{
			backendtest.Stdio("taux-officiel", backendtest.Taux, serveurTest, empreinteServeur),
			imposteur,
		},
	}
	b := demarrer(t, reg, jeuImposteur)
	r := appeler(t, b.cs, "banque-synthetique__attribuer_pret", map[string]any{"montant": 1000, "siren": "552100554"})
	m := texte(r)
	if !r.IsError || !strings.Contains(m, "D-005 v1") || !strings.Contains(m, "DORA art. 28") || strings.Contains(m, "accordé") {
		t.Fatalf("refus %q", m)
	}
	if n := requetes.Load(); n != 0 {
		t.Fatalf("%d requête(s) HTTP ont atteint l'imposteur", n)
	}
	entrees := b.rejouer(t)
	if a := attestationJournalisee(t, entrees, "banque-synthetique"); a.Valide || a.Observe != cleImposteur {
		t.Fatalf("attestation journalisée %+v", a)
	}
}

// certificat fabrique un certificat autosigné pour nom et 127.0.0.1, sur
// une clé neuve.
func certificat(t *testing.T, nom string) tls.Certificate {
	t.Helper()
	cle, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	modele := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: nom},
		DNSNames:     []string{nom},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, modele, modele, &cle.PublicKey, cle)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: cle}
}
