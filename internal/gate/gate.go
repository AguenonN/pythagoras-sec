// Package gate est l'agrégateur : un serveur MCP qui expose à l'agent les
// outils de tous les backends du registre, sous le nom serveur__outil, et
// fait passer chaque tools/call par l'engine avant de le router.
//
// Pipeline d'un tools/call, dans l'ordre du contrat de l'engine
// (docs/contrat-engine.md) :
//
//  1. décomposer le nom exposé en (serveur, outil) ;
//  2. entrer dans la session (verrou tenu jusqu'à Terminer, en defer) ;
//  3. lire l'horloge une fois, déposer les arguments d'origine au coffre ;
//  4. Decide, avec cette date, l'état de session et l'attestation ;
//  5. refuser : entrée appel non_transmis, erreur à l'agent avec le motif ;
//  6. attendre_validation : refus explicite et champ structuré en_attente ;
//  7. sinon, écrire l'entrée transmission (refus si elle échoue), puis
//     transmettre les arguments décidés et sceller le résultat ;
//  8. écrire l'entrée appel à la date passée à Decide, puis Terminer.
//
// L'engine n'expose jamais ce qu'un serveur annonce : l'agent voit le
// catalogue épinglé au registre, que le serveur soit attesté ou non. Un
// serveur non attesté voit chacun de ses appels refusés : par la directive
// source_attestee qui le cible s'il y en a une (elle cite son fondement),
// et sinon par le contrôle du registre, toujours actif.
package gate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/internal/annuaire"
	"mcprogue/internal/approval"
	"mcprogue/internal/backend"
	"mcprogue/internal/controle"
	"mcprogue/internal/directive"
	"mcprogue/internal/envelope"
	"mcprogue/internal/journal"
	"mcprogue/internal/nommage"
	"mcprogue/internal/registre"
	"mcprogue/internal/session"
)

// AppelantStdio est l'appelant d'une session servie en stdio : le processus
// parent, seul à tenir les tubes.
const AppelantStdio = "local-process"

// Config assemble l'engine.
type Config struct {
	Registre *registre.Registre
	Jeu      directive.Jeu
	Journal  *journal.Ecrivain
	Coffre   *journal.Coffre
	Session  session.Config
	// Appelant est l'identité prouvée par le transport de l'agent.
	Appelant string
	// Horloge est lue une fois par appel ; défaut time.Now.
	Horloge func() time.Time
	// Connecter ouvre un backend ; défaut backend.Connecter.
	Connecter func(context.Context, registre.Serveur, backend.Options) (backend.Backend, error)
	// Backend est passé à chaque connexion (Stderr, Logger, délai).
	Backend backend.Options
	Logger  *slog.Logger
	// Annuaire est la source d'identité (demandeur, valideurs). Nulle :
	// aucun demandeur n'est résolu et aucune validation n'est possible.
	Annuaire annuaire.Source
	// JetonDemandeur est le jeton d'identité du demandeur remis au
	// lancement, signé par une identité de l'annuaire. Vide : pas de
	// demandeur, et les directives qui le lisent sont indéterminées.
	JetonDemandeur string
	// Resolveur rend les groupes des personnes concernées (outils dont le
	// registre déclare personnes_concernees). Zéro : aucune n'est résolue.
	Resolveur Resolveur
}

// Gate est l'agrégateur démarré.
type Gate struct {
	cfg Config
	// jeu est le jeu actif. Chaque appel en prend un instantané, une fois,
	// juste avant Decide : un rechargement bascule entre deux appels,
	// jamais au milieu d'un.
	jeu atomic.Pointer[directive.Jeu]
	// recharge sérialise les rechargements et tient les versions déjà
	// chargées (voir Recharger).
	recharge sync.Mutex
	charges  map[versionJeu]string
	journal  *journal.Ecrivain
	coffre   *journal.Coffre
	sessions *session.Gestionnaire
	horloge  func() time.Time
	log      *slog.Logger
	serveur  *mcp.Server
	backends map[string]backend.Backend
	suivies  sync.Map // *mcp.ServerSession → struct{}
	// enPanne est posé quand le journal refuse une écriture : sans preuve,
	// l'engine refuse tout appel.
	enPanne atomic.Bool
	// complements tient, par session, ce que l'engine sait d'une
	// consultation en plus de ce que la session garde : étiquettes et
	// version du registre au moment de l'appel, source déclarée par le
	// serveur (voir completer).
	complements sync.Map // id de session → *complementsSession
	idsSession  sync.Map // *mcp.ServerSession → id de session, pour oublier les compléments
	// demandeurs tient le demandeur résolu à l'ouverture de chaque
	// session, tel que consigné dans session_ouverte.
	demandeurs sync.Map // id de session → *directive.Demandeur
}

