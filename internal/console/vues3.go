package console

import (
	"encoding/json"
	"strings"
	"time"

	"mcprogue/internal/directive"
	"mcprogue/internal/journal"
	"mcprogue/internal/relecture"
)

// ---------------------------------------------------------------------------
// Écran 5 : file du superviseur (lecture seule cette nuit)
// ---------------------------------------------------------------------------

// Attente est un appel retenu, à valider par un humain.
type Attente struct {
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
	Applications  []AppVue           `json:"applications"`
	Consultations []Lien             `json:"consultations"`
}

// File liste les appels en attente de validation : ceux dont la décision est
// attendre_validation et qui n'ont pas d'approbation journalisée.
func File(nom string, st *Etat) []Attente {
	out := []Attente{}
	for _, e := range st.Entrees {
		if e.Type != journal.AppelT || e.Decision == nil || e.Decision.Effet != string(directive.AttendreValidation) || e.Approbation != nil {
			continue
		}
		a := Attente{Journal: nom, Session: e.Session, Seq: e.Seq, TS: e.TS, Serveur: e.Serveur, Outil: e.Outil, Motif: e.Decision.Motif,
			SousAgent: e.SousAgentDeclare, Demandeur: e.Demandeur, Applications: []AppVue{}, Consultations: []Lien{}}
		for _, k := range e.EtatSession {
			a.Consultations = append(a.Consultations, Lien{Seq: k.Seq, Serveur: k.Serveur, Outil: k.Outil, Resultat: k.Resultat})
		}
		for _, ap := range e.Decision.Applications {
			if ap.Appliquee {
				av := AppVue{ID: ap.ID, Version: ap.Version, Fondement: ap.Fondement, Effet: ap.Effet, Declench: ap.Declenchee,
					Appliquee: ap.Appliquee, Resultat: ap.Resultat, Motif: ap.Motif}
				if len(ap.Fondements) > 0 {
					av.Fondement = ap.Fondements[0]
				}
				a.Applications = append(a.Applications, av)
			}
		}
		for _, ap := range e.Decision.Applications {
			if ap.Appliquee && ap.Effet == string(directive.AttendreValidation) {
				a.Directive, a.Version, a.Fondement = ap.ID, ap.Version, ap.Fondement
				if len(ap.Fondements) > 0 {
					a.Fondement = ap.Fondements[0]
				}
				break
			}
		}
		if v := e.Decision.Validation; v != nil {
			a.Roles, a.QuatreYeux = v.Roles, v.QuatreYeux
		}
		if e.Appel != nil {
			a.Empreinte = e.Appel.EmpreinteTransmise
			if args, ok := st.Coffre.transmis[e.Appel.EmpreinteOrigine]; ok {
				a.Arguments = args
			} else if args, ok := st.Coffre.index[e.Appel.EmpreinteOrigine]; ok {
				a.Arguments = args
			} else if _, ok := st.Coffre.purgees[e.Appel.EmpreinteOrigine]; ok {
				a.Purgee = true
			}
		}
		out = append(out, a)
	}
	return out
}

// ---------------------------------------------------------------------------
// Écran 6 : rédaction
// ---------------------------------------------------------------------------

// DirRelue est une directive acceptée par le chargeur strict, relue.
type DirRelue struct {
	ID        string `json:"id"`
	Version   int    `json:"version"`
	Statut    string `json:"statut"`
	Texte     string `json:"texte"`
	Relecture string `json:"relecture"`
}

// Redaction est le retour de la relecture d'un texte de jeu.
type Redaction struct {
	Valide    bool     `json:"valide"`
	Erreurs   []string `json:"erreurs,omitempty"`
	Relecture string   `json:"relecture,omitempty"`
	Empreinte string   `json:"empreinte,omitempty"`
	// Directives : chacune avec sa relecture en français.
	Directives []DirRelue `json:"directives,omitempty"`
	Questions  []string   `json:"questions"`
}

// Relire passe le texte au chargeur strict, le seul juge, puis à la
// relecture en français. Le compilateur n'est pas relié : un texte libre
// reçoit la question qui le dit, jamais une directive inventée.
func Relire(texte string) Redaction {
	r := Redaction{Questions: []string{}}
	t := strings.TrimSpace(texte)
	if t == "" {
		r.Erreurs = []string{"Le texte est vide."}
		return r
	}
	if !json.Valid([]byte(t)) {
		r.Erreurs = []string{"Ce texte n'est pas un jeu de directives. Le compilateur, qui transforme un texte libre en directive, n'est pas relié à cette console cette nuit."}
		r.Questions = append(r.Questions, "Collez un jeu de directives au format JSON pour en obtenir la relecture.")
		return r
	}
	jeu, err := directive.Charger([]byte(t))
	if err != nil {
		r.Erreurs = depiler(err)
		return r
	}
	r.Valide = true
	r.Empreinte = jeu.Meta.Empreinte
	r.Relecture = relecture.Jeu(jeu)
	for _, d := range jeu.Directives {
		r.Directives = append(r.Directives, DirRelue{ID: d.ID, Version: d.Version, Statut: string(d.Statut), Texte: d.TexteFR, Relecture: regleEnFrancais(d)})
	}
	return r
}

func depiler(err error) []string {
	var out []string
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		for _, e := range j.Unwrap() {
			out = append(out, depiler(e)...)
		}
		return out
	}
	for _, l := range strings.Split(err.Error(), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}
