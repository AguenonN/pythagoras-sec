package console

import (
	"encoding/json"
	"fmt"
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

// catalogueDe met en tête l'ensemble sous lequel l'entrée a été journalisée :
// le catalogue du rejeu cherche par identifiant et version seulement, et un
// ensemble de travail porte les mêmes (« demo », version n) que les règles en
// vigueur, avec une autre empreinte.
func catalogueDe(jeux []JeuCharge, ref *journal.RefJeu) rejeu.Catalogue {
	cat := rejeu.Catalogue{}
	if j := jeuChargeDe(jeux, ref); j != nil {
		cat = append(cat, j.Jeu)
	}
	return append(cat, catalogue(jeux)...)
}

// rejoue est le rejeu d'une entrée appel, avec son erreur éventuelle (jeu
// introuvable) : un appel non rejouable ne doit pas arrêter l'écran.
type rejoue struct {
	Entree journal.Entree
	Rejeu  rejeu.Rejeu
	Err    error
}

func rejouerTout(st *Etat, jeux []JeuCharge, opts rejeu.Options) []rejoue {
	purges := rejeu.PurgesChainees(st.Entrees)
	var out []rejoue
	for _, en := range st.Entrees {
		if en.Type != journal.AppelT || en.Jeu == nil || en.Decision == nil || en.Appel == nil {
			continue
		}
		r, err := rejeu.Rejouer(en, st.Coffre, catalogueDe(jeux, en.Jeu), purges, opts)
		out = append(out, rejoue{Entree: en, Rejeu: r, Err: err})
	}
	return out
}

// EnVigueur rend les règles en vigueur : demo/directives/demo.json.
func EnVigueur(jeux []JeuCharge) *JeuCharge { return Courant(jeux) }

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
	// Relecture est la relecture complète de internal/relecture
	// (Quand / Si / Alors, règle de l'indéterminé), la même que
	// « cmd/directive relire ».
	Relecture string `json:"relecture"`
	Fuseaux   bool   `json:"fuseaux"` // la directive évalue l'heure
	// Auteur et date d'activation : première version des règles où la
	// directive est en observation ou active.
	Auteur       string            `json:"auteur,omitempty"`
	ActiveDepuis *time.Time        `json:"active_depuis,omitempty"`
	Origine      string            `json:"origine"`
	Decisions    []DecisionRejouee `json:"decisions"`
	NbDecisions  int               `json:"nb_decisions"`
	Journaux     []string          `json:"journaux"`
	Verifies     bool              `json:"verifies"` // chaînes de ces journaux vérifiées
}

// VueRegles est l'écran 3.
type VueRegles struct {
	Depuis     time.Time  `json:"depuis"`
	Auteur     string     `json:"auteur"`
	Version    int        `json:"version"`
	Directives []DirListe `json:"directives"`
	Choix      *DirDetail `json:"choix,omitempty"`
}

