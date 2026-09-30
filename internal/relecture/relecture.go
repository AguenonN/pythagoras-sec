// Package relecture rend un jeu de directives en français, par gabarit
// déterministe, sans modèle de langue. C'est ce que le juriste relit avant de
// valider : la règle telle que l'engine l'appliquera, et non le texte qu'il a
// écrit. Même jeu, même texte, octet pour octet : aucune horloge, aucun
// fuseau local, aucun parcours de map.
package relecture

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"mcprogue/internal/directive"
)

// Jeu rend la relecture complète d'un jeu chargé par directive.Charger.
func Jeu(j *directive.Jeu) string {
	var b strings.Builder
	m := j.Meta
	fmt.Fprintf(&b, "Jeu %s, version %d, en vigueur à partir du %s, auteur %s.\n",
		m.ID, m.Version, date(m.DateEffet), m.Auteur)
	if m.Empreinte != "" {
		fmt.Fprintf(&b, "Empreinte : %s.\n", m.Empreinte)
	}
	fmt.Fprintf(&b, "%s, évaluée%s dans l'ordre ci-dessous ; si plusieurs s'appliquent au même appel, "+
		"l'effet le plus contraignant l'emporte (refus, puis validation humaine, puis caviardage).\n",
		compte(len(j.Directives), "directive", "directives"), s(len(j.Directives)))
	for _, d := range j.Directives {
		b.WriteString("\n")
		b.WriteString(Directive(d))
	}
	return b.String()
}

// Directive rend une directive en trois lignes : identité et fondement,
// validité, règle.
func Directive(d directive.Directive) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Directive %s, version %d, %s, fondée sur %s.\n",
		d.ID, d.Version, statut(d.Statut), fondement(d.Fondement))
	fmt.Fprintf(&b, "Applicable aux appels à partir du %s, sans date de fin ; %s.\n",
		date(d.DateEffet), revision(d.RevisionAvant))
	fmt.Fprintf(&b, "Verrou %s. %s\n", d.Verrou, regle(d))
	return b.String()
}

func statut(s directive.Statut) string {
	switch s {
	case directive.Brouillon:
		return "brouillon (non appliquée tant que le juriste ne l'a pas passée en observation ou active)"
	case directive.Observation:
		return "en observation (journalisée, non appliquée)"
	case directive.Active:
		return "active (appliquée)"
	case directive.Retiree:
		return "retirée (non appliquée)"
	}
	return "au statut inconnu " + strconv.Quote(string(s))
}

func fondement(f directive.Fondement) string {
	var out string
	switch {
	case f.Article == "":
		out = f.Texte
	case strings.HasPrefix(strings.ToLower(f.Article), "art"):
		out = "l'" + f.Article + " " + f.Texte
	default:
		out = "l'art. " + f.Article + " " + f.Texte
	}
	if f.URL != "" {
		out += " (source : " + f.URL + ")"
	}
	return out
}

func revision(r *time.Time) string {
	if r == nil {
		return "sans échéance de relecture"
	}
	return "à relire avant le " + date(*r) +
		" (passée cette date, la directive reste appliquée et une alerte est émise)"
}

// date écrit toujours en UTC, quel que soit le fuseau de la machine.
func date(t time.Time) string {
	t = t.UTC()
	h := t.Format("15:04")
	if t.Second() != 0 {
		h = t.Format("15:04:05")
	}
	return t.Format("02/01/2006") + " à " + h + " UTC"
}

func regle(d directive.Directive) string {
	p := perimetre(d.Perimetre) + " : "
	c := cible(d.Cible)
	switch d.Verrou {
	case directive.Interdiction:
		return p + c + " " + effet(d.Effet) + ", sans condition."
	case directive.ApprobationHumaine:
		return p + c + " " + effet(d.Effet) + "."
	case directive.SeuilArgument:
		return p + c + " " + conditions(d.Condition) + " " + effet(d.Effet) + ". " + indetermine(d)
	case directive.ProvenanceSession:
		return p + provenance(d.Provenance) + ", " + c + " " + effet(d.Effet) + "."
	case directive.SourceAttestee:
		return p + c + " " + effet(d.Effet) +
			" si l'attestation de ce serveur au registre (identité, schémas épinglés) a échoué."
	case directive.ChampsAutorises:
		return p + champs(c, d.Champs)
	}
	return p + c + " " + effet(d.Effet) + "."
}

// perimetre reprend la règle de Decide : un appelant non prouvé tombe sous
// toute directive à périmètre.
func perimetre(p *directive.Perimetre) string {
	if p == nil {
		return "Pour tous les appelants"
	}
	for _, a := range p.Appelants {
		if a == "*" {
			return "Pour tous les appelants"
		}
	}
	return "Pour " + pluriel(len(p.Appelants), "l'appelant", "les appelants") + " " +
		strings.Join(p.Appelants, ", ") + ", et pour tout appelant dont l'identité n'est pas prouvée"
}

