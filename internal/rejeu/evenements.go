package rejeu

import (
	"fmt"
	"reflect"
	"time"

	"mcprogue/internal/directive"
	"mcprogue/internal/journal"
)

// RejeuEvenement est le rejeu d'un évènement journalisé (ouverture de
// session, attestation en échec, purge) dont l'entrée porte une décision.
type RejeuEvenement struct {
	Type    journal.Type `json:"type"`
	Session string       `json:"session,omitempty"`
	Serveur string       `json:"serveur,omitempty"`
	TS      time.Time    `json:"ts"`
	Effet   string       `json:"effet"`
	Ecarts  []Ecart      `json:"ecarts,omitempty"`
}

// RejouerEvenements rejoue, sous le jeu qu'elle cite et à sa date, chaque
// entrée d'évènement qui porte une décision, et la confronte à ce que le
// journal affirme : décision (effet, origine, motif, applications,
// notifications, marques), remises. Toujours en cadre historique.
func RejouerEvenements(entrees []journal.Entree, jeux Jeux) ([]RejeuEvenement, error) {
	var out []RejeuEvenement
	for i, en := range entrees {
		var e directive.EntreeEvenement
		switch {
		case en.Type == journal.SessionOuverte:
			e = directive.EntreeEvenement{Evenement: directive.EvOuvertureSession, Appelant: en.Appelant,
				Demandeur: en.Demandeur.VersDirective()}
		case en.Type == journal.AttestationT && en.Attestation != nil && !en.Attestation.Valide:
			e = directive.EntreeEvenement{Evenement: directive.EvAttestationEchouee, Serveur: en.Serveur,
				Attestation: en.Attestation.VersDirective()}
		case en.Type == journal.PurgeT:
			e = directive.EntreeEvenement{Evenement: directive.EvPurge}
		default:
			continue
		}
		if en.Decision == nil || en.Jeu == nil {
			if en.Decision != nil || len(en.Remises) > 0 {
				out = append(out, RejeuEvenement{Type: en.Type, Session: en.Session, Serveur: en.Serveur, TS: en.TS,
					Ecarts: []Ecart{{Cause: CauseAlteration, Nature: Violation, Controle: ControleJeu, Detail: "décision d'évènement sans jeu cité"}}})
			}
			continue
		}
		e.Horodatage = en.TS
		r := RejeuEvenement{Type: en.Type, Session: en.Session, Serveur: en.Serveur, TS: en.TS, Effet: en.Decision.Effet}
		ecart := func(controle, format string, args ...any) {
			r.Ecarts = append(r.Ecarts, Ecart{Cause: CauseAlteration, Nature: Violation, Controle: controle, Detail: fmt.Sprintf(format, args...)})
		}
		jeu, ok := jeux.Jeu(en.Jeu.ID, en.Jeu.Version)
		if !ok {
			return out, fmt.Errorf("rejeu: %s du %s : jeu %s v%d introuvable", en.Type, en.TS.Format(time.RFC3339), en.Jeu.ID, en.Jeu.Version)
		}
		if jeu.Meta.Empreinte != en.Jeu.Empreinte {
			ecart(ControleJeu, "le jeu %s v%d fourni a l'empreinte %s, le journal cite %s", en.Jeu.ID, en.Jeu.Version, jeu.Meta.Empreinte, en.Jeu.Empreinte)
			out = append(out, r)
			continue
		}
		dec, parNotification := journal.ControleNotifications(directive.DecideEvenement(jeu, e), en.Remises)
		if string(dec.Effet) != en.Decision.Effet {
			ecart(ControleDecision, "la décision rejouée est %s, le journal affirme %s", dec.Effet, en.Decision.Effet)
		}
		if parNotification != (en.Decision.Origine == journal.OrigineNotification) || (en.Decision.Origine != "" && en.Decision.Origine != journal.OrigineNotification) {
			ecart(ControleDecision, "origine de décision %q incohérente avec les remises", en.Decision.Origine)
		}
		if detail, ok := sorties(en.Decision, dec); !ok {
			ecart(ControleDecision, "%s", detail)
		}
		var remises []journal.Notification
		for _, x := range en.Remises {
			remises = append(remises, x.Notification)
		}
		if !reflect.DeepEqual(remises, en.Decision.Notifications) {
			ecart(ControleNotification, "les notifications remises ne sont pas celles que la décision demande")
		}
		var chainees []journal.Remise
		for j := 0; j < i; j++ {
			c := entrees[j]
			if c.Type == journal.NotificationT && c.Notification != nil && c.Notification.Evenement == string(e.Evenement) &&
				c.Session == en.Session && c.Serveur == en.Serveur && c.TS.Equal(en.TS) {
				chainees = append(chainees, *c.Notification)
			}
		}
		if (len(chainees) > 0 || len(en.Remises) > 0) && !reflect.DeepEqual(chainees, en.Remises) {
			ecart(ControleNotification, "l'évènement porte %d remise(s), le journal en chaîne %d autrement", len(en.Remises), len(chainees))
		}
		out = append(out, r)
	}
	return out, nil
}
