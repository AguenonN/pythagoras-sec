package gate

import (
	"context"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"mcprogue/internal/directive"
	"mcprogue/internal/registre"
)

// Personnes concernées (feuille sujet de Decide v2). Le registre déclare,
// par outil, le pointeur de l'argument qui les désigne ; avant Decide,
// l'engine lit leurs identifiants et demande leurs groupes au résolveur,
// un outil d'un serveur du registre, seulement si ce serveur est attesté.
// Ce qui est fourni à Decide et consigné n'est jamais l'identifiant en
// clair : son empreinte (directive.EmpreinteSujet) et ses groupes. Un
// sujet introuvable, ou que le résolveur n'a pas pu résoudre, n'est pas
// fourni : la feuille sujet est indéterminée, et si_indetermine tranche.

// Resolveur désigne l'outil qui rend les groupes d'une personne.
type Resolveur struct {
	Serveur  string // nom logique au registre
	Outil    string
	Argument string // nom de l'argument qui porte l'identifiant
}

// ResolveurReferentielRH est le résolveur de démonstration :
// referentiel-rh/groupes_salarie.
var ResolveurReferentielRH = Resolveur{Serveur: "referentiel-rh", Outil: "groupes_salarie", Argument: "salarie_id"}

// Bornes de la résolution.
const (
	// SujetsMax : au-delà, aucun sujet n'est résolu (indéterminé).
	SujetsMax = 100
	// DelaiResolution borne chaque appel au résolveur, verrou de session
	// tenu (contrat §2 : tout ce qui est fait verrou tenu a son délai).
	DelaiResolution = 10 * time.Second
)

// resoudreSujets lit les identifiants des personnes concernées dans les
// arguments et résout leurs groupes. Rend les sujets résolus et les
// empreintes des sujets non résolus, triés.
func (g *Gate) resoudreSujets(ctx context.Context, o registre.Outil, args map[string]any) ([]directive.Sujet, []string) {
	if o.PersonnesConcernees == "" {
		return nil, nil
	}
	ids, lisible := identifiants(args, o.PersonnesConcernees)
	if !lisible {
		return nil, nil
	}
	var sujets []directive.Sujet
	var nonResolus []string
	r := g.cfg.Resolveur
	b := g.backends[r.Serveur]
	attestee := r.Serveur != "" && b != nil && b.Attestation().Valide()
	for _, id := range ids {
		emp := directive.EmpreinteSujet(id)
		if !attestee {
			nonResolus = append(nonResolus, emp)
			continue
		}
		groupes, ok := g.groupes(ctx, id)
		if !ok {
			nonResolus = append(nonResolus, emp)
			continue
		}
		sujets = append(sujets, directive.Sujet{Empreinte: emp, Groupes: groupes})
	}
	sort.Slice(sujets, func(i, j int) bool { return sujets[i].Empreinte < sujets[j].Empreinte })
	sort.Strings(nonResolus)
	return sujets, nonResolus
}

// groupes interroge le résolveur pour un identifiant. Seule une réponse
// structurée de statut ok, pour ce même identifiant, avec une liste de
// groupes, résout le sujet.
func (g *Gate) groupes(ctx context.Context, id string) ([]string, bool) {
	r := g.cfg.Resolveur
	ctx, annuler := context.WithTimeout(ctx, DelaiResolution)
	defer annuler()
	res, err := g.backends[r.Serveur].Appeler(ctx, r.Outil, map[string]any{r.Argument: id}, nil)
	if err != nil || res == nil || res.IsError {
		return nil, false
	}
	sc, _ := res.StructuredContent.(map[string]any)
	if sc == nil || sc["statut"] != "ok" || sc[r.Argument] != id {
		return nil, false
	}
	liste, ok := sc["groupes"].([]any)
	if !ok {
		return nil, false
	}
	var out []string
	for _, e := range liste {
		m, _ := e.(map[string]any)
		nom, _ := m["groupe"].(string)
		if nom == "" {
			return nil, false
		}
		out = append(out, nom)
	}
	sort.Strings(out)
	return out, true
}

// identifiants lit au pointeur un identifiant ou une liste
// d'identifiants (chaînes non vides, ou entiers sous leur forme décimale,
// comme Decide les lit). Absent : aucun. Illisible ou trop nombreux :
// faux.
func identifiants(args map[string]any, ptr string) ([]string, bool) {
	v, trouve := pointer(args, ptr)
	if !trouve || v == nil {
		return nil, true
	}
	elements, liste := v.([]any)
	if !liste {
		elements = []any{v}
	}
	if len(elements) > SujetsMax {
		return nil, false
	}
	vus := map[string]bool{}
	var out []string
	for _, e := range elements {
		id, ok := identifiant(e)
		if !ok {
			continue // la feuille sujet le verra illisible
		}
		if !vus[id] {
			vus[id] = true
			out = append(out, id)
		}
	}
	return out, true
}

func identifiant(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, x != ""
	case float64:
		// Comme Decide : un entier, sous sa forme décimale complète.
		if !math.IsInf(x, 0) && x == math.Trunc(x) {
			return strconv.FormatFloat(x, 'f', -1, 64), true
		}
	}
	return "", false
}

// pointer résout un pointeur JSON (RFC 6901) dans les arguments.
func pointer(racine any, ptr string) (any, bool) {
	if ptr == "" {
		return racine, true
	}
	if !strings.HasPrefix(ptr, "/") {
		return nil, false
	}
	cur := racine
	for _, seg := range strings.Split(ptr[1:], "/") {
		seg = strings.ReplaceAll(strings.ReplaceAll(seg, "~1", "/"), "~0", "~")
		switch n := cur.(type) {
		case map[string]any:
			v, ok := n[seg]
			if !ok {
				return nil, false
			}
			cur = v
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(n) {
				return nil, false
			}
			cur = n[i]
		default:
			return nil, false
		}
	}
	return cur, true
}
