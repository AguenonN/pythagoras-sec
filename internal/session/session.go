// Package session tient l'état de session que Decide lit : qui appelle, et
// ce que la session a réellement consulté. L'état est tenu par l'engine,
// jamais déclaré par l'agent : il est indexé par la *mcp.ServerSession du
// go-sdk, pas par un identifiant que le client fournirait.
//
// Un appel passe par un Tour :
//
//	tour, err := g.Entrer(ctx, ss, appelant) // verrou de session pris
//	defer tour.Terminer()                    // verrou rendu, bornes appliquées
//	dec := directive.Decide(jeu, tour.Etat(), appel)
//	if dec.Effet != directive.Refuser {
//		c, retour := tour.Transmettre(ctx, serveur, outil, dec.EmpreinteTransmise, backend)
//	}
//
// Les appels d'une même session sont sérialisés : Decide voit toujours un
// état qui contient toutes les consultations précédentes. Un refus de
// l'engine (tour terminé sans Transmettre) n'est jamais une consultation.
package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/internal/directive"
	"mcprogue/internal/journal"
)

var (
	// ErrIdentiteChangee : l'appelant diffère de celui fixé à la création.
	ErrIdentiteChangee = errors.New("session: l'appelant a changé en cours de session")
	// ErrFermee : la session a été fermée, par le client ou par une borne.
	ErrFermee = errors.New("session: fermée")
	// ErrDejaTransmis : un tour ne transmet qu'une fois ; rien n'est parti.
	ErrDejaTransmis = errors.New("session: tour déjà transmis")
	// ErrSessionNulle : pas de ServerSession, pas d'état à tenir.
	ErrSessionNulle = errors.New("session: ServerSession nulle")
	// ErrTourNul : Transmettre sur un tour que Entrer n'a pas rendu.
	ErrTourNul = errors.New("session: tour nul, rien n'est transmis")
	// ErrBackendNul : pas de backend, rien n'est transmis ni consulté.
	ErrBackendNul = errors.New("session: backend nul, rien n'est transmis")
	// ErrBackendPanique : le backend a paniqué ; la consultation est en
	// erreur, l'appel a pu partir.
	ErrBackendPanique = errors.New("session: le backend a paniqué")
	// ErrGestionnaireNul : Entrer sur un gestionnaire non construit.
	ErrGestionnaireNul = errors.New("session: gestionnaire nul")
	// ErrIdentifiant : l'identifiant de session n'a pas pu être généré ;
	// la session n'est pas ouverte.
	ErrIdentifiant = errors.New("session: identifiant de session impossible à générer")
)

// Motifs de fermeture, repris dans le message de session_fermee.
const (
	MotifClient          = "session terminée par le client"
	MotifConsultationMax = "nombre maximal de consultations atteint"
	MotifAgeMax          = "âge maximal de session atteint"
)

// Config borne les sessions. Une borne nulle est désactivée, sauf le délai
// du backend qui a une valeur par défaut : un verrou tenu sans limite
// bloquerait la session entière derrière une source muette.
type Config struct {
	// DelaiBackend est la durée maximale d'un appel au backend pendant que
	// le verrou est tenu. À l'expiration, la consultation est indisponible
	// et le verrou est rendu, que le backend ait répondu ou non.
	DelaiBackend time.Duration
	// MaxConsultations ferme la session après ce nombre de consultations.
	MaxConsultations int
	// AgeMax ferme la session passé cet âge.
	AgeMax time.Duration
	// Horloge et NouvelID sont remplaçables pour les tests.
	Horloge  func() time.Time
	NouvelID func() (string, error)
	// AFermeture est appelée après une fermeture forcée, hors de tout
	// verrou ; l'engine y ferme la connexion (ss.Close).
	AFermeture func(*mcp.ServerSession, string)
	// AOuverture, si posée, complète l'entrée session_ouverte avant son
	// écriture (demandeur, source d'identité) : TS, Session et Appelant y
	// sont déjà. Elle rend un motif non vide pour refuser l'ouverture : la
	// session est alors ouverte puis fermée par le premier tour, avec ce
	// motif, et aucun appel n'y passe.
	AOuverture func(en *journal.Entree) (refus string)
}

