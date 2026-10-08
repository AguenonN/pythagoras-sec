package directive

import (
	"crypto/sha256"
	"encoding/hex"
)

// Schéma v2 : une directive porte l'enveloppe de provenance (schema.go), un
// déclencheur, une condition en arbre et une liste d'effets. Le contrat
// complet est docs/schema-directive-v2.md. C'est la seule forme admise :
// un document au format v1 est refusé au chargement (ErrFormatV1).

// Evenement est ce qui déclenche l'évaluation d'une directive : appel_outil
// par Decide, les autres par DecideEvenement (evenement.go).
type Evenement string

const (
	EvAppelOutil         Evenement = "appel_outil"
	EvOuvertureSession   Evenement = "ouverture_session"
	EvAttestationEchouee Evenement = "attestation_echouee"
	EvPurge              Evenement = "purge"
)

// Acces est le grant du registre sur un outil.
type Acces string

const (
	AccesLecture Acces = "read"
	AccesAction  Acces = "action"
)

// CibleV2 combine en ET ses clés présentes. serveur et outil admettent "*" ;
// etiquette et acces viennent du registre.
type CibleV2 struct {
	Serveur   string `json:"serveur,omitempty"`
	Outil     string `json:"outil,omitempty"`
	Etiquette string `json:"etiquette,omitempty"`
	Acces     Acces  `json:"acces,omitempty"`
}

// Declencheur dit quand la directive est évaluée.
type Declencheur struct {
	Evenement Evenement `json:"evenement"`
	Cible     *CibleV2  `json:"cible,omitempty"`
}

// Noeud est un nœud de l'arbre de condition : exactement un champ posé.
type Noeud struct {
	Et     []*Noeud        `json:"et,omitempty"`
	Ou     []*Noeud        `json:"ou,omitempty"`
	Non    *Noeud          `json:"non,omitempty"`
	Existe *Quantificateur `json:"existe,omitempty"`
	Tous   *Quantificateur `json:"tous,omitempty"`

	Argument          *Condition           `json:"argument,omitempty"`
	AConsulte         *FeuilleConsultation `json:"session.a_consulte,omitempty"`
	Appelant          *FeuilleChaine       `json:"appelant,omitempty"`
	SousAgentDeclare  *FeuilleChaine       `json:"sous_agent_declare,omitempty"`
	Demandeur         *FeuilleDemandeur    `json:"demandeur,omitempty"`
	Attestation       *FeuilleAttestation  `json:"attestation,omitempty"`
	Temps             *FeuilleTemps        `json:"temps,omitempty"`
	EtiquetteCible    *FeuilleEtiquette    `json:"etiquette_cible,omitempty"`
	MetadonneeReponse *FeuilleMetadonnee   `json:"metadonnee_reponse,omitempty"`
	Sujet             *FeuilleSujet        `json:"sujet,omitempty"`
}

// Quantificateur évalue Condition sur chaque élément du tableau Champ.
type Quantificateur struct {
	Champ     string `json:"champ"`
	Condition *Noeud `json:"condition"`
}

// Opérateurs d'appartenance et de préfixe de la feuille argument.
const (
	OpDansV2  Op = "dans"
	OpPrefixe Op = "prefixe"
)

// FeuilleConsultation : la session a-t-elle consulté un serveur, un outil
// ("serveur/outil") ou un serveur portant une étiquette.
type FeuilleConsultation struct {
	Serveur   string  `json:"serveur,omitempty"`
	Outil     string  `json:"outil,omitempty"`
	Etiquette string  `json:"etiquette,omitempty"`
	Critere   Critere `json:"critere"`
}

// FeuilleChaine compare une chaîne : égalité, préfixe ou appartenance.
// Chaque feuille n'en admet qu'une partie (voir le schéma).
type FeuilleChaine struct {
	Egal    string   `json:"egal,omitempty"`
	Prefixe string   `json:"prefixe,omitempty"`
	Dans    []string `json:"dans,omitempty"`
}

// FeuilleDemandeur porte sur la personne pour qui l'agent travaille.
type FeuilleDemandeur struct {
	Identite *FeuilleChaine `json:"identite,omitempty"`
	Groupe   string         `json:"groupe,omitempty"`
}

// FeuilleAttestation : Verifiee est obligatoire.
type FeuilleAttestation struct {
	Verifiee *bool  `json:"verifiee"`
	Mode     string `json:"mode,omitempty"`
}

// PlageHoraire est l'intervalle local [De, A[, "HH:MM".
type PlageHoraire struct {
	De string `json:"de"`
	A  string `json:"a"`
}

// CritereDate compare la date locale "AAAA-MM-JJ".
type CritereDate struct {
	Op     Op     `json:"op"`
	Valeur string `json:"valeur"`
}