// Demarrer journalise le jeu et le registre, connecte chaque backend et
// journalise son attestation, puis construit le serveur MCP. Un backend
// en échec n'arrête pas le démarrage : ses outils sont exposés depuis le
// registre et chacun de ses appels est refusé.
func Demarrer(ctx context.Context, cfg Config) (*Gate, error) {
	if cfg.Registre == nil {
		return nil, errors.New("gate: registre manquant")
	}
	if cfg.Journal == nil || cfg.Coffre == nil {
		return nil, errors.New("gate: journal et coffre obligatoires")
	}
	if cfg.Jeu.Meta.Empreinte == "" {
		return nil, errors.New("gate: jeu non validé (empreinte absente)")
	}
	if err := verifierCibles(cfg.Jeu, cfg.Registre); err != nil {
		return nil, err
	}
	if cfg.Horloge == nil {
		cfg.Horloge = time.Now
	}
	if cfg.Connecter == nil {
		cfg.Connecter = backend.Connecter
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.DiscardHandler)
	}
	if cfg.JetonDemandeur != "" {
		if cfg.Annuaire == nil {
			return nil, errors.New("gate: jeton du demandeur sans annuaire pour le vérifier")
		}
		// Vérifié dès le lancement : un jeton refusé est une erreur de
		// l'opérateur, pas une session sans demandeur.
		if _, err := annuaire.VerifierIdentite(cfg.Annuaire, cfg.JetonDemandeur, cfg.Horloge().UTC()); err != nil {
			return nil, fmt.Errorf("gate: jeton du demandeur refusé : %w", err)
		}
	}
	g := &Gate{
		cfg: cfg, journal: cfg.Journal, coffre: cfg.Coffre, charges: map[versionJeu]string{},
		horloge: cfg.Horloge, log: cfg.Logger, backends: map[string]backend.Backend{},
	}
	jeu := cfg.Jeu
	g.jeu.Store(&jeu)
	g.charges[versionJeu{jeu.Meta.ID, jeu.Meta.Version}] = jeu.Meta.Empreinte
	// Contrat §3 : le journal est attaché au coffre dès l'ouverture.
	g.coffre.Journaliser(g.journal)
	g.coffre.AvantPurge(g.avantPurge)
	sc := cfg.Session
	if sc.Horloge == nil {
		// Une seule horloge : l'ouverture de session (qui date la
		// résolution du demandeur) et les appels.
		sc.Horloge = cfg.Horloge
	}
	sc.AOuverture = g.ouvrir
	fermeture := sc.AFermeture
	sc.AFermeture = func(ss *mcp.ServerSession, motif string) {
		if fermeture != nil {
			fermeture(ss, motif)
		}
		// Hors du gestionnaire de l'appel : la fermeture forcée est
		// demandée pendant un tools/call, et ServerSession.Close attend la
		// fin de tous les gestionnaires, celui-ci compris.
		go ss.Close()
	}
	g.sessions = session.Nouveau(sc, g.journal)

	if err := g.journaliserDemarrage(); err != nil {
		return nil, err
	}
	for _, s := range cfg.Registre.Serveurs {
		opts := cfg.Backend
		auChangement := opts.AuChangement
		opts.AuChangement = func(nom string, a directive.Attestation) {
			if auChangement != nil {
				auChangement(nom, a)
			}
			g.journaliserAttestation(nom, a, "attestation modifiée après la connexion")
		}
		b, err := cfg.Connecter(ctx, s, opts)
		if b == nil {
			g.Fermer()
			return nil, fmt.Errorf("gate: %s: aucun backend rendu: %v", s.Nom, err)
		}
		if err != nil {
			g.log.Warn("backend non attesté", "serveur", s.Nom, "erreur", err)
		}
		g.backends[s.Nom] = b
		if err := g.journaliserAttestation(s.Nom, b.Attestation(), "attestation figée à la connexion"); err != nil {
			g.Fermer()
			return nil, err
		}
	}
	if err := g.construireServeur(); err != nil {
		g.Fermer()
		return nil, err
	}
	return g, nil
}

func (g *Gate) journaliserDemarrage() error {
	r := g.cfg.Registre
	if err := g.journaliserJeu(journal.Entree{}, *g.jeu.Load(), "chargé au démarrage"); err != nil {
		return err
	}
	if _, err := g.journal.Ecrire(journal.Entree{Type: journal.RegistreCharge,
		Message: fmt.Sprintf("registre %s v%d %s, auteur %s : %s", r.Meta.ID, r.Meta.Version, r.Meta.Empreinte,
			r.Meta.Auteur, strings.Join(r.Noms(), ", "))}); err != nil {
		return fmt.Errorf("gate: journal: %w", err)
	}
	if a := g.cfg.Annuaire; a != nil {
		ref := a.Reference()
		if _, err := g.journal.Ecrire(journal.Entree{Type: journal.AnnuaireCharge, SourceIdentite: a.Nom(),
			Annuaire: &journal.RefAnnuaire{ID: ref.ID, Version: ref.Version, Empreinte: ref.Empreinte},
			Message:  fmt.Sprintf("annuaire %s v%d %s (source %s)", ref.ID, ref.Version, ref.Empreinte, a.Nom())}); err != nil {
			return fmt.Errorf("gate: journal: %w", err)
		}
	}
	return nil
}

