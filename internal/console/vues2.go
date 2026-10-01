package console

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
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
// Règles en vigueur
// ---------------------------------------------------------------------------

// EnVigueur rend les règles que les journaux citent en dernier ; à défaut
// (aucun journal, jeu inconnu), la première du catalogue.
func EnVigueur(etats []*Etat, jeux []JeuCharge) *JeuCharge {
	var dernier *journal.RefJeu
	var ts time.Time
	for _, st := range etats {
		for _, e := range st.Entrees {
			if e.Type == journal.AppelT && e.Jeu != nil && !e.TS.Before(ts) {
				dernier, ts = e.Jeu, e.TS
			}
		}
	}
	if dernier != nil {
		for i := range jeux {
			m := jeux[i].Jeu.Meta
			if m.ID == dernier.ID && m.Version == dernier.Version && m.Empreinte == dernier.Empreinte {
				return &jeux[i]
			}
		}
	}
	if len(jeux) > 0 {
		return &jeux[0]
	}
	return nil
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
	Acces      string        `json:"acces,omitempty"`
	TS         time.Time     `json:"ts"`
	Journalise string        `json:"journalise"`
	Rejoue     string        `json:"rejoue"`
	Motif      string        `json:"motif"`
	Declenchee bool          `json:"declenchee"`
	Identique  bool          `json:"identique"`
	Ecarts     []rejeu.Ecart `json:"ecarts,omitempty"`
}

// DirListe est une ligne de la liste des directives.
type DirListe struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Statut  string `json:"statut"`
	Titre   string `json:"titre"`
}

// DirDetail est la directive choisie : texte, relecture, décisions réelles.
type DirDetail struct {
	DirListe
	Texte      string                `json:"texte"`
	DateEffet  time.Time             `json:"date_effet"`
	Fondements []directive.Fondement `json:"fondements"`
	Relecture  string                `json:"relecture"`
	Fuseaux    bool                  `json:"fuseaux"` // la directive évalue l'heure
	Decisions  []DecisionRejouee     `json:"decisions"`
	Journaux   []string              `json:"journaux"`
	Verifies   bool                  `json:"verifies"` // chaînes de ces journaux vérifiées
}

// VueRegles est l'écran 3.
type VueRegles struct {
	Depuis     time.Time  `json:"depuis"`
	Auteur     string     `json:"auteur"`
	Directives []DirListe `json:"directives"`
	Choix      *DirDetail `json:"choix,omitempty"`
}

func fondementsDe(d directive.Directive) []directive.Fondement {
	if len(d.Fondements) > 0 {
		return d.Fondements
	}
	return []directive.Fondement{d.Fondement}
}

var reVerrou = regexp.MustCompile(`^Verrou \S*\. ?`)

// regleEnFrancais rend la phrase de règle de la relecture (sa troisième
// ligne), sans le nom technique du verrou.
func regleEnFrancais(d directive.Directive) string {
	lignes := strings.Split(strings.TrimSpace(relecture.Directive(d)), "\n")
	if len(lignes) < 3 {
		return strings.Join(lignes, " ")
	}
	return strings.TrimSpace(reVerrou.ReplaceAllString(lignes[2], ""))
}

func evalueHeure(d directive.Directive) bool {
	ok := false
	if d.Arbre != nil {
		parcourir(d.Arbre, func(n *directive.Noeud) {
			if n.Temps != nil {
				ok = true
			}
		})
	}
	return ok
}

