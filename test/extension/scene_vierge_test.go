package extension

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"mcprogue/internal/journal"
)

// La scène « à partir de zéro », hors ligne : scripts/lancer-demo.sh
// scene-vierge prépare un dossier autonome au jeu vierge (T-001, traçabilité
// seule) ; la demande « fuite par courriel » y est autorisée ; la règle de la
// juriste passe par le vrai service du compilateur (réponses de Mistral
// enregistrées, parcours exemples/service/fuite-courriel.json) et devient
// D-001 ; activée comme le fait la console, la même demande est refusée par
// D-001. Le dépôt n'est pas touché.
func TestSceneVierge(t *testing.T) {
	if imbrique() {
		t.Skip("test imbriqué")
	}
	if testing.Short() {
		t.Skip("-short : préparation complète d'un dossier de démo")
	}
	r := racine(t)
	cle := os.Getenv("MCP_GATE_CLE_JOURNAL")
	if cle == "" {
		t.Skip("MCP_GATE_CLE_JOURNAL absente")
	}
	if l, _ := filepath.Glob(filepath.Join(r, "servers", "gels-avoirs", "enregistrements_locaux", "*")); len(l) == 0 {
		t.Skip("registre local des gels absent : lancer d'abord scripts/lancer-demo.sh")
	}
	sans := dernier(t, r, "op-scene-fuite-sans")
	avec := dernier(t, r, "op-divulgation-piege")
	env := []string{"MCP_GATE_CLE_JOURNAL=" + cle, "DEMO_GELS_HORS_LIGNE=1"}

	avant := statut(t, r)
	d := filepath.Join(t.TempDir(), "arceau")
	bash(t, r, "scripts/lancer-demo.sh scene-vierge "+shellQuote(d), env...)
	jeu := lireJeu(t, filepath.Join(d, "demo", "directives", "demo.json"))
	if ids := idsDe(jeu); strings.Join(ids, ",") != "T-001" {
		t.Fatalf("jeu vierge : %v", ids)
	}

	// 1. Sans règle : le courriel part.
	s1 := demandeRejeu(t, d, sans, env)
	if st, _ := reponseDe(t, s1); st != "termine" {
		t.Fatalf("sans règle : statut %s", st)
	}
	if e := effets(t, s1, cle)["messagerie/envoyer_message"]; !e["autoriser"] || e["refuser"] {
		t.Fatalf("sans règle : envoyer_message %v", e)
	}

	// 2. La juriste écrit la règle : compilateur (service, rejeu).
	sortie := bash(t, filepath.Join(d, "agents"),
		"python3 -m compilateur.service --mode rejeu --parcours compilateur/exemples/service/fuite-courriel.json")
	lignes := strings.Split(strings.TrimSpace(sortie), "\n")
	var fin struct {
		Etat        string   `json:"etat"`
		JeuCandidat string   `json:"jeu_candidat"`
		Erreurs     []string `json:"erreurs"`
	}
	if err := json.Unmarshal([]byte(lignes[len(lignes)-1]), &fin); err != nil {
		t.Fatalf("%v :\n%s", err, sortie)
	}
	if fin.Etat != "jeu" {
		t.Fatalf("compilation : %s %v", fin.Etat, fin.Erreurs)
	}
	cand := lireJeu(t, filepath.Join(d, fin.JeuCandidat))
	ds := cand["directives"].([]any)
	nouvelle := ds[len(ds)-1].(map[string]any)
	if nouvelle["id"] != "D-001" || nouvelle["statut"] != "brouillon" {
		t.Fatalf("directive compilée : %v %v", nouvelle["id"], nouvelle["statut"])
	}

	// 3. Activation, comme la console : statut actif, version n+1, ancienne
	// version archivée, directive verifier.
	bash(t, d, `
set -e
python3 - "`+fin.JeuCandidat+`" <<'PY'
import json, os, shutil, sys
cand = json.load(open(sys.argv[1], encoding="utf-8"))
courant = json.load(open("demo/directives/demo.json", encoding="utf-8"))
shutil.copy("demo/directives/demo.json", "demo/directives/historique/demo.v%d.json" % courant["jeu"]["version"])
cand["directives"][-1]["statut"] = "active"
cand["jeu"]["version"] = courant["jeu"]["version"] + 1
open("demo/directives/demo.json.tmp", "w", encoding="utf-8").write(json.dumps(cand, ensure_ascii=False, indent=1))
os.replace("demo/directives/demo.json.tmp", "demo/directives/demo.json")
PY
bin/directive verifier < demo/directives/demo.json
`)

	// 4. Même demande : refusée par D-001.
	s2 := demandeRejeu(t, d, avec, env)
	st, citees := reponseDe(t, s2)
	if st != "bloque" || strings.Join(citees, ",") != "D-001" {
		t.Fatalf("avec D-001 : statut %s, directives %v", st, citees)
	}
	if e := effets(t, s2, cle)["messagerie/envoyer_message"]; !e["refuser"] {
		t.Fatalf("avec D-001 : envoyer_message %v", e)
	}

	if apres := statut(t, r); apres != avant {
		t.Fatalf("le dépôt a changé :\navant :\n%s\naprès :\n%s", avant, apres)
	}
}

func dernier(t *testing.T, r, nom string) string {
	t.Helper()
	l, _ := filepath.Glob(filepath.Join(r, "demo", "scenarios", "enregistrements", nom, "*.json"))
	if len(l) == 0 {
		t.Skipf("%s pas encore enregistré", nom)
	}
	sort.Strings(l)
	return l[len(l)-1]
}

func demandeRejeu(t *testing.T, d, enr string, env []string) string {
	t.Helper()
	sortie := bashCodes(t, d, "scripts/lancer-demo.sh demande --mode rejeu --enregistrement "+shellQuote(enr), []int{0, 2}, env...)
	m := reSession.FindStringSubmatch(sortie)
	if m == nil {
		t.Fatalf("session introuvable :\n%s", sortie)
	}
	return m[1]
}

func lireJeu(t *testing.T, chemin string) map[string]any {
	t.Helper()
	var j map[string]any
	if err := json.Unmarshal([]byte(lire(t, chemin)), &j); err != nil {
		t.Fatal(err)
	}
	return j
}

func idsDe(j map[string]any) []string {
	var ids []string
	for _, x := range j["directives"].([]any) {
		ids = append(ids, x.(map[string]any)["id"].(string))
	}
	return ids
}

func reponseDe(t *testing.T, session string) (string, []string) {
	t.Helper()
	var rep struct {
		Statut     string   `json:"statut"`
		Directives []string `json:"directives_citees"`
	}
	if err := json.Unmarshal([]byte(lire(t, filepath.Join(session, "reponse.json"))), &rep); err != nil {
		t.Fatal(err)
	}
	return rep.Statut, rep.Directives
}

// effets : par « serveur/outil », les effets de l'engine au journal (chaîne
// vérifiée).
func effets(t *testing.T, session, cleHex string) map[string]map[string]bool {
	t.Helper()
	brut := []byte(lire(t, filepath.Join(session, "journal.jsonl")))
	cle, err := hex.DecodeString(cleHex)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Verifier(bytes.NewReader(brut), cle); err != nil {
		t.Fatalf("chaîne du journal : %v", err)
	}
	entrees, err := journal.Lire(bufio.NewReader(bytes.NewReader(brut)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]map[string]bool{}
	for _, e := range entrees {
		if e.Type != journal.AppelT {
			continue
		}
		k := e.Serveur + "/" + e.Outil
		if out[k] == nil {
			out[k] = map[string]bool{}
		}
		out[k][e.Decision.Effet] = true
	}
	return out
}
