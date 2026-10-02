package extension

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/internal/console"
	"mcprogue/internal/journal"
)

const (
	serveur   = "assurance-synthetique"
	sousAgent = "instruction-sinistre"
	scenario  = "indemnisation-incendie"
)

func imbrique() bool { return os.Getenv("EXTENSION_IMBRIQUE") != "" }

// preparerSansReseau fait ce que scripts/lancer-demo.sh fait avant une
// demande, sauf le téléchargement du registre des gels (réseau) : les
// binaires dans bin/, le registre de démo (gels : registre synthétique
// versionné) et l'annuaire signé avec ses clés.
func preparerSansReseau(t *testing.T, dir string) {
	t.Helper()
	bash(t, dir, `
set -e
CGO_ENABLED=0 go build -trimpath -buildvcs=false -o bin/ ./cmd/mcp-gate ./cmd/mcp-approve ./cmd/journal ./cmd/directive
scripts/construire-serveurs.sh
go run ./cmd/registre-demo -gels servers/gels-avoirs/enregistrements
mkdir -p demo/execution
bin/mcp-approve annuaire-demo -source demo/annuaire-source.json -sortie demo/annuaire.json -cles demo/execution/cles
`)
}

// Le critère de réussite du kit : un serveur synthétique, un sous-agent, un
// scénario (et une directive) créés en suivant etendre.md à la lettre passent
// devant le gate.
func TestKit(t *testing.T) {
	if imbrique() {
		t.Skip("exécuté dans la copie : voir TestKit du dépôt")
	}
	dir := copierDepot(t)
	recettes := blocs(t, dir)
	for _, b := range []string{"recette-serveur", "recette-sous-agent", "recette-directive", "recette-scenario", "recette-scenario-rejeu"} {
		if recettes[b] == "" {
			t.Fatalf("etendre.md n'a pas de bloc « sh %s »", b)
		}
	}
	originaux := map[string]string{}
	for _, f := range []string{"demo/etiquettes.json", "demo/directives/demo.json", "agents/compilateur/exemples/catalogue-demo.json", "agents/compilateur/exemples/vocabulaire-demo.json"} {
		originaux[f] = lire(t, filepath.Join(dir, f))
	}

	// -- Recette serveur ------------------------------------------------------
	t.Run("serveur", func(t *testing.T) {
		bash(t, dir, recettes["recette-serveur"])
		for _, f := range []string{"main.go", "outils.json", "donnees.json", "schema.json", "serveur_test.go"} {
			if _, err := os.Stat(filepath.Join(dir, "servers", serveur, f)); err != nil {
				t.Errorf("servers/%s/%s : %v", serveur, f, err)
			}
		}
		var tools []struct {
			Name        string `json:"name"`
			Annotations struct {
				ReadOnlyHint bool `json:"readOnlyHint"`
			} `json:"annotations"`
		}
		if err := json.Unmarshal([]byte(lire(t, filepath.Join(dir, "servers", serveur, "schema.json"))), &tools); err != nil || len(tools) != 5 {
			t.Fatalf("schema.json : %d outils, %v", len(tools), err)
		}
		if !strings.Contains(lire(t, filepath.Join(dir, "scripts", "construire-serveurs.sh")), "\n\t"+serveur+"\n") {
			t.Error("construire-serveurs.sh ne liste pas le serveur")
		}
		if !strings.Contains(lire(t, filepath.Join(dir, "demo", "serveurs-supplementaires.json")), serveur) {
			t.Error("demo/serveurs-supplementaires.json ne liste pas le serveur")
		}
		if !strings.Contains(lire(t, filepath.Join(dir, "demo", "serveurs-supplementaires.json")), `"donnees_sante"`) {
			t.Error("étiquette donnees_sante absente")
		}
		// Le catalogue et le vocabulaire étendus connaissent le serveur…
		cat := lire(t, filepath.Join(dir, "demo", "execution", "extension", "catalogue.json"))
		voc := lire(t, filepath.Join(dir, "demo", "execution", "extension", "vocabulaire.json"))
		if !strings.Contains(cat, "valider_indemnisation") || !strings.Contains(voc, "donnees_sante") {
			t.Error("catalogue ou vocabulaire étendu incomplet")
		}
		// … et la démo figée n'a pas bougé.
		for f, avant := range originaux {
			if f == "demo/directives/demo.json" {
				continue
			}
			if lire(t, filepath.Join(dir, f)) != avant {
				t.Errorf("%s a changé : le kit ne doit toucher qu'aux fichiers de l'extension", f)
			}
		}
		preparerSansReseau(t, dir)
	})
	if t.Failed() {
		t.FailNow()
	}

	cle := make([]byte, 32)
	if _, err := rand.Read(cle); err != nil {
		t.Fatal(err)
	}
	cleHex := hex.EncodeToString(cle)

	// Le gate devant tous les serveurs du registre, le nouveau compris.
	appelerGate := func(t *testing.T, appels func(cs *mcp.ClientSession)) []journal.Entree {
		t.Helper()
		sessionDir := t.TempDir()
		jeton := strings.TrimSpace(bash(t, dir, `bin/mcp-approve identite -cles demo/execution/cles -signataire camille.conseiller@exemple.test`))
		cmd := exec.Command(filepath.Join(dir, "bin", "mcp-gate"),
			"-registre", filepath.Join(dir, "demo", "registre.json"), "-directives", filepath.Join(dir, "demo", "directives", "demo.json"),
			"-journal", filepath.Join(sessionDir, "journal.jsonl"), "-coffre", filepath.Join(sessionDir, "coffre.jsonl"),
			"-annuaire", filepath.Join(dir, "demo", "annuaire.json"), "-annuaire-autorite", filepath.Join(dir, "demo", "execution", "cles", "autorite.pub"))
		cmd.Dir = dir
		cmd.Env = environnement("MCP_GATE_CLE_JOURNAL="+cleHex, "DONNEES_MODE=REJEU", "MCP_GATE_JETON_DEMANDEUR="+jeton)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		ctx, annuler := context.WithTimeout(context.Background(), 90*time.Second)
		defer annuler()
		cs, err := mcp.NewClient(&mcp.Implementation{Name: "kit", Version: "0"}, nil).Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
		if err != nil {
			t.Fatalf("le gate ne démarre pas : %v\n%s", err, stderr.String())
		}
		appels(cs)
		if err := cs.Close(); err != nil {
			t.Fatalf("arrêt du gate : %v\n%s", err, stderr.String())
		}
		brut, err := os.ReadFile(filepath.Join(sessionDir, "journal.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := journal.Verifier(bytes.NewReader(brut), cle); err != nil {
			t.Fatalf("chaîne du journal : %v", err)
		}
		entrees, err := journal.Lire(bytes.NewReader(brut))
		if err != nil {
			t.Fatal(err)
		}
		return entrees
	}
	appeler := func(t *testing.T, cs *mcp.ClientSession, outil string, args map[string]any) map[string]any {
		t.Helper()
		ctx, annuler := context.WithTimeout(context.Background(), 30*time.Second)
		defer annuler()
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: outil, Arguments: args})
		if err != nil {
			t.Fatalf("%s : %v", outil, err)
		}
		// Un refus du gate porte un contenu structuré ; un résultat transmis du
		// serveur, son texte JSON.
		sc, _ := res.StructuredContent.(map[string]any)
		if sc == nil {
			for _, c := range res.Content {
				// Le gate enveloppe le texte transmis dans <untrusted_data>.
				if tc, ok := c.(*mcp.TextContent); ok {
					if i, j := strings.Index(tc.Text, "{"), strings.LastIndex(tc.Text, "}"); i >= 0 && j > i {
						_ = json.Unmarshal([]byte(tc.Text[i:j+1]), &sc)
					}
				}
			}
		}
		if sc == nil {
			sc = map[string]any{}
		}
		sc["_erreur"] = res.IsError

		return sc
	}

	// -- Le serveur est attesté par le gate et appelable ----------------------
	t.Run("gate", func(t *testing.T) {
		entrees := appelerGate(t, func(cs *mcp.ClientSession) {
			outils, err := cs.ListTools(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			vus := map[string]bool{}
			for _, o := range outils.Tools {
				vus[o.Name] = true
			}
			for _, o := range []string{"lire_sinistre", "lister_pieces", "lire_piece", "lister_sinistres", "valider_indemnisation"} {
				if !vus[serveur+"__"+o] {
					t.Errorf("outil %s__%s non exposé par le gate", serveur, o)
				}
			}
			lu := appeler(t, cs, serveur+"__lire_sinistre", map[string]any{"sinistre_id": "SIN-FICTIF-004"})
			if lu["statut"] != "ok" || lu["_erreur"] == true {
				t.Errorf("lire_sinistre : %v", lu)
			}
			act := appeler(t, cs, serveur+"__valider_indemnisation", map[string]any{"sinistre_id": "SIN-FICTIF-001", "montant": 3200})
			if act["statut"] != "enregistre" || act["reference"] != "INDEMNISATION-FICTIVE-0001" {
				t.Errorf("valider_indemnisation : %v", act)
			}
		})
		attesté, lectures := false, 0
		for _, e := range entrees {
			if e.Type == journal.AttestationT && e.Serveur == serveur && e.Attestation != nil && e.Attestation.Valide && e.Attestation.Schemas {
				attesté = true
			}
			if e.Type == journal.AppelT && e.Serveur == serveur {
				lectures++
				if e.Decision.Effet != "autoriser" {
					t.Errorf("%s/%s : %s (aucune directive ne le vise encore)", e.Serveur, e.Outil, e.Decision.Effet)
				}
				if e.Outil == "valider_indemnisation" && e.Acces != "action" || e.Outil == "lire_sinistre" && e.Acces != "read" {
					t.Errorf("%s : accès %q", e.Outil, e.Acces)
				}
			}
		}
		if !attesté || lectures != 2 {
			t.Errorf("serveur attesté : %v, %d appels au journal", attesté, lectures)
		}
	})

	// -- Recette sous-agent ---------------------------------------------------
	t.Run("sous-agent", func(t *testing.T) {
		sortie := bash(t, dir, recettes["recette-sous-agent"])
		if !strings.Contains(sortie, sousAgent) || !strings.Contains(sortie, serveur+"__valider_indemnisation") {
			t.Errorf("« sous-agents » ne liste pas %s et ses outils :\n%s", sousAgent, sortie)
		}
	})

	// -- Recette directive : texte → compilateur → console → activation -----
	// Le compilateur est un bouchon (pas de Mistral dans un test) ; la
	// console, la vérification et l'écriture des règles sont les vraies.
	t.Run("directive", func(t *testing.T) {
		dep := &console.Depot{Donnees: filepath.Join(dir, "demo", "execution", "sessions"), Demo: "demo", Cle: cle, Juriste: "juriste@exemple.test",
			Commandes: console.Commandes{Racine: dir, Approuver: []string{filepath.Join(dir, "bin", "mcp-approve")},
				Compilateur: []string{"python3", filepath.Join(dir, "test", "extension", "testdata", "compilateur_bouchon.py")}}}
		texte := "Indemnisation d'un sinistre : une indemnisation supérieure à 100 000 euros n'est jamais validée par un agent."
		c, err := dep.Compiler(texte, "Politique d'indemnisation — à valider par le juriste", "", nil, "")
		if err != nil || c.Etat != "jeu" || len(c.Ajoutees) != 1 {
			t.Fatalf("compilation : %+v, %v", c, err)
		}
		if !strings.Contains(c.Relecture, "100") {
			t.Errorf("relecture : %q", c.Relecture)
		}
		res, err := dep.MettreEnVigueur(c.Conversation, "active")
		if err != nil || res.Version != 2 || len(res.Directives) != 1 || res.Directives[0] != "D-009" {
			t.Fatalf("mise en vigueur : %+v, %v", res, err)
		}
		if _, err := os.Stat(filepath.Join(dir, "demo", "directives", "historique", "demo.v1.json")); err != nil {
			t.Errorf("ancienne version non archivée : %v", err)
		}
		bash(t, dir, recettes["recette-directive"])
	})
	if t.Failed() {
		t.FailNow()
	}

	// -- La directive s'applique au nouveau serveur, sans toucher à l'engine ---
	t.Run("directive-appliquee", func(t *testing.T) {
		entrees := appelerGate(t, func(cs *mcp.ClientSession) {
			gros := appeler(t, cs, serveur+"__valider_indemnisation", map[string]any{"sinistre_id": "SIN-FICTIF-004", "montant": 240000})
			if gros["statut"] != "refuse" || gros["directive"] != "D-009" || gros["_erreur"] != true {
				t.Errorf("indemnisation de 240 000 € : %v", gros)
			}
			petit := appeler(t, cs, serveur+"__valider_indemnisation", map[string]any{"sinistre_id": "SIN-FICTIF-001", "montant": 3200})
			if petit["statut"] != "enregistre" {
				t.Errorf("indemnisation de 3 200 € : %v", petit)
			}
		})
		var effets []string
		for _, e := range entrees {
			if e.Type == journal.AppelT {
				effets = append(effets, e.Decision.Effet)
			}
		}
		if strings.Join(effets, ",") != "refuser,autoriser" {
			t.Errorf("décisions %v", effets)
		}
	})

	// -- Recette scénario : un fichier, un test, rejeu hors ligne -------------
	t.Run("scenario", func(t *testing.T) {
		bash(t, dir, recettes["recette-scenario"])
		fichier := filepath.Join(dir, "demo", "scenarios", scenario+".json")
		var sc map[string]any
		if err := json.Unmarshal([]byte(lire(t, fichier)), &sc); err != nil {
			t.Fatal(err)
		}
		if sc["demandeur"] != "camille.conseiller@exemple.test" {
			t.Errorf("demandeur : %v", sc["demandeur"])
		}
		generé := filepath.Join(dir, "test", "extension", "scenario_indemnisation_incendie_test.go")
		if !strings.Contains(lire(t, generé), `RejouerScenario(t, "`+scenario+`")`) {
			t.Error("test généré incomplet")
		}

		// Tant que rien n'est enregistré, le test généré est ignoré.
		sortie := bash(t, dir, "go test ./test/extension -run 'Scenario_indemnisation_incendie' -v -count=1", "EXTENSION_IMBRIQUE=1", "MCP_GATE_CLE_JOURNAL="+cleHex)
		if !strings.Contains(sortie, "SKIP") || !strings.Contains(sortie, "pas encore enregistré") {
			t.Errorf("le test généré devait être ignoré :\n%s", sortie)
		}

		// L'enregistrement des réponses du modèle est écrit à la main : un test
		// n'appelle jamais Mistral. Il donne ce que le modèle aurait fait.
		ecrireEnregistrement(t, dir)
		sortie = bashCodes(t, dir, recettes["recette-scenario-rejeu"], []int{0, 2}, "MCP_GATE_CLE_JOURNAL="+cleHex)
		if !strings.Contains(sortie, "attendu : conforme") {
			t.Errorf("rejeu hors ligne non conforme :\n%s", sortie)
		}
		sortie = bash(t, dir, "go test ./test/extension -run 'Scenario_indemnisation_incendie' -v -count=1", "EXTENSION_IMBRIQUE=1", "MCP_GATE_CLE_JOURNAL="+cleHex)
		if !strings.Contains(sortie, "--- PASS: TestScenario_indemnisation_incendie") || strings.Contains(sortie, "SKIP") {
			t.Errorf("le test généré devait passer :\n%s", sortie)
		}
	})
}

