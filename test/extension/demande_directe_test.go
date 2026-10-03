package extension

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Une demande jouée en direct (mode réel, données en MIXTE) n'écrit aucun
// fichier versionné : les serveurs de données enregistrent leurs réponses
// dans la copie de travail demo/execution/serveurs (registre de démo), et
// « scenario » en réel écrit les réponses de Mistral sous
// demo/execution/enregistrements. Seule « enregistrer » écrit le secours
// versionné.
//
// Le passage est lancé sans MISTRAL_API_KEY : l'orchestrateur s'arrête en
// erreur au premier appel au modèle, après avoir lancé le gate et ses
// serveurs en MIXTE et écrit son enregistrement ; aucun appel réel.
func TestDemandeDirecteNEcritRienDeVersionne(t *testing.T) {
	if imbrique() {
		t.Skip("test imbriqué")
	}
	r := racine(t)
	for _, f := range []string{"bin/mcp-gate", "bin/mcp-approve", "demo/registre.json", "demo/annuaire.json", "demo/execution/cles/autorite.pub"} {
		if _, err := os.Stat(filepath.Join(r, f)); err != nil {
			t.Skipf("%s absent : lancer d'abord scripts/lancer-demo.sh", f)
		}
	}
	cle := os.Getenv("MCP_GATE_CLE_JOURNAL")
	if cle == "" {
		t.Skip("MCP_GATE_CLE_JOURNAL absente")
	}

	// Les enregistrements des serveurs du registre sont tous hors Git.
	var reg struct {
		Serveurs []struct {
			Nom      string `json:"nom"`
			Commande struct {
				Args []string `json:"args"`
			} `json:"commande"`
		} `json:"serveurs"`
	}
	if err := json.Unmarshal([]byte(lire(t, filepath.Join(r, "demo", "registre.json"))), &reg); err != nil {
		t.Fatal(err)
	}
	for _, s := range reg.Serveurs {
		for i, a := range s.Commande.Args {
			if a != "-enregistrements" || i+1 >= len(s.Commande.Args) {
				continue
			}
			cmd := exec.Command("git", "check-ignore", "-q", filepath.Join(s.Commande.Args[i+1], "x.json"))
			cmd.Dir = r
			if err := cmd.Run(); err != nil {
				t.Errorf("%s enregistre ses réponses dans %s, suivi par Git", s.Nom, s.Commande.Args[i+1])
			}
		}
	}

	avant := statut(t, r)
	sortie := bashCodes(t, r, "scripts/lancer-demo.sh scenario op-initie-temoin", []int{0, 1, 2},
		"MCP_GATE_CLE_JOURNAL="+cle, "DEMO_DONNEES_MODE=MIXTE")
	if !strings.Contains(sortie, "enregistrement demo/execution/enregistrements/op-initie-temoin/") {
		t.Errorf("scenario en réel : enregistrement hors de demo/execution :\n%s", sortie)
	}
	if apres := statut(t, r); apres != avant {
		t.Fatalf("git status a changé après une demande en direct :\navant :\n%s\naprès :\n%s", avant, apres)
	}
}

func statut(t *testing.T, r string) string {
	t.Helper()
	cmd := exec.Command("git", "status", "--porcelain", "--untracked-files=all")
	cmd.Dir = r
	b, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