// DelaiBackendDefaut s'applique quand Config.DelaiBackend est nul.
const DelaiBackendDefaut = 30 * time.Second

// Retour est ce que le backend rend, classé par l'engine. Reponse est
// opaque pour ce paquet.
type Retour struct {
	Resultat         directive.Resultat
	EmpreinteReponse string
	Reponse          any
	Err              error
}

// Backend transmet l'appel. Il devrait respecter ctx ; s'il ne le fait
// pas, son résultat tardif est simplement ignoré.
type Backend func(ctx context.Context) Retour

// Gestionnaire tient les sessions ouvertes.
type Gestionnaire struct {
	cfg     Config
	journal *journal.Ecrivain

	mu       sync.Mutex
	sessions map[*mcp.ServerSession]*Session
}

// Nouveau crée un gestionnaire. Un journal nul n'écrit rien.
func Nouveau(cfg Config, j *journal.Ecrivain) *Gestionnaire {
	if cfg.DelaiBackend <= 0 {
		cfg.DelaiBackend = DelaiBackendDefaut
	}
	if cfg.Horloge == nil {
		cfg.Horloge = time.Now
	}
	if cfg.NouvelID == nil {
		cfg.NouvelID = nouvelID
	}
	return &Gestionnaire{cfg: cfg, journal: j, sessions: map[*mcp.ServerSession]*Session{}}
}

func nouvelID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// Session est l'état d'une session. Ses champs exportés sont fixés à la
// création.
type Session struct {
	ID       string
	Appelant string
	Ouverte  time.Time

	ss    *mcp.ServerSession
	tour  chan struct{} // verrou des appels, annulable
	mu    sync.Mutex    // protège ce qui suit
	seq   int
	cons  []directive.Consultation
	close string // motif de fermeture différée, appliqué par Terminer
	ferme bool
}