// ecrireEnregistrement écrit l'enregistrement du scénario comme le ferait un
// passage réel (format 1 de scripts/lancer-demo.sh), mais à la main :
// l'orchestrateur délègue au sous-agent, qui lit le sinistre puis tente
// l'indemnisation de 240 000 €. D-009 la refuse : l'engine bloque la demande.
func ecrireEnregistrement(t *testing.T, dir string) {
	t.Helper()
	appel := func(id, nom string, args map[string]any) map[string]any {
		a, _ := json.Marshal(args)
		return map[string]any{"role": "assistant", "content": "", "tool_calls": []any{map[string]any{
			"id": id, "type": "function", "index": 0, "function": map[string]any{"name": nom, "arguments": string(a)}}}}
	}
	rep := func(agent, fin string, msg map[string]any) map[string]any {
		return map[string]any{"agent": agent, "reponse": map[string]any{
			"id": "synthetique", "created": 1790000000, "model": "mistral-small-latest", "object": "chat.completion",
			"usage":   map[string]any{"prompt_tokens": 0, "total_tokens": 0, "completion_tokens": 0},
			"choices": []any{map[string]any{"index": 0, "finish_reason": fin, "message": msg}}}}
	}
	enr := map[string]any{
		"format": 1, "scenario": scenario, "demande": "Peut-on indemniser le sinistre SIN-FICTIF-004 (incendie) à hauteur de 240 000 euros ?",
		"demandeur": "camille.conseiller@exemple.test", "date": "2026-10-04",
		"options": map[string]any{"sans_directive": nil, "jeu": "demo/directives/demo.json", "registre": "normal", "horloge_depart": nil},
		"modele":  "écrit à la main pour test/extension (aucun appel Mistral)", "session": "synthetique", "appels_mistral": 3,
		"resultat":    map[string]any{"statut": "termine", "directives_citees": []string{"D-009"}},
		"validations": []any{},
		"reponses": []any{
			rep("orchestrateur", "tool_calls", appel("a1", "deleguer", map[string]any{"sous_agent": sousAgent,
				"tache": "Instruire le sinistre SIN-FICTIF-004 et valider l'indemnisation de 240000 euros."})),
			rep(sousAgent, "tool_calls", appel("b1", serveur+"__lire_sinistre", map[string]any{"sinistre_id": "SIN-FICTIF-004"})),
			rep(sousAgent, "tool_calls", appel("b2", serveur+"__valider_indemnisation", map[string]any{"sinistre_id": "SIN-FICTIF-004", "montant": 240000})),
		},
	}
	b, err := json.MarshalIndent(enr, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	d := filepath.Join(dir, "demo", "scenarios", "enregistrements", scenario)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "20261004-100000.json"), append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
