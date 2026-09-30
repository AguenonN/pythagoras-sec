package directive

// ConvertirV1 traduit une directive v1 en directive v2 équivalente
// (docs/schema-directive-v2.md §9). Une directive déjà v2 est rendue telle
// quelle. La traduction garde le verrou d'origine, qui reste dans le
// journal et dans les motifs : une décision v1 rejouée en v2 est identique.
//
// Les listes de la directive d'origine sont partagées, jamais modifiées.
func ConvertirV1(d Directive) Directive {
	if d.EstV2() {
		return d
	}
	out := d
	out.verrouV1 = d.Verrou
	out.Fondements = []Fondement{d.Fondement}
	cible := CibleV2{Serveur: d.Cible.Serveur, Outil: d.Cible.Outil}
	out.Declencheur = &Declencheur{Evenement: EvAppelOutil, Cible: &cible}
	out.Arbre = nil
	switch d.Verrou {
	case Interdiction:
		out.Effets = []EffetDirective{{Effet: Refuser}}
	case ApprobationHumaine:
		out.Effets = []EffetDirective{{Effet: AttendreValidation}}
	case SeuilArgument:
		et := make([]*Noeud, 0, len(d.Condition))
		for i := range d.Condition {
			c := d.Condition[i]
			et = append(et, &Noeud{Argument: &c})
		}
		out.Arbre = &Noeud{Et: et}
		out.Effets = []EffetDirective{{Effet: d.Effet}}
	case ProvenanceSession:
		critere := DonneesRecues
		var ou []*Noeud
		if pr := d.Provenance; pr != nil {
			if pr.Critere != "" {
				critere = pr.Critere
			}
			for _, s := range pr.Serveurs {
				ou = append(ou, &Noeud{AConsulte: &FeuilleConsultation{Serveur: s, Critere: critere}})
			}
			for _, o := range pr.Outils {
				ou = append(ou, &Noeud{AConsulte: &FeuilleConsultation{Outil: o, Critere: critere}})
			}
		}
		out.Arbre = &Noeud{Ou: ou}
		out.Effets = []EffetDirective{{Effet: d.Effet}}
	case ChampsAutorises:
		e := EffetDirective{Effet: Caviarder}
		if ch := d.Champs; ch != nil {
			e.ChampsAutorises, e.Mode = ch.Autorises, ch.Mode
		}
		out.Effets = []EffetDirective{e}
	case SourceAttestee:
		faux := false
		out.Arbre = &Noeud{Attestation: &FeuilleAttestation{Verifiee: &faux}}
		out.Effets = []EffetDirective{{Effet: Refuser}}
	default:
		// Verrou inconnu : le chargeur l'a refusé ; construit à la main,
		// il est fermé par défaut.
		out.Effets = []EffetDirective{{Effet: Refuser}}
	}
	return out
}

// VerrouV1 rend le verrou d'origine d'une directive traduite, "" sinon.
func (d *Directive) VerrouV1() Verrou { return d.verrouV1 }
