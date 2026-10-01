package journal

// Types des champs v2 de l'entrée (docs/schema-directive-v2.md §8.5). Tous
// sont portés par des champs omitempty : un journal v1 se relit à
// l'identique.

// Demandeur est la personne pour qui l'agent travaille, résolue par
// l'annuaire à l'ouverture de session.
type Demandeur struct {
	Identite string   `json:"identite"`
	Groupes  []string `json:"groupes,omitempty"`
}

// Sujet est une personne concernée, par empreinte de son identifiant.
type Sujet struct {
	Empreinte string   `json:"empreinte"`
	Groupes   []string `json:"groupes,omitempty"`
}

// Notification est une notification que la décision demande d'émettre.
type Notification struct {
	Directive        string `json:"directive"`
	Version          int    `json:"version"`
	DestinataireRole string `json:"destinataire_role"`
	Rythme           string `json:"rythme"`
	SiEchec          string `json:"si_echec"`
}

// Marque est une étiquette posée sur l'appel par une directive.
type Marque struct {
	Etiquette string `json:"etiquette"`
	Directive string `json:"directive"`
	Version   int    `json:"version"`
}

// Validation résume une décision attendre_validation.
type Validation struct {
	Roles      []string `json:"roles,omitempty"`
	QuatreYeux bool     `json:"quatre_yeux,omitempty"`
}

// RefAnnuaire identifie la version de l'annuaire utilisée.
type RefAnnuaire struct {
	ID        string `json:"id"`
	Version   int    `json:"version"`
	Empreinte string `json:"empreinte"`
}

// Statuts de remise d'une notification.
const (
	RemiseFaite     = "remise"    // remise au canal, au moins un destinataire l'a reçue
	RemiseEchec     = "echec"     // le canal n'a pu la remettre à personne
	RemisePlanifiee = "planifiee" // rythme synthese : regroupée, remise plus tard
)

// Remise est une notification et son statut de remise.
type Remise struct {
	Notification
	Statut string `json:"statut"`
	Canal  string `json:"canal"`
	Remis  int    `json:"remis,omitempty"` // nombre de destinataires atteints
	// Evenement est l'évènement qui l'a demandée : appel_outil (défaut),
	// ouverture_session, attestation_echouee, purge.
	Evenement string `json:"evenement,omitempty"`
}
