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
	b.WriteString("Les notifications et les marques des directives appliquées se cumulent, quel que soit l'effet retenu.\n")
	for _, d := range j.Directives {
		b.WriteString("\n")
		b.WriteString(Directive(d))
	}
	return b.String()
}

// Directive rend une directive (schéma v2, docs/schema-directive-v2.md) :
// identité et fondements, validité, déclencheur, condition, effets. Le
// format v1 est retiré : cmd/directive le refuse avant toute relecture.
func Directive(d directive.Directive) string {
	return directiveV2(d)
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
