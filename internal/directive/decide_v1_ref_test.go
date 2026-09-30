package directive

// Copie figée du Decide v1 (commit af46f0e), gardée en test comme oracle :
// toute décision sur un jeu v1 doit être identique sous Decide v2
// (TestRejeu_V1IdentiqueSousV2). Ne pas modifier.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"mcprogue/internal/approval"
)

func plusFortV1Ref(a, b Effet) Effet {
	if rangV1Ref(b) > rangV1Ref(a) {
		return b
	}
	return a
}

func rangV1Ref(e Effet) int {
	switch e {
	case Caviarder:
		return 1
	case AttendreValidation:
		return 2
	case Refuser:
		return 3
	}
	return 0
}

// Decide évalue un appel contre un jeu. Pure : pas d'horloge (l'horodatage
// est dans l'appel), pas d'I/O, pas de vérification de jeton (l'engine la
// fait ensuite sur EmpreinteTransmise). Les directives sont parcourues dans
// l'ordre du jeu, ce qui rend le caviardage cumulé déterministe.
func DecideV1Ref(jeu Jeu, etat EtatSession, appel Appel) Decision {
	args := clonerV1Ref(appel.Arguments)
	dec := Decision{
		Effet:            Autoriser,
		EmpreinteOrigine: approval.Digest(appel.Outil, appel.Arguments),
	}
	retires := map[string]bool{}

	for _, d := range jeu.Directives {
		if d.Statut != Active && d.Statut != Observation {
			continue
		}
		if d.DateEffet.After(appel.Horodatage) {
			continue // non-rétroactivité
		}
		if !cibleCorrespondV1Ref(d.Cible, appel.Serveur, appel.Outil) {
			continue
		}
		app := Application{
			ID: d.ID, Version: d.Version, Verrou: d.Verrou, Fondement: d.Fondement,
			Statut: d.Statut, Effet: d.Effet,
		}
		var notes []string
		if d.Perimetre != nil {
			if etat.Appelant == "" {
				notes = append(notes, "appelant non prouvé, périmètre appliqué par défaut")
			} else if !dansPerimetreV1Ref(d.Perimetre, etat.Appelant) {
				continue
			}
		}
		if d.RevisionAvant != nil && !appel.Horodatage.Before(*d.RevisionAvant) {
			app.RevisionEchue = true
			dec.Alertes = append(dec.Alertes, fmt.Sprintf("%s: échéance de relecture %s dépassée, directive maintenue",
				d.ID, d.RevisionAvant.Format(time.RFC3339)))
		}

		switch d.Verrou {
		case Interdiction:
			app.Declenchee = true
			notes = append(notes, "outil interdit")
		case ApprobationHumaine:
			app.Declenchee = true
			notes = append(notes, "validation humaine requise")
		case SeuilArgument:
			vrai, indetermine, detail := evaluerConditionsV1Ref(d.Condition, args)
			notes = append(notes, detail)
			switch {
			case indetermine && d.SiIndetermine == IndetermineIgnorer:
				app.Indetermine = true
				notes = append(notes, "condition indéterminée ignorée sur validation de "+d.SiIndetermineValidePar)
			case indetermine:
				app.Indetermine = true
				app.Declenchee = true
				app.Effet = Refuser
				notes = append(notes, "condition indéterminée, refus par défaut")
			default:
				app.Declenchee = vrai
			}
		case ProvenanceSession:
			quoi, ok := provenanceCorrespondV1Ref(d.Provenance, etat.Consultations)
			app.Declenchee = ok
			if ok {
				notes = append(notes, "la session a consulté "+quoi)
			} else {
				notes = append(notes, "aucune consultation correspondante dans la session")
			}
		case SourceAttestee:
			if !appel.Attestation.Valide() {
				app.Declenchee = true
				notes = append(notes, "attestation du serveur en échec ("+decrireAttestationV1Ref(appel.Attestation)+")")
			} else {
				notes = append(notes, "serveur attesté ("+decrireAttestationV1Ref(appel.Attestation)+")")
			}
		case ChampsAutorises:
			filtres, enleves := filtrerV1Ref(args, d.Champs.Autorises)
			if len(enleves) == 0 {
				notes = append(notes, "aucun champ hors liste")
				break
			}
			app.Declenchee = true
			app.ChampsRetires = enleves
			// Le motif cite la liste autorisée, qui vient de la directive,
			// et un nombre : jamais les clés reçues, que l'agent contrôle.
			liste := fmt.Sprintf("%d champ(s) hors liste (autorisés : %s)", len(enleves), strings.Join(d.Champs.Autorises, ", "))
			if d.Champs.Mode == ModeRefuser {
				app.Effet = Refuser
				notes = append(notes, liste)
			} else {
				notes = append(notes, "caviardage de "+liste)
				if d.Statut == Active {
					args = filtres
					for _, p := range enleves {
						retires[p] = true
					}
				}
			}
		}

		if app.Declenchee && d.Statut == Active {
			app.Appliquee = true
			dec.Effet = plusFortV1Ref(dec.Effet, app.Effet)
		} else if app.Declenchee {
			notes = append([]string{"observation, non appliquée"}, notes...)
		}
		app.Motif = strings.Join(notes, " ; ")
		dec.Applications = append(dec.Applications, app)
	}

	dec.Arguments = args
	for p := range retires {
		dec.ChampsRetires = append(dec.ChampsRetires, p)
	}
	sort.Strings(dec.ChampsRetires)
	dec.EmpreinteTransmise = approval.Digest(appel.Outil, args)
	dec.Motif = motifV1Ref(dec)
	return dec
}

