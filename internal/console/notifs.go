package console

import (
	"sort"
	"time"

	"mcprogue/internal/journal"
)

// NotifLigne est une notification demandée par une directive : à qui, au
// titre de quelle directive, et ce qu'il en est advenu.
type NotifLigne struct {
	Journal      string    `json:"journal"`
	Session      string    `json:"session"`
	TS           time.Time `json:"ts"`
	Destinataire string    `json:"destinataire"` // rôle
	Directive    string    `json:"directive"`
	Version      int       `json:"version"`
	Titre        string    `json:"titre,omitempty"`
	Rythme       string    `json:"rythme"`
	SiEchec      string    `json:"si_echec"`
	Statut       string    `json:"statut"` // remise | echec | planifiee
	Canal        string    `json:"canal,omitempty"`
	Remis        int       `json:"remis"`
	Evenement    string    `json:"evenement,omitempty"`
	Serveur      string    `json:"serveur,omitempty"`
	Outil        string    `json:"outil,omitempty"`
	// Source : « abonné » (notifications.jsonl) ou « journal » (à défaut).
	Source string `json:"source"`
}

// Notifications liste les notifications de toutes les sessions, les plus
// récentes d'abord : l'écran de l'abonné (notifications.jsonl de chaque
// session) ; à défaut de ce fichier, les entrées notification du journal.
func Notifications(etats []*Etat, tous []JeuCharge) []NotifLigne {
	titres := map[string]string{}
	for _, j := range tous {
		for _, d := range j.Jeu.Directives {
			titres[d.ID] = Titre(d.TexteFR)
		}
	}
	out := []NotifLigne{}
	for _, st := range etats {
		src, entrees := "abonné", st.Notifs
		if entrees == nil {
			src = "journal"
			for _, e := range st.Entrees {
				if e.Type == journal.NotificationT {
					entrees = append(entrees, e)
				}
			}
		}
		for _, e := range entrees {
			if e.Notification == nil {
				continue
			}
			n := e.Notification
			out = append(out, NotifLigne{Journal: st.Journal.ID, Session: e.Session, TS: e.TS, Destinataire: n.DestinataireRole,
				Directive: n.Directive, Version: n.Version, Titre: titres[n.Directive], Rythme: n.Rythme, SiEchec: n.SiEchec,
				Statut: n.Statut, Canal: n.Canal, Remis: n.Remis, Evenement: n.Evenement, Serveur: e.Serveur, Outil: e.Outil, Source: src})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TS.After(out[j].TS) })
	return out
}
