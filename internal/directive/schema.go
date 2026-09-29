// Package directive est le contrat entre le produit et l'engine : le juriste
// écrit une directive en français avec son fondement, le compilateur la met
// sous cette forme, le juriste la valide, et l'engine l'applique à chaque
// appel d'outil sans qu'aucun modèle ne soit sur le chemin.
//
// Le paquet tient trois choses et rien d'autre :
//
//   - le schéma d'un jeu de directives versionné et daté (ce fichier) ;
//   - un chargeur strict qui refuse tout ce que le verrou n'exige pas
//     (charger.go) ;
//   - Decide, une fonction pure : (jeu, état de session, appel) → décision,
//     sans horloge, sans I/O, sans vérification de jeton (decide.go). C'est
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

// Verrou est la liste fermée de ce qu'une directive peut demander. Les quatre
// premiers partagent l'évaluateur (cible, conditions, provenance, effet) ;
// les deux derniers ont chacun le leur.
type Verrou string

const (
	// Interdiction refuse l'outil ciblé, sans condition.
	Interdiction Verrou = "interdiction"
	// SeuilArgument refuse ou met en attente selon une condition sur les
	// arguments (montant, catégorie...).
	SeuilArgument Verrou = "seuil_argument"
	// ProvenanceSession refuse ou met en attente si la session a déjà
	// consulté certains serveurs ou outils.
	ProvenanceSession Verrou = "provenance_session"
	// ApprobationHumaine met l'outil ciblé en attente de validation.
	ApprobationHumaine Verrou = "approbation_humaine"
	// ChampsAutorises borne les champs qu'un serveur de destination reçoit :
	// caviarde ou refuse, selon champs.mode.
	ChampsAutorises Verrou = "champs_autorises_par_destination"
	// SourceAttestee refuse tout appel vers un serveur dont l'attestation
	// (registre) a échoué, en citant la directive et son fondement.
	SourceAttestee Verrou = "source_attestee"
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

// Op est l'opérateur d'une condition. present et absent n'ont pas de valeur ;
// les comparaisons d'ordre exigent des nombres.
type Op string

const (
	OpSup     Op = ">"
	OpSupEgal Op = ">="
	OpInf     Op = "<"
	OpInfEgal Op = "<="
	OpEgal    Op = "=="
	OpDiff    Op = "!="
	OpDans    Op = "in"
	OpPresent Op = "present"
	OpAbsent  Op = "absent"
)

// Critere précise ce qui compte comme « consulté » pour une provenance :
// un appel transmis quel que soit son résultat, ou seulement un appel dont
// des données sont revenues. Le défaut est donnees_recues.
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
// absent, type inattendu). refuser est le défaut ; ignorer n'est accepté par
// le chargeur que validé nommément par le juriste.
type SiIndetermine string

const (
	IndetermineRefuser SiIndetermine = "refuser"
	IndetermineIgnorer SiIndetermine = "ignorer"
)

// Fondement est la base juridique citée par la directive et reprise dans
// chaque entrée de journal.
type Fondement struct {
	Texte   string `json:"texte"`
	Article string `json:"article,omitempty"`
	URL     string `json:"url,omitempty"`
}

// Cible désigne (serveur logique, outil). "*" accepte tout sur un champ.
type Cible struct {
	Serveur string `json:"serveur"`
	Outil   string `json:"outil"`
}

// Condition porte sur un champ des arguments désigné par un pointeur JSON.
type Condition struct {
	Champ  string `json:"champ"`
	Op     Op     `json:"op"`
	Valeur any    `json:"valeur,omitempty"`
}

// Provenance déclenche si la session a consulté l'un des serveurs, ou l'un
// des outils notés "serveur/outil".
type Provenance struct {
	Serveurs []string `json:"serveurs,omitempty"`
	Outils   []string `json:"outils,omitempty"`
	Critere  Critere  `json:"critere,omitempty"`
}

// Champs est la liste blanche de pointeurs JSON qu'un serveur peut recevoir.
// La correspondance est par préfixe : "/emprunteur" couvre tout l'objet.
type Champs struct {
	Autorises []string   `json:"autorises"`
	Mode      ModeChamps `json:"mode"`
}

// Perimetre restreint la directive à certains appelants (SPIFFE ID ou "*").
// Un appelant non prouvé tombe sous toute directive à périmètre : une
// restriction s'applique par défaut, jamais l'inverse.
type Perimetre struct {
	Appelants []string `json:"appelants"`
}

// Directive est une règle validée par le juriste.
type Directive struct {
	ID        string    `json:"id"`
	Version   int       `json:"version"`
	Statut    Statut    `json:"statut"`
	DateEffet time.Time `json:"date_effet"`
	// RevisionAvant est une échéance de relecture. Passée, la directive reste
	// active et une alerte est émise ; jamais de désactivation silencieuse.
	RevisionAvant *time.Time `json:"revision_avant,omitempty"`
	TexteFR       string     `json:"texte_fr"`
	Fondement     Fondement  `json:"fondement"`
	Verrou        Verrou     `json:"verrou"`
	Cible         Cible      `json:"cible"`
	Perimetre     *Perimetre `json:"perimetre,omitempty"`

	Condition              []Condition   `json:"condition,omitempty"`
	SiIndetermine          SiIndetermine `json:"si_indetermine,omitempty"`
	SiIndetermineValidePar string        `json:"si_indetermine_valide_par,omitempty"`
	Provenance             *Provenance   `json:"provenance,omitempty"`
	Champs                 *Champs       `json:"champs,omitempty"`

	Effet Effet `json:"effet"`
}

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
// juriste, comme une échéance de relecture passée.
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
