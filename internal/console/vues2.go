package console

import (
	"fmt"
	"strings"
	"time"

	"mcprogue/internal/directive"
	"mcprogue/internal/journal"
	"mcprogue/internal/rejeu"
	"mcprogue/internal/relecture"
)

func catalogue(jeux []JeuCharge) rejeu.Catalogue {
	cat := rejeu.Catalogue{}
	for _, j := range jeux {
		cat = append(cat, j.Jeu)
	}
	return cat
}

// rejoue est le rejeu d'une entrée appel, avec son erreur éventuelle (jeu
// introuvable) : un appel non rejouable ne doit pas arrêter l'écran.
type rejoue struct {
	Entree journal.Entree
	Rejeu  rejeu.Rejeu
	Err    error
}

func rejouerTout(st *Etat, jeux []JeuCharge, opts rejeu.Options) []rejoue {
	cat := catalogue(jeux)
	purges := rejeu.PurgesChainees(st.Entrees)
	var out []rejoue
	for _, en := range st.Entrees {
		if en.Type != journal.AppelT || en.Jeu == nil || en.Decision == nil || en.Appel == nil {
			continue
		}
		r, err := rejeu.Rejouer(en, st.Coffre, cat, purges, opts)
		out = append(out, rejoue{Entree: en, Rejeu: r, Err: err})
	}
	return out
}

// ---------------------------------------------------------------------------
// Écran 3 : vitrine des règles
// ---------------------------------------------------------------------------

// DecisionRejouee est une décision réelle, tirée d'un journal et rejouée.
type DecisionRejouee struct {
	Journal    string        `json:"journal"`
	Session    string        `json:"session"`
	Seq        int           `json:"seq"`
	Serveur    string        `json:"serveur"`
	Outil      string        `json:"outil"`
	TS         time.Time     `json:"ts"`
	Journalise string        `json:"journalise"`
	Rejoue     string        `json:"rejoue"`
	Motif      string        `json:"motif"`
	Declenchee bool          `json:"declenchee"`
	Identique  bool          `json:"identique"`
	Ecarts     []rejeu.Ecart `json:"ecarts,omitempty"`
}

// VitrineDirective est une directive, relue, avec une décision réelle.
type VitrineDirective struct {
	ID         string                `json:"id"`
	Version    int                   `json:"version"`
	Statut     string                `json:"statut"`
	Texte      string                `json:"texte"`
	Fondements []directive.Fondement `json:"fondements"`
	Relecture  string                `json:"relecture"`
	Decision   *DecisionRejouee      `json:"decision,omitempty"`
}

// VueVitrine est l'écran 3 pour un jeu.
type VueVitrine struct {
	Jeu        string             `json:"jeu"`
	Empreinte  string             `json:"empreinte"`
	Source     string             `json:"source"`
	Relecture  string             `json:"relecture"`
	Directives []VitrineDirective `json:"directives"`
}

func fondementsDe(d directive.Directive) []directive.Fondement {
	if len(d.Fondements) > 0 {
		return d.Fondements
	}
	return []directive.Fondement{d.Fondement}
}