// Entrer obtient ou crée la session de ss, vérifie l'appelant, attend le
// verrou de session et rend le tour. L'appelant d'une session existante
// doit être celui de sa création ; sinon l'appel est refusé et journalisé
// en identite_changee, et la session reste inchangée.
func (g *Gestionnaire) Entrer(ctx context.Context, ss *mcp.ServerSession, appelant string) (*Tour, error) {
	if g == nil {
		return nil, ErrGestionnaireNul
	}
	if ss == nil {
		return nil, ErrSessionNulle
	}
	s, err := g.obtenir(ss, appelant)
	if err != nil {
		return nil, err
	}
	select {
	case s.tour <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	s.mu.Lock()
	if !s.ferme && g.cfg.AgeMax > 0 && g.cfg.Horloge().Sub(s.Ouverte) >= g.cfg.AgeMax {
		s.close = MotifAgeMax
	}
	s.mu.Unlock()
	if motif := g.appliquerFermeture(s); s.estFermee() {
		<-s.tour
		g.notifier(s, motif)
		return nil, ErrFermee
	}
	s.mu.Lock()
	s.seq++
	t := &Tour{g: g, s: s, seq: s.seq}
	s.mu.Unlock()
	return t, nil
}

func (g *Gestionnaire) obtenir(ss *mcp.ServerSession, appelant string) (*Session, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	s, ok := g.sessions[ss]
	if !ok {
		id, err := g.cfg.NouvelID()
		if err != nil || id == "" {
			return nil, fmt.Errorf("%w: %v", ErrIdentifiant, err)
		}
		s = &Session{
			ID: id, Appelant: appelant, Ouverte: g.cfg.Horloge().UTC(),
			ss: ss, tour: make(chan struct{}, 1),
		}
		// Sous g.mu : aucune entrée de la session ne peut précéder celle-ci.
		en := journal.Entree{TS: s.Ouverte, Type: journal.SessionOuverte, Session: s.ID, Appelant: s.Appelant}
		if g.cfg.AOuverture != nil {
			s.close = g.cfg.AOuverture(&en)
			en.TS, en.Type, en.Session, en.Appelant = s.Ouverte, journal.SessionOuverte, s.ID, s.Appelant
		}
		if _, err := g.journal.Ecrire(en); err != nil {
			return nil, err
		}
		g.sessions[ss] = s
		return s, nil
	}
	if s.estFermee() {
		return nil, ErrFermee
	}
	if appelant != s.Appelant {
		if _, err := g.journal.Ecrire(journal.Entree{
			Type: journal.IdentiteChangee, Session: s.ID, Appelant: appelant,
			Message: fmt.Sprintf("appelant fixé à la création : %q ; appel refusé", s.Appelant),
		}); err != nil {
			return nil, err
		}
		return nil, ErrIdentiteChangee
	}
	return s, nil
}

// Session rend la session de ss, ou nil.
func (g *Gestionnaire) Session(ss *mcp.ServerSession) *Session {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.sessions[ss]
}

// Fermer ferme la session de ss (départ du client) et l'oublie. Elle attend
// le tour en cours, borné par le délai du backend.
func (g *Gestionnaire) Fermer(ss *mcp.ServerSession, motif string) {
	g.mu.Lock()
	s := g.sessions[ss]
	delete(g.sessions, ss)
	g.mu.Unlock()
	if s == nil {
		return
	}
	s.tour <- struct{}{}
	s.mu.Lock()
	if s.close == "" {
		s.close = motif
	}
	s.mu.Unlock()
	g.appliquerFermeture(s)
	<-s.tour
}

// Suivre ferme la session quand le client s'en va.
func (g *Gestionnaire) Suivre(ss *mcp.ServerSession) {
	if ss == nil {
		return
	}
	go func() {
		ss.Wait()
		g.Fermer(ss, MotifClient)
	}()
}

// Balayer ferme les sessions qui ont dépassé l'âge maximal. Une session
// dont un tour est en cours est marquée et fermée par ce tour à sa fin :
// la fermeture se fait toujours verrou tenu, pour que session_fermee
// compte toutes les consultations. Rend le nombre de sessions touchées.
func (g *Gestionnaire) Balayer() int {
	if g.cfg.AgeMax <= 0 {
		return 0
	}
	g.mu.Lock()
	var cibles []*Session
	for _, s := range g.sessions {
		cibles = append(cibles, s)
	}
	g.mu.Unlock()
	n := 0
	maintenant := g.cfg.Horloge()
	for _, s := range cibles {
		if s.estFermee() || maintenant.Sub(s.Ouverte) < g.cfg.AgeMax {
			continue
		}
		n++
		s.mu.Lock()
		if s.close == "" {
			s.close = MotifAgeMax
		}
		s.mu.Unlock()
		select {
		case s.tour <- struct{}{}:
			motif := g.appliquerFermeture(s)
			<-s.tour
			g.notifier(s, motif)
		default: // un tour est en cours ; il fermera en terminant
		}
	}
	return n
}

// appliquerFermeture ferme la session si un motif est posé. Verrou de tour
// tenu. Rend le motif appliqué, "" si rien n'a été fait maintenant.
func (g *Gestionnaire) appliquerFermeture(s *Session) string {
	s.mu.Lock()
	if s.ferme || s.close == "" {
		s.mu.Unlock()
		return ""
	}
	s.ferme = true
	motif, nb, dernier := s.close, len(s.cons), s.seq
	s.mu.Unlock()
	g.journal.Ecrire(journal.Entree{
		Type: journal.SessionFermee, Session: s.ID, Appelant: s.Appelant,
		NbConsultations: nb, DernierSeq: dernier, Message: motif,
	})
	return motif
}

// notifier prévient l'engine d'une fermeture forcée, hors verrou.
func (g *Gestionnaire) notifier(s *Session, motif string) {
	if motif == "" || motif == MotifClient {
		return
	}
	if g.cfg.AFermeture != nil {
		g.cfg.AFermeture(s.ss, motif)
	}
}

func (s *Session) estFermee() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ferme
}

