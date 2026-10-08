// Package directive est le contrat entre le produit et l'engine : l'auteur
// de la politique écrit une directive en français avec son fondement, le
// compilateur la met sous cette forme, l'auteur la valide, et l'engine
// l'applique à chaque appel d'outil sans qu'aucun modèle ne soit sur le
// chemin.
//
// Le paquet tient trois choses et rien d'autre :
//
//   - le schéma d'un jeu de directives versionné et daté (ce fichier) ;
//   - un chargeur strict qui refuse tout champ que le schéma n'admet pas
//     (charger.go, charger_v2.go) ; le format v1 (verrous) n'est plus pris
//     en charge ;
//   - Decide, une fonction pure : (jeu, état de session, appel) → décision,
//     sans horloge, sans I/O, sans vérification de jeton (decide_v2.go). C'est
//     ce qui rend l'évaluation rejouable : le journal, les directives à une
//     date, et l'on retrouve la décision.
//
// Ce qui n'est pas une directive : les grants read/action, le pin des
// schémas, l'empreinte binaire, le pin TLS, le SPIFFE ID, le blocage du
// sampling. Ce sont des contrôles du registre, décidés par l'opérateur,
// toujours actifs, exposés en lecture seule par ControlesEnVigueur.
package directive

import "time"

// Statut d'une directive dans le jeu. Seules active et observation sont
// évaluées ; observation journalise sans appliquer.
type Statut string

const (
	Brouillon   Statut = "brouillon"
	Observation Statut = "observation"
	Active      Statut = "active"
	Retiree     Statut = "retiree"
)

// Effet est ce qu'une décision demande à l'engine. Les valeurs sont ordonnées
// du plus faible au plus fort ; Decide retient le plus fort.
type Effet string

const (
	Autoriser          Effet = "autoriser"
	Caviarder          Effet = "caviarder"
	AttendreValidation Effet = "attendre_validation"
	Refuser            Effet = "refuser"
)

func (e Effet) rang() int {
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

// plusFort retourne l'effet le plus contraignant des deux.
func plusFort(a, b Effet) Effet {
	if b.rang() > a.rang() {
		return b
	}
	return a
}

// Op est l'opérateur d'une feuille argument (ou d'une comparaison de date ou
// de métadonnée). present et absent n'ont pas de valeur ; les comparaisons
// d'ordre exigent des nombres. dans et prefixe : schema_v2.go.
type Op string

const (
	OpSup     Op = ">"
	OpSupEgal Op = ">="
	OpInf     Op = "<"
	OpInfEgal Op = "<="
	OpEgal    Op = "=="
	OpDiff    Op = "!="
	OpPresent Op = "present"
	OpAbsent  Op = "absent"
)

// Critere précise ce qui compte comme « consulté » pour session.a_consulte :
// un appel transmis quel que soit son résultat, ou seulement un appel dont
// des données sont revenues. Il est obligatoire.
type Critere string

const (
	Transmis      Critere = "transmis"
	DonneesRecues Critere = "donnees_recues"
)

// ModeChamps dit ce qu'il advient d'un champ hors liste blanche.
type ModeChamps string

const (
	ModeRefuser   ModeChamps = "refuser"
	ModeCaviarder ModeChamps = "caviarder"
)

// SiIndetermine tranche une condition qui ne peut pas être évaluée (champ
// absent, type inattendu), à la racine seulement. refuser est le défaut ;
// ignorer n'est accepté par le chargeur que validé nommément par l'auteur.
type SiIndetermine string

const (
	IndetermineRefuser SiIndetermine = "refuser"
	IndetermineIgnorer SiIndetermine = "ignorer"
)

// Fondement est ce qui fonde la directive — norme, politique interne,
// décision tracée ; le premier de Fondements est cité par le motif et repris
// dans chaque entrée de journal.
type Fondement struct {
	Texte   string `json:"texte"`
	Article string `json:"article,omitempty"`
	URL     string `json:"url,omitempty"`
}

// Condition est une feuille argument : un champ des arguments désigné par
// un pointeur JSON, un opérateur et la valeur de la directive.
type Condition struct {
	Champ  string `json:"champ"`
	Op     Op     `json:"op"`
	Valeur any    `json:"valeur,omitempty"`
}

// Perimetre restreint la directive à certains appelants (SPIFFE ID ou "*").
// Un appelant non prouvé tombe sous toute directive à périmètre : une
// restriction s'applique par défaut, jamais l'inverse.
type Perimetre struct {
	Appelants []string `json:"appelants"`
}

// Directive est une règle validée par l'auteur de la politique : une
// enveloppe de provenance, un déclencheur, une condition en arbre et une
// liste d'effets (docs/schema-directive-v2.md). Sa forme JSON est
// directive_json.go.
type Directive struct {
	ID        string
	Version   int
	Statut    Statut
	DateEffet time.Time
	// RevisionAvant est une échéance de relecture. Passée, la directive reste
	// active et une alerte est émise ; jamais de désactivation silencieuse.
	RevisionAvant          *time.Time
	TexteFR                string
	Fondements             []Fondement
	Perimetre              *Perimetre
	SiIndetermine          SiIndetermine
	SiIndetermineValidePar string
	Declencheur            *Declencheur
	// Arbre s'écrit "condition" dans le document ; nil : vrai.
	Arbre  *Noeud
	Effets []EffetDirective
}

// EstV2 vaut vrai pour toute directive chargée : il n'y a plus d'autre
// forme. Une directive construite à la main sans déclencheur n'est pas une
// directive valide (Valider la refuse, Decide la tient pour un refus).
func (d *Directive) EstV2() bool { return d.Declencheur != nil }

// MetaJeu identifie le jeu : c'est lui qui est versionné, daté et signé.
type MetaJeu struct {
	ID        string    `json:"id"`
	Version   int       `json:"version"`
	DateEffet time.Time `json:"date_effet"`
	Auteur    string    `json:"auteur"`
	// Empreinte est sha256 du tableau directives[] canonique. Absente, elle
	// est calculée au chargement ; présente, elle doit correspondre.
	Empreinte string `json:"empreinte,omitempty"`
	// Signature Ed25519 de l'empreinte, P1. Un jeu qui en porte une est
	// refusé tant que la vérification n'existe pas : mieux vaut un refus
	// qu'une signature affichée et jamais contrôlée.
	Signature string `json:"signature,omitempty"`
}

// Jeu est l'unité de chargement et de rejeu.
type Jeu struct {
	Meta       MetaJeu     `json:"jeu"`
	Directives []Directive `json:"directives"`
}

// Alerte signale un fait qui ne change pas la décision mais doit remonter au
// l'auteur, comme une échéance de relecture passée.
type Alerte struct {
	DirectiveID string
	Message     string
}

// Alertes liste les directives dont l'échéance de relecture est passée à la
// date donnée. Elles restent actives ; c'est l'appelant qui affiche.
func (j *Jeu) Alertes(a time.Time) []Alerte {
	var out []Alerte
	for _, d := range j.Directives {
		if d.RevisionAvant != nil && !a.Before(*d.RevisionAvant) {
			out = append(out, Alerte{
				DirectiveID: d.ID,
				Message: "échéance de relecture " + d.RevisionAvant.Format(time.RFC3339) +
					" dépassée ; la directive reste appliquée",
			})
		}
	}
	return out
}
