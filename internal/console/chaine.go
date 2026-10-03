package console

import (
	"fmt"
	"time"

	"mcprogue/internal/directive"
	"mcprogue/internal/journal"
)

// libelleOrigine dit en français d'où vient une décision que l'engine a
// journalisée avec une origine (absente : une directive).
func libelleOrigine(o string) string {
	switch o {
	case "registre":
		return "Refus du registre des serveurs (attestation non valide)"
	case journal.OrigineNotification:
		return "Refus : une notification exigée n'a pas pu être remise"
	}
	return ""
}

// notifsDe rapproche les notifications que la décision demande des remises
// que l'entrée porte, par directive et destinataire.
func notifsDe(e journal.Entree) []NotifVue {
	var out []NotifVue
	for _, n := range e.Decision.Notifications {
		v := NotifVue{Directive: n.Directive, Version: n.Version, Role: n.DestinataireRole, Rythme: n.Rythme, SiEchec: n.SiEchec}
		for _, r := range e.Remises {
			if r.Directive == n.Directive && r.DestinataireRole == n.DestinataireRole {
				v.Statut, v.Canal, v.Remis = r.Statut, r.Canal, r.Remis
				break
			}
		}
		out = append(out, v)
	}
	return out
}

func approbVue(e journal.Entree) ApprobVue {
	a := e.Approbation
	return ApprobVue{TS: e.TS, Statut: a.Statut, Motif: a.Motif, Roles: a.Roles, QuatreYeux: a.QuatreYeux, Valideurs: a.Valideurs}
}

// badgeEtape est le badge d'un appel. L'effet de Decide d'un appel validé
// puis transmis reste « attendre_validation » au journal : l'écran le dit
// autorisé, parce que l'entrée approbation l'établit, et il ne le présume
// pas.
func badgeEtape(e journal.Entree, transmis bool) (string, string) {
	b := badge(e)
	if b == BadgeAttente && transmis && e.Approbation != nil && e.Approbation.Statut == journal.ApprobationAcceptee {
		return BadgeAutorise, "Autorisé après validation"
	}
	return b, libelleBadge[b]
}

// Chaine construit l'écran 1 pour une session.
func (d *Depot) Chaine(st *Etat, jeux []JeuCharge, session string) (*VueChaine, bool) {
	jeux = d.AvecTravail(jeux, []*Etat{st})
	v, ok := Chaine(st, jeux, session)
	if !ok {
		return nil, false
	}
	for i := range v.Etapes {
		for k := range v.Etapes[i].ValidePar {
			v.Etapes[i].ValidePar[k].Nom = d.NomIdentite(v.Etapes[i].ValidePar[k].Identite)
		}
	}
	if v.Demandeur != nil {
		quand := v.Ouverte
		if quand.IsZero() {
			quand = time.Now()
		}
		if f, ok := d.Fiche(v.Demandeur.Identite, quand); ok {
			v.DemandeurFiche = &f
		}
	}
	return v, true
}

