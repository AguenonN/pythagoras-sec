package console

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"mcprogue/internal/directive"
	"mcprogue/internal/journal"
)

// ---------------------------------------------------------------------------
// Écran 5 : file du superviseur
// ---------------------------------------------------------------------------

// reEmpreinte : l'empreinte d'un appel nomme un dossier. Elle vient de
// etat.json, jamais du navigateur, et passe ce contrôle avant tout usage.
var reEmpreinte = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9:_-]{7,199}$`)

// Rejet est le refus consigné d'un superviseur : approbations/<empreinte>/
// rejet.json, que l'orchestrateur lit comme une fin « bloque ».
type Rejet struct {
	Valideur string    `json:"valideur"`
	Nom      string    `json:"nom,omitempty"`
	Motif    string    `json:"motif"`
	Date     time.Time `json:"date"`
}

// Controle est l'un des quatre contrôles que l'engine fait avant la
// transmission. Son état vient de l'entrée approbation du journal, jamais
// d'une présomption : « non établi » tant que l'engine n'a pas repris l'appel.
type Controle struct {
	Etat   string `json:"etat"` // ok | echec | non_etabli | non_requis
	Detail string `json:"detail"`
}

// Controles sont les quatre contrôles de l'engine, d'après la dernière
// entrée approbation de l'appel.
type Controles struct {
	Etabli         bool               `json:"etabli"`
	Statut         string             `json:"statut,omitempty"` // acceptee | refusee
	Motif          string             `json:"motif,omitempty"`
	TS             *time.Time         `json:"ts,omitempty"`
	Signature      Controle           `json:"signature"`
	RoleADate      Controle           `json:"role_a_date"`
	QuatreYeux     Controle           `json:"quatre_yeux"`
	UnJetonParRole Controle           `json:"un_jeton_par_role"`
	Valideurs      []journal.Valideur `json:"valideurs,omitempty"`
}

// AttenteVue est un appel retenu, à valider par un humain, ou repris.
type AttenteVue struct {
	Journal    string              `json:"journal"`
	Session    string              `json:"session"`
	Seq        int                 `json:"seq"`
	TS         time.Time           `json:"ts"`
	Serveur    string              `json:"serveur"`
	Outil      string              `json:"outil"`
	Directive  string              `json:"directive"`
	Version    int                 `json:"version"`
	Fondement  directive.Fondement `json:"fondement"`
	Motif      string              `json:"motif"`
	Roles      []string            `json:"roles"`
	QuatreYeux bool                `json:"quatre_yeux"`
	Empreinte  string              `json:"empreinte_transmise"`
	Arguments  map[string]any      `json:"arguments,omitempty"`
	Purgee     bool                `json:"purgee,omitempty"`

	SousAgent     string             `json:"sous_agent,omitempty"`
	Demandeur     *journal.Demandeur `json:"demandeur,omitempty"`
	Demande       string             `json:"demande,omitempty"`
	Applications  []AppVue           `json:"applications"`
	Consultations []Lien             `json:"consultations"`

	// Statut de la session d'après etat.json : en_attente (à traiter) ou
	// autre (reprise : les contrôles sont ceux du journal).
	StatutSession string `json:"statut_session"`
	// Compteur : jetons déposés / rôles requis (« 1 / 2 »).
	Deposes    int        `json:"jetons_deposes"`
	Requis     int        `json:"jetons_requis"`
	Valideurs  []Valideur `json:"valideurs_possibles"`
	Rejet      *Rejet     `json:"rejet,omitempty"`
	MotifJeton string     `json:"motif_jeton,omitempty"`
	Controles  Controles  `json:"controles"`
}

// FileVue est l'écran 5.
type FileVue struct {
	Attentes []AttenteVue            `json:"attentes"` // statut en_attente
	Reprises []AttenteVue            `json:"reprises"` // reprises après validation, avec les contrôles de l'engine
	Verifs   map[string]Verification `json:"verifications"`
	Erreur   string                  `json:"erreur_annuaire,omitempty"`
}

// dernierNonVide rend le dernier élément d'une liste ou la valeur zéro.
func dossierApprobations(ch Chemins, empreinte string) string {
	return filepath.Join(ch.Approbations, empreinte)
}

// File construit l'écran 5 : les sessions dont etat.json est en_attente
// (requête exacte lue au coffre, directives, rôles, compteur), et celles
// qui ont été reprises, avec les contrôles tels que l'engine les a
// journalisés.
func (d *Depot) File(etats []*Etat, tous []JeuCharge, maintenant time.Time) FileVue {
	out := FileVue{Attentes: []AttenteVue{}, Reprises: []AttenteVue{}, Verifs: map[string]Verification{}}
	tous = d.AvecTravail(tous, etats)
	if _, _, err := d.Annuaire(); err != nil {
		out.Erreur = err.Error()
	}
	for _, st := range etats {
		out.Verifs[st.Journal.ID] = st.Verif
		if st.Dossier == nil {
			continue
		}
		if st.Dossier.Statut == StatutEnAttente && st.Dossier.Attente != nil {
			if a, ok := d.attenteDe(st, tous, maintenant); ok {
				out.Attentes = append(out.Attentes, a)
			}
		}
		for _, a := range d.reprisesDe(st, tous, maintenant) {
			out.Reprises = append(out.Reprises, a)
		}
	}
	sort.SliceStable(out.Attentes, func(i, j int) bool { return out.Attentes[i].TS.After(out.Attentes[j].TS) })
	sort.SliceStable(out.Reprises, func(i, j int) bool { return out.Reprises[i].TS.After(out.Reprises[j].TS) })
	return out
}

// appelEnAttente retrouve l'entrée appel dont la décision met l'appel en
// attente avec cette empreinte transmise (la dernière).
func appelEnAttente(st *Etat, empreinte string) *journal.Entree {
	var res *journal.Entree
	for i := range st.Entrees {
		e := &st.Entrees[i]
		if e.Type == journal.AppelT && e.Decision != nil && e.Decision.Effet == string(directive.AttendreValidation) &&
			e.Appel != nil && e.Appel.EmpreinteTransmise == empreinte && e.Approbation == nil {
			res = e
		}
	}
	return res
}

func (d *Depot) attenteDe(st *Etat, tous []JeuCharge, maintenant time.Time) (AttenteVue, bool) {
	at := st.Dossier.Attente
	a := AttenteVue{Journal: st.Journal.ID, Empreinte: at.EmpreinteTransmise, Roles: append([]string{}, at.Roles...), QuatreYeux: at.QuatreYeux,
		Applications: []AppVue{}, Consultations: []Lien{}, StatutSession: st.Dossier.Statut, Demande: st.Dossier.Demande,
		MotifJeton: at.MotifJeton, Valideurs: []Valideur{}, TS: maintenant}
	if st.Dossier.Debut != nil {
		a.TS = *st.Dossier.Debut
	}
	e := appelEnAttente(st, at.EmpreinteTransmise)
	if e != nil {
		d.remplir(&a, st, e, tous)
	}
	if a.Directive == "" && len(at.Directives) > 0 {
		a.Directive = at.Directives[0]
	}
	if len(a.Roles) == 0 && e != nil && e.Decision.Validation != nil {
		a.Roles, a.QuatreYeux = e.Decision.Validation.Roles, e.Decision.Validation.QuatreYeux
	}
	a.Requis = len(a.Roles)
	if a.Requis == 0 {
		a.Requis = 1
	}
	if reEmpreinte.MatchString(a.Empreinte) {
		a.Deposes = len(jetonsDeposes(d.Chemins(), a.Empreinte))
		a.Rejet = d.LireRejet(a.Empreinte)
	}
	if v, err := d.Valideurs(a.Roles, maintenant); err == nil {
		a.Valideurs = v
	}
	a.Controles = controlesDe(st, a.Empreinte)
	return a, true
}

// reprisesDe liste les appels de la session qui ont été repris : il existe
// une entrée approbation pour leur empreinte (acceptée ou refusée).
func (d *Depot) reprisesDe(st *Etat, tous []JeuCharge, maintenant time.Time) []AttenteVue {
	var out []AttenteVue
	vus := map[string]bool{}
	for i := len(st.Entrees) - 1; i >= 0; i-- {
		en := st.Entrees[i]
		if en.Type != journal.ApprobationT || en.Approbation == nil || vus[en.Approbation.EmpreinteTransmise] {
			continue
		}
		emp := en.Approbation.EmpreinteTransmise
		if st.Dossier != nil && st.Dossier.Statut == StatutEnAttente && st.Dossier.Attente != nil && st.Dossier.Attente.EmpreinteTransmise == emp {
			continue // encore à traiter : dans la file
		}
		vus[emp] = true
		a := AttenteVue{Journal: st.Journal.ID, Session: en.Session, Seq: en.Seq, TS: en.TS, Serveur: en.Serveur, Outil: en.Outil,
			Empreinte: emp, Roles: append([]string{}, en.Approbation.Roles...), QuatreYeux: en.Approbation.QuatreYeux,
			Applications: []AppVue{}, Consultations: []Lien{}, Valideurs: []Valideur{}}
		if st.Dossier != nil {
			a.StatutSession, a.Demande = st.Dossier.Statut, st.Dossier.Demande
		}
		if e := appelEnAttente(st, emp); e != nil {
			d.remplir(&a, st, e, tous)
		}
		a.Requis = len(a.Roles)
		a.Controles = controlesDe(st, emp)
		out = append(out, a)
	}
	return out
}

// remplir complète une attente depuis l'entrée appel du journal.
func (d *Depot) remplir(a *AttenteVue, st *Etat, e *journal.Entree, tous []JeuCharge) {
	a.Session, a.Seq, a.TS, a.Serveur, a.Outil, a.Motif = e.Session, e.Seq, e.TS, e.Serveur, e.Outil, e.Decision.Motif
	a.SousAgent, a.Demandeur = e.SousAgentDeclare, e.Demandeur
	jeu, _ := trouverJeu(tous, e.Jeu)
	for _, k := range e.EtatSession {
		a.Consultations = append(a.Consultations, Lien{Seq: k.Seq, Serveur: k.Serveur, Outil: k.Outil, Resultat: k.Resultat})
	}
	for _, ap := range e.Decision.Applications {
		if !ap.Appliquee {
			continue
		}
		av := AppVue{ID: ap.ID, Version: ap.Version, Fondement: ap.Fondement, Effet: ap.Effet, Declench: ap.Declenchee,
			Appliquee: ap.Appliquee, Resultat: ap.Resultat, Motif: ap.Motif}
		if len(ap.Fondements) > 0 {
			av.Fondement = ap.Fondements[0]
		}
		if dd := directiveDu(jeu, ap.ID, ap.Version); dd != nil {
			av.Texte, av.Titre = dd.TexteFR, Titre(dd.TexteFR)
		}
		a.Applications = append(a.Applications, av)
		if a.Directive == "" && ap.Effet == string(directive.AttendreValidation) {
			a.Directive, a.Version, a.Fondement = ap.ID, ap.Version, av.Fondement
		}
	}
	if v := e.Decision.Validation; v != nil && len(a.Roles) == 0 {
		a.Roles, a.QuatreYeux = v.Roles, v.QuatreYeux
	}
	if e.Appel != nil && st.Coffre != nil {
		if args, ok := st.Coffre.transmis[e.Appel.EmpreinteOrigine]; ok {
			a.Arguments = args
		} else if args, ok := st.Coffre.index[e.Appel.EmpreinteOrigine]; ok {
			a.Arguments = args
		} else if _, ok := st.Coffre.purgees[e.Appel.EmpreinteOrigine]; ok {
			a.Purgee = true
		}
	}
}

// jetonsDeposes liste les valideurs qui ont déposé un jeton pour cette
// empreinte (un fichier <valideur>.jwt chacun).
func jetonsDeposes(ch Chemins, empreinte string) []string {
	ents, _ := os.ReadDir(dossierApprobations(ch, empreinte))
	var out []string
	for _, e := range ents {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jwt") && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, strings.TrimSuffix(e.Name(), ".jwt"))
		}
	}
	sort.Strings(out)
	return out
}

// LireRejet lit approbations/<empreinte>/rejet.json.
func (d *Depot) LireRejet(empreinte string) *Rejet {
	if !reEmpreinte.MatchString(empreinte) {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(dossierApprobations(d.Chemins(), empreinte), "rejet.json"))
	if err != nil {
		return nil
	}
	var r Rejet
	if json.Unmarshal(b, &r) != nil {
		return nil
	}
	return &r
}

// controlesDe lit les quatre contrôles dans la dernière entrée approbation
// du journal pour cette empreinte. Sans entrée, rien n'est établi.
func controlesDe(st *Etat, empreinte string) Controles {
	ne := Controle{Etat: "non_etabli", Detail: "pas encore vérifié par l'engine : l'appel n'a pas été repris"}
	c := Controles{Signature: ne, RoleADate: ne, QuatreYeux: ne, UnJetonParRole: ne}
	var en *journal.Entree
	for i := range st.Entrees {
		e := &st.Entrees[i]
		if e.Type == journal.ApprobationT && e.Approbation != nil && e.Approbation.EmpreinteTransmise == empreinte {
			en = e
		}
	}
	if en == nil {
		return c
	}
	a := en.Approbation
	t := en.TS
	c.Etabli, c.Statut, c.Motif, c.TS, c.Valideurs = true, a.Statut, a.Motif, &t, a.Valideurs
	if a.Statut == journal.ApprobationAcceptee {
		qui := []string{}
		for _, v := range a.Valideurs {
			qui = append(qui, v.Identite+" ("+v.Role+")")
		}
		c.Signature = Controle{"ok", "jeton signé par " + strings.Join(qui, ", ") + ", clé de l'annuaire"}
		roles := []string{}
		for _, v := range a.Valideurs {
			roles = append(roles, v.Identite+" : « "+v.Role+" » au "+v.Date.In(Paris()).Format("02/01/2006 à 15 h 04"))
		}
		c.RoleADate = Controle{"ok", "rôle détenu à la date de validation — " + strings.Join(roles, " ; ")}
		if a.QuatreYeux {
			c.QuatreYeux = Controle{"ok", "valideurs distincts, et différents du demandeur"}
		} else {
			c.QuatreYeux = Controle{"non_requis", "non demandé par la directive"}
		}
		n := len(a.Roles)
		if n == 0 {
			n = len(a.Valideurs)
		}
		c.UnJetonParRole = Controle{"ok", plurielN(len(a.Valideurs), "jeton accepté", "jetons acceptés") + " pour " + plurielN(n, "rôle requis", "rôles requis")}
		return c
	}
	// Refusée : le motif de l'engine dit quel contrôle a échoué ; les
	// autres restent non établis.
	m := strings.ToLower(a.Motif)
	inc := Controle{"non_etabli", "non établi : un contrôle précédent a échoué"}
	c.Signature, c.RoleADate, c.QuatreYeux, c.UnJetonParRole = inc, inc, inc, inc
	echec := Controle{"echec", a.Motif}
	switch {
	case strings.Contains(m, "rôle") && strings.Contains(m, "jeton") && strings.Contains(m, "refusé"):
		c.RoleADate = echec
	case strings.Contains(m, "demandeur") || strings.Contains(m, "déjà validé") || strings.Contains(m, "quatre yeux"):
		c.QuatreYeux = echec
	case strings.Contains(m, "rôle(s) sans validation") || strings.Contains(m, "un jeton par rôle") || strings.Contains(m, "un seul jeton"):
		c.UnJetonParRole = echec
	default:
		c.Signature = echec
	}
	return c
}

func plurielN(n int, un, plusieurs string) string {
	if n > 1 {
		return itoa(n) + " " + plusieurs
	}
	return itoa(n) + " " + un
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
