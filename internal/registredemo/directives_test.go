package registredemo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/internal/backend"
	"mcprogue/internal/directive"
	"mcprogue/internal/gate"
	"mcprogue/internal/journal"
	"mcprogue/internal/registre"
)

// cheminDirectives est le jeu de démonstration, relatif à la racine.
const cheminDirectives = "demo/directives/credit-demo.json"

// registreSchemas est le registre de démo tel que Generer le produirait,
// sans binaires : les outils viennent des schema.json versionnés, les
// commandes et empreintes sont factices. Le contrôle des cibles ne lit que
// les noms de serveurs et d'outils.
func registreSchemas(t *testing.T, racine string) *registre.Registre {
	t.Helper()
	r := &registre.Registre{Meta: Meta}
	for _, nom := range Serveurs {
		b, err := os.ReadFile(filepath.Join(racine, "servers", nom, "schema.json"))
		if err != nil {
			t.Fatal(err)
		}
		outils, err := Convertir(b)
		if err != nil {
			t.Fatal(err)
		}
		r.Serveurs = append(r.Serveurs, registre.Serveur{
			Nom: nom, Transport: registre.Stdio, Commande: &registre.Commande{Chemin: "/nulle-part/" + nom},
			Attestation: registre.Attestation{Mode: registre.ModeBinaire, Attendu: "sha256:" + strings.Repeat("0", 64)},
			Outils:      outils,
		})
	}
	if err := registre.Valider(r); err != nil {
		t.Fatal(err)
	}
	return r
}

// horsService est un backend jamais connecté : le démarrage du gate ne
// lance aucun processus.
type horsService struct{ nom string }

func (h horsService) Nom() string { return h.nom }
func (h horsService) Attestation() directive.Attestation {
	return directive.Attestation{Mode: registre.ModeBinaire, Detail: "essai : non connecté"}
}
func (h horsService) Outils(context.Context) ([]*mcp.Tool, error) { return nil, backend.ErrNonConnecte }
func (h horsService) Appeler(context.Context, string, map[string]any, mcp.Meta) (*mcp.CallToolResult, error) {
	return nil, backend.ErrNonConnecte
}
func (h horsService) Fermer() error { return nil }

func demarrer(t *testing.T, reg *registre.Registre, jeu directive.Jeu) error {
	t.Helper()
	dir := t.TempDir()
	j, err := journal.Ouvrir(filepath.Join(dir, "journal.jsonl"), []byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Fermer()
	c, err := journal.OuvrirCoffre(filepath.Join(dir, "coffre.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Fermer()
	g, err := gate.Demarrer(context.Background(), gate.Config{
		Registre: reg, Jeu: jeu, Journal: j, Coffre: c, Appelant: gate.AppelantStdio,
		Connecter: func(_ context.Context, s registre.Serveur, _ backend.Options) (backend.Backend, error) {
			return horsService{s.Nom}, backend.ErrNonConnecte
		},
	})
	if err == nil {
		g.Fermer()
	}
	return err
}

// Le jeu de démonstration passe le chargeur strict et le démarrage du gate
// devant le registre de démo, contrôle des cibles et des provenances
// compris.
func TestDirectivesDemo(t *testing.T) {
	racine, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(racine, cheminDirectives))
	if err != nil {
		t.Fatal(err)
	}
	jeu, err := directive.Charger(b)
	if err != nil {
		t.Fatal(err)
	}
	attendu := map[string]directive.Verrou{
		"D-001": directive.ProvenanceSession, "D-002": directive.SeuilArgument,
		"D-003": directive.SourceAttestee, "D-004": directive.ChampsAutorises,
	}
	if len(jeu.Directives) != len(attendu) {
		t.Fatalf("%d directives", len(jeu.Directives))
	}
	for _, d := range jeu.Directives {
		if d.Verrou != attendu[d.ID] || d.Statut != directive.Active {
			t.Fatalf("%s : %s %s", d.ID, d.Verrou, d.Statut)
		}
		if !strings.Contains(d.Fondement.Texte, "à valider par le juriste") {
			t.Fatalf("%s : fondement non marqué « à valider par le juriste »", d.ID)
		}
	}
	reg := registreSchemas(t, racine)
	if err := demarrer(t, reg, *jeu); err != nil {
		t.Fatal(err)
	}

	// Le contrôle est bien actif : sans taux-officiel au registre, le même
	// jeu est refusé (D-001 l'attend en provenance, D-003 le vise).
	sans := &registre.Registre{Meta: reg.Meta}
	for _, s := range reg.Serveurs {
		if s.Nom != "taux-officiel" {
			sans.Serveurs = append(sans.Serveurs, s)
		}
	}
	sans.Meta.Empreinte = ""
	if err := registre.Valider(sans); err != nil {
		t.Fatal(err)
	}
	err = demarrer(t, sans, *jeu)
	if !errors.Is(err, gate.ErrCibleHorsRegistre) || !strings.Contains(err.Error(), "D-001") || !strings.Contains(err.Error(), "D-003") {
		t.Fatalf("jeu accepté sans taux-officiel : %v", err)
	}
}

// Tous les jeux de demo/directives passent le chargeur strict et le
// démarrage du gate devant le registre de démo étiqueté (schémas et
// étiquettes versionnés), et devant demo/registre.json s'il a été
// régénéré (go run ./cmd/registre-demo). Le registre de démo porte
// production_rapport sur banque-synthetique/remettre_rapport et
// donnees_client sur evaluer_dossier, que rapport.json vise.
func TestTousLesJeuxDemo(t *testing.T) {
	racine, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	reg := registreSchemas(t, racine)
	e, err := LireEtiquettes(filepath.Join(racine, FichierEtiquettes))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Appliquer(reg); err != nil {
		t.Fatal(err)
	}
	reg.Meta.Empreinte = ""
	if err := registre.Valider(reg); err != nil {
		t.Fatal(err)
	}
	registres := map[string]*registre.Registre{"schémas versionnés": reg}
	if b, err := os.ReadFile(filepath.Join(racine, "demo", "registre.json")); err == nil {
		r, err := registre.Charger(b)
		if err != nil {
			t.Fatalf("demo/registre.json : %v", err)
		}
		registres["demo/registre.json"] = r
	} else {
		t.Log("demo/registre.json absent : seul le registre des schémas est essayé")
	}
	for nom, r := range registres {
		b, _ := r.Serveur("banque-synthetique")
		for outil, attendu := range map[string]string{"remettre_rapport": "production_rapport", "evaluer_dossier": "donnees_client"} {
			if got := strings.Join(b.EtiquettesOutil(outil), ","); got != attendu {
				t.Errorf("%s : banque-synthetique/%s porte %q, attendu %q", nom, outil, got, attendu)
			}
		}
	}

	jeux, err := filepath.Glob(filepath.Join(racine, "demo", "directives", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(jeux) == 0 {
		t.Fatal("aucun jeu dans demo/directives")
	}
	for _, chemin := range jeux {
		b, err := os.ReadFile(chemin)
		if err != nil {
			t.Fatal(err)
		}
		jeu, err := directive.Charger(b)
		if err != nil {
			t.Errorf("%s : %v", filepath.Base(chemin), err)
			continue
		}
		for nom, r := range registres {
			if err := demarrer(t, r, *jeu); err != nil {
				t.Errorf("%s devant %s : %v", filepath.Base(chemin), nom, err)
			}
		}
	}
}