// ReglesEnVigueur construit l'écran 3 : la liste des directives des règles en vigueur
// et, pour la directive choisie, sa relecture et ses décisions réelles
// rejouées. choix vide : la première.
func ReglesEnVigueur(jc JeuCharge, etats []*Etat, tous []JeuCharge, choix string) *VueRegles {
	v := &VueRegles{Depuis: jc.Jeu.Meta.DateEffet, Auteur: jc.Jeu.Meta.Auteur, Directives: []DirListe{}}
	var sel *directive.Directive
	for i := range jc.Jeu.Directives {
		d := &jc.Jeu.Directives[i]
		v.Directives = append(v.Directives, DirListe{ID: d.ID, Version: d.Version, Statut: string(d.Statut), Titre: Titre(d.TexteFR)})
		if sel == nil && (choix == "" || choix == d.ID) {
			sel = d
		}
	}
	if sel == nil {
		return v
	}
	det := &DirDetail{DirListe: DirListe{ID: sel.ID, Version: sel.Version, Statut: string(sel.Statut), Titre: Titre(sel.TexteFR)},
		Texte: sel.TexteFR, DateEffet: sel.DateEffet, Fondements: fondementsDe(*sel), Relecture: regleEnFrancais(*sel),
		Fuseaux: evalueHeure(*sel), Decisions: []DecisionRejouee{}, Journaux: []string{}, Verifies: true}
	for _, st := range etats {
		utilise := false
		for _, r := range rejouerTout(st, tous, rejeu.Options{}) {
			en := r.Entree
			if r.Err != nil || !r.Rejeu.Rejouable || r.Rejeu.Rejouee == nil ||
				en.Jeu.ID != jc.Jeu.Meta.ID || en.Jeu.Version != jc.Jeu.Meta.Version {
				continue
			}
			for _, a := range r.Rejeu.Rejouee.Applications {
				if a.ID != sel.ID || a.Version != sel.Version {
					continue
				}
				utilise = true
				det.Decisions = append(det.Decisions, DecisionRejouee{
					Journal: st.Journal.ID, Session: en.Session, Seq: en.Seq, Serveur: en.Serveur, Outil: en.Outil, Acces: en.Acces, TS: en.TS,
					Journalise: string(r.Rejeu.Journalise), Rejoue: string(r.Rejeu.Rejouee.Effet), Motif: r.Rejeu.Rejouee.Motif,
					Declenchee: a.Declenchee, Identique: string(r.Rejeu.Journalise) == string(r.Rejeu.Rejouee.Effet) && len(r.Rejeu.Ecarts) == 0,
					Ecarts: r.Rejeu.Ecarts})
			}
		}
		if utilise {
			det.Journaux = append(det.Journaux, st.Journal.ID)
			det.Verifies = det.Verifies && st.Verif.OK
		}
	}
	sort.SliceStable(det.Decisions, func(i, j int) bool { return det.Decisions[i].TS.Before(det.Decisions[j].TS) })
	if len(det.Decisions) > 12 {
		// Garde d'abord les décisions où la directive s'est déclenchée.
		var gardees []DecisionRejouee
		for _, d := range det.Decisions {
			if d.Declenchee && len(gardees) < 12 {
				gardees = append(gardees, d)
			}
		}
		for _, d := range det.Decisions {
			if !d.Declenchee && len(gardees) < 12 {
				gardees = append(gardees, d)
			}
		}
		sort.SliceStable(gardees, func(i, j int) bool { return gardees[i].TS.Before(gardees[j].TS) })
		det.Decisions = gardees
	}
	v.Choix = det
	return v
}

// ---------------------------------------------------------------------------
// Écran 4 : avant / après
// ---------------------------------------------------------------------------

// OptionAA est une directive comparable : celle qu'un autre ensemble de
// règles apporte (nouvelle, ou autre version) par rapport aux règles en
// vigueur au moment de la session.
type OptionAA struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Titre   string `json:"titre"`
	Avant   string `json:"avant"` // intitulé de la colonne « avant »
	Apres   string `json:"apres"`
}

func memeDirective(a, b directive.Directive) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// OptionsAvantApres liste les directives que les autres règles disponibles
// changent ou ajoutent par rapport à base.
func OptionsAvantApres(base JeuCharge, tous []JeuCharge) []OptionAA {
	var out []OptionAA
	vus := map[string]bool{}
	for _, j := range tous {
		if j.Jeu.Meta.ID == base.Jeu.Meta.ID && j.Jeu.Meta.Version == base.Jeu.Meta.Version && j.Jeu.Meta.Empreinte == base.Jeu.Meta.Empreinte {
			continue
		}
		for _, d := range j.Jeu.Directives {
			cle := fmt.Sprintf("%s#%d", d.ID, d.Version)
			if o, ok := optionPour(base, d); ok && !vus[cle] {
				vus[cle] = true
				out = append(out, o)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Version < out[j].Version
	})
	return out
}

// optionPour dit si d change quelque chose par rapport à base, et comment
// intituler les deux colonnes.
func optionPour(base JeuCharge, d directive.Directive) (OptionAA, bool) {
	o := OptionAA{ID: d.ID, Version: d.Version, Titre: Titre(d.TexteFR), Avant: "Sans la directive", Apres: "Avec " + d.ID}
	for _, b := range base.Jeu.Directives {
		if b.ID != d.ID {
			continue
		}
		if memeDirective(b, d) {
			return o, false
		}
		o.Avant = fmt.Sprintf("Avec %s, version %d", b.ID, b.Version)
		o.Apres = fmt.Sprintf("Avec %s, version %d", d.ID, d.Version)
	}
	return o, true
}

// avecDirective rend base où la directive d remplace celle de même
// identifiant, ou s'ajoute à la fin.
func avecDirective(base directive.Jeu, d directive.Directive) directive.Jeu {
	out := base
	out.Directives = append([]directive.Directive(nil), base.Directives...)
	for i := range out.Directives {
		if out.Directives[i].ID == d.ID {
			out.Directives[i] = d
			return out
		}
	}
	out.Directives = append(out.Directives, d)
	return out
}

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
	Journal       string       `json:"journal"`
	Session       string       `json:"session"`
	Ouverte       time.Time    `json:"ouverte"`
	Options       []OptionAA   `json:"options"`
	Choix         *OptionAA    `json:"choix,omitempty"`
	Lignes        []LigneAA    `json:"lignes"`
	Changes       int          `json:"changes"`
	Sessions      int          `json:"sessions"`
	NonRejouables int          `json:"non_rejouables"`
	Explication   string       `json:"explication"`
	Verif         Verification `json:"verification"`
}

