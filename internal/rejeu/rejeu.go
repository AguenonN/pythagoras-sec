// Package rejeu rejoue les décisions du journal. Une entrée appel porte
// tout ce que Decide a lu, sauf les arguments, qui sont au coffre sous leur
// empreinte d'origine : le jeu (id, version, empreinte), l'attestation,
// l'instantané de session, l'appelant, la date. Rejouer remet ces pièces
// ensemble et vérifie qu'elles redonnent ce que le journal affirme.
//
// Deux contrôles internes, toujours faits dans le cadre historique (jeu
// d'origine, date de l'entrée) :
//
//   - les arguments du coffre doivent retrouver empreinte_origine ;
//   - la décision rejouée doit retrouver empreinte_transmise, et l'effet
//     journalisé.
//
// Tout écart est nommé : purge (le coffre a une pierre tombale : l'appel
// n'est plus rejouable, son entrée reste vérifiable dans la chaîne) ou
// altération (tout le reste : arguments modifiés ou disparus sans trace,
// jeu différent, décision qui ne se retrouve pas). Chaque écart porte aussi
// sa nature : violation quand le rejeu se fait à la date historique sous le
// jeu d'origine, exposition quand AuDate force la date ou qu'un autre jeu
// est imposé.
//
// Le rejeu n'est pas en cascade : un appel est rejoué avec l'état de
// session qu'il a réellement vu, même si, sous un autre jeu, un appel
// antérieur aurait été refusé.
//
// Le journal lu doit avoir été vérifié (journal.Verifier) : le rejeu ne
// refait pas la chaîne.
package rejeu

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"time"

	"mcprogue/internal/approval"
	"mcprogue/internal/directive"
	"mcprogue/internal/journal"
)

// Cause d'un écart.
type Cause string

const (
	CausePurge      Cause = "purge"
	CauseAlteration Cause = "alteration"
	// CauseDivergence : hors cadre historique, la décision rejouée diffère
	// de la décision journalisée. C'est ce que la vue après montre.
	CauseDivergence Cause = "divergence"
)

// Nature d'un écart : le cadre dans lequel il a été constaté.
type Nature string

const (
	Violation  Nature = "violation"
	Exposition Nature = "exposition"
)

// Contrôles, nommés dans chaque écart.
const (
	ControleJeu       = "jeu"
	ControleArguments = "arguments"
	ControleTransmis  = "transmis"
	ControleDecision  = "decision"
	ControleEtat      = "etat"
)

// NonRejouable remplace l'effet dans les vues quand les arguments manquent.
const NonRejouable = "non_rejouable"

// Ecart est un désaccord entre le journal et le rejeu.
type Ecart struct {
	Cause    Cause  `json:"cause"`
	Nature   Nature `json:"nature"`
	Controle string `json:"controle"`
	Detail   string `json:"detail"`
}

// Resolveur rend les arguments d'origine d'une empreinte et dit si elle a
// été purgée. *journal.Coffre l'implémente.
type Resolveur interface {
	Arguments(empreinte string) (map[string]any, bool)
	Purgee(empreinte string) (time.Time, bool)
}

// Jeux retrouve un jeu par identifiant et version.
type Jeux interface {
	Jeu(id string, version int) (directive.Jeu, bool)
}

// Catalogue est une liste de jeux chargés.
type Catalogue []directive.Jeu

func (c Catalogue) Jeu(id string, version int) (directive.Jeu, bool) {
	for _, j := range c {
		if j.Meta.ID == id && j.Meta.Version == version {
			return j, true
		}
	}
	return directive.Jeu{}, false
}

// Options fixent le cadre du rejeu. Zéro : le cadre historique.
type Options struct {
	// Jeu remplace le jeu d'origine.
	Jeu *directive.Jeu
	// AuDate force la date de l'appel.
	AuDate time.Time
}

// Historique dit si le cadre est celui de l'entrée.
func (o Options) Historique() bool { return o.Jeu == nil && o.AuDate.IsZero() }

func (o Options) nature() Nature {
	if o.Historique() {
		return Violation
	}
	return Exposition
}

