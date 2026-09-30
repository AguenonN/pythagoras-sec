package directive

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"mcprogue/internal/approval"
)

// Attestation est ce que le registre a observé sur le serveur résolu, figé à
// la connexion et transmis tel quel à Decide. Decide ne croit pas Verifiee :
// il recalcule la vérification à partir de Mode, Attendu, Observe et Schemas
// (Valide), pour qu'aucun appelant ne puisse attester un serveur en posant
// un booléen. Verifiee reste porté pour le journal et l'affichage.
type Attestation struct {
	Mode     string // spiffe | tls-pin | binaire | aucune
	Attendu  string
	Observe  string
	Schemas  bool // catalogue conforme au pin
	Verifiee bool // déclaré par le registre ; ignoré par Decide
	Detail   string
}

// Valide recalcule la vérification : un mode connu, une valeur attendue non
// vide, l'observé égal à l'attendu et le catalogue conforme. Tout le reste
// est un serveur non attesté.
func (a Attestation) Valide() bool {
	switch a.Mode {
	case "spiffe", "tls-pin", "binaire":
	default:
		return false
	}
	return a.Attendu != "" && a.Attendu == a.Observe && a.Schemas
}

// Resultat d'une consultation, vu par l'engine.
type Resultat string

const (
	ResultatOK           Resultat = "ok"
	ResultatErreur       Resultat = "erreur"       // le serveur a répondu par une erreur
	ResultatIndisponible Resultat = "indisponible" // source injoignable ou hors délai
	ResultatRefuse       Resultat = "refuse"       // le serveur de destination a refusé
)

// Consultation est un appel que l'engine a transmis à un serveur. Un appel
// refusé par l'engine n'est jamais une consultation : la provenance ne
// retient que ce qui est réellement sorti.
type Consultation struct {
	Seq              int // rang dans la session, attribué par l'engine
	Serveur          string
	Outil            string
	Horodatage       time.Time
	Empreinte        string // action_digest de l'appel transmis
	Resultat         Resultat
	EmpreinteReponse string // sha256 de la réponse canonique, "" si rien n'est revenu

	// v2, consignés au journal avec la consultation. Etiquettes sont celles
	// du registre à la consultation ; VersionRegistre vide : étiquettes
	// inconnues. Source est ce que le serveur a déclaré de sa réponse.
	Etiquettes      []string
	VersionRegistre string
	Source          *SourceReponse
}

// EtatSession est tenu par l'engine, jamais déclaré par l'agent.
type EtatSession struct {
	ID            string
	Appelant      string // identité prouvée par le transport, "" sinon
	Consultations []Consultation
}

// Appel est ce que l'engine sait d'un tools/call au moment de décider.
type Appel struct {
	Serveur     string
	Outil       string
	Arguments   map[string]any
	Attestation Attestation
	Horodatage  time.Time

	// v2 : tout vient de l'appelant de Decide, rien n'est lu par Decide.
	// VersionRegistre vide : Acces et EtiquettesCible sont inconnus.
	Acces            Acces
	EtiquettesCible  []string
	VersionRegistre  string
	Demandeur        *Demandeur // nil : non résolu
	Sujets           []Sujet    // personnes concernées résolues, par empreinte
	SousAgentDeclare string     // _meta.sous_agent, déclaratif
}

// Application est une directive dont la cible correspondait, avec ce qu'elle
// a demandé et si elle a compté. Toutes vont au journal, appliquées ou non.
type Application struct {
	ID            string
	Version       int
	Verrou        Verrou
	Fondement     Fondement
	Statut        Statut
	Effet         Effet // ce que la directive demande, si déclenchée
	Declenchee    bool  // la condition/provenance/attestation a déclenché
	Appliquee     bool  // déclenchée ET statut active
	Indetermine   bool
	RevisionEchue bool
	ChampsRetires []string
	Motif         string

	// v2 : résultat à trois valeurs de et(cible, condition), et tous les
	// fondements (Fondement est le premier).
	Resultat   Trivalent
	Fondements []Fondement
}

// Decision est le résultat de Decide.
type Decision struct {
	Effet Effet
	// Arguments à transmettre : identiques à l'entrée sauf caviardage.
	Arguments     map[string]any
	ChampsRetires []string
	// EmpreinteOrigine porte sur les arguments reçus, EmpreinteTransmise sur
	// Arguments. C'est la seconde qu'un jeton d'approbation doit viser.
	EmpreinteOrigine   string
	EmpreinteTransmise string
	Applications       []Application
	Alertes            []string
	Motif              string

	// v2, triés. Validation n'est posée que si Effet est attendre_validation.
	Notifications   []Notification
	Marques         []Marque
	Validation      *Validation
	VersionRegistre string
}