// FeuilleTemps porte sur l'horodatage de l'appel, dans Fuseau.
type FeuilleTemps struct {
	JourSemaine []string      `json:"jour_semaine,omitempty"`
	Heure       *PlageHoraire `json:"heure,omitempty"`
	Date        *CritereDate  `json:"date,omitempty"`
	Fuseau      string        `json:"fuseau"`
}

// FeuilleEtiquette : les étiquettes du serveur cible contiennent Contient.
type FeuilleEtiquette struct {
	Contient string `json:"contient"`
}

// Champs de métadonnée de réponse admis.
const (
	MetaSourceOrigine = "source.origine"
	MetaSourceDate    = "source.date"
)

// FeuilleMetadonnee porte sur les métadonnées déclarées par les serveurs
// consultés, telles que consignées.
type FeuilleMetadonnee struct {
	Serveur   string `json:"serveur,omitempty"`
	Etiquette string `json:"etiquette,omitempty"`
	Champ     string `json:"champ"`
	Op        Op     `json:"op"`
	Valeur    any    `json:"valeur"`
}

// FeuilleSujet : la personne concernée, désignée par l'identifiant au
// pointeur Champ, appartient au groupe. Champ nil ou "" : l'élément courant
// d'un quantificateur.
type FeuilleSujet struct {
	Groupe string  `json:"groupe"`
	Champ  *string `json:"champ,omitempty"`
}

// Effets propres à la v2, qui se cumulent au lieu de se dominer.
const (
	Notifier Effet = "notifier"
	Marquer  Effet = "marquer"
)

// Rythme d'une notification.
type Rythme string

const (
	TempsReel Rythme = "temps_reel"
	Synthese  Rythme = "synthese"
)

// SiEchec dit ce que l'engine fait si la notification ne part pas.
type SiEchec string

const (
	EchecRefuser   SiEchec = "refuser"
	EchecContinuer SiEchec = "continuer"
)

// EffetDirective est un élément de la liste d'effets. Chaque paramètre n'est
// admis que pour son effet.
type EffetDirective struct {
	Effet Effet `json:"effet"`

	// attendre_validation
	ParRole    string `json:"par_role,omitempty"`
	QuatreYeux *bool  `json:"quatre_yeux,omitempty"`

	// caviarder
	ChampsAutorises []string   `json:"champs_autorises,omitempty"`
	Mode            ModeChamps `json:"mode,omitempty"`

	// notifier
	DestinataireRole string  `json:"destinataire_role,omitempty"`
	Rythme           Rythme  `json:"rythme,omitempty"`
	SiEchec          SiEchec `json:"si_echec,omitempty"`

	// marquer
	Etiquette string `json:"etiquette,omitempty"`
}

// Trivalent est le résultat d'une condition en logique de Kleene forte.
// L'ordre Faux < Indetermine < Vrai fait de ET un minimum, de OU un maximum.
type Trivalent int8

const (
	Faux Trivalent = iota
	Indetermine
	Vrai
)

func (t Trivalent) String() string {
	switch t {
	case Vrai:
		return "vrai"
	case Faux:
		return "faux"
	}
	return "indetermine"
}

// MarshalText rend "vrai", "faux" ou "indetermine".
func (t Trivalent) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

// Non, Et, Ou : les tables de docs/schema-directive-v2.md §7.1.
func Non(a Trivalent) Trivalent { return Vrai - a }

func Et(a, b Trivalent) Trivalent {
	if a < b {
		return a
	}
	return b
}

func Ou(a, b Trivalent) Trivalent {
	if a > b {
		return a
	}
	return b
}

// Demandeur est la personne pour qui l'agent travaille, résolue par
// l'annuaire à l'ouverture de session et consignée au journal.
type Demandeur struct {
	Identite string
	Groupes  []string
}

// Sujet est une personne concernée, résolue par un résolveur attesté.
// L'identifiant n'est jamais porté en clair : seule son empreinte
// (EmpreinteSujet) l'est.
type Sujet struct {
	Empreinte string
	Groupes   []string
}

// EmpreinteSujet hache l'identifiant d'une personne concernée. C'est la clé
// sous laquelle l'engine fournit Appel.Sujets et que Decide recherche.
func EmpreinteSujet(id string) string {
	s := sha256.Sum256([]byte(id))
	return "sha256:" + hex.EncodeToString(s[:])
}

// SourceReponse est ce qu'un serveur a déclaré de la provenance de sa
// réponse (direct, rejeu...), tel que consigné.
type SourceReponse struct {
	Origine string
	Date    string // RFC 3339
}

// Notification est à émettre par l'engine.
type Notification struct {
	Directive        string
	Version          int
	DestinataireRole string
	Rythme           Rythme
	SiEchec          SiEchec
}

// Marque est une étiquette posée sur l'appel par une directive.
type Marque struct {
	Etiquette string
	Directive string
	Version   int
}

// Validation résume ce que demande une décision attendre_validation.
type Validation struct {
	Roles      []string // triés, sans doublon ; vide : tout validateur
	QuatreYeux bool
}