// Rejeu est le résultat du rejeu d'une entrée appel.
type Rejeu struct {
	Session string    `json:"session"`
	Seq     int       `json:"seq"`
	Serveur string    `json:"serveur"`
	Outil   string    `json:"outil"`
	TS      time.Time `json:"ts"`

	// Journalise est l'effet que le journal affirme, établi par la chaîne
	// même quand l'appel n'est plus rejouable.
	Journalise directive.Effet `json:"journalise"`
	// Origine est la décision rejouée sous le jeu d'origine à la date
	// historique ; Rejouee celle du cadre demandé (la même en cadre
	// historique). Nulles si l'appel n'est pas rejouable.
	Origine *directive.Decision `json:"-"`
	Rejouee *directive.Decision `json:"-"`

	Rejouable bool    `json:"rejouable"`
	Nature    Nature  `json:"nature"`
	Ecarts    []Ecart `json:"ecarts,omitempty"`
}

// Effet rend l'effet du rejeu dans son cadre, ou non_rejouable.
func (r Rejeu) Effet() string {
	if r.Rejouee == nil {
		return NonRejouable
	}
	return string(r.Rejouee.Effet)
}

// ErrPasUnAppel : l'entrée n'est pas une entrée appel complète.
var ErrPasUnAppel = errors.New("rejeu: l'entrée n'est pas un appel rejouable")

// Rejouer rejoue une entrée appel. Une erreur signale une entrée
// structurellement inutilisable ou un jeu introuvable ; les désaccords
// sont des écarts, pas des erreurs.
func Rejouer(en journal.Entree, res Resolveur, jeux Jeux, opts Options) (Rejeu, error) {
	if en.Type != journal.AppelT || en.Appel == nil || en.Decision == nil || en.Jeu == nil {
		return Rejeu{}, ErrPasUnAppel
	}
	nature := opts.nature()
	r := Rejeu{
		Session: en.Session, Seq: en.Seq, Serveur: en.Serveur, Outil: en.Outil, TS: en.TS,
		Journalise: directive.Effet(en.Decision.Effet), Nature: nature,
	}
	ecart := func(c Cause, controle, format string, args ...any) {
		r.Ecarts = append(r.Ecarts, Ecart{Cause: c, Nature: nature, Controle: controle, Detail: fmt.Sprintf(format, args...)})
	}

	origine, ok := jeux.Jeu(en.Jeu.ID, en.Jeu.Version)
	if !ok {
		return r, fmt.Errorf("rejeu: jeu %s v%d introuvable", en.Jeu.ID, en.Jeu.Version)
	}
	if origine.Meta.Empreinte != en.Jeu.Empreinte {
		ecart(CauseAlteration, ControleJeu, "le jeu %s v%d fourni a l'empreinte %s, le journal cite %s",
			en.Jeu.ID, en.Jeu.Version, origine.Meta.Empreinte, en.Jeu.Empreinte)
		return r, nil
	}

	// Contrôle 1 : les arguments du coffre retrouvent empreinte_origine.
	args, ok := res.Arguments(en.Appel.EmpreinteOrigine)
	if !ok {
		if le, purge := res.Purgee(en.Appel.EmpreinteOrigine); purge {
			ecart(CausePurge, ControleArguments, "arguments purgés du coffre le %s ; l'entrée reste vérifiable dans la chaîne",
				le.UTC().Format(time.RFC3339))
		} else {
			ecart(CauseAlteration, ControleArguments, "arguments absents du coffre sans trace de purge")
		}
		return r, nil
	}
	if got := approval.Digest(en.Outil, args); got != en.Appel.EmpreinteOrigine {
		ecart(CauseAlteration, ControleArguments, "les arguments du coffre hachent vers %s, le journal cite %s",
			got, en.Appel.EmpreinteOrigine)
		return r, nil
	}
	r.Rejouable = true

	etat := directive.EtatSession{ID: en.Session, Appelant: en.Appelant, Consultations: journal.VersConsultations(en.EtatSession)}
	appel := directive.Appel{
		Serveur: en.Serveur, Outil: en.Outil, Arguments: args,
		Attestation: en.Attestation.VersDirective(), Horodatage: en.TS,
	}

	// Contrôle 2 : sous le jeu d'origine, à la date historique, la décision
	// retrouve empreinte_transmise et l'effet journalisé.
	dec := directive.Decide(origine, etat, appel)
	r.Origine = &dec
	if dec.EmpreinteTransmise != en.Appel.EmpreinteTransmise {
		ecart(CauseAlteration, ControleTransmis, "la décision rejouée transmet %s, le journal cite %s",
			dec.EmpreinteTransmise, en.Appel.EmpreinteTransmise)
	}
	if dec.Effet != r.Journalise {
		ecart(CauseAlteration, ControleDecision, "la décision rejouée est %s, le journal affirme %s",
			dec.Effet, r.Journalise)
	}

	if opts.Historique() {
		r.Rejouee = r.Origine
		return r, nil
	}

	jeu := origine
	if opts.Jeu != nil {
		jeu = *opts.Jeu
	}
	if !opts.AuDate.IsZero() {
		appel.Horodatage = opts.AuDate
	}
	autre := directive.Decide(jeu, etat, appel)
	r.Rejouee = &autre
	if autre.Effet != r.Journalise {
		ecart(CauseDivergence, ControleDecision, "sous %s v%d au %s, la décision serait %s ; le journal affirme %s",
			jeu.Meta.ID, jeu.Meta.Version, appel.Horodatage.UTC().Format(time.RFC3339), autre.Effet, r.Journalise)
	}
	return r, nil
}