// Decide évalue un appel contre un jeu. Pure : pas d'horloge (l'horodatage
// est dans l'appel), pas d'I/O, pas de vérification de jeton (l'engine la
// fait ensuite sur EmpreinteTransmise). Les directives sont parcourues dans
// l'ordre du jeu, ce qui rend le caviardage cumulé déterministe.
func Decide(jeu Jeu, etat EtatSession, appel Appel) Decision {
	args := cloner(appel.Arguments)
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
		if d.EstV2() {
			// Schéma figé, évaluateur à venir : fermé par défaut.
			dec.Effet = Refuser
			dec.Applications = append(dec.Applications, Application{
				ID: d.ID, Version: d.Version, Statut: d.Statut, Effet: Refuser,
				Declenchee: true, Appliquee: true, Motif: "directive v2 pas encore évaluée : refus par défaut",
			})
			continue
		}
		if !cibleCorrespond(d.Cible, appel.Serveur, appel.Outil) {
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
			} else if !dansPerimetre(d.Perimetre, etat.Appelant) {
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
			vrai, indetermine, detail := evaluerConditions(d.Condition, args)
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
			quoi, ok := provenanceCorrespond(d.Provenance, etat.Consultations)
			app.Declenchee = ok
			if ok {
				notes = append(notes, "la session a consulté "+quoi)
			} else {
				notes = append(notes, "aucune consultation correspondante dans la session")
			}
		case SourceAttestee:
			if !appel.Attestation.Valide() {
				app.Declenchee = true
				notes = append(notes, "attestation du serveur en échec ("+decrireAttestation(appel.Attestation)+")")
			} else {
				notes = append(notes, "serveur attesté ("+decrireAttestation(appel.Attestation)+")")
			}
		case ChampsAutorises:
			filtres, enleves := filtrer(args, d.Champs.Autorises)
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
			dec.Effet = plusFort(dec.Effet, app.Effet)
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
	dec.Motif = motif(dec)
	return dec
}

// motif résume la décision en une phrase pour l'agent et le journal.
func motif(dec Decision) string {
	var ids []string
	for _, a := range dec.Applications {
		if a.Appliquee && a.Effet == dec.Effet {
			ids = append(ids, fmt.Sprintf("%s v%d (%s, %s)", a.ID, a.Version, a.Verrou, citer(a.Fondement)))
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

func citer(f Fondement) string {
	if f.Article != "" {
		return f.Texte + " art. " + f.Article
	}
	return f.Texte
}

func decrireAttestation(a Attestation) string {
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

func cibleCorrespond(c Cible, serveur, outil string) bool {
	return (c.Serveur == "*" || c.Serveur == serveur) && (c.Outil == "*" || c.Outil == outil)
}

func dansPerimetre(p *Perimetre, appelant string) bool {
	for _, a := range p.Appelants {
		if a == "*" || a == appelant {
			return true
		}
	}
	return false
}

// provenanceCorrespond dit si une consultation de la session satisfait la
// provenance, et laquelle.
func provenanceCorrespond(p *Provenance, consultations []Consultation) (string, bool) {
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
func evaluerConditions(conds []Condition, args map[string]any) (vrai, indetermine bool, detail string) {
	var faux, indet []string
	for _, c := range conds {
		v, trouve := pointer(args, c.Champ)
		res, ok := evaluer(c, v, trouve)
		switch {
		case !ok:
			indet = append(indet, c.Champ+" "+string(c.Op)+" "+valeurTexte(c.Valeur)+" (champ absent ou type inattendu)")
		case !res:
			faux = append(faux, c.Champ+" "+string(c.Op)+" "+valeurTexte(c.Valeur)+" faux")
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
func evaluer(c Condition, v any, trouve bool) (bool, bool) {
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
		a, okA := nombre(v)
		b, okB := nombre(c.Valeur)
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
		eq, ok := egaux(v, c.Valeur)
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
			if eq, ok := egaux(v, e); ok && eq {
				return true, true
			}
		}
		return false, true
	}
	return false, false
}

// egaux compare deux scalaires JSON : nombres entre eux, chaînes entre elles,
// booléens entre eux. Types différents : indéterminé.
func egaux(a, b any) (bool, bool) {
	if x, ok := nombre(a); ok {
		y, ok := nombre(b)
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
func nombre(v any) (float64, bool) {
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

func valeurTexte(v any) string {
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
func pointer(args map[string]any, ptr string) (any, bool) {
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
func filtrer(args map[string]any, autorises []string) (map[string]any, []string) {
	var enleves []string
	out := filtrerObjet(args, "", autorises, &enleves)
	sort.Strings(enleves)
	return out, enleves
}

func filtrerObjet(obj map[string]any, chemin string, autorises []string, enleves *[]string) map[string]any {
	out := make(map[string]any, len(obj))
	for k, v := range obj {
		p := chemin + "/" + echapper(k)
		switch {
		case couvert(p, autorises):
			out[k] = v
		case surLeChemin(p, autorises):
			if sub, ok := v.(map[string]any); ok {
				out[k] = filtrerObjet(sub, p, autorises, enleves)
			} else {
				*enleves = append(*enleves, p)
			}
		default:
			*enleves = append(*enleves, p)
		}
	}
	return out
}

func couvert(p string, autorises []string) bool {
	for _, a := range autorises {
		if p == a || strings.HasPrefix(p, a+"/") {
			return true
		}
	}
	return false
}

func surLeChemin(p string, autorises []string) bool {
	for _, a := range autorises {
		if strings.HasPrefix(a, p+"/") {
			return true
		}
	}
	return false
}

func echapper(k string) string {
	return strings.ReplaceAll(strings.ReplaceAll(k, "~", "~0"), "/", "~1")
}

// cloner copie en profondeur ce qu'un décodeur JSON produit, pour que le
// caviardage ne touche jamais les arguments de l'appelant.
func cloner(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return clonerValeur(m).(map[string]any)
}

func clonerValeur(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = clonerValeur(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = clonerValeur(e)
		}
		return out
	}
	return v
}
