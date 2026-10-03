// Command console sert les écrans de démonstration sur 127.0.0.1. Tout
// est embarqué dans le binaire (HTML, CSS, JavaScript) : aucune ressource
// n'est chargée depuis le réseau (l'option -dev les lit sur disque). La
// console lit les journaux, l'état des sessions et les règles ; elle écrit
// seulement les règles (version suivante, ancienne version archivée), leur
// origine, les jetons et refus de validation, et ses décisions sur les
// propositions de l'Analyste. Elle ne décide rien : tout ce qu'elle affiche
// vient du journal, de etat.json ou d'une commande de l'engine.
//
//	MCP_GATE_CLE_JOURNAL=<hex> console -racine . -demo demo
//
// La clé du journal n'est lue que dans l'environnement, comme le fait le
// gate. Sans clé, les écrans s'affichent mais la chaîne est déclarée non
// vérifiable, en rouge.
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mcprogue/internal/console"
)

// VarCle est la variable qui porte la clé du journal, la même que le gate.
const VarCle = "MCP_GATE_CLE_JOURNAL"

func main() {
	if err := lancer(); err != nil {
		fmt.Fprintln(os.Stderr, "console:", err)
		os.Exit(1)
	}
}

func lancer() error {
	addr := flag.String("addr", "127.0.0.1:8088", "adresse d'écoute (boucle locale seulement)")
	racine := flag.String("racine", ".", "racine du dépôt : les commandes y tournent et demo/ s'y trouve")
	demo := flag.String("demo", "demo", "dossier de la démonstration, sous la racine")
	donnees := flag.String("donnees", "", "dossier des sessions (défaut : <demo>/execution/sessions)")
	produit := flag.String("produit", "Console de gouvernance", "nom du produit affiché dans l'en-tête")
	juriste := flag.String("juriste", "juriste@exemple.test", "identité du juriste, auteur des nouvelles versions des règles")
	dev := flag.String("dev", "", "répertoire des fichiers HTML, CSS et JS, servis depuis le disque (rechargement sans recompiler) ; ex. internal/console/web")
	approuver := flag.String("approuver", "", "commande de signature (mcp-approve) ; défaut : bin/mcp-approve s'il existe, sinon « go run ./cmd/mcp-approve »")
	compilateur := flag.String("compilateur", "", "« reel » ou « rejeu » : mode du service du compilateur (défaut : reel si MISTRAL_API_KEY est dans l'environnement, sinon rejeu) ; toute autre valeur : commande du service (défaut : « python -m compilateur.service » avec agents/.venv s'il existe)")
	modeCompil := flag.String("mode-compilateur", "", "« reel » ou « rejeu » : force le mode quand -compilateur donne une commande")
	lanceur := flag.String("lanceur", "scripts/lancer-demo.sh", "commande qui exécute une demande (sous-commande « demande »)")
	flag.Parse()

	host, _, err := net.SplitHostPort(*addr)
	if err != nil {
		return fmt.Errorf("adresse %q : %w", *addr, err)
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("adresse %q refusée : la console n'écoute que sur la boucle locale", *addr)
	}
	var cle []byte
	if v := os.Getenv(VarCle); v != "" {
		if cle, err = hex.DecodeString(v); err != nil {
			return fmt.Errorf("%s n'est pas de l'hexadécimal", VarCle)
		}
	} else {
		fmt.Fprintf(os.Stderr, "console: %s absente : les chaînes ne pourront pas être vérifiées.\n", VarCle)
	}
	abs, err := filepath.Abs(*racine)
	if err != nil {
		return err
	}
	sessions := *donnees
	if sessions == "" {
		sessions = filepath.Join(abs, *demo, "execution", "sessions")
	}
	cmdCompil, mode, err := choisirCompilateur(*compilateur, *modeCompil, os.Getenv)
	if err != nil {
		return err
	}
	cmds := console.Commandes{Racine: abs, Approuver: strings.Fields(*approuver), Compilateur: cmdCompil, ModeCompilateur: mode, Lanceur: strings.Fields(*lanceur)}
	if len(cmds.Approuver) == 0 {
		if _, err := os.Stat(filepath.Join(abs, "bin", "mcp-approve")); err == nil {
			cmds.Approuver = []string{filepath.Join(abs, "bin", "mcp-approve")}
		} else {
			cmds.Approuver = []string{"go", "run", "./cmd/mcp-approve"}
		}
	}
	if len(cmds.Compilateur) == 0 {
		py := "python3"
		if _, err := os.Stat(filepath.Join(abs, "agents", ".venv", "bin", "python")); err == nil {
			py = filepath.Join(abs, "agents", ".venv", "bin", "python")
		}
		cmds.Compilateur = []string{py, "-m", "compilateur.service"}
		cmds.Env = append(cmds.Env, "PYTHONPATH="+filepath.Join(abs, "agents"))
	}
	if len(cmds.Lanceur) > 0 && !filepath.IsAbs(cmds.Lanceur[0]) && strings.Contains(cmds.Lanceur[0], "/") {
		cmds.Lanceur[0] = filepath.Join(abs, cmds.Lanceur[0])
	}
	dep := &console.Depot{Donnees: sessions, Demo: *demo, Cle: cle, Juriste: *juriste, Commandes: cmds}
	srv := &http.Server{
		Addr:              *addr,
		Handler:           (&console.Serveur{Depot: dep, Produit: *produit, Dev: *dev}).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      6 * time.Minute, // la compilation attend le service
	}
	interf := "interface embarquée"
	if *dev != "" {
		interf = "interface lue sur disque : " + *dev
	}
	fmt.Fprintf(os.Stderr, "console: http://%s (%s ; sessions : %s ; compilation %s)\n", *addr, interf, sessions, libelleMode[cmds.ModeCompilateur])
	return srv.ListenAndServe()
}

var libelleMode = map[string]string{"reel": "réelle", "rejeu": "rejouée"}

// choisirCompilateur lit l'option -compilateur : « reel » ou « rejeu »
// force le mode du service ; toute autre valeur est la commande du service
// (rôle d'origine de l'option). -mode-compilateur force le mode avec une
// commande donnée. Sans mode forcé : réel si MISTRAL_API_KEY est dans
// l'environnement de la console (la clé n'est jamais lue ni affichée, sa
// présence seule compte), sinon rejoué.
func choisirCompilateur(valeur, modeForce string, getenv func(string) string) (cmd []string, mode string, err error) {
	switch v := strings.TrimSpace(valeur); v {
	case "reel", "rejeu":
		mode = v
	default:
		cmd = strings.Fields(v)
	}
	switch modeForce {
	case "":
	case "reel", "rejeu":
		mode = modeForce
	default:
		return nil, "", fmt.Errorf("-mode-compilateur %q : « reel » ou « rejeu » attendu", modeForce)
	}
	if mode == "" {
		mode = "rejeu"
		if getenv("MISTRAL_API_KEY") != "" {
			mode = "reel"
		}
	}
	return cmd, mode, nil
}
