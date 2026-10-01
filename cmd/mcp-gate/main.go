// Commande mcp-gate est l'agrégateur : un serveur MCP en stdio qui expose à
// l'agent les outils de tous les serveurs du registre sous le nom
// serveur__outil, et fait passer chaque appel par l'engine (directives du
// juriste, attestation des serveurs, journal chaîné, coffre des arguments)
// avant de le router.
//
//	mcp-gate -registre registre.json -directives jeu.json \
//	         -journal journal.jsonl -coffre coffre.jsonl
//
// Avec -annuaire et -annuaire-autorite, l'annuaire des habilitations est
// chargé (signature vérifiée) ; le jeton d'identité du demandeur est lu
// dans MCP_GATE_JETON_DEMANDEUR, vérifié au lancement, et le demandeur est
// résolu à l'ouverture de la session.
//
// Avec -journaux-precedents <motif>, les journaux des processus précédents
// (même clé) sont relus au démarrage : un jeton d'approbation qu'ils ont
// accepté est refusé comme déjà utilisé. Une chaîne rompue empêche le
// démarrage.
//
// La clé HMAC du journal est lue dans MCP_GATE_CLE_JOURNAL (hexadécimal,
// 32 octets au moins), jamais dans un fichier ni sur la ligne de commande.
// Sans clé, sans registre ou sans jeu valide, rien ne démarre.
//
// SIGHUP recharge le jeu de directives depuis le même fichier, sans
// redémarrer : le document passe par le chargeur strict, et s'il est
// refusé l'ancien jeu reste actif (entrée alerte au journal). Un signal
// plutôt qu'une surveillance du fichier : la bascule est un acte explicite
// de l'opérateur une fois le jeu validé, jamais l'effet d'une sauvegarde
// en cours d'écriture, et elle ne demande aucune dépendance.
//
//	kill -HUP $(pidof mcp-gate)
//
// stdout est le canal MCP : toutes les traces vont sur stderr.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/internal/annuaire"
	"mcprogue/internal/backend"
	"mcprogue/internal/directive"
	"mcprogue/internal/gate"
	"mcprogue/internal/journal"
	"mcprogue/internal/registre"
	"mcprogue/internal/session"
)

// VarCle est la variable d'environnement qui porte la clé du journal.
const VarCle = "MCP_GATE_CLE_JOURNAL"

// VarJetonDemandeur porte le jeton d'identité du demandeur, signé par une
// identité de l'annuaire (mcp-approve identite). Lu dans l'environnement,
// jamais sur la ligne de commande, où ps le montrerait.
const VarJetonDemandeur = "MCP_GATE_JETON_DEMANDEUR"

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
	cheminNotifications := flag.String("notifications", "", "démonstration : écran des destinataires, abonné au journal ; chaque notification reçue y est ajoutée (JSONL). Sans écran, aucune notification n'est remise")
	precedents := flag.String("journaux-precedents", "", "motif (glob) des journaux de processus précédents : les jetons d'approbation qu'ils ont acceptés sont refusés ; une chaîne rompue empêche le démarrage")
	depart := flag.String("horloge-depart", "", "démonstration et tests : horloge simulée qui part de cet instant (RFC 3339) et avance au rythme réel ; signalée au journal")
	cheminAnnuaire := flag.String("annuaire", "", "annuaire des habilitations signé (JSON) ; exige -annuaire-autorite")
	cheminAutorite := flag.String("annuaire-autorite", "", "clé publique Ed25519 (base64) de l'autorité qui signe l'annuaire")
	flag.Parse()
	if *cheminRegistre == "" || *cheminJeu == "" || *cheminJournal == "" || *cheminCoffre == "" {
		flag.Usage()
		return errors.New("-registre, -directives, -journal et -coffre sont obligatoires")
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	// Écouté dès maintenant : un SIGHUP reçu pendant le démarrage ne doit
	// pas tuer le processus (action par défaut) ; il est traité ensuite.
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	defer signal.Stop(hup)

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
	src, err := chargerAnnuaire(*cheminAnnuaire, *cheminAutorite)
	if err != nil {
		return err
	}
	horloge, err := horlogeSimulee(*depart, time.Now)
	if err != nil {
		return err
	}
	var consommes map[string]bool
	if *precedents != "" {
		var relus []string
		if consommes, relus, err = gate.JetonsConsommes(*precedents, cle); err != nil {
			return err
		}
		log.Info("journaux précédents relus", "fichiers", len(relus), "jetons consommés", len(consommes))
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
	if *cheminNotifications != "" {
		arret, err := ecranNotifications(j, *cheminNotifications)
		if err != nil {
			return err
		}
		defer arret()
	}

	ctx, arreter := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer arreter()
	ctxConnexion, annuler := context.WithTimeout(ctx, 30*time.Second)
	defer annuler()
	g, err := gate.Demarrer(ctxConnexion, gate.Config{
		Registre: reg, Jeu: *jeu, Journal: j, Coffre: coffre, Appelant: gate.AppelantStdio,
		Session: session.Config{DelaiBackend: *delai, MaxConsultations: *maxConsultations, AgeMax: *ageMax},
		Backend: backend.Options{Stderr: os.Stderr, Logger: log},
		Logger:  log, Annuaire: src, JetonDemandeur: os.Getenv(VarJetonDemandeur), Horloge: horloge,
		Resolveur: resolveur(reg), JetonsConsommes: consommes,
	})
	if err != nil {
		return err
	}
	if horloge != nil {
		g.Alerter("horloge simulée : départ " + *depart + " ; les horodatages de ce journal ne sont pas ceux de l'horloge réelle")
	}
	defer g.Fermer()
	go rechargerSurSignal(ctx, hup, g, *cheminJeu, log)
	for _, s := range reg.Serveurs {
		a := g.Backend(s.Nom).Attestation()
		log.Info("backend", "serveur", s.Nom, "transport", s.Transport, "attesté", a.Valide(), "détail", a.Detail)
	}
	return g.Servir(ctx, &mcp.StdioTransport{})
}

// rechargerSurSignal recharge le jeu à chaque SIGHUP, jusqu'à la fin de
// ctx. Les rechargements sont sérialisés par le gate.
func rechargerSurSignal(ctx context.Context, hup <-chan os.Signal, g *gate.Gate, chemin string, log *slog.Logger) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-hup:
			r, err := g.RechargerFichier(chemin)
			switch {
			case err != nil:
				log.Error("rechargement du jeu refusé, l'ancien reste actif", "erreur", err,
					"id", r.Jeu.Meta.ID, "version", r.Jeu.Meta.Version)
			case !r.Bascule:
				log.Info("rechargement : jeu identique, aucune bascule", "version", r.Jeu.Meta.Version)
			}
		}
	}
}