func cible(c directive.Cible) string {
	switch {
	case c.Serveur == "*" && c.Outil == "*":
		return "tout appel d'outil, sur tout serveur,"
	case c.Serveur == "*":
		return "tout appel à " + c.Outil + ", sur tout serveur,"
	case c.Outil == "*":
		return "tout appel à un outil du serveur " + c.Serveur
	}
	return "tout appel à " + c.Outil + " sur " + c.Serveur
}

func effet(e directive.Effet) string {
	switch e {
	case directive.Refuser:
		return "est refusé"
	case directive.AttendreValidation:
		return "exige une validation humaine"
	case directive.Caviarder:
		return "est transmis après caviardage"
	case directive.Autoriser:
		return "est autorisé"
	}
	return "a l'effet inconnu " + strconv.Quote(string(e))
}

func conditions(cs []directive.Condition) string {
	parts := make([]string, len(cs))
	for i, c := range cs {
		parts[i] = "dont l'argument " + c.Champ + " " + operation(c)
	}
	return strings.Join(parts, " et ")
}

func operation(c directive.Condition) string {
	switch c.Op {
	case directive.OpSup:
		return "est strictement supérieur à " + valeur(c.Valeur)
	case directive.OpSupEgal:
		return "est supérieur ou égal à " + valeur(c.Valeur)
	case directive.OpInf:
		return "est strictement inférieur à " + valeur(c.Valeur)
	case directive.OpInfEgal:
		return "est inférieur ou égal à " + valeur(c.Valeur)
	case directive.OpEgal:
		return "est égal à " + valeur(c.Valeur)
	case directive.OpDiff:
		return "est différent de " + valeur(c.Valeur)
	case directive.OpDans:
		return "vaut l'une des valeurs " + valeur(c.Valeur)
	case directive.OpPresent:
		return "est présent"
	case directive.OpAbsent:
		return "est absent"
	}
	return "vérifie l'opérateur inconnu " + strconv.Quote(string(c.Op)) + " " + valeur(c.Valeur)
}

// valeur écrit un nombre tel que le juriste l'a validé (json.Number garde le
// littéral), une chaîne entre guillemets français, une liste élément par
// élément.
func valeur(v any) string {
	switch x := v.(type) {
	case nil:
		return "(aucune)"
	case json.Number:
		return x.String()
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case string:
		return "« " + x + " »"
	case bool:
		if x {
			return "vrai"
		}
		return "faux"
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = valeur(e)
		}
		return strings.Join(parts, ", ")
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// indetermine suit Decide : sans ignorer validé, une condition impossible à
// évaluer refuse l'appel, quel que soit l'effet demandé.
func indetermine(d directive.Directive) string {
	si := "Si l'une de ces conditions ne peut pas être évaluée (argument absent ou de type inattendu), "
	if len(d.Condition) == 1 {
		si = "Si cette condition ne peut pas être évaluée (argument absent ou de type inattendu), "
	}
	if d.SiIndetermine == directive.IndetermineIgnorer {
		return si + "la directive ne s'applique pas à cet appel (choix validé par " + d.SiIndetermineValidePar + ")."
	}
	return si + "l'appel est refusé."
}

func provenance(p *directive.Provenance) string {
	if p == nil {
		return "si la session a consulté (provenance absente)"
	}
	var sources []string
	for _, sv := range p.Serveurs {
		sources = append(sources, "du serveur "+sv)
	}
	for _, o := range p.Outils {
		sv, outil, _ := strings.Cut(o, "/")
		sources = append(sources, "de l'outil "+outil+" du serveur "+sv)
	}
	liste := strings.Join(sources, " ou ")
	if p.Critere == directive.Transmis {
		return "si la session a déjà transmis un appel à destination " + liste + " (quel qu'en soit le résultat)"
	}
	return "si la session a reçu des données " + liste
}

func champs(c string, ch *directive.Champs) string {
	if ch == nil {
		return c + " (liste de champs absente)."
	}
	liste := pluriel(len(ch.Autorises), "le champ", "les champs") + " " + strings.Join(ch.Autorises, ", ")
	const couvre = " (un champ autorisé couvre aussi tout son contenu)"
	if ch.Mode == directive.ModeRefuser {
		return c + " ne peut contenir que " + liste + couvre + " ; un appel qui en contient un autre est refusé."
	}
	return c + " ne transmet que " + liste + couvre + " ; tout autre champ est retiré avant transmission."
}

// pluriel choisit la forme selon n ; compte la précède du nombre.
func pluriel(n int, un, plusieurs string) string {
	if n == 1 {
		return un
	}
	return plusieurs
}

func compte(n int, un, plusieurs string) string {
	return strconv.Itoa(n) + " " + pluriel(n, un, plusieurs)
}

func s(n int) string {
	if n > 1 {
		return "s"
	}
	return ""
}
