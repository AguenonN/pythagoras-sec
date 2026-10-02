package directive

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
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
	Fondement     Fondement // le premier de Fondements
	Statut        Statut
	Effet         Effet // ce que la directive demande, si déclenchée
	Declenchee    bool  // et(cible, condition) a déclenché un effet
	Appliquee     bool  // déclenchée ET statut active
	Indetermine   bool
	RevisionEchue bool
	ChampsRetires []string
	Motif         string

	// Résultat à trois valeurs de et(cible, condition), et tous les
	// fondements de la directive.
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

func dansPerimetre(p *Perimetre, appelant string) bool {
	for _, a := range p.Appelants {
		if a == "*" || a == appelant {
			return true
		}
	}
	return false
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