// ecranNotifications abonne un écran de démonstration au journal : chaque
// entrée notification reçue est ajoutée au fichier. C'est le canal de
// démonstration : une notification n'est remise que s'il y a un abonné.
func ecranNotifications(j *journal.Ecrivain, chemin string) (func(), error) {
	f, err := os.OpenFile(chemin, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	ch, desabonner := j.Abonner(256)
	fini := make(chan struct{})
	go func() {
		defer close(fini)
		enc := json.NewEncoder(f)
		for en := range ch {
			if en.Type == journal.NotificationT {
				enc.Encode(en)
			}
		}
	}()
	return func() {
		desabonner()
		<-fini
		f.Close()
	}, nil
}

// resolveur rend le résolveur des personnes concernées : l'outil
// groupes_salarie de referentiel-rh, si le registre le déclare.
func resolveur(reg *registre.Registre) gate.Resolveur {
	r := gate.ResolveurReferentielRH
	if s, ok := reg.Serveur(r.Serveur); ok {
		if _, ok := s.Outil(r.Outil); ok {
			return r
		}
	}
	return gate.Resolveur{}
}

// horlogeSimulee rend une horloge qui part de depart et avance au rythme
// de maintenant ; nil si depart est vide (horloge réelle).
func horlogeSimulee(depart string, maintenant func() time.Time) (func() time.Time, error) {
	if depart == "" {
		return nil, nil
	}
	d, err := time.Parse(time.RFC3339Nano, depart)
	if err != nil {
		return nil, fmt.Errorf("-horloge-depart : %w", err)
	}
	t0 := maintenant()
	return func() time.Time { return d.Add(maintenant().Sub(t0)) }, nil
}

// chargerAnnuaire charge l'annuaire et vérifie sa signature avec la clé
// de l'autorité, fournie à part. Sans annuaire, la source est nulle : pas
// de demandeur, pas de validation.
func chargerAnnuaire(chemin, autorite string) (annuaire.Source, error) {
	if chemin == "" && autorite == "" {
		return nil, nil
	}
	if chemin == "" || autorite == "" {
		return nil, errors.New("-annuaire et -annuaire-autorite vont ensemble")
	}
	cle, err := annuaire.LireClePublique(autorite)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(chemin)
	if err != nil {
		return nil, err
	}
	l, err := annuaire.Charger(b, cle)
	if err != nil {
		return nil, err
	}
	return l, nil
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