// journaliserJeu écrit l'entrée jeu_charge (identifiant, version,
// empreinte), complétée des champs posés dans base, puis les alertes de
// relecture échues du jeu.
func (g *Gate) journaliserJeu(base journal.Entree, jeu directive.Jeu, contexte string) error {
	base.Type, base.Jeu = journal.JeuCharge, journal.DepuisJeu(jeu)
	base.Message = fmt.Sprintf("%s : %d directive(s), auteur %s", contexte, len(jeu.Directives), jeu.Meta.Auteur)
	if _, err := g.journal.Ecrire(base); err != nil {
		return fmt.Errorf("gate: journal: %w", err)
	}
	for _, a := range jeu.Alertes(g.horloge()) {
		if _, err := g.journal.Ecrire(journal.Entree{Type: journal.AlerteT, Message: a.DirectiveID + ": " + a.Message}); err != nil {
			return fmt.Errorf("gate: journal: %w", err)
		}
	}
	return nil
}

func (g *Gate) journaliserAttestation(nom string, a directive.Attestation, message string) error {
	en := journal.Entree{TS: g.horloge().UTC(), Type: journal.AttestationT, Serveur: nom,
		Attestation: journal.DepuisAttestation(a), Message: message}
	if !a.Valide() {
		// Directives de l'évènement attestation_echouee.
		if _, err := g.evenement(&en, directive.EntreeEvenement{Evenement: directive.EvAttestationEchouee,
			Horodatage: en.TS, Serveur: nom, Attestation: a}); err != nil {
			return fmt.Errorf("gate: journal: %w", err)
		}
	}
	if _, err := g.journal.Ecrire(en); err != nil {
		g.enPanne.Store(true)
		g.log.Error("journal indisponible", "erreur", err)
		return fmt.Errorf("gate: journal: %w", err)
	}
	return nil
}

// avantPurge consigne dans l'entrée purge la décision des directives de
// l'évènement purge. Une purge n'est jamais refusée par une directive :
// elle est notifiée et marquée.
func (g *Gate) avantPurge(en *journal.Entree) {
	g.evenement(en, directive.EntreeEvenement{Evenement: directive.EvPurge, Horodatage: en.TS})
}

// construireServeur expose chaque outil épinglé sous serveur__outil. Deux
// serveurs ne peuvent pas produire le même nom exposé : le nom logique est
// unique au registre et ne contient pas le séparateur.
func (g *Gate) construireServeur() error {
	g.serveur = mcp.NewServer(&mcp.Implementation{Name: "mcp-gate", Version: "0.1.0"}, &mcp.ServerOptions{Logger: g.log})
	vus := map[string]bool{}
	for _, s := range g.cfg.Registre.Serveurs {
		for _, o := range s.Outils {
			expose, err := nommage.Exposer(s.Nom, o.Nom)
			if err != nil {
				return fmt.Errorf("gate: %w", err)
			}
			if vus[expose] {
				return fmt.Errorf("gate: nom exposé %q en double", expose)
			}
			vus[expose] = true
			g.serveur.AddTool(outilExpose(expose, o), g.gestionnaire(expose))
		}
	}
	return nil
}

// outilExpose est l'outil tel que l'agent le voit : tout vient du registre.
func outilExpose(expose string, o registre.Outil) *mcp.Tool {
	t := &mcp.Tool{Name: expose, Description: envelope.Defang(o.Description), InputSchema: o.InputSchema}
	if o.Acces == registre.Lecture {
		t.Annotations = &mcp.ToolAnnotations{ReadOnlyHint: true}
	} else {
		vrai := true
		t.Annotations = &mcp.ToolAnnotations{DestructiveHint: &vrai}
	}
	return t
}

// Serveur rend le serveur MCP à servir à l'agent.
func (g *Gate) Serveur() *mcp.Server { return g.serveur }

// Backend rend le backend d'un nom logique.
func (g *Gate) Backend(nom string) backend.Backend { return g.backends[nom] }

// Sessions rend le gestionnaire de sessions.
func (g *Gate) Sessions() *session.Gestionnaire { return g.sessions }

// EnPanne dit si le journal a refusé une écriture.
func (g *Gate) EnPanne() bool { return g.enPanne.Load() }

