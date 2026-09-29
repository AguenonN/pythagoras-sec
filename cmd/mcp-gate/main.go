// Commande mcp-gate est l'agrégateur : un serveur MCP en stdio qui expose à
// l'agent les outils de tous les serveurs du registre sous le nom
// serveur__outil, et fait passer chaque appel par l'engine (directives du
// juriste, attestation des serveurs, journal chaîné, coffre des arguments)
// avant de le router.
//
//	mcp-gate -registre registre.json -directives jeu.json \
//	         -journal journal.jsonl -coffre coffre.jsonl
//
// La clé HMAC du journal est lue dans MCP_GATE_CLE_JOURNAL (hexadécimal,
// 32 octets au moins), jamais dans un fichier ni sur la ligne de commande.
// Sans clé, sans registre ou sans jeu valide, rien ne démarre.
//
// stdout est le canal MCP : toutes les traces vont sur stderr.
package main

import (
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/internal/backend"
	"mcprogue/internal/directive"
	"mcprogue/internal/gate"
	"mcprogue/internal/journal"
	"mcprogue/internal/registre"
	"mcprogue/internal/session"
)

// VarCle est la variable d'environnement qui porte la clé du journal.
const VarCle = "MCP_GATE_CLE_JOURNAL"

func main() {
	if err := lancer(); err != nil {
		fmt.Fprintln(os.Stderr, "mcp-gate:", err)
		os.Exit(1)
	}
}

func lancer() error {
	cheminRegistre := flag.String("registre", "", "registre des serveurs (JSON)")
	cheminJeu := flag.String("directives", "", "jeu de directives validé par le juriste (JSON)")
	cheminJournal := flag.String("journal", "", "journal chaîné (JSONL, prolongé s'il existe)")
	cheminCoffre := flag.String("coffre", "", "coffre des arguments (JSONL, 0600)")
	delai := flag.Duration("delai-backend", session.DelaiBackendDefaut, "durée maximale d'un appel à un serveur")
	maxConsultations := flag.Int("max-consultations", 0, "ferme la session après ce nombre de consultations (0 : sans borne)")
	ageMax := flag.Duration("age-max", 0, "ferme la session passé cet âge (0 : sans borne)")
	flag.Parse()
	if *cheminRegistre == "" || *cheminJeu == "" || *cheminJournal == "" || *cheminCoffre == "" {
		flag.Usage()
		return errors.New("-registre, -directives, -journal et -coffre sont obligatoires")
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	cle, err := lireCle()
	if err != nil {
		return err
	}
	b, err := os.ReadFile(*cheminRegistre)
	if err != nil {
		return err
	}
	reg, err := registre.Charger(b)
	if err != nil {
		return err
	}
	if b, err = os.ReadFile(*cheminJeu); err != nil {
		return err
	}
	jeu, err := directive.Charger(b)
	if err != nil {
		return err
	}
	j, err := journal.Ouvrir(*cheminJournal, cle)
	if err != nil {
		return err
	}
	defer j.Fermer()
	coffre, err := journal.OuvrirCoffre(*cheminCoffre)
	if err != nil {
		return err
	}
	defer coffre.Fermer()

	ctx, arreter := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer arreter()
	ctxConnexion, annuler := context.WithTimeout(ctx, 30*time.Second)
	defer annuler()
	g, err := gate.Demarrer(ctxConnexion, gate.Config{
		Registre: reg, Jeu: *jeu, Journal: j, Coffre: coffre, Appelant: gate.AppelantStdio,
		Session: session.Config{DelaiBackend: *delai, MaxConsultations: *maxConsultations, AgeMax: *ageMax},
		Backend: backend.Options{Stderr: os.Stderr, Logger: log},
		Logger:  log,
	})
	if err != nil {
		return err
	}
	defer g.Fermer()
	for _, s := range reg.Serveurs {
		a := g.Backend(s.Nom).Attestation()
		log.Info("backend", "serveur", s.Nom, "transport", s.Transport, "attesté", a.Valide(), "détail", a.Detail)
	}
	return g.Servir(ctx, &mcp.StdioTransport{})
}

func lireCle() ([]byte, error) {
	v := os.Getenv(VarCle)
	if v == "" {
		return nil, fmt.Errorf("%s absente : pas de journal sans clé", VarCle)
	}
	cle, err := hex.DecodeString(v)
	if err != nil {
		return nil, fmt.Errorf("%s n'est pas en hexadécimal", VarCle)
	}
	if len(cle) < 32 {
		return nil, fmt.Errorf("%s trop courte : 32 octets au moins", VarCle)
	}
	return cle, nil
}