// motif résume la décision en une phrase pour l'agent et le journal.
func motifV1Ref(dec Decision) string {
	var ids []string
	for _, a := range dec.Applications {
		if a.Appliquee && a.Effet == dec.Effet {
			ids = append(ids, fmt.Sprintf("%s v%d (%s, %s)", a.ID, a.Version, a.Verrou, citerV1Ref(a.Fondement)))
		}
	}
	switch dec.Effet {
	case Refuser:
		return "refusé par " + strings.Join(ids, ", ")
	case AttendreValidation:
		s := "en attente de validation humaine par " + strings.Join(ids, ", ")
		if len(dec.ChampsRetires) > 0 {
			s += fmt.Sprintf(" ; arguments caviardés (%d champ(s))", len(dec.ChampsRetires))
		}
		return s
	case Caviarder:
		return fmt.Sprintf("autorisé après caviardage de %d champ(s) par %s", len(dec.ChampsRetires), strings.Join(ids, ", "))
	}
	return "autorisé : aucune directive applicable"
}

func citerV1Ref(f Fondement) string {
	if f.Article != "" {
		return f.Texte + " art. " + f.Article
	}
	return f.Texte
}

func decrireAttestationV1Ref(a Attestation) string {
	if a.Mode == "" || a.Mode == "aucune" {
		return "aucune attestation"
	}
	s := a.Mode + " attendu " + a.Attendu + " observé " + a.Observe
	if !a.Schemas {
		s += ", schémas non conformes"
	}
	if a.Detail != "" {
		s += " : " + a.Detail
	}
	return s
}

func cibleCorrespondV1Ref(c Cible, serveur, outil string) bool {
	return (c.Serveur == "*" || c.Serveur == serveur) && (c.Outil == "*" || c.Outil == outil)
}

func dansPerimetreV1Ref(p *Perimetre, appelant string) bool {
	for _, a := range p.Appelants {
		if a == "*" || a == appelant {
			return true
		}
	}
	return false
}

// provenanceCorrespond dit si une consultation de la session satisfait la
// provenance, et laquelle.
func provenanceCorrespondV1Ref(p *Provenance, consultations []Consultation) (string, bool) {
	for _, c := range consultations {
		if p.Critere != Transmis && c.Resultat != ResultatOK {
			continue
		}
		for _, s := range p.Serveurs {
			if s == c.Serveur {
				return c.Serveur + " (" + string(c.Resultat) + ")", true
			}
		}
		for _, o := range p.Outils {
			if o == c.Serveur+"/"+c.Outil {
				return o + " (" + string(c.Resultat) + ")", true
			}
		}
	}
	return "", false
}

// evaluerConditions combine en ET. Une condition fausse et déterminée rend
// faux ; sinon une condition indéterminée rend indéterminé ; sinon vrai.
func evaluerConditionsV1Ref(conds []Condition, args map[string]any) (vrai, indetermine bool, detail string) {
	var faux, indet []string
	for _, c := range conds {
		v, trouve := pointerV1Ref(args, c.Champ)
		res, ok := evaluerV1Ref(c, v, trouve)
		switch {
		case !ok:
			indet = append(indet, c.Champ+" "+string(c.Op)+" "+valeurTexteV1Ref(c.Valeur)+" (champ absent ou type inattendu)")
		case !res:
			faux = append(faux, c.Champ+" "+string(c.Op)+" "+valeurTexteV1Ref(c.Valeur)+" faux")
		}
	}
	if len(faux) > 0 {
		return false, false, strings.Join(faux, ", ")
	}
	if len(indet) > 0 {
		return false, true, strings.Join(indet, ", ")
	}
	return true, false, "conditions satisfaites"
}