// Fermer ferme les backends.
func (g *Gate) Fermer() error {
	var errs []error
	for _, b := range g.backends {
		if err := b.Fermer(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (g *Gate) gestionnaire(expose string) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var brut json.RawMessage
		var meta mcp.Meta
		if req.Params != nil {
			brut, meta = req.Params.Arguments, req.Params.Meta
		}
		return g.Traiter(ctx, req.Session, expose, brut, meta), nil
	}
}

// Traiter fait passer un tools/call par l'engine. Il rend toujours un
// résultat : un refus est un résultat en erreur que l'agent peut lire.
// meta est le _meta de l'appel : seul sous_agent en est lu, pour le
// journal ; rien n'en est transmis au serveur.
func (g *Gate) Traiter(ctx context.Context, ss *mcp.ServerSession, expose string, brut json.RawMessage, meta mcp.Meta) *mcp.CallToolResult {
	if g.enPanne.Load() {
		return refusEngine("Appel refusé : le journal de l'engine est indisponible ; aucun appel n'est transmis sans preuve.")
	}
	// 1. Décomposer.
	serveur, outil, err := nommage.Decomposer(expose)
	b := g.backends[serveur]
	if err != nil || b == nil {
		return refusEngine("Appel refusé : outil inconnu de l'engine.")
	}
	args, err := decoderArguments(brut)
	if err != nil {
		g.alerte(serveur, outil, "arguments illisibles, appel refusé avant la session")
		return refusEngine("Appel refusé : les arguments doivent former un objet JSON.")
	}

	sousAgent, lisible := sousAgentDeclare(meta)
	if !lisible {
		g.alerte(serveur, outil, "_meta.sous_agent illisible (type, longueur ou caractères), ignoré")
	}

	// 2. Entrer : le verrou est rendu sur tous les chemins.
	g.suivre(ss)
	tour, err := g.sessions.Entrer(ctx, ss, g.cfg.Appelant)
	defer tour.Terminer()
	if err != nil {
		if !erreurSessionAttendue(err) {
			// Journal ou générateur d'identifiants en panne : l'engine ne
			// peut plus prouver ce qu'il fait.
			g.enPanne.Store(true)
			g.log.Error("session impossible, l'engine refuse désormais tout appel", "erreur", err)
		}
		return refusEngine("Appel refusé : " + motifSession(err))
	}
	if ss != nil {
		g.idsSession.LoadOrStore(ss, tour.SessionID())
	}

	// 3. Une seule lecture d'horloge ; dépôt au coffre avant l'entrée appel.
	ts := g.horloge().UTC()
	origine := approval.Digest(outil, args)
	if err := g.coffre.Deposer(journal.Depot{TS: ts, Empreinte: origine, Outil: outil, ArgumentsOrigine: args}); err != nil {
		g.log.Error("coffre indisponible", "erreur", err)
		g.alerte(serveur, outil, "coffre indisponible, appel refusé")
		return refusEngine("Appel refusé : le coffre de l'engine est indisponible ; aucun appel n'est transmis sans preuve.")
	}

	// 4. Décider, sous le jeu actif à cet instant, lu une seule fois. Tout
	// ce que Decide v2 lit du registre vient de l'entrée épinglée du
	// serveur : accès, étiquettes, version.
	jeu := *g.jeu.Load()
	reg := g.cfg.Registre
	srv, _ := reg.Serveur(serveur)
	o, _ := srv.Outil(outil)
	version, acces, etiquettes := reg.Version(), directive.Acces(o.Acces), srv.EtiquettesOutil(outil)
	etat := g.completer(tour.Etat())
	att := b.Attestation()
	demandeur := g.demandeur(tour.SessionID())
	sujets, nonResolus := g.resoudreSujets(ctx, o, args)
	dec := directive.Decide(jeu, etat, directive.Appel{
		Serveur: serveur, Outil: outil, Arguments: args, Attestation: att, Horodatage: ts,
		Acces: acces, EtiquettesCible: etiquettes, VersionRegistre: version, SousAgentDeclare: sousAgent,
		Demandeur: demandeur, Sujets: sujets,
	})
	// Contrôle du registre, toujours actif, même sans directive
	// source_attestee pour ce serveur : refus d'origine registre.
	dec, parRegistre := controle.Registre(dec, serveur, att)
	// Notifications des directives appliquées : remises avant toute
	// transmission, chacune journalisée avec son statut de remise. Une
	// notification si_echec: refuser non remise refuse l'appel.
	remises, err := g.notifier(dec, journal.Entree{TS: ts, Session: tour.SessionID(), Seq: tour.Seq(), Appelant: etat.Appelant,
		Serveur: serveur, OutilExpose: expose, Outil: outil}, directive.EvAppelOutil)
	if err != nil {
		g.enPanne.Store(true)
		g.log.Error("journal indisponible, l'engine refuse désormais tout appel", "erreur", err)
		return refusEngine("Appel refusé : le journal de l'engine est indisponible ; rien n'est transmis sans preuve.")
	}
	parNotification := false
	if !parRegistre {
		dec, parNotification = journal.ControleNotifications(dec, remises)
	}
	decJ := journal.DepuisDecision(dec)
	switch {
	case parRegistre:
		decJ.Origine = controle.OrigineRegistre
	case parNotification:
		decJ.Origine = journal.OrigineNotification
	}
	entree := journal.Entree{
		TS: ts, Type: journal.AppelT, Session: tour.SessionID(), Seq: tour.Seq(), Appelant: etat.Appelant,
		SousAgentDeclare: sousAgent,
		Jeu:              journal.DepuisJeu(jeu), Serveur: serveur, OutilExpose: expose, Outil: outil,
		Appel: &journal.Appel{EmpreinteOrigine: dec.EmpreinteOrigine, EmpreinteTransmise: dec.EmpreinteTransmise,
			ChampsRetires: dec.ChampsRetires},
		Attestation:     journal.DepuisAttestation(att),
		EtatSession:     journal.DepuisConsultations(etat.Consultations),
		Decision:        decJ,
		Resultat:        &journal.Resultat{Statut: journal.StatutNonTransmis},
		VersionRegistre: version, Acces: string(acces), EtiquettesCible: nonVide(etiquettes),
		Demandeur: journal.DepuisDemandeur(demandeur), Remises: remises,
		Sujets: journal.DepuisSujets(sujets), SujetsNonResolus: nonResolus,
	}

	var rep *mcp.CallToolResult
	transmettre := false
	switch {
	case parRegistre:
		rep = g.refusRegistre(dec)
	case parNotification:
		rep = refusNotification(dec, remises)
	case dec.Effet == directive.Refuser:
		// 5.
		rep = refusDirective(dec)
	case dec.Effet == directive.AttendreValidation:
		// 6. Sans jeton, l'appel attend. Avec des jetons (reprise), ils
		// sont vérifiés, l'entrée approbation est écrite, et l'appel ne
		// part que si elle est acceptée.
		jetons, lisibles := jetonsApprobation(meta)
		if !lisibles {
			g.alerte(serveur, outil, "_meta.approvalToken illisible (type, nombre ou longueur), ignoré")
			rep = enAttente(dec, "jeton d'approbation illisible")
			break
		}
		if len(jetons) == 0 {
			rep = enAttente(dec, "")
			break
		}
		appro := g.verifierValidation(dec, jetons, demandeur, ts)
		if _, err := g.journal.Ecrire(journal.Entree{
			TS: ts, Type: journal.ApprobationT, Session: entree.Session, Seq: entree.Seq, Appelant: entree.Appelant,
			Serveur: serveur, OutilExpose: expose, Outil: outil, Approbation: appro, Demandeur: entree.Demandeur,
		}); err != nil {
			g.enPanne.Store(true)
			g.log.Error("journal indisponible, l'engine refuse désormais tout appel", "erreur", err)
			return refusEngine("Appel refusé : le journal de l'engine est indisponible ; rien n'est transmis sans preuve.")
		}
		if appro.Statut != journal.ApprobationAcceptee {
			rep = enAttente(dec, appro.Motif)
			break
		}
		entree.Approbation = appro
		transmettre = true
	default:
		if _, ok := meta[CleJetonApprobation]; ok {
			g.alerte(serveur, outil, "jeton d'approbation fourni pour un appel qui n'attend pas de validation : ignoré")
		}
		transmettre = true
	}

	if transmettre {
		// 7. D'abord la trace chaînée de ce qui va partir : sans elle,
		// rien ne part.
		if _, err := g.journal.Ecrire(journal.Entree{
			TS: ts, Type: journal.TransmissionT, Session: entree.Session, Seq: entree.Seq, Appelant: entree.Appelant,
			Jeu: entree.Jeu, Serveur: serveur, OutilExpose: expose, Outil: outil,
			Appel:    &journal.Appel{EmpreinteTransmise: dec.EmpreinteTransmise},
			Decision: entree.Decision,
		}); err != nil {
			g.enPanne.Store(true)
			g.log.Error("journal indisponible, l'engine refuse désormais tout appel", "erreur", err)
			return refusEngine("Appel refusé : le journal de l'engine est indisponible ; rien n'est transmis sans preuve.")
		}
		debut := time.Now()
		c, retour := tour.Transmettre(ctx, serveur, outil, dec.EmpreinteTransmise, func(ctx context.Context) session.Retour {
			return appeler(ctx, b, outil, dec.Arguments)
		})
		entree.Resultat = &journal.Resultat{Statut: string(c.Resultat), EmpreinteReponse: c.EmpreinteReponse,
			DureeMs: time.Since(debut).Milliseconds(), Source: sourceDeclaree(retour.Reponse)}
		g.retenir(entree)
		if retour.Err != nil {
			g.log.Warn("appel au backend", "serveur", serveur, "outil", outil, "resultat", c.Resultat, "erreur", retour.Err)
		}
		rep = g.repondre(c, retour, dec)
	}

	// 8. L'entrée appel, à la date passée à Decide, avant Terminer.
	if _, err := g.journal.Ecrire(entree); err != nil {
		g.enPanne.Store(true)
		g.log.Error("journal indisponible, l'engine refuse désormais tout appel", "erreur", err)
		return refusEngine("Appel non confirmé : le journal de l'engine est indisponible ; la réponse est retenue.")
	}
	return rep
}

// suivre ferme la session quand le client s'en va. Une fois par session,
// au premier appel : avec le protocole sans initialize, rien d'autre ne
// signale l'ouverture.
func (g *Gate) suivre(ss *mcp.ServerSession) {
	if ss == nil {
		return
	}
	if _, deja := g.suivies.LoadOrStore(ss, struct{}{}); !deja {
		g.sessions.Suivre(ss)
		go func() {
			ss.Wait()
			if id, ok := g.idsSession.LoadAndDelete(ss); ok {
				g.complements.Delete(id)
				g.demandeurs.Delete(id)
			}
		}()
	}
}

// ouvrir complète l'entrée session_ouverte : source d'identité, référence
// de l'annuaire, et demandeur prouvé par le jeton remis au lancement, avec
// ses groupes résolus à la date d'ouverture. Un jeton qui ne se vérifie
// plus (expiré depuis le lancement) laisse la session sans demandeur ; le
// message le dit, sans citer le jeton.
func (g *Gate) ouvrir(en *journal.Entree) string {
	var d *directive.Demandeur
	if a := g.cfg.Annuaire; a != nil {
		ref := a.Reference()
		en.SourceIdentite = a.Nom()
		en.Annuaire = &journal.RefAnnuaire{ID: ref.ID, Version: ref.Version, Empreinte: ref.Empreinte}
		if g.cfg.JetonDemandeur == "" {
			en.Message = "aucun jeton de demandeur : demandeur non résolu"
		} else if id, err := annuaire.VerifierIdentite(a, g.cfg.JetonDemandeur, en.TS); err != nil {
			en.Message = "jeton du demandeur refusé (" + err.Error() + ") : demandeur non résolu"
		} else {
			fiche, _ := a.Resoudre(id.Demandeur, en.TS)
			d = &directive.Demandeur{Identite: fiche.Identite, Groupes: fiche.Groupes}
			en.Demandeur = journal.DepuisDemandeur(d)
			en.Message = "demandeur " + id.Demandeur + " (jeton signé par " + id.Emetteur + ")"
			g.demandeurs.Store(en.Session, d)
		}
	}
	// Directives de l'évènement ouverture_session.
	dec, err := g.evenement(en, directive.EntreeEvenement{Evenement: directive.EvOuvertureSession, Horodatage: en.TS,
		Appelant: en.Appelant, Demandeur: d})
	if err != nil {
		return "ouverture refusée : journal indisponible"
	}
	if dec.Effet == directive.Refuser {
		return "ouverture refusée : " + dec.Motif
	}
	return ""
}

// evenement évalue un évènement (autre qu'appel_outil) sous le jeu actif,
// remet ses notifications et consigne la décision dans en, l'entrée qui
// journalise l'évènement (session_ouverte, attestation, purge) : jeu,
// décision, remises. Rien n'est consigné si aucune directive du jeu ne
// porte sur l'évènement. Une notification si_echec: refuser non remise
// refuse l'ouverture.
func (g *Gate) evenement(en *journal.Entree, e directive.EntreeEvenement) (directive.Decision, error) {
	jeu := *g.jeu.Load()
	if !porteSur(jeu, e.Evenement) {
		return directive.Decision{Effet: directive.Autoriser}, nil
	}
	dec := directive.DecideEvenement(jeu, e)
	remises, err := g.notifier(dec, journal.Entree{TS: e.Horodatage, Session: en.Session, Appelant: en.Appelant, Serveur: en.Serveur}, e.Evenement)
	if err != nil {
		g.enPanne.Store(true)
		g.log.Error("journal indisponible, l'engine refuse désormais tout appel", "erreur", err)
		return dec, err
	}
	dec, parNotification := journal.ControleNotifications(dec, remises)
	en.Jeu, en.Decision, en.Remises = journal.DepuisJeu(jeu), journal.DepuisDecision(dec), remises
	if parNotification {
		en.Decision.Origine = journal.OrigineNotification
	}
	return dec, nil
}

// porteSur dit si une directive du jeu, quel que soit son statut, porte
// sur l'évènement.
func porteSur(jeu directive.Jeu, ev directive.Evenement) bool {
	for _, d := range jeu.Directives {
		if d.EstV2() && d.Declencheur.Evenement == ev {
			return true
		}
	}
	return false
}

// demandeur rend le demandeur résolu à l'ouverture de la session, ou nil.
func (g *Gate) demandeur(session string) *directive.Demandeur {
	if v, ok := g.demandeurs.Load(session); ok {
		return v.(*directive.Demandeur)
	}
	return nil
}

// complement est ce que l'engine consigne d'une consultation en plus de ce
// que la session garde. Il est pris dans l'entrée appel qui l'a produite,
// avec les mêmes valeurs que celles que le rejeu relit dans cette entrée
// (rejeu.ReconstituerEtat) : étiquettes et version du registre au moment
// de l'appel, source déclarée.
type complement struct {
	etiquettes []string
	version    string
	source     *directive.SourceReponse
}

// complementsSession est tenu verrou de session : un seul tour à la fois
// y lit ou y écrit.
type complementsSession struct {
	mu     sync.Mutex
	parSeq map[int]complement
}

// retenir garde le complément d'une consultation, à partir de l'entrée
// appel qui la journalise.
func (g *Gate) retenir(en journal.Entree) {
	v, _ := g.complements.LoadOrStore(en.Session, &complementsSession{parSeq: map[int]complement{}})
	cs := v.(*complementsSession)
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.parSeq[en.Seq] = complement{etiquettes: nonVide(en.EtiquettesCible), version: en.VersionRegistre,
		source: en.Resultat.Source.VersDirective()}
}

// completer ajoute aux consultations de l'état ce que l'engine en a
// retenu. Une consultation sans complément garde des étiquettes inconnues
// (version vide) : Decide la traite comme indéterminée.
func (g *Gate) completer(etat directive.EtatSession) directive.EtatSession {
	v, ok := g.complements.Load(etat.ID)
	if !ok {
		return etat
	}
	cs := v.(*complementsSession)
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for i := range etat.Consultations {
		if c, ok := cs.parSeq[etat.Consultations[i].Seq]; ok {
			etat.Consultations[i].Etiquettes, etat.Consultations[i].VersionRegistre, etat.Consultations[i].Source =
				c.etiquettes, c.version, c.source
		}
	}
	return etat
}

func nonVide(l []string) []string {
	if len(l) == 0 {
		return nil
	}
	return l
}

// notifier remet chaque notification que la décision demande, dans
// l'ordre canonique de la décision, par le canal des abonnés du journal.
// base porte la session, le seq et la cible.
func (g *Gate) notifier(dec directive.Decision, base journal.Entree, ev directive.Evenement) ([]journal.Remise, error) {
	var out []journal.Remise
	evenement := ""
	if ev != directive.EvAppelOutil {
		evenement = string(ev)
	}
	for _, n := range journal.DepuisDecision(dec).Notifications {
		en := base
		en.Notification = &journal.Remise{Notification: n, Evenement: evenement}
		ecrite, err := g.journal.EcrireRemise(en)
		if err != nil {
			return nil, err
		}
		out = append(out, *ecrite.Notification)
	}
	return out, nil
}

// Alerter écrit une entrée alerte au journal (par exemple, pour signaler
// une horloge simulée).
func (g *Gate) Alerter(message string) { g.alerte("", "", message) }

func (g *Gate) alerte(serveur, outil, message string) {
	if _, err := g.journal.Ecrire(journal.Entree{Type: journal.AlerteT, Serveur: serveur, Outil: outil, Message: message}); err != nil {
		g.enPanne.Store(true)
		g.log.Error("journal indisponible", "erreur", err)
	}
}

// decoderArguments accepte un objet JSON, ou rien. Les nombres sont des
// float64, comme à la relecture du coffre : l'empreinte d'origine se
// recalcule à l'identique au rejeu.
func decoderArguments(brut json.RawMessage) (map[string]any, error) {
	if len(brut) == 0 || string(brut) == "null" {
		return map[string]any{}, nil
	}
	var args map[string]any
	if err := json.Unmarshal(brut, &args); err != nil {
		return nil, err
	}
	if args == nil {
		args = map[string]any{}
	}
	return args, nil
}

func erreurSessionAttendue(err error) bool {
	for _, e := range []error{session.ErrIdentiteChangee, session.ErrFermee, session.ErrSessionNulle,
		context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

// CleSousAgent est la clé de _meta où l'agent déclare le sous-agent qui
// appelle.
const CleSousAgent = "sous_agent"

// LongueurMaxSousAgent borne l'étiquette déclarée, qui va telle quelle au
// journal.
const LongueurMaxSousAgent = 128

// sousAgentDeclare lit _meta.sous_agent. Absent : "" et lisible. Une
// valeur qui n'est pas une chaîne UTF-8 de 1 à 128 octets sans caractère
// de contrôle n'est pas journalisée. La valeur n'est qu'une déclaration de
// l'agent : elle passe à Decide (feuille sous_agent_declare, qui ne sert
// qu'à resserrer) et au journal, jamais à l'état de session ni au serveur.
func sousAgentDeclare(meta mcp.Meta) (string, bool) {
	v, ok := meta[CleSousAgent]
	if !ok || v == nil {
		return "", true
	}
	s, ok := v.(string)
	if !ok || s == "" || len(s) > LongueurMaxSousAgent || !utf8.ValidString(s) {
		return "", false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	return s, true
}

func motifSession(err error) string {
	switch {
	case errors.Is(err, session.ErrIdentiteChangee):
		return "l'appelant a changé en cours de session."
	case errors.Is(err, session.ErrFermee):
		return "la session est fermée."
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "appel annulé avant son tour."
	}
	return "session indisponible."
}

// appeler transmet au backend et classe le retour.
func appeler(ctx context.Context, b backend.Backend, outil string, args map[string]any) session.Retour {
	res, err := b.Appeler(ctx, outil, args, nil)
	if err != nil {
		var rpc *jsonrpc.Error
		switch {
		case errors.As(err, &rpc), errors.Is(err, backend.ErrEntreeDemandee):
			return session.Retour{Resultat: directive.ResultatErreur, Err: err}
		default:
			return session.Retour{Resultat: directive.ResultatIndisponible, Err: err}
		}
	}
	r := session.Retour{Resultat: directive.ResultatOK, Reponse: res, EmpreinteReponse: empreinteReponse(res)}
	if res.IsError {
		r.Resultat = directive.ResultatErreur
	}
	return r
}

// Origines de la donnée, telles que les serveurs de données les déclarent
// dans structuredContent.source.origine, et OrigineNonDeclaree quand le
// résultat n'en déclare pas de reconnaissable.
const (
	OrigineDirect      = "direct" // obtenue de l'API officielle pendant ce processus
	OrigineRejeu       = "rejeu"  // lue dans un enregistrement daté
	OrigineAucune      = "aucune" // aucune donnée servie
	OrigineNonDeclaree = journal.OrigineNonDeclaree
)

// sourceDeclaree reprend la provenance que le serveur déclare dans
// structuredContent.source : origine et date. L'engine ne l'invente pas :
// sans déclaration, l'origine est non_declaree. Ce qui va au journal vient
// du serveur, donc est borné : une origine hors vocabulaire vaut
// non_declaree, une date qui n'est pas en RFC 3339 est omise.
func sourceDeclaree(reponse any) *journal.Source {
	src := &journal.Source{Origine: OrigineNonDeclaree}
	res, _ := reponse.(*mcp.CallToolResult)
	if res == nil {
		return src
	}
	sc, _ := res.StructuredContent.(map[string]any)
	decl, _ := sc["source"].(map[string]any)
	if decl == nil {
		return src
	}
	switch o, _ := decl["origine"].(string); o {
	case OrigineDirect, OrigineRejeu, OrigineAucune:
		src.Origine = o
	default:
		return src
	}
	if d, _ := decl["date"].(string); d != "" {
		if t, err := time.Parse(time.RFC3339Nano, d); err == nil {
			src.Date = t.UTC().Format(time.RFC3339Nano)
		}
	}
	return src
}

// empreinteReponse : sha256 de la réponse en JSON canonique.
func empreinteReponse(res *mcp.CallToolResult) string {
	b, err := json.Marshal(res)
	if err != nil {
		return ""
	}
	var generique any
	if json.Unmarshal(b, &generique) == nil {
		if canon, err := json.Marshal(generique); err == nil {
			b = canon
		}
	}
	somme := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(somme[:])
}

// repondre construit la réponse à l'agent. Tout ce qui vient du serveur
// est scellé comme donnée ; rien d'autre n'en est repris (ni _meta, ni
// contenu non textuel, ni structuredContent en clair).
func (g *Gate) repondre(c directive.Consultation, r session.Retour, dec directive.Decision) *mcp.CallToolResult {
	res, _ := r.Reponse.(*mcp.CallToolResult)
	if res == nil {
		switch c.Resultat {
		case directive.ResultatIndisponible:
			return echec(c.Serveur, c.Outil, "Source indisponible : le serveur "+c.Serveur+" n'a pas répondu.")
		default:
			return echec(c.Serveur, c.Outil, "Le serveur "+c.Serveur+" a répondu par une erreur que l'engine ne transmet pas.")
		}
	}
	out, err := sceller(res)
	if err != nil {
		g.log.Error("scellement impossible", "erreur", err)
		return refusEngine("Réponse retenue : l'engine n'a pas pu la sceller.")
	}
	if dec.Effet == directive.Caviarder {
		out.Content = append([]mcp.Content{&mcp.TextContent{Text: "Note de l'engine : " + dec.Motif + "."}}, out.Content...)
	}
	return out
}

// sceller range la réponse d'un serveur dans des enveloppes de donnée.
func sceller(res *mcp.CallToolResult) (*mcp.CallToolResult, error) {
	out := &mcp.CallToolResult{IsError: res.IsError}
	for _, c := range res.Content {
		switch c := c.(type) {
		case *mcp.TextContent:
			s, err := envelope.Seal(c.Text)
			if err != nil {
				return nil, err
			}
			out.Content = append(out.Content, &mcp.TextContent{Text: s})
		default:
			out.Content = append(out.Content, &mcp.TextContent{
				Text: fmt.Sprintf("[contenu %s retiré par l'engine : seul le texte est transmis]", typeContenu(c))})
		}
	}
	if res.StructuredContent != nil {
		b, err := json.Marshal(res.StructuredContent)
		if err != nil {
			return nil, err
		}
		s, err := envelope.Seal(string(b))
		if err != nil {
			return nil, err
		}
		out.Content = append(out.Content, &mcp.TextContent{Text: s})
	}
	if len(out.Content) == 0 {
		out.Content = []mcp.Content{&mcp.TextContent{Text: "[réponse vide]"}}
	}
	return out, nil
}

func typeContenu(c mcp.Content) string {
	switch c.(type) {
	case *mcp.ImageContent:
		return "image"
	case *mcp.AudioContent:
		return "audio"
	case *mcp.ResourceLink:
		return "lien de ressource"
	case *mcp.EmbeddedResource:
		return "ressource intégrée"
	}
	return "non textuel"
}

// Servir sert le gate sur un transport jusqu'à la fin de la session.
func (g *Gate) Servir(ctx context.Context, t mcp.Transport) error {
	err := g.serveur.Run(ctx, t)
	if err != nil && strings.HasSuffix(err.Error(), io.EOF.Error()) {
		return nil // l'agent a fermé stdin
	}
	return err
}