// consultation rend la consultation qu'une entrée appel a produite : un
// appel transmis, quel que soit son résultat. Un refus de l'engine
// (non_transmis) n'en est pas une.
func consultation(en journal.Entree) (journal.Consultation, bool) {
	if en.Type != journal.AppelT || en.Resultat == nil || en.Appel == nil {
		return journal.Consultation{}, false
	}
	switch en.Resultat.Statut {
	case journal.StatutOK, journal.StatutErreur, journal.StatutIndisponible, journal.StatutRefuse:
	default:
		return journal.Consultation{}, false
	}
	return journal.Consultation{
		Seq: en.Seq, Serveur: en.Serveur, Outil: en.Outil, TS: en.TS, Resultat: en.Resultat.Statut,
		Empreinte: en.Appel.EmpreinteTransmise, EmpreinteReponse: en.Resultat.EmpreinteReponse,
	}, true
}

// ReconstituerEtat recompose, à partir du journal seul, l'état de la
// session tel qu'il était avant l'appel de rang avantSeq : l'appelant fixé
// à l'ouverture et les consultations des appels transmis. L'horodatage
// d'une consultation est celui de l'entrée, pas celui de la transmission.
func ReconstituerEtat(entrees []journal.Entree, session string, avantSeq int) (directive.EtatSession, error) {
	etat := directive.EtatSession{ID: session}
	ouverte := false
	var cs []journal.Consultation
	for _, en := range entrees {
		if en.Session != session {
			continue
		}
		switch en.Type {
		case journal.SessionOuverte:
			ouverte = true
			etat.Appelant = en.Appelant
		case journal.AppelT:
			if en.Seq >= avantSeq {
				continue
			}
			if c, ok := consultation(en); ok {
				cs = append(cs, c)
			}
		}
	}
	if !ouverte {
		return etat, fmt.Errorf("rejeu: session %s sans entrée session_ouverte", session)
	}
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].Seq < cs[j].Seq })
	etat.Consultations = journal.VersConsultations(cs)
	return etat, nil
}

// RejouerJournal rejoue chaque entrée appel et confronte, en plus des deux
// contrôles, l'instantané de session enregistré à l'état reconstitué à
// partir des entrées précédentes de la même session.
func RejouerJournal(entrees []journal.Entree, res Resolveur, jeux Jeux, opts Options) ([]Rejeu, error) {
	var out []Rejeu
	for _, en := range entrees {
		if en.Type != journal.AppelT {
			continue
		}
		r, err := Rejouer(en, res, jeux, opts)
		if err != nil {
			return out, fmt.Errorf("session %s seq %d: %w", en.Session, en.Seq, err)
		}
		etat, err := ReconstituerEtat(entrees, en.Session, en.Seq)
		if err != nil {
			r.Ecarts = append(r.Ecarts, Ecart{Cause: CauseAlteration, Nature: r.Nature, Controle: ControleEtat, Detail: err.Error()})
		} else if !memesConsultations(etat.Consultations, journal.VersConsultations(en.EtatSession)) {
			r.Ecarts = append(r.Ecarts, Ecart{Cause: CauseAlteration, Nature: r.Nature, Controle: ControleEtat,
				Detail: fmt.Sprintf("l'instantané enregistré porte %d consultation(s), le journal en reconstitue %d",
					len(en.EtatSession), len(etat.Consultations))})
		} else if etat.Appelant != en.Appelant {
			r.Ecarts = append(r.Ecarts, Ecart{Cause: CauseAlteration, Nature: r.Nature, Controle: ControleEtat,
				Detail: fmt.Sprintf("l'appel est attribué à %q, la session a été ouverte par %q", en.Appelant, etat.Appelant)})
		}
		out = append(out, r)
	}
	return out, nil
}