// evaluer rend (résultat, déterminé).
func evaluerV1Ref(c Condition, v any, trouve bool) (bool, bool) {
	switch c.Op {
	case OpPresent:
		return trouve, true
	case OpAbsent:
		return !trouve, true
	}
	if !trouve {
		return false, false
	}
	switch c.Op {
	case OpSup, OpSupEgal, OpInf, OpInfEgal:
		a, okA := nombreV1Ref(v)
		b, okB := nombreV1Ref(c.Valeur)
		if !okA || !okB {
			return false, false
		}
		switch c.Op {
		case OpSup:
			return a > b, true
		case OpSupEgal:
			return a >= b, true
		case OpInf:
			return a < b, true
		default:
			return a <= b, true
		}
	case OpEgal, OpDiff:
		eq, ok := egauxV1Ref(v, c.Valeur)
		if !ok {
			return false, false
		}
		if c.Op == OpEgal {
			return eq, true
		}
		return !eq, true
	case OpDans:
		liste, _ := c.Valeur.([]any)
		for _, e := range liste {
			if eq, ok := egauxV1Ref(v, e); ok && eq {
				return true, true
			}
		}
		return false, true
	}
	return false, false
}

// egaux compare deux scalaires JSON : nombres entre eux, chaînes entre elles,
// booléens entre eux. Types différents : indéterminé.
func egauxV1Ref(a, b any) (bool, bool) {
	if x, ok := nombreV1Ref(a); ok {
		y, ok := nombreV1Ref(b)
		return x == y, ok
	}
	switch x := a.(type) {
	case string:
		y, ok := b.(string)
		return x == y, ok
	case bool:
		y, ok := b.(bool)
		return x == y, ok
	}
	return false, false
}

// nombre accepte les formes qu'un décodeur JSON produit selon ses options.
func nombreV1Ref(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

func valeurTexteV1Ref(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return strconv.Quote(x)
	case json.Number:
		return x.String()
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// pointer résout un pointeur JSON (RFC 6901) dans les arguments. Les tableaux
// sont indexés par entier.
func pointerV1Ref(args map[string]any, ptr string) (any, bool) {
	var cur any = args
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

// filtrer ne garde des arguments que ce que la liste blanche couvre. Un
// pointeur autorisé couvre tout son sous-arbre ; un objet sur le chemin d'un
// pointeur autorisé est conservé et filtré ; un tableau est atomique : gardé
// entier s'il est couvert, retiré sinon. Les pointeurs retirés sont les
// sous-arbres maximaux enlevés, triés.
func filtrerV1Ref(args map[string]any, autorises []string) (map[string]any, []string) {
	var enleves []string
	out := filtrerObjetV1Ref(args, "", autorises, &enleves)
	sort.Strings(enleves)
	return out, enleves
}

func filtrerObjetV1Ref(obj map[string]any, chemin string, autorises []string, enleves *[]string) map[string]any {
	out := make(map[string]any, len(obj))
	for k, v := range obj {
		p := chemin + "/" + echapperV1Ref(k)
		switch {
		case couvertV1Ref(p, autorises):
			out[k] = v
		case surLeCheminV1Ref(p, autorises):
			if sub, ok := v.(map[string]any); ok {
				out[k] = filtrerObjetV1Ref(sub, p, autorises, enleves)
			} else {
				*enleves = append(*enleves, p)
			}
		default:
			*enleves = append(*enleves, p)
		}
	}
	return out
}

func couvertV1Ref(p string, autorises []string) bool {
	for _, a := range autorises {
		if p == a || strings.HasPrefix(p, a+"/") {
			return true
		}
	}
	return false
}

func surLeCheminV1Ref(p string, autorises []string) bool {
	for _, a := range autorises {
		if strings.HasPrefix(a, p+"/") {
			return true
		}
	}
	return false
}

func echapperV1Ref(k string) string {
	return strings.ReplaceAll(strings.ReplaceAll(k, "~", "~0"), "/", "~1")
}

// cloner copie en profondeur ce qu'un décodeur JSON produit, pour que le
// caviardage ne touche jamais les arguments de l'appelant.
func clonerV1Ref(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return clonerValeurV1Ref(m).(map[string]any)
}

func clonerValeurV1Ref(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = clonerValeurV1Ref(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = clonerValeurV1Ref(e)
		}
		return out
	}
	return v
}