func fondementsDe(d directive.Directive) []directive.Fondement {
	return d.Fondements
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

// ReglesEnVigueur construit l'écran 3 : la liste des directives des règles
// en vigueur et, pour la directive choisie, sa relecture complète et les
// décisions réelles qu'elle a produites dans les journaux. choix vide : la
// première.
func (d *Depot) ReglesEnVigueur(jc JeuCharge, etats []*Etat, tous []JeuCharge, choix string) *VueRegles {
	v := &VueRegles{Depuis: jc.Jeu.Meta.DateEffet, Auteur: jc.Jeu.Meta.Auteur, Version: jc.Jeu.Meta.Version, Directives: []DirListe{}}
	var sel *directive.Directive
	for i := range jc.Jeu.Directives {
		dd := &jc.Jeu.Directives[i]
		v.Directives = append(v.Directives, DirListe{ID: dd.ID, Version: dd.Version, Statut: string(dd.Statut), Titre: Titre(dd.TexteFR)})
		if sel == nil && (choix == "" || choix == dd.ID) {
			sel = dd
		}
	}
	if sel == nil {
		return v
	}
	act := Activations(tous)[sel.ID]
	tous = d.AvecTravail(tous, etats)
	det := &DirDetail{DirListe: DirListe{ID: sel.ID, Version: sel.Version, Statut: string(sel.Statut), Titre: Titre(sel.TexteFR)},
		Texte: sel.TexteFR, DateEffet: sel.DateEffet, Fondements: fondementsDe(*sel), Relecture: strings.TrimSpace(relecture.Directive(*sel)),
		Fuseaux: evalueHeure(*sel), Decisions: []DecisionRejouee{}, Journaux: []string{}, Verifies: true,
		Origine: d.LibelleOrigine(sel.ID, act.Auteur, d.LireOrigines())}
	if act.Auteur != "" {
		det.Auteur = act.Auteur
		t := act.Date
		det.ActiveDepuis = &t
	}
	for _, st := range etats {
		utilise := false
		for _, r := range rejouerTout(st, tous, rejeu.Options{}) {
			en := r.Entree
			if r.Err != nil || !r.Rejeu.Rejouable || r.Rejeu.Rejouee == nil {
				continue
			}
			for _, a := range r.Rejeu.Rejouee.Applications {
				if a.ID != sel.ID {
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
	det.NbDecisions = len(det.Decisions)
	if len(det.Decisions) > 40 {
		// Garde d'abord les décisions où la directive s'est déclenchée.
		var gardees []DecisionRejouee
		for _, x := range det.Decisions {
			if x.Declenchee && len(gardees) < 40 {
				gardees = append(gardees, x)
			}
		}
		for _, x := range det.Decisions {
			if !x.Declenchee && len(gardees) < 40 {
				gardees = append(gardees, x)
			}
		}
		sort.SliceStable(gardees, func(i, j int) bool { return gardees[i].TS.Before(gardees[j].TS) })
		det.Decisions = gardees
	}
	v.Choix = det
	return v
}

// ---------------------------------------------------------------------------
// Écran 4 : avant / après une directive
// ---------------------------------------------------------------------------

// CoteAA est ce que les règles décident pour un appel, d'un côté de la
// comparaison.
type CoteAA struct {
	Effet         string   `json:"effet"`
	Motif         string   `json:"motif,omitempty"`
	Directives    []string `json:"directives"` // appliquées
	Roles         []string `json:"roles"`      // rôles requis (attente)
	QuatreYeux    bool     `json:"quatre_yeux,omitempty"`
	Notifications []string `json:"notifications"` // « rôle · rythme »
	ChampsRetires []string `json:"champs_retires"`
}

// LigneAA est un appel dans la vue avant / après.
type LigneAA struct {
	Journal string    `json:"journal"`
	Session string    `json:"session"`
	Seq     int       `json:"seq"`
	Serveur string    `json:"serveur"`
	Outil   string    `json:"outil"`
	TS      time.Time `json:"ts"`
	Avant   CoteAA    `json:"avant"`
	Apres   CoteAA    `json:"apres"`
	Change  bool      `json:"change"`
	// Differences : ce qui diffère entre les deux côtés (effet, directives,
	// roles, notifications, champs), pour surligner.
	Differences []string `json:"differences"`
	// Qualification d'un appel qui change : exposition (la directive est
	// postérieure à l'appel) ou violation (elle était en vigueur à la date
	// de l'appel).
	Qualification string        `json:"qualification,omitempty"`
	Ecarts        []rejeu.Ecart `json:"ecarts,omitempty"`
}

// SessionAA est une session que l'on peut rejouer.
type SessionAA struct {
	Journal string    `json:"journal"`
	Session string    `json:"session"`
	Ouverte time.Time `json:"ouverte"`
	Demande string    `json:"demande,omitempty"`
	Choisie bool      `json:"choisie"`
	// Regles : ensemble de travail sous lequel la session a tourné (« sans
	// la directive D-009 », « règles de travail « … » ») ; absent : règles en vigueur.
	Regles string `json:"regles,omitempty"`
}

// CompteursAA sont les compteurs réels de la comparaison.
type CompteursAA struct {
	Sessions      int `json:"sessions"`
	Appels        int `json:"appels"`
	Changes       int `json:"changes"`
	Exposition    int `json:"exposition"`
	Violation     int `json:"violation"`
	NonRejouables int `json:"non_rejouables"`
}

// VueAvantApres est l'écran 4.
type VueAvantApres struct {
	Directives   []DirListe  `json:"directives"`
	Choix        *DirListe   `json:"choix,omitempty"`
	LibelleAvant string      `json:"libelle_avant,omitempty"`
	LibelleApres string      `json:"libelle_apres,omitempty"`
	Sessions     []SessionAA `json:"sessions"`
	Lignes       []LigneAA   `json:"lignes"`
	Compteurs    CompteursAA `json:"compteurs"`
	Explication  string      `json:"explication"`
	Verifiees    bool        `json:"verifiees"` // chaînes des journaux rejoués vérifiées
}

// sansDirective rend les règles privées de la directive.
func sansDirective(base directive.Jeu, id string) directive.Jeu {
	out := base
	out.Directives = nil
	for _, dd := range base.Directives {
		if dd.ID != id {
			out.Directives = append(out.Directives, dd)
		}
	}
	return out
}

func coteDe(dec *directive.Decision) CoteAA {
	c := CoteAA{Directives: []string{}, Roles: []string{}, Notifications: []string{}, ChampsRetires: []string{}}
	if dec == nil {
		c.Effet = rejeu.NonRejouable
		return c
	}
	c.Effet, c.Motif = string(dec.Effet), dec.Motif
	for _, a := range dec.Applications {
		if a.Appliquee {
			c.Directives = append(c.Directives, a.ID)
		}
	}
	if dec.Validation != nil {
		c.Roles, c.QuatreYeux = append(c.Roles, dec.Validation.Roles...), dec.Validation.QuatreYeux
	}
	for _, n := range dec.Notifications {
		c.Notifications = append(c.Notifications, n.DestinataireRole+" · "+rythmeFR(string(n.Rythme)))
	}
	c.ChampsRetires = append(c.ChampsRetires, dec.ChampsRetires...)
	sort.Strings(c.ChampsRetires)
	return c
}

func rythmeFR(r string) string {
	if r == "synthese" {
		return "en synthèse"
	}
	return "en temps réel"
}

func memes(a, b []string) bool { return strings.Join(a, "\x00") == strings.Join(b, "\x00") }

func differences(a, b CoteAA) []string {
	out := []string{}
	if a.Effet != b.Effet {
		out = append(out, "effet")
	}
	if !memes(a.Directives, b.Directives) {
		out = append(out, "directives")
	}
	if !memes(a.Roles, b.Roles) || a.QuatreYeux != b.QuatreYeux {
		out = append(out, "roles")
	}
	if !memes(a.Notifications, b.Notifications) {
		out = append(out, "notifications")
	}
	if !memes(a.ChampsRetires, b.ChampsRetires) {
		out = append(out, "champs")
	}
	return out
}

// Comparaison rejoue des sessions sous deux ensembles de règles qui ne
// diffèrent que par la directive dir, chaque appel à sa date d'origine, ou
// à la date d'effet de la directive quand elle est postérieure à l'appel
// (une directive n'est pas rétroactive : sans cela elle ne s'appliquerait
// à aucun appel passé et rien ne se comparerait).
type Comparaison struct {
	Avec, Sans directive.Jeu
	Dir        directive.Directive
	Tous       []JeuCharge
}

// Rejouer produit les lignes et les compteurs pour les journaux donnés ;
// choisies, s'il est non vide, restreint aux sessions « journal|session ».
func (c Comparaison) Rejouer(etats []*Etat, choisies map[string]bool) ([]LigneAA, CompteursAA, bool) {
	var lignes []LigneAA
	var cpt CompteursAA
	sess := map[string]bool{}
	verifiees := true
	for _, st := range etats {
		purges := rejeu.PurgesChainees(st.Entrees)
		utilise := false
		for _, en := range st.Entrees {
			if en.Type != journal.AppelT || en.Jeu == nil || en.Decision == nil || en.Appel == nil {
				continue
			}
			if len(choisies) > 0 && !choisies[st.Journal.ID+"|"+en.Session] {
				continue
			}
			var opts [2]rejeu.Options
			for i, jeu := range []directive.Jeu{c.Sans, c.Avec} {
				jeu := jeu
				opts[i] = rejeu.Options{Jeu: &jeu}
				if c.Dir.DateEffet.After(en.TS) {
					opts[i].AuDate = c.Dir.DateEffet
				}
			}
			cat := catalogueDe(c.Tous, en.Jeu)
			ra, ea := rejeu.Rejouer(en, st.Coffre, cat, purges, opts[0])
			rb, eb := rejeu.Rejouer(en, st.Coffre, cat, purges, opts[1])
			cpt.Appels++
			utilise = true
			sess[st.Journal.ID+"|"+en.Session] = true
			l := LigneAA{Journal: st.Journal.ID, Session: en.Session, Seq: en.Seq, Serveur: en.Serveur, Outil: en.Outil, TS: en.TS,
				Differences: []string{}}
			if ea == nil && eb == nil && ra.Rejouable && rb.Rejouable {
				l.Avant, l.Apres = coteDe(ra.Rejouee), coteDe(rb.Rejouee)
				l.Ecarts = fusionnerEcarts(ra.Ecarts, rb.Ecarts)
			} else {
				l.Avant, l.Apres = coteDe(nil), coteDe(nil)
				cpt.NonRejouables++
			}
			l.Differences = differences(l.Avant, l.Apres)
			l.Change = len(l.Differences) > 0 && l.Avant.Effet != rejeu.NonRejouable
			if l.Change {
				cpt.Changes++
				if c.Dir.DateEffet.After(en.TS) {
					l.Qualification = "exposition"
					cpt.Exposition++
				} else {
					l.Qualification = "violation"
					cpt.Violation++
				}
			}
			lignes = append(lignes, l)
		}
		if utilise {
			verifiees = verifiees && st.Verif.OK
		}
	}
	cpt.Sessions = len(sess)
	sort.SliceStable(lignes, func(i, j int) bool { return lignes[i].TS.Before(lignes[j].TS) })
	return lignes, cpt, verifiees
}

// fusionnerEcarts joint les écarts des deux rejeux sans répétition ; seuls
// ceux qui ne sont pas l'effet de la comparaison elle-même sont gardés (un
// effet « de règle » est ce que l'écran montre déjà).
func fusionnerEcarts(a, b []rejeu.Ecart) []rejeu.Ecart {
	var out []rejeu.Ecart
	vus := map[string]bool{}
	for _, e := range append(append([]rejeu.Ecart(nil), a...), b...) {
		if e.Cause == rejeu.CauseEffetDeRegle {
			continue
		}
		k := string(e.Cause) + "|" + e.Controle + "|" + e.Detail
		if !vus[k] {
			vus[k] = true
			out = append(out, e)
		}
	}
	return out
}

// Explication décrit, sans interpréter, ce qui change entre les deux colonnes.
func explication(lignes []LigneAA, id string) string {
	var parts []string
	for _, l := range lignes {
		if !l.Change {
			continue
		}
		if l.Avant.Effet != l.Apres.Effet {
			parts = append(parts, fmt.Sprintf("l'appel n° %d (%s) serait %s au lieu de %s", l.Seq, libelleDe(l.Serveur, l.Outil, ""), libelleEffet(l.Apres.Effet), libelleEffet(l.Avant.Effet)))
			continue
		}
		var quoi []string
		for _, d := range l.Differences {
			quoi = append(quoi, map[string]string{"directives": "les directives appliquées", "roles": "les rôles requis",
				"notifications": "les notifications", "champs": "les champs retirés"}[d])
		}
		parts = append(parts, fmt.Sprintf("l'appel n° %d (%s) reste %s, mais changent : %s", l.Seq, libelleDe(l.Serveur, l.Outil, ""), libelleEffet(l.Apres.Effet), strings.Join(quoi, ", ")))
	}
	if len(parts) == 0 {
		return "Aucun appel rejoué ne change avec la directive " + id + "."
	}
	if len(parts) > 6 {
		return fmt.Sprintf("Avec la directive %s, %d appels changent, dont : %s.", id, len(parts), strings.Join(parts[:6], " ; "))
	}
	return "Avec la directive " + id + ", " + strings.Join(parts, " ; ") + "."
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

// AvantApres rejoue les sessions choisies (toutes si choisies est vide)
// avec les règles telles qu'elles sont et avec ces mêmes règles privées de
// la directive choisie (la première si choix est vide). Aucun autre
// ensemble de règles n'entre dans la comparaison.
func (d *Depot) AvantApres(etats []*Etat, tous []JeuCharge, choix string, choisies map[string]bool) *VueAvantApres {
	v := &VueAvantApres{Directives: []DirListe{}, Sessions: []SessionAA{}, Lignes: []LigneAA{}, Verifiees: true}
	base := Courant(tous)
	if base == nil {
		return v
	}
	tous = d.AvecTravail(tous, etats)
	var dir *directive.Directive
	avec := base.Jeu // l'ensemble qui porte la directive comparée
	vues := map[string]bool{}
	for i := range base.Jeu.Directives {
		dd := &base.Jeu.Directives[i]
		vues[dd.ID] = true
		v.Directives = append(v.Directives, DirListe{ID: dd.ID, Version: dd.Version, Statut: string(dd.Statut), Titre: Titre(dd.TexteFR)})
		if dir == nil && (choix == "" || choix == dd.ID) {
			dir = dd
		}
	}
	// Une directive qui n'existe que dans un jeu de travail sous lequel des
	// sessions ont tourné (D-009 avant son activation) se compare aussi :
	// « sans » = ce jeu privé d'elle, « avec » = ce jeu.
	for i := range tous {
		if !strings.HasPrefix(tous[i].Travail, "règles de travail") {
			continue
		}
		for k := range tous[i].Jeu.Directives {
			dd := &tous[i].Jeu.Directives[k]
			if vues[dd.ID] {
				continue
			}
			vues[dd.ID] = true
			v.Directives = append(v.Directives, DirListe{ID: dd.ID, Version: dd.Version, Statut: string(dd.Statut), Titre: Titre(dd.TexteFR)})
			if dir == nil && choix == dd.ID {
				dir, avec = dd, tous[i].Jeu
			}
		}
	}
	for _, st := range etats {
		for _, s := range Sessions(st) {
			v.Sessions = append(v.Sessions, SessionAA{Journal: st.Journal.ID, Session: s.ID, Ouverte: s.Ouverte,
				Demande: demandeDe(st), Choisie: len(choisies) == 0 || choisies[st.Journal.ID+"|"+s.ID],
				Regles: reglesDeSession(st, s.ID, tous)})
		}
	}
	sort.SliceStable(v.Sessions, func(i, j int) bool { return v.Sessions[i].Ouverte.After(v.Sessions[j].Ouverte) })
	if dir == nil {
		return v
	}
	v.Choix = &DirListe{ID: dir.ID, Version: dir.Version, Statut: string(dir.Statut), Titre: Titre(dir.TexteFR)}
	v.LibelleAvant, v.LibelleApres = "Sans la directive "+dir.ID, "Avec la directive "+dir.ID
	cmp := Comparaison{Avec: avec, Sans: sansDirective(avec, dir.ID), Dir: *dir, Tous: tous}
	v.Lignes, v.Compteurs, v.Verifiees = cmp.Rejouer(etats, choisies)
	if v.Lignes == nil {
		v.Lignes = []LigneAA{}
	}
	v.Explication = explication(v.Lignes, dir.ID)
	return v
}

func demandeDe(st *Etat) string {
	if st.Dossier != nil {
		return st.Dossier.Demande
	}
	return ""
}

func memeDirective(a, b directive.Directive) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// reglesDeSession dit sous quel ensemble de règles la session a tourné quand
// ce n'est pas celui en vigueur ni une version de son historique : « sans la
// directive D-009 », « règles de travail « … » ». Vide sinon.
func reglesDeSession(st *Etat, session string, jeux []JeuCharge) string {
	for _, e := range st.Entrees {
		if e.Type == journal.AppelT && e.Session == session && e.Jeu != nil {
			if j := jeuChargeDe(jeux, e.Jeu); j != nil {
				return j.Travail
			}
			return ""
		}
	}
	return ""
}