// AvantApres rejoue la session sous les règles citées par le journal, avec
// et sans la directive choisie (ou dans sa version précédente), à la date
// de chaque appel. choix vide : la première option.
func AvantApres(st *Etat, tous []JeuCharge, session, choix string) (*VueAvantApres, error) {
	v := &VueAvantApres{Journal: st.Journal.ID, Session: session, Options: []OptionAA{}, Lignes: []LigneAA{}, Verif: st.Verif}
	var ref *journal.RefJeu
	for _, e := range st.Entrees {
		switch {
		case e.Type == journal.SessionOuverte && e.Session == session:
			v.Ouverte = e.TS
		case e.Type == journal.AppelT && e.Session == session && ref == nil && e.Jeu != nil:
			ref = e.Jeu
		}
	}
	base, ok := trouverJeu(tous, ref)
	if !ok {
		return v, nil
	}
	baseCharge := JeuCharge{Jeu: *base}
	v.Options = OptionsAvantApres(baseCharge, tous)
	for i := range v.Options {
		if choix == "" && v.Choix == nil || choix == fmt.Sprintf("%s#%d", v.Options[i].ID, v.Options[i].Version) {
			v.Choix = &v.Options[i]
		}
	}
	if v.Choix == nil {
		return v, nil
	}
	var dir *directive.Directive
	for _, j := range tous {
		for i := range j.Jeu.Directives {
			if d := &j.Jeu.Directives[i]; d.ID == v.Choix.ID && d.Version == v.Choix.Version {
				dir = d
			}
		}
	}
	if dir == nil {
		return v, nil
	}
	cat := catalogue(tous)
	hybride := avecDirective(*base, *dir)
	// Sans la directive : si elle n'existe pas dans les règles en vigueur, on
	// la retire ; sinon c'est sa version en vigueur.
	avant := *base
	if v.Choix.Avant == "Sans la directive" {
		avant = directive.Jeu{Meta: base.Meta}
		for _, d := range base.Directives {
			if d.ID != dir.ID {
				avant.Directives = append(avant.Directives, d)
			}
		}
	}
	optsAvant, optsApres := rejeu.Options{Jeu: &avant}, rejeu.Options{Jeu: &hybride}
	ra, _ := rejeu.RejouerJournal(st.Entrees, st.Coffre, cat, optsAvant)
	rb, _ := rejeu.RejouerJournal(st.Entrees, st.Coffre, cat, optsApres)
	sess := map[string]bool{}
	for i := range ra {
		if i >= len(rb) || ra[i].Session != session {
			continue
		}
		sess[ra[i].Session] = true
		l := LigneAA{Session: session, Seq: ra[i].Seq, Serveur: ra[i].Serveur, Outil: ra[i].Outil, Avant: ra[i].Effet(), Apres: rb[i].Effet()}
		if ra[i].Rejouee != nil {
			l.MotifAvant = ra[i].Rejouee.Motif
		}
		if rb[i].Rejouee != nil {
			l.MotifApres = rb[i].Rejouee.Motif
		}
		l.Change = l.Avant != l.Apres
		if l.Change {
			v.Changes++
		}
		if l.Avant == rejeu.NonRejouable || l.Apres == rejeu.NonRejouable {
			v.NonRejouables++
		}
		v.Lignes = append(v.Lignes, l)
	}
	v.Sessions = len(sess)
	v.Explication = expliquer(v)
	return v, nil
}

func libelleEffet(e string) string {
	switch e {
	case "autoriser":
		return "autorisé"
	case "caviarder":
		return "caviardé"
	case "attendre_validation":
		return "mis en attente de validation"
	case "refuser":
		return "refusé"
	}
	return "non rejouable"
}

// expliquer décrit, sans interpréter, ce qui change entre les deux colonnes.
func expliquer(v *VueAvantApres) string {
	if v.Changes == 0 {
		return "Aucun appel de cette session ne change d'effet avec cette directive."
	}
	var parts []string
	for _, l := range v.Lignes {
		if l.Change {
			parts = append(parts, fmt.Sprintf("l'appel n° %d (%s) serait %s au lieu de %s", l.Seq, l.Outil, libelleEffet(l.Apres), libelleEffet(l.Avant)))
		}
	}
	return "Avec cette directive, " + strings.Join(parts, " ; ") + "."
}
