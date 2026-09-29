// Package backend tient les sessions MCP client que l'engine ouvre vers les
// serveurs du registre. Un backend est une session tenue par l'engine, avec
// une attestation figée à la connexion : ce que l'engine a observé du
// serveur (empreinte du binaire, clé TLS) et si son catalogue d'outils est
// celui que le registre épingle.
//
// Ce qu'un backend garantit par construction :
//
//   - aucun gestionnaire de sampling ni d'elicitation n'est câblé et aucune
//     capacité n'est annoncée : ces requêtes initiées par le serveur sont
//     refusées, et un résultat qui réclame une entrée est une erreur ;
//   - un binaire stdio dont l'empreinte diffère n'est jamais lancé ;
//   - une notification tools/list_changed suspend le backend le temps de
//     revérifier le catalogue ; une dérive le met en quarantaine, sans
//     retour possible avant reconnexion ;
//   - Appeler refuse tout outil hors du catalogue épinglé, et tout appel
//     quand l'attestation n'est pas valide.
//
// Connecter rend toujours un Backend, même en échec : l'engine a besoin de
// son nom et de son attestation (en échec) pour exposer le schéma épinglé
// et journaliser le refus.
package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/internal/directive"
	"mcprogue/internal/registre"
	"mcprogue/internal/toolpin"
)

// Backend est une session MCP client vers un serveur du registre.
type Backend interface {
	Nom() string
	Attestation() directive.Attestation
	Outils(ctx context.Context) ([]*mcp.Tool, error)
	Appeler(ctx context.Context, outil string, args map[string]any, meta mcp.Meta) (*mcp.CallToolResult, error)
	Fermer() error
}

var (
	// ErrNonAttest : l'attestation n'est pas valide (échec à la connexion,
	// revérification en cours ou quarantaine) ; rien n'est transmis.
	ErrNonAttest = errors.New("backend: serveur non attesté, rien n'est transmis")
	// ErrOutilInconnu : l'outil n'est pas au catalogue épinglé.
	ErrOutilInconnu = errors.New("backend: outil absent du catalogue épinglé, rien n'est transmis")
	// ErrEntreeDemandee : le serveur a répondu par une demande d'entrée
	// (sampling ou elicitation) ; l'engine n'y répond jamais.
	ErrEntreeDemandee = errors.New("backend: le serveur réclame une entrée (sampling ou elicitation), refusée par construction")
	// ErrNonConnecte : la connexion a échoué.
	ErrNonConnecte = errors.New("backend: non connecté")
)

// Options de connexion.
type Options struct {
	// AuChangement est appelée, hors verrou, chaque fois que l'attestation
	// change après la connexion : suspension pour revérification, retour
	// après une revérification conforme, quarantaine. L'engine y journalise.
	AuChangement func(nom string, a directive.Attestation)
	// DelaiVerification borne une revérification du catalogue. Défaut 10 s.
	DelaiVerification time.Duration
	// Stderr reçoit la sortie d'erreur d'un serveur stdio ; nil la jette.
	Stderr io.Writer
	// Logger reçoit les traces du client MCP ; nil les jette.
	Logger *slog.Logger
}

// Implementation est ce que l'engine annonce aux serveurs.
var Implementation = &mcp.Implementation{Name: "mcp-gate", Version: "0.1.0"}

// Connecter ouvre la session vers le serveur selon son transport, vérifie
// l'attestation, liste les outils et les compare au catalogue épinglé. Le
// Backend rendu n'est jamais nul ; l'erreur dit pourquoi il n'est pas
// attesté.
func Connecter(ctx context.Context, s registre.Serveur, opts Options) (Backend, error) {
	c := nouveau(s, opts)
	var err error
	switch s.Transport {
	case registre.Stdio:
		err = c.connecterStdio(ctx)
	case registre.HTTP:
		err = c.connecterHTTP(ctx)
	case registre.HTTPSpiffe:
		// Emplacement du transport HTTP + SPIFFE (mTLS, SPIFFE ID du
		// serveur vérifié) : pas encore disponible.
		err = c.echec("", "transport http-spiffe pas encore disponible", nil)
	default:
		err = c.echec("", "transport inconnu", nil)
	}
	return c, err
}

// etat d'un backend connecté.
type etat int

const (
	connecte    etat = iota
	suspendu         // tools/list_changed reçu, revérification en cours
	quarantaine      // dérive constatée
	horsService      // connexion échouée ou fermée
)

type client struct {
	s    registre.Serveur
	opts Options
	pins map[string]string

	mu               sync.Mutex
	att              directive.Attestation // figée à la connexion
	etat             etat
	gen              int // numéro de la dernière notification tools/list_changed
	motifQuarantaine string
	cs               *mcp.ClientSession
	liberer          func() // ferme les connexions HTTP inactives, s'il y en a
}