// Consultations rend une copie des consultations de la session.
func (s *Session) Consultations() []directive.Consultation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]directive.Consultation(nil), s.cons...)
}

// Tour est un appel en cours, verrou de session tenu jusqu'à Terminer.
type Tour struct {
	g   *Gestionnaire
	s   *Session
	seq int

	transmis bool
	fin      sync.Once
}

// Seq est le rang de l'appel dans la session, refus compris.
func (t *Tour) Seq() int {
	if t == nil {
		return 0
	}
	return t.seq
}

// SessionID est l'identifiant généré par l'engine.
func (t *Tour) SessionID() string {
	if t == nil || t.s == nil {
		return ""
	}
	return t.s.ID
}

// Etat est l'instantané que Decide doit recevoir.
func (t *Tour) Etat() directive.EtatSession {
	if t == nil || t.s == nil {
		return directive.EtatSession{}
	}
	return directive.EtatSession{ID: t.s.ID, Appelant: t.s.Appelant, Consultations: t.s.Consultations()}
}

// Transmettre appelle le backend sous délai et enregistre la consultation.
// À l'expiration, la consultation est indisponible et Transmettre rend la
// main sans attendre le backend. Une seule transmission par tour : la
// seconde n'appelle rien, n'enregistre rien et rend ErrDejaTransmis.
func (t *Tour) Transmettre(ctx context.Context, serveur, outil, empreinte string, backend Backend) (directive.Consultation, Retour) {
	if t == nil || t.s == nil {
		return directive.Consultation{}, Retour{Err: ErrTourNul}
	}
	if t.transmis {
		return directive.Consultation{}, Retour{Err: ErrDejaTransmis}
	}
	if backend == nil {
		return directive.Consultation{}, Retour{Err: ErrBackendNul}
	}
	t.transmis = true
	c := directive.Consultation{
		Seq: t.seq, Serveur: serveur, Outil: outil, Empreinte: empreinte,
		Horodatage: t.g.cfg.Horloge().UTC(),
	}
	ctxB, annuler := context.WithTimeout(ctx, t.g.cfg.DelaiBackend)
	defer annuler()
	fait := make(chan Retour, 1) // tamponné : un backend tardif ne bloque pas
	go func() {
		// Une panique du backend tuerait le processus depuis cette
		// goroutine : elle devient une consultation en erreur.
		defer func() {
			if p := recover(); p != nil {
				fait <- Retour{Resultat: directive.ResultatErreur, Err: fmt.Errorf("%w: %v", ErrBackendPanique, p)}
			}
		}()
		fait <- backend(ctxB)
	}()
	var r Retour
	select {
	case r = <-fait:
		if r.Resultat == "" {
			r.Resultat = directive.ResultatErreur
		}
	case <-ctxB.Done():
		r = Retour{Resultat: directive.ResultatIndisponible, Err: fmt.Errorf("session: backend sans réponse: %w", ctxB.Err())}
	}
	c.Resultat, c.EmpreinteReponse = r.Resultat, r.EmpreinteReponse

	s := t.s
	s.mu.Lock()
	s.cons = append(s.cons, c)
	if max := t.g.cfg.MaxConsultations; max > 0 && len(s.cons) >= max && s.close == "" {
		s.close = MotifConsultationMax
	}
	s.mu.Unlock()
	return c, r
}

// Terminer applique une fermeture en attente et rend le verrou.
// Idempotent, et sans effet sur un tour nul : on peut le différer avant
// même d'avoir testé l'erreur d'Entrer.
func (t *Tour) Terminer() {
	if t == nil || t.s == nil {
		return
	}
	t.fin.Do(func() {
		motif := t.g.appliquerFermeture(t.s)
		<-t.s.tour
		t.g.notifier(t.s, motif)
	})
}
