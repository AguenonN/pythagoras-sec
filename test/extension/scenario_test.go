package extension

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"mcprogue/internal/journal"
)

// scenarioAttendu est ce que demo/scenarios/<nom>.json attend d'un passage.
type scenarioAttendu struct {
	Attendu struct {
		Statut     string            `json:"statut"`
		Directives *[]string         `json:"directives"`
		SousAgents []string          `json:"sous_agents"`
		Decisions  map[string]string `json:"decisions"`
	} `json:"attendu"`
}

var reSession = regexp.MustCompile(`lancer-demo: session (\S+)`)

// RejouerScenario rejoue hors ligne le scénario <nom> depuis son dernier
// enregistrement (scripts/lancer-demo.sh scenario <nom> --mode rejeu), sans
// réseau ni Mistral, devant le vrai gate, puis vérifie :
//
//   - le comparatif « attendu : conforme » du lanceur (statut, directives) ;
//   - les sous-agents attendus, vus au journal (_meta.sous_agent) ;
//   - la décision attendue de l'engine par outil (« serveur/outil »: effet) ;
//   - la chaîne du journal et le rejeu de ses décisions
//     (scripts/lancer-demo.sh verifier).
//
// Le test est ignoré tant que le scénario n'est pas enregistré, que la démo
// n'est pas préparée (scripts/lancer-demo.sh) ou que MCP_GATE_CLE_JOURNAL
// n'est pas exportée.
func RejouerScenario(t *testing.T, nom string) {
	t.Helper()
	r := racine(t)
	fichier := filepath.Join(r, "demo", "scenarios", nom+".json")
	var sc scenarioAttendu
	if err := json.Unmarshal([]byte(lire(t, fichier)), &sc); err != nil {
		t.Fatalf("%s : %v", fichier, err)
	}
	enr, _ := filepath.Glob(filepath.Join(r, "demo", "scenarios", "enregistrements", nom, "*.json"))
	if len(enr) == 0 {
		t.Skipf("scénario %s pas encore enregistré : scripts/lancer-demo.sh scenario %s", nom, nom)
	}
	for _, f := range []string{"bin/mcp-gate", "bin/mcp-approve", "bin/journal", "demo/registre.json", "demo/annuaire.json", "demo/execution/cles/autorite.pub"} {
		if _, err := os.Stat(filepath.Join(r, f)); err != nil {
			t.Skipf("%s absent : lancer d'abord scripts/lancer-demo.sh", f)
		}
	}
	// La clé des journaux de la démo : le gate relit les journaux des passages
	// précédents (demo/execution/sessions) et refuse de démarrer si la chaîne
	// de l'un d'eux n'a pas été signée par la même clé.
	cleHex := os.Getenv("MCP_GATE_CLE_JOURNAL")
	if cleHex == "" {
		t.Skip("MCP_GATE_CLE_JOURNAL absente : l'exporter (la même clé que pour scripts/lancer-demo.sh)")
	}
	cle, err := hex.DecodeString(cleHex)
	if err != nil {
		t.Fatal("MCP_GATE_CLE_JOURNAL n'est pas de l'hexadécimal")
	}
	env := "MCP_GATE_CLE_JOURNAL=" + cleHex

	sortie := bashCodes(t, r, "scripts/lancer-demo.sh scenario "+shellQuote(nom)+" --mode rejeu", []int{0, 2}, env)
	if !strings.Contains(sortie, "attendu : conforme") {
		t.Fatalf("le rejeu n'est pas conforme à l'attendu :\n%s", sortie)
	}
	m := reSession.FindStringSubmatch(sortie)
	if m == nil {
		t.Fatalf("dossier de session introuvable dans :\n%s", sortie)
	}
	session := m[1]
	brut := lire(t, filepath.Join(session, "journal.jsonl"))
	if _, err := journal.Verifier(bytes.NewReader([]byte(brut)), cle); err != nil {
		t.Fatalf("chaîne du journal : %v", err)
	}
	entrees, err := journal.Lire(bytes.NewReader([]byte(brut)))
	if err != nil {
		t.Fatal(err)
	}
	agents := map[string]bool{}
	effets := map[string]map[string]bool{}
	for _, e := range entrees {
		if e.Type != journal.AppelT {
			continue
		}
		if e.SousAgentDeclare != "" {
			agents[e.SousAgentDeclare] = true
		}
		cle := e.Serveur + "/" + e.Outil
		if effets[cle] == nil {
			effets[cle] = map[string]bool{}
		}
		effets[cle][e.Decision.Effet] = true
	}
	for _, a := range sc.Attendu.SousAgents {
		if !agents[a] {
			t.Errorf("sous-agent %s attendu, absent du journal (vus : %v)", a, cles(agents))
		}
	}
	for outil, attendu := range sc.Attendu.Decisions {
		if !effets[outil][attendu] {
			t.Errorf("%s : décision %q attendue, le journal a %v", outil, attendu, cles(effets[outil]))
		}
	}
	bash(t, r, "scripts/lancer-demo.sh verifier "+shellQuote(session), env)
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func cles[V any](m map[string]V) []string {
	var l []string
	for k := range m {
		l = append(l, k)
	}
	sort.Strings(l)
	return l
}