func nouveau(s registre.Serveur, opts Options) *client {
	if opts.DelaiVerification <= 0 {
		opts.DelaiVerification = 10 * time.Second
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	c := &client{s: s, opts: opts, pins: map[string]string{}, etat: horsService,
		att: directive.Attestation{Mode: s.Attestation.Mode, Attendu: s.Attestation.Attendu}}
	for _, o := range s.Outils {
		if h, err := o.Empreinte(); err == nil {
			c.pins[o.Nom] = h
		}
	}
	return c
}

// echec fige une attestation en échec et rend l'erreur correspondante. Le
// détail va dans l'attestation, donc au journal et dans le motif rendu à
// l'agent : il ne contient jamais de texte venu du serveur. La cause, qui
// peut en contenir, ne va que dans l'erreur rendue à l'opérateur.
func (c *client) echec(observe, detail string, cause error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.etat = horsService
	c.att = directive.Attestation{Mode: c.s.Attestation.Mode, Attendu: c.s.Attestation.Attendu,
		Observe: observe, Detail: detail}
	if cause != nil {
		return fmt.Errorf("%w: %s: %s: %v", ErrNonConnecte, c.s.Nom, detail, cause)
	}
	return fmt.Errorf("%w: %s: %s", ErrNonConnecte, c.s.Nom, detail)
}

// clientMCP construit le client : aucune capacité, aucun gestionnaire de
// sampling ni d'elicitation, pas de multi-aller-retour automatique.
func (c *client) clientMCP() *mcp.Client {
	return mcp.NewClient(Implementation, c.optionsClient())
}

func (c *client) optionsClient() *mcp.ClientOptions {
	return &mcp.ClientOptions{
		Logger:                 c.opts.Logger,
		Capabilities:           &mcp.ClientCapabilities{},
		MultiRoundTrip:         &mcp.MultiRoundTripOptions{Disabled: true},
		ToolListChangedHandler: func(context.Context, *mcp.ToolListChangedRequest) { c.catalogueChange() },
	}
}

// etablir connecte le client sur le transport, compare le catalogue et fige
// l'attestation. observe est ce que le transport a mesuré.
func (c *client) etablir(ctx context.Context, t mcp.Transport, observe func() string) error {
	cs, err := c.clientMCP().Connect(ctx, t, nil)
	if err != nil {
		return c.echec(observe(), "connexion MCP impossible", err)
	}
	conforme, detail, err := comparer(ctx, cs, c.pins)
	if err != nil {
		cs.Close()
		return c.echec(observe(), "liste des outils impossible", err)
	}
	c.mu.Lock()
	c.cs = cs
	c.att = directive.Attestation{Mode: c.s.Attestation.Mode, Attendu: c.s.Attestation.Attendu,
		Observe: observe(), Schemas: conforme, Detail: detail}
	c.att.Verifiee = c.att.Valide()
	if c.att.Verifiee {
		c.etat = connecte
	} else {
		c.etat = horsService
	}
	att := c.att
	c.mu.Unlock()
	if !att.Verifiee {
		return fmt.Errorf("%w: %s: attestation en échec (%s)", ErrNonAttest, c.s.Nom, detailOuObserve(att))
	}
	return nil
}

func detailOuObserve(a directive.Attestation) string {
	if a.Detail != "" {
		return a.Detail
	}
	return "observé " + a.Observe + ", attendu " + a.Attendu
}

// comparer liste le catalogue annoncé et le confronte aux pins : chaque
// outil épinglé doit être annoncé à l'identique, et aucun autre outil ne
// doit l'être. Le détail ne cite que des noms venant du registre et des
// nombres, jamais une chaîne fournie par le serveur.
func comparer(ctx context.Context, cs *mcp.ClientSession, pins map[string]string) (bool, string, error) {
	vus := map[string]bool{}
	var modifies []string
	inconnus := 0
	for t, err := range cs.Tools(ctx, nil) {
		if err != nil {
			return false, "", err
		}
		pin, ok := pins[t.Name]
		if !ok || vus[t.Name] {
			inconnus++
			continue
		}
		vus[t.Name] = true
		h, err := empreinteOutil(t)
		if err != nil || h != pin {
			modifies = append(modifies, t.Name)
		}
	}
	var absents []string
	for nom := range pins {
		if !vus[nom] {
			absents = append(absents, nom)
		}
	}
	sort.Strings(modifies)
	sort.Strings(absents)
	var ecarts []string
	if len(modifies) > 0 {
		ecarts = append(ecarts, "outil(s) épinglé(s) modifié(s) : "+strings.Join(modifies, ", "))
	}
	if len(absents) > 0 {
		ecarts = append(ecarts, "outil(s) épinglé(s) absent(s) : "+strings.Join(absents, ", "))
	}
	if inconnus > 0 {
		ecarts = append(ecarts, fmt.Sprintf("%d outil(s) annoncé(s) hors registre", inconnus))
	}
	if len(ecarts) > 0 {
		return false, "catalogue non conforme au registre : " + strings.Join(ecarts, " ; "), nil
	}
	return true, "", nil
}

// empreinteOutil calcule le pin d'un outil annoncé, comme
// registre.Outil.Empreinte, sur sa forme JSON.
func empreinteOutil(t *mcp.Tool) (string, error) {
	b, err := json.Marshal(t)
	if err != nil {
		return "", err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return "", err
	}
	return toolpin.Hash(m)
}

// catalogueChange suspend le backend et lance la revérification. Appelée
// par le client MCP à chaque tools/list_changed.
func (c *client) catalogueChange() {
	c.mu.Lock()
	if c.etat != connecte && c.etat != suspendu {
		c.mu.Unlock()
		return
	}
	c.gen++
	gen, cs := c.gen, c.cs
	c.etat = suspendu
	c.mu.Unlock()
	c.notifier()
	// Hors du gestionnaire de notification : la liste est une requête, qui
	// ne doit pas attendre la boucle de lecture qui nous appelle.
	go c.reverifier(gen, cs)
}

func (c *client) reverifier(gen int, cs *mcp.ClientSession) {
	ctx, annuler := context.WithTimeout(context.Background(), c.opts.DelaiVerification)
	defer annuler()
	conforme, detail, err := comparer(ctx, cs, c.pins)
	if err != nil {
		c.opts.Logger.Warn("revérification du catalogue impossible", "serveur", c.s.Nom, "erreur", err)
		conforme, detail = false, "revérification du catalogue impossible"
	}
	c.mu.Lock()
	switch {
	case c.etat != suspendu:
		// Quarantaine ou fermeture entre-temps : rien à rétablir.
		c.mu.Unlock()
		return
	case !conforme:
		c.etat = quarantaine
		c.motifQuarantaine = "quarantaine après tools/list_changed : " + detail
	case gen == c.gen:
		c.etat = connecte
	default:
		// Une notification plus récente a sa propre revérification.
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()
	c.notifier()
}

func (c *client) notifier() {
	if c.opts.AuChangement != nil {
		c.opts.AuChangement(c.s.Nom, c.Attestation())
	}
}

func (c *client) Nom() string { return c.s.Nom }

// Attestation rend l'attestation figée à la connexion, dégradée pendant une
// revérification et en quarantaine : Schemas passe à faux, et le détail dit
// pourquoi.
func (c *client) Attestation() directive.Attestation {
	c.mu.Lock()
	defer c.mu.Unlock()
	a := c.att
	switch c.etat {
	case suspendu:
		a.Schemas, a.Detail = false, "revérification du catalogue en cours après tools/list_changed"
	case quarantaine:
		a.Schemas, a.Detail = false, c.motifQuarantaine
	}
	a.Verifiee = a.Valide()
	return a
}

// Outils rend le catalogue tel que le serveur l'annonce maintenant. Il
// n'est pas vérifié : l'engine expose le catalogue épinglé, jamais celui-ci.
func (c *client) Outils(ctx context.Context) ([]*mcp.Tool, error) {
	c.mu.Lock()
	cs := c.cs
	c.mu.Unlock()
	if cs == nil {
		return nil, ErrNonConnecte
	}
	var out []*mcp.Tool
	for t, err := range cs.Tools(ctx, nil) {
		if err != nil {
			return out, err
		}
		out = append(out, t)
	}
	return out, nil
}

// Appeler transmet un appel d'outil épinglé à un serveur attesté.
func (c *client) Appeler(ctx context.Context, outil string, args map[string]any, meta mcp.Meta) (*mcp.CallToolResult, error) {
	c.mu.Lock()
	cs, ok := c.cs, c.etat == connecte
	c.mu.Unlock()
	if !ok || cs == nil || !c.Attestation().Valide() {
		return nil, ErrNonAttest
	}
	if _, ok := c.pins[outil]; !ok {
		return nil, ErrOutilInconnu
	}
	if args == nil {
		args = map[string]any{}
	}
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: outil, Arguments: args, Meta: meta})
	if err != nil {
		return nil, err
	}
	if res.NeedsInput() || len(res.InputRequests) > 0 {
		return nil, ErrEntreeDemandee
	}
	return res, nil
}

// Fermer ferme la session ; le backend passe hors service.
func (c *client) Fermer() error {
	c.mu.Lock()
	cs, liberer := c.cs, c.liberer
	c.cs = nil
	c.etat = horsService
	c.mu.Unlock()
	var err error
	if cs != nil {
		err = cs.Close()
	}
	if liberer != nil {
		liberer()
	}
	return err
}
