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
	g := &Gate{
		cfg: cfg, journal: cfg.Journal, coffre: cfg.Coffre, charges: map[versionJeu]string{},
		horloge: cfg.Horloge, log: cfg.Logger, backends: map[string]backend.Backend{},
	}
	jeu := cfg.Jeu
	g.jeu.Store(&jeu)
	g.charges[versionJeu{jeu.Meta.ID, jeu.Meta.Version}] = jeu.Meta.Empreinte
	// Contrat §3 : le journal est attaché au coffre dès l'ouverture.
	g.coffre.Journaliser(g.journal)
	sc := cfg.Session
	fermeture := sc.AFermeture
	sc.AFermeture = func(ss *mcp.ServerSession, motif string) {
		if fermeture != nil {
			fermeture(ss, motif)
		}
		ss.Close()
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
	if _, err := g.journal.Ecrire(journal.Entree{Type: journal.AttestationT, Serveur: nom,
		Attestation: journal.DepuisAttestation(a), Message: message}); err != nil {
		g.enPanne.Store(true)
		g.log.Error("journal indisponible", "erreur", err)
		return fmt.Errorf("gate: journal: %w", err)
	}
	return nil
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
		return refus("Appel refusé : le journal de l'engine est indisponible ; aucun appel n'est transmis sans preuve.")
	}
	// 1. Décomposer.
	serveur, outil, err := nommage.Decomposer(expose)
	b := g.backends[serveur]
	if err != nil || b == nil {
		return refus("Appel refusé : outil inconnu de l'engine.")
	}
	args, err := decoderArguments(brut)
	if err != nil {
		g.alerte(serveur, outil, "arguments illisibles, appel refusé avant la session")
		return refus("Appel refusé : les arguments doivent former un objet JSON.")
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
		return refus("Appel refusé : " + motifSession(err))
	}

	// 3. Une seule lecture d'horloge ; dépôt au coffre avant l'entrée appel.
	ts := g.horloge().UTC()
	origine := approval.Digest(outil, args)
	if err := g.coffre.Deposer(journal.Depot{TS: ts, Empreinte: origine, Outil: outil, ArgumentsOrigine: args}); err != nil {
		g.log.Error("coffre indisponible", "erreur", err)
		g.alerte(serveur, outil, "coffre indisponible, appel refusé")
		return refus("Appel refusé : le coffre de l'engine est indisponible ; aucun appel n'est transmis sans preuve.")
	}

	// 4. Décider, sous le jeu actif à cet instant, lu une seule fois.
	jeu := *g.jeu.Load()
	etat := tour.Etat()
	att := b.Attestation()
	dec := directive.Decide(jeu, etat, directive.Appel{
		Serveur: serveur, Outil: outil, Arguments: args, Attestation: att, Horodatage: ts,
	})
	// Contrôle du registre, toujours actif, même sans directive
	// source_attestee pour ce serveur : refus d'origine registre.
	dec, parRegistre := controle.Registre(dec, serveur, att)
	decJ := journal.DepuisDecision(dec)
	if parRegistre {
		decJ.Origine = controle.OrigineRegistre
	}
	entree := journal.Entree{
		TS: ts, Type: journal.AppelT, Session: tour.SessionID(), Seq: tour.Seq(), Appelant: etat.Appelant,
		SousAgentDeclare: sousAgent,
		Jeu:              journal.DepuisJeu(jeu), Serveur: serveur, OutilExpose: expose, Outil: outil,
		Appel: &journal.Appel{EmpreinteOrigine: dec.EmpreinteOrigine, EmpreinteTransmise: dec.EmpreinteTransmise,
			ChampsRetires: dec.ChampsRetires},
		Attestation: journal.DepuisAttestation(att),
		EtatSession: journal.DepuisConsultations(etat.Consultations),
		Decision:    decJ,
		Resultat:    &journal.Resultat{Statut: journal.StatutNonTransmis},
	}

	var rep *mcp.CallToolResult
	switch {
	case parRegistre:
		rep = refus("Appel " + dec.Motif + ".")
	case dec.Effet == directive.Refuser:
		// 5.
		rep = refus("Appel refusé par l'engine : " + dec.Motif + ".")
	case dec.Effet == directive.AttendreValidation:
		// 6.
		rep = enAttente(dec)
	default:
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
			return refus("Appel refusé : le journal de l'engine est indisponible ; rien n'est transmis sans preuve.")
		}
		debut := time.Now()
		c, retour := tour.Transmettre(ctx, serveur, outil, dec.EmpreinteTransmise, func(ctx context.Context) session.Retour {
			return appeler(ctx, b, outil, dec.Arguments)
		})
		entree.Resultat = &journal.Resultat{Statut: string(c.Resultat), EmpreinteReponse: c.EmpreinteReponse,
			DureeMs: time.Since(debut).Milliseconds(), Source: sourceDeclaree(retour.Reponse)}
		if retour.Err != nil {
			g.log.Warn("appel au backend", "serveur", serveur, "outil", outil, "resultat", c.Resultat, "erreur", retour.Err)
		}
		rep = g.repondre(c, retour, dec)
	}

	// 8. L'entrée appel, à la date passée à Decide, avant Terminer.
	if _, err := g.journal.Ecrire(entree); err != nil {
		g.enPanne.Store(true)
		g.log.Error("journal indisponible, l'engine refuse désormais tout appel", "erreur", err)
		return refus("Appel non confirmé : le journal de l'engine est indisponible ; la réponse est retenue.")
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
	}
}

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
// l'agent : elle ne passe ni à Decide, ni à l'état de session, ni au
// serveur.
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
	OrigineNonDeclaree = "non_declaree"
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
			return refus("Source indisponible : le serveur " + c.Serveur + " n'a pas répondu.")
		default:
			return refus("Le serveur " + c.Serveur + " a répondu par une erreur que l'engine ne transmet pas.")
		}
	}
	out, err := sceller(res)
	if err != nil {
		g.log.Error("scellement impossible", "erreur", err)
		return refus("Réponse retenue : l'engine n'a pas pu la sceller.")
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

func refus(motif string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: motif}}}
}

// enAttente : la reprise avec jeton n'existe pas encore ; l'appel est
// refusé, avec de quoi l'identifier quand elle existera.
func enAttente(dec directive.Decision) *mcp.CallToolResult {
	var ids []string
	for _, a := range dec.Applications {
		if a.Appliquee && a.Effet == directive.AttendreValidation {
			ids = append(ids, a.ID)
		}
	}
	directiveID := ""
	if len(ids) > 0 {
		directiveID = ids[0]
	}
	r := refus("Appel non transmis : " + dec.Motif + ". La reprise après validation humaine n'est pas encore disponible ; l'appel est refusé.")
	r.StructuredContent = map[string]any{
		"statut":              "en_attente",
		"empreinte_transmise": dec.EmpreinteTransmise,
		"directive":           directiveID,
		"directives":          ids,
	}
	return r
}

// Servir sert le gate sur un transport jusqu'à la fin de la session.
func (g *Gate) Servir(ctx context.Context, t mcp.Transport) error {
	err := g.serveur.Run(ctx, t)
	if err != nil && strings.HasSuffix(err.Error(), io.EOF.Error()) {
		return nil // l'agent a fermé stdin
	}
	return err
}