// memesConsultations compare sans l'horodatage, que Decide ne lit pas et
// que l'instantané prend à la transmission, pas à l'écriture.
func memesConsultations(a, b []directive.Consultation) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		x.Horodatage, y.Horodatage = time.Time{}, time.Time{}
		if !reflect.DeepEqual(x, y) {
			return false
		}
	}
	return true
}

// Ligne est un appel dans la vue avant / après.
type Ligne struct {
	Session string  `json:"session"`
	Seq     int     `json:"seq"`
	Serveur string  `json:"serveur"`
	Outil   string  `json:"outil"`
	Avant   string  `json:"avant"`
	Apres   string  `json:"apres"`
	Ecarts  []Ecart `json:"ecarts,omitempty"`
}

// Transition compte les appels passés d'un effet à un autre.
type Transition struct {
	Avant  string `json:"avant"`
	Apres  string `json:"apres"`
	Nombre int    `json:"nombre"`
}

// Comparaison est la vue avant / après.
type Comparaison struct {
	Lignes      []Ligne      `json:"lignes"`
	Transitions []Transition `json:"transitions"`
}

// Comparer rejoue le journal dans deux cadres et produit le tableau des
// transitions d'effet. Dans le cadre historique, l'effet avant est celui
// que le journal affirme : il reste établi par la chaîne même quand les
// arguments ont été purgés. Dans tout autre cadre, un appel sans arguments
// est non_rejouable. Les écarts des deux cadres sont joints à la ligne.
func Comparer(entrees []journal.Entree, res Resolveur, jeux Jeux, avant, apres Options) (Comparaison, error) {
	ra, err := RejouerJournal(entrees, res, jeux, avant)
	if err != nil {
		return Comparaison{}, err
	}
	rb, err := RejouerJournal(entrees, res, jeux, apres)
	if err != nil {
		return Comparaison{}, err
	}
	effet := func(r Rejeu, o Options) string {
		if o.Historique() {
			return string(r.Journalise)
		}
		return r.Effet()
	}
	var cmp Comparaison
	compte := map[[2]string]int{}
	for i := range ra {
		a, b := ra[i], rb[i]
		l := Ligne{Session: a.Session, Seq: a.Seq, Serveur: a.Serveur, Outil: a.Outil,
			Avant: effet(a, avant), Apres: effet(b, apres)}
		l.Ecarts = fusionner(a.Ecarts, b.Ecarts)
		cmp.Lignes = append(cmp.Lignes, l)
		compte[[2]string{l.Avant, l.Apres}]++
	}
	for k, n := range compte {
		cmp.Transitions = append(cmp.Transitions, Transition{Avant: k[0], Apres: k[1], Nombre: n})
	}
	sort.Slice(cmp.Transitions, func(i, j int) bool {
		x, y := cmp.Transitions[i], cmp.Transitions[j]
		if x.Avant != y.Avant {
			return rang(x.Avant) < rang(y.Avant)
		}
		return rang(x.Apres) < rang(y.Apres)
	})
	return cmp, nil
}

// fusionner joint les écarts des deux cadres sans répéter ceux que les
// deux ont constatés dans les mêmes termes (les contrôles internes).
func fusionner(a, b []Ecart) []Ecart {
	out := append([]Ecart(nil), a...)
	vus := map[[3]string]bool{}
	for _, e := range a {
		vus[[3]string{string(e.Cause), e.Controle, e.Detail}] = true
	}
	for _, e := range b {
		if !vus[[3]string{string(e.Cause), e.Controle, e.Detail}] {
			out = append(out, e)
		}
	}
	return out
}

func rang(effet string) int {
	switch directive.Effet(effet) {
	case directive.Autoriser:
		return 0
	case directive.Caviarder:
		return 1
	case directive.AttendreValidation:
		return 2
	case directive.Refuser:
		return 3
	}
	return 4 // non_rejouable en dernier
}
