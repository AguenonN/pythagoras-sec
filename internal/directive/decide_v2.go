package directive

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"mcprogue/internal/approval"
)

// Decide évalue un appel contre un jeu. Pure : pas d'horloge (l'horodatage
// est dans l'appel), pas d'I/O, pas de vérification de jeton (l'engine la
// fait ensuite sur EmpreinteTransmise), aucune entrée lue ailleurs que dans
// ses arguments. Les directives sont parcourues dans l'ordre du jeu, ordre
// canonique fixé par le document haché ; le caviardage cumulé en dépend.
//
// Toute directive est évaluée sous sa forme v2 : une directive v1 est
// d'abord traduite par ConvertirV1, et garde ses motifs v1 à l'identique.
func Decide(jeu Jeu, etat EtatSession, appel Appel) Decision {
	args := cloner(appel.Arguments)
	dec := Decision{
		Effet:            Autoriser,
		EmpreinteOrigine: approval.Digest(appel.Outil, appel.Arguments),
		VersionRegistre:  appel.VersionRegistre,
	}
	retires := map[string]bool{}
	var roles []string
	quatreYeux := false

	for _, brute := range jeu.Directives {
		d := ConvertirV1(brute)
		if d.Statut != Active && d.Statut != Observation {
			continue
		}
		if d.DateEffet.After(appel.Horodatage) {
			continue // non-rétroactivité
		}
		if d.Declencheur.Evenement != EvAppelOutil {
			continue // évènement réservé, pas un tools/call
		}
		rCible, noteCible := cibleV2(d.Declencheur.Cible, &appel)
		if rCible == Faux {
			continue
		}
		v1 := d.verrouV1 != ""
		app := Application{
			ID: d.ID, Version: d.Version, Verrou: d.verrouV1, Fondement: premierFondement(d),
			Statut: d.Statut, Effet: effetDeclare(d),
		}
		if !v1 {
			app.Fondements = d.Fondements
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
		if noteCible != "" {
			notes = append(notes, noteCible)
		}

		// Condition, puis et(cible, condition) : si_indetermine ne tranche
		// que ce résultat-là.
		ctx := contexte{etat: &etat, appel: &appel}
		res, texte := Vrai, ""
		if d.Arbre != nil {
			res, texte = ctx.noeud(d.Arbre, args)
		}
		res = Et(rCible, res)
		app.Resultat = res
		if v1 {
			notes = append(notes, notesV1(d, res, texte, etat.Consultations)...)
		} else if d.Arbre != nil {
			notes = append(notes, "condition : "+texte)
		} else {
			notes = append(notes, "sans condition")
		}

		declenchee, force := false, false
		switch res {
		case Vrai:
			declenchee = true
		case Indetermine:
			app.Indetermine = true
			if d.SiIndetermine == IndetermineIgnorer {
				notes = append(notes, "condition indéterminée ignorée sur validation de "+d.SiIndetermineValidePar)
			} else {
				declenchee, force = true, true
				notes = append(notes, "condition indéterminée, refus par défaut")
			}
		}

		var notifs []Notification
		var marques []Marque
		var rolesD []string
		quatreD := false
		if declenchee {
			effet, produit := Autoriser, false
			for _, e := range d.Effets {
				switch e.Effet {
				case Refuser:
					effet, produit = plusFort(effet, Refuser), true
				case AttendreValidation:
					effet, produit = plusFort(effet, AttendreValidation), true
					if e.ParRole != "" {
						rolesD = append(rolesD, e.ParRole)
					}
					quatreD = quatreD || (e.QuatreYeux != nil && *e.QuatreYeux)
				case Caviarder:
					filtres, enleves := filtrer(args, e.ChampsAutorises)
					if len(enleves) == 0 {
						notes = append(notes, "aucun champ hors liste")
						continue
					}
					produit = true
					app.ChampsRetires = enleves
					// Le motif cite la liste autorisée, qui vient de la
					// directive, et un nombre : jamais les clés reçues.
					liste := fmt.Sprintf("%d champ(s) hors liste (autorisés : %s)", len(enleves), strings.Join(e.ChampsAutorises, ", "))
					if e.Mode == ModeRefuser {
						effet = plusFort(effet, Refuser)
						notes = append(notes, liste)
					} else {
						effet = plusFort(effet, Caviarder)
						notes = append(notes, "caviardage de "+liste)
						if d.Statut == Active {
							args = filtres
							for _, p := range enleves {
								retires[p] = true
							}
						}
					}
				case Notifier:
					produit = true
					notifs = append(notifs, Notification{Directive: d.ID, Version: d.Version,
						DestinataireRole: e.DestinataireRole, Rythme: e.Rythme, SiEchec: e.SiEchec})
				case Marquer:
					produit = true
					marques = append(marques, Marque{Etiquette: e.Etiquette, Directive: d.ID, Version: d.Version})
				}
			}
			if force {
				effet, produit = Refuser, true
			}
			app.Declenchee = produit
			if produit {
				app.Effet = effet
			}
		}

		if app.Declenchee && d.Statut == Active {
			app.Appliquee = true
			dec.Effet = plusFort(dec.Effet, app.Effet)
			dec.Notifications = append(dec.Notifications, notifs...)
			dec.Marques = append(dec.Marques, marques...)
			if app.Effet == AttendreValidation {
				roles = append(roles, rolesD...)
				quatreYeux = quatreYeux || quatreD
			}
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
	trierSorties(&dec)
	if dec.Effet == AttendreValidation {
		dec.Validation = &Validation{Roles: sansDoublon(roles), QuatreYeux: quatreYeux}
	}
	dec.Motif = motif(dec)
	return dec
}

// cibleV2 rend la correspondance de cible : faux dès qu'une clé nommée ne
// correspond pas ; indéterminé si une clé du registre n'est pas fournie.
func cibleV2(c *CibleV2, appel *Appel) (Trivalent, string) {
	if c == nil {
		return Vrai, ""
	}
	r := Vrai
	var inconnu []string
	if c.Serveur != "" && c.Serveur != "*" && c.Serveur != appel.Serveur {
		r = Faux
	}
	if c.Outil != "" && c.Outil != "*" && c.Outil != appel.Outil {
		r = Faux
	}
	if c.Etiquette != "" {
		if appel.VersionRegistre == "" {
			r = Et(r, Indetermine)
			inconnu = append(inconnu, "étiquettes")
		} else {
			r = Et(r, vf(contient(appel.EtiquettesCible, c.Etiquette)))
		}
	}
	if c.Acces != "" {
		if appel.VersionRegistre == "" || appel.Acces == "" {
			r = Et(r, Indetermine)
			inconnu = append(inconnu, "accès")
		} else {
			r = Et(r, vf(appel.Acces == c.Acces))
		}
	}
	if r == Indetermine {
		return r, "cible indéterminée (" + strings.Join(inconnu, ", ") + " de la cible inconnus : pas de version de registre)"
	}
	return r, ""
}

// notesV1 rend les notes d'une directive traduite d'une v1, mot pour mot
// celles du Decide v1 : les journaux existants se relisent à l'identique.
func notesV1(d Directive, res Trivalent, texte string, cs []Consultation) []string {
	switch d.verrouV1 {
	case Interdiction:
		return []string{"outil interdit"}
	case ApprobationHumaine:
		return []string{"validation humaine requise"}
	case SeuilArgument:
		if res == Vrai {
			return []string{"conditions satisfaites"}
		}
		return []string{texte}
	case ProvenanceSession:
		if d.Provenance != nil {
			if quoi, ok := provenanceCorrespond(d.Provenance, cs); ok {
				return []string{"la session a consulté " + quoi}
			}
		}
		return []string{"aucune consultation correspondante dans la session"}
	case SourceAttestee:
		return []string{texte}
	}
	return nil
}

// effetDeclare est ce que la directive demande si elle se déclenche : le
// plus fort de ses effets, caviarder en mode refuser comptant pour refuser.
func effetDeclare(d Directive) Effet {
	e := Autoriser
	for _, x := range d.Effets {
		switch {
		case x.Effet == Caviarder && x.Mode == ModeRefuser:
			e = plusFort(e, Refuser)
		case x.Effet == Refuser, x.Effet == AttendreValidation, x.Effet == Caviarder:
			e = plusFort(e, x.Effet)
		}
	}
	return e
}

func premierFondement(d Directive) Fondement {
	if len(d.Fondements) > 0 {
		return d.Fondements[0]
	}
	return d.Fondement
}

// trierSorties met les notifications et les marques dans l'ordre canonique.
func trierSorties(dec *Decision) {
	sort.SliceStable(dec.Notifications, func(i, j int) bool {
		a, b := dec.Notifications[i], dec.Notifications[j]
		switch {
		case a.DestinataireRole != b.DestinataireRole:
			return a.DestinataireRole < b.DestinataireRole
		case a.Rythme != b.Rythme:
			return a.Rythme < b.Rythme
		case a.SiEchec != b.SiEchec:
			return a.SiEchec < b.SiEchec
		case a.Directive != b.Directive:
			return a.Directive < b.Directive
		}
		return a.Version < b.Version
	})
	sort.SliceStable(dec.Marques, func(i, j int) bool {
		a, b := dec.Marques[i], dec.Marques[j]
		switch {
		case a.Etiquette != b.Etiquette:
			return a.Etiquette < b.Etiquette
		case a.Directive != b.Directive:
			return a.Directive < b.Directive
		}
		return a.Version < b.Version
	})
}

func sansDoublon(l []string) []string {
	if len(l) == 0 {
		return nil
	}
	s := append([]string(nil), l...)
	sort.Strings(s)
	out := s[:1]
	for _, x := range s[1:] {
		if x != out[len(out)-1] {
			out = append(out, x)
		}
	}
	return out
}

// motif résume la décision en une phrase pour l'agent et le journal. Une
// directive v1 est citée avec son verrou, une v2 avec son seul fondement.
func motif(dec Decision) string {
	var ids []string
	for _, a := range dec.Applications {
		if a.Appliquee && a.Effet == dec.Effet {
			if a.Verrou != "" {
				ids = append(ids, fmt.Sprintf("%s v%d (%s, %s)", a.ID, a.Version, a.Verrou, citer(a.Fondement)))
			} else {
				ids = append(ids, fmt.Sprintf("%s v%d (%s)", a.ID, a.Version, citer(a.Fondement)))
			}
		}
	}
	switch dec.Effet {
	case Refuser:
		return "refusé par " + strings.Join(ids, ", ")
	case AttendreValidation:
		s := "en attente de validation humaine par " + strings.Join(ids, ", ")
		if v := dec.Validation; v != nil && len(v.Roles) > 0 {
			s += " ; rôle(s) : " + strings.Join(v.Roles, ", ")
		}
		if v := dec.Validation; v != nil && v.QuatreYeux {
			s += " ; quatre yeux"
		}
		if len(dec.ChampsRetires) > 0 {
			s += fmt.Sprintf(" ; arguments caviardés (%d champ(s))", len(dec.ChampsRetires))
		}
		return s
	case Caviarder:
		return fmt.Sprintf("autorisé après caviardage de %d champ(s) par %s", len(dec.ChampsRetires), strings.Join(ids, ", "))
	}
	return "autorisé : aucune directive applicable"
}
