package directive

import "time"

// Évaluateurs des évènements autres qu'un appel d'outil
// (docs/schema-directive-v2.md §4) : ouverture_session,
// attestation_echouee, purge. Même moteur que Decide (cible, périmètre,
// condition à trois valeurs, si_indetermine, effets), sur une entrée
// propre à l'évènement. Les feuilles qu'un évènement ne renseigne pas
// sont indéterminées ; le chargeur les refuse (FeuillesAdmises).
//
// Effets admis : notifier et marquer pour tous ; refuser pour
// ouverture_session seulement (la session n'est pas ouverte).

// EntreeEvenement est ce que l'engine sait de l'évènement.
type EntreeEvenement struct {
	Evenement  Evenement
	Horodatage time.Time
	// ouverture_session : l'appelant prouvé et le demandeur résolu.
	Appelant  string
	Demandeur *Demandeur
	// attestation_echouee : le serveur (nom logique) et l'attestation
	// observée.
	Serveur     string
	Attestation Attestation
}

// DecideEvenement évalue un évènement contre le jeu. Pure, comme Decide.
// L'effet de la décision est refuser (ouverture refusée) ou autoriser ;
// notifications et marques sont dans la décision.
func DecideEvenement(jeu Jeu, e EntreeEvenement) Decision {
	if e.Evenement == EvAppelOutil || e.Evenement == "" {
		return Decision{Effet: Refuser, Motif: "évènement invalide"}
	}
	dec := decider(jeu, EtatSession{Appelant: e.Appelant}, Appel{
		Serveur: e.Serveur, Attestation: e.Attestation, Horodatage: e.Horodatage, Demandeur: e.Demandeur,
	}, e.Evenement)
	// Ni arguments ni empreintes : ce n'est pas un appel.
	dec.Arguments, dec.EmpreinteOrigine, dec.EmpreinteTransmise = nil, "", ""
	return dec
}

// FeuillesAdmises rend, pour un évènement autre qu'appel_outil, les
// feuilles que son entrée renseigne. Les autres seraient toujours
// indéterminées : le chargeur les refuse.
func FeuillesAdmises(ev Evenement) []string {
	switch ev {
	case EvOuvertureSession:
		return []string{"appelant", "demandeur", "temps"}
	case EvAttestationEchouee:
		return []string{"attestation", "temps"}
	case EvPurge:
		return []string{"temps"}
	}
	return nil
}