// Vitrine rend, pour chaque directive du jeu, son texte, sa relecture et la
// meilleure décision réelle que les journaux fournissent : d'abord une
// décision où la directive s'est déclenchée, à défaut une où elle a été
// évaluée sans se déclencher.
func Vitrine(jc JeuCharge, etats []*Etat, tous []JeuCharge) *VueVitrine {
	v := &VueVitrine{Jeu: jc.Libelle(), Empreinte: jc.Jeu.Meta.Empreinte, Source: jc.Source,
		Relecture: relecture.Jeu(&jc.Jeu)}
	type cand struct {
		d          DecisionRejouee
		declenchee bool
	}
	meilleur := map[string]*cand{}
	for _, st := range etats {
		for _, r := range rejouerTout(st, tous, rejeu.Options{}) {
			if r.Err != nil || !r.Rejeu.Rejouable || r.Rejeu.Rejouee == nil {
				continue
			}
			en := r.Entree
			if en.Jeu.ID != jc.Jeu.Meta.ID || en.Jeu.Version != jc.Jeu.Meta.Version {
				continue
			}
			for _, a := range r.Rejeu.Rejouee.Applications {
				cle := fmt.Sprintf("%s#%d", a.ID, a.Version)
				if old := meilleur[cle]; old != nil && (old.declenchee || !a.Declenchee) {
					continue
				}
				meilleur[cle] = &cand{declenchee: a.Declenchee, d: DecisionRejouee{
					Journal: st.Journal.ID, Session: en.Session, Seq: en.Seq, Serveur: en.Serveur, Outil: en.Outil, TS: en.TS,
					Journalise: string(r.Rejeu.Journalise), Rejoue: string(r.Rejeu.Rejouee.Effet), Motif: r.Rejeu.Rejouee.Motif,
					Declenchee: a.Declenchee, Identique: string(r.Rejeu.Journalise) == string(r.Rejeu.Rejouee.Effet) && len(r.Rejeu.Ecarts) == 0,
					Ecarts: r.Rejeu.Ecarts}}
			}
		}
	}
	for _, d := range jc.Jeu.Directives {
		vd := VitrineDirective{ID: d.ID, Version: d.Version, Statut: string(d.Statut), Texte: d.TexteFR,
			Fondements: fondementsDe(d), Relecture: strings.TrimSpace(relecture.Directive(d))}
		if c := meilleur[fmt.Sprintf("%s#%d", d.ID, d.Version)]; c != nil {
			dd := c.d
			vd.Decision = &dd
		}
		v.Directives = append(v.Directives, vd)
	}
	return v
}

// ---------------------------------------------------------------------------
// Écran 4 : avant / après
// ---------------------------------------------------------------------------

// LigneAA est un appel dans la vue avant / après.
type LigneAA struct {
	Session    string        `json:"session"`
	Seq        int           `json:"seq"`
	Serveur    string        `json:"serveur"`
	Outil      string        `json:"outil"`
	Avant      string        `json:"avant"`
	Apres      string        `json:"apres"`
	Change     bool          `json:"change"`
	MotifAvant string        `json:"motif_avant,omitempty"`
	MotifApres string        `json:"motif_apres,omitempty"`
	Ecarts     []rejeu.Ecart `json:"ecarts,omitempty"`
}

// VueAvantApres est l'écran 4.
type VueAvantApres struct {
	Journal     string             `json:"journal"`
	Avant       string             `json:"avant"`
	Apres       string             `json:"apres"`
	Lignes      []LigneAA          `json:"lignes"`
	Transitions []rejeu.Transition `json:"transitions"`
	Changes     int                `json:"changes"`
	Verif       Verification       `json:"verification"`
}

// AvantApres rejoue le journal sous le jeu d'origine (avant) et sous un autre
// jeu (après), à la date de chaque appel, et surligne les transitions. Si
// avant est nil, « avant » est le cadre historique : les décisions que le
// journal affirme.
func AvantApres(st *Etat, tous []JeuCharge, avant, apres JeuCharge) (*VueAvantApres, error) {
	cat := catalogue(tous)
	optsAvant := rejeu.Options{Jeu: &avant.Jeu}
	optsApres := rejeu.Options{Jeu: &apres.Jeu}
	cmp, err := rejeu.Comparer(st.Entrees, st.Coffre, cat, optsAvant, optsApres)
	if err != nil {
		return nil, err
	}
	ra, _ := rejeu.RejouerJournal(st.Entrees, st.Coffre, cat, optsAvant)
	rb, _ := rejeu.RejouerJournal(st.Entrees, st.Coffre, cat, optsApres)
	v := &VueAvantApres{Journal: st.Journal.ID, Avant: avant.Libelle(), Apres: apres.Libelle(),
		Transitions: cmp.Transitions, Verif: st.Verif, Lignes: []LigneAA{}}
	for i, l := range cmp.Lignes {
		la := LigneAA{Session: l.Session, Seq: l.Seq, Serveur: l.Serveur, Outil: l.Outil, Avant: l.Avant, Apres: l.Apres,
			Change: l.Avant != l.Apres, Ecarts: l.Ecarts}
		if i < len(ra) && ra[i].Rejouee != nil {
			la.MotifAvant = ra[i].Rejouee.Motif
		}
		if i < len(rb) && rb[i].Rejouee != nil {
			la.MotifApres = rb[i].Rejouee.Motif
		}
		if la.Change {
			v.Changes++
		}
		v.Lignes = append(v.Lignes, la)
	}
	return v, nil
}