// Chaine construit l'écran 1 pour une session, sans l'annuaire.
func Chaine(st *Etat, jeux []JeuCharge, session string) (*VueChaine, bool) {
	// Sans identifiant : la première session d'outils de la demande, ou, si
	// elle n'en a pas, la demande seule (etat.json, reponse.json).
	if session == "" {
		if ss := Sessions(st); len(ss) > 0 {
			session = ss[0].ID
		}
	}
	v := &VueChaine{Journal: st.Journal.ID, Session: session, Verif: st.Verif, Ecart: st.Verif.PremierEcart(),
		SousAgents: []string{}, Serveurs: []ServeurVue{}, Etapes: []Etape{}}
	if st.Dossier != nil {
		v.Demande, v.StatutSession = st.Dossier.Demande, st.Dossier.Statut
	}
	trouvee := false
	transmis := map[int]bool{}
	attestations := map[string]*journal.Attestation{}
	approbs := map[int][]ApprobVue{}
	ordre := []string{}
	note := func(s string, a *journal.Attestation) {
		if _, ok := attestations[s]; !ok {
			ordre = append(ordre, s)
		}
		if a != nil || attestations[s] == nil {
			attestations[s] = a
		}
	}
	for _, e := range st.Entrees {
		if e.Type == journal.AttestationT && e.Serveur != "" {
			note(e.Serveur, e.Attestation)
		}
		if e.Type == journal.AnnuaireCharge && v.AnnuaireRef == nil {
			v.AnnuaireRef, v.SourceIdentite = e.Annuaire, e.SourceIdentite
		}
		if e.Session != session {
			continue
		}
		switch {
		case e.Type == journal.TransmissionT:
			transmis[e.Seq] = true
		case e.Type == journal.ApprobationT && e.Approbation != nil:
			approbs[e.Seq] = append(approbs[e.Seq], approbVue(e))
		}
	}
	vusSA := map[string]bool{}
	suspects := v.Ecart
	for i, e := range st.Entrees {
		if e.Session != session {
			continue
		}
		switch e.Type {
		case journal.SessionOuverte:
			trouvee = true
			v.Appelant, v.Ouverte = e.Appelant, e.TS
			v.Demandeur = e.Demandeur
			v.Message = e.Message
			if e.SourceIdentite != "" {
				v.SourceIdentite = e.SourceIdentite
			}
			if e.Annuaire != nil {
				v.AnnuaireRef = e.Annuaire
			}
		case journal.SessionFermee:
			v.Fermee, v.Fermeture = true, e.Message
		case journal.AppelT:
			trouvee = true
			if e.Decision == nil {
				continue
			}
			if v.Jeu == nil && e.Jeu != nil {
				v.Jeu = e.Jeu
				j, ok := trouverJeu(jeux, e.Jeu)
				v.JeuConnu = ok
				v.Regles = reglesDe(j)
			}
			if v.Demandeur == nil && e.Demandeur != nil {
				v.Demandeur = e.Demandeur
			}
			jeu, _ := trouverJeu(jeux, e.Jeu)
			b, lib := badgeEtape(e, transmis[e.Seq])
			et := Etape{Index: i, Numero: i + 1, Prev: e.Prev, Acces: e.Acces, Seq: e.Seq, TS: e.TS, SousAgent: e.SousAgentDeclare, Serveur: e.Serveur, Outil: e.Outil,
				Attestation: e.Attestation, Effet: e.Decision.Effet, Origine: e.Decision.Origine, Motif: e.Decision.Motif,
				Badge: b, Libelle: lib, Transmis: transmis[e.Seq], Alertes: e.Decision.Alertes,
				Validation: e.Decision.Validation, Apps: []AppVue{},
				Suspect:        suspects >= 0 && i >= suspects,
				OrigineLibelle: libelleOrigine(e.Decision.Origine),
				Etiquettes:     e.EtiquettesCible, Notifications: notifsDe(e), DureeUs: e.Decision.DureeUs,
				Sujets: e.Sujets, NonResolus: len(e.SujetsNonResolus)}
			et.Approbations = approbs[e.Seq]
			if e.Approbation != nil && len(et.Approbations) == 0 {
				et.Approbations = []ApprobVue{approbVue(e)}
			}
			if e.Approbation != nil && e.Approbation.Statut == journal.ApprobationAcceptee && et.Transmis {
				for _, va := range e.Approbation.Valideurs {
					et.ValidePar = append(et.ValidePar, Valideur{Identite: va.Identite, Role: va.Role})
				}
			}
			if e.Resultat != nil {
				et.Statut, et.Source, et.DureeMs = e.Resultat.Statut, e.Resultat.Source, e.Resultat.DureeMs
			}
			if e.Appel != nil {
				et.Champs = e.Appel.ChampsRetires
				et.Empreinte = e.Appel.EmpreinteTransmise
				et.EmpreinteOrigine = e.Appel.EmpreinteOrigine
			}
			for _, a := range e.Decision.Applications {
				et.Evaluees++
				if a.Appliquee {
					et.Appliquees++
				}
				if a.Indetermine {
					et.Indeterm++
				}
				av := AppVue{ID: a.ID, Version: a.Version, Fondement: a.Fondement, Effet: a.Effet, Declench: a.Declenchee,
					Appliquee: a.Appliquee, Resultat: a.Resultat, Motif: a.Motif}
				if len(a.Fondements) > 0 {
					av.Fondement = a.Fondements[0]
				}
				if dd := directiveDu(jeu, a.ID, a.Version); dd != nil {
					av.Texte = dd.TexteFR
					av.Titre = Titre(dd.TexteFR)
				}
				et.Apps = append(et.Apps, av)
			}
			et.Liens = liens(e, jeu)
			v.Etapes = append(v.Etapes, et)
			for _, n := range et.Notifications {
				v.Notifications = append(v.Notifications, n)
			}
			if e.SousAgentDeclare != "" && !vusSA[e.SousAgentDeclare] {
				vusSA[e.SousAgentDeclare] = true
				v.SousAgents = append(v.SousAgents, e.SousAgentDeclare)
			}
			note(e.Serveur, e.Attestation)
		}
	}
	if !trouvee && session == "" && (st.Dossier != nil || st.Reponse != nil) {
		trouvee = true
	}
	if !trouvee {
		return nil, false
	}
	if st.Dossier != nil {
		if v.Ouverte.IsZero() && st.Dossier.Debut != nil {
			v.Ouverte = *st.Dossier.Debut
		}
		if v.Demandeur == nil && st.Dossier.Demandeur != "" {
			v.Demandeur = &journal.Demandeur{Identite: st.Dossier.Demandeur}
		}
	}
	// Un appel en attente que l'agent a repris : l'appel repris (même
	// empreinte transmise, approbation acceptée) le suit.
	for i := range v.Etapes {
		if v.Etapes[i].Badge != BadgeAttente {
			continue
		}
		for _, f := range v.Etapes[i+1:] {
			if f.Empreinte == v.Etapes[i].Empreinte && f.Empreinte != "" && len(f.ValidePar) > 0 {
				v.Etapes[i].ValideAu = f.Seq
				break
			}
		}
	}
	for _, s := range ordre {
		v.Serveurs = append(v.Serveurs, ServeurVue{Nom: s, Attestation: attestations[s]})
	}
	if n := len(st.Entrees); n > 0 {
		v.Signature = fmt.Sprintf("%d:%s", n, st.Entrees[n-1].MAC)
	}
	if st.Dossier != nil {
		v.Signature += "|" + st.Dossier.Statut
	}
	if st.Reponse != nil {
		v.Signature += "|r"
	}
	return v, true
}

// effetDe est utilisé pour qualifier les décisions rejouées.
func effetDe(e string) directive.Effet { return directive.Effet(e) }
