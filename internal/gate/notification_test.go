package gate

import (
	"strings"
	"testing"

	"mcprogue/internal/journal"
	"mcprogue/internal/rejeu"
)

const jeuNotification = `{
  "jeu": {"id": "gate-notification", "version": 1, "date_effet": "2026-01-01T00:00:00Z", "auteur": "juriste@cabinet.fr"},
  "directives": [
    {"id": "N-001", "version": 1, "statut": "active", "date_effet": "2026-01-01T00:00:00Z",
     "texte_fr": "Tout prêt est signalé au juridique, en temps réel, avant d'être attribué.",
     "fondements": [{"texte": "Politique interne", "article": "9"}],
     "declencheur": {"evenement": "appel_outil", "cible": {"serveur": "banque-synthetique", "outil": "attribuer_pret"}},
     "effets": [{"effet": "notifier", "destinataire_role": "juridique", "rythme": "temps_reel", "si_echec": "refuser"},
                {"effet": "marquer", "etiquette": "pret-signale"}]},
    {"id": "N-002", "version": 1, "statut": "active", "date_effet": "2026-01-01T00:00:00Z",
     "texte_fr": "Les lectures de taux figurent dans la synthèse du RSSI.",
     "fondements": [{"texte": "Politique interne", "article": "10"}],
     "declencheur": {"evenement": "appel_outil", "cible": {"serveur": "taux-officiel", "outil": "*"}},
     "effets": [{"effet": "notifier", "destinataire_role": "rssi", "rythme": "synthese", "si_echec": "continuer"}]}
  ]
}`

func entreesDeType(b *banc, t *testing.T, ty journal.Type) []journal.Entree {
	var out []journal.Entree
	for _, e := range b.entrees(t) {
		if e.Type == ty {
			out = append(out, e)
		}
	}
	return out
}

// Sans abonné, la notification si_echec: refuser n'est pas remise :
// l'appel est refusé, d'origine notification, et rien ne part.
func TestNotification_EchecRefuse(t *testing.T) {
	b := monter(t, func(b *banc) { b.docJeu = jeuNotification })
	cs := b.agent(t)
	r := appelerAgent(t, cs, "banque-synthetique__attribuer_pret", map[string]any{"montant": 1.0, "siren": "1"})
	rf := structure[Refus](t, r)
	if !r.IsError || rf.Origine != OrigineRefusNotification || rf.Directive != "N-001" || rf.Fondement.Article != "9" ||
		!strings.Contains(rf.Motif, "n'a pas pu être remise") {
		t.Fatalf("%+v", rf)
	}
	if len(b.banque.appels()) != 0 {
		t.Fatal("transmis")
	}
	// Synthèse : planifiée, l'appel passe.
	if r := appelerAgent(t, cs, "taux-officiel__get_taux", map[string]any{"devise": "EUR"}); r.IsError {
		t.Fatalf("%s", texte(r))
	}
	ns := entreesDeType(b, t, journal.NotificationT)
	if len(ns) != 2 || ns[0].Notification.Statut != journal.RemiseEchec || ns[0].Notification.DestinataireRole != "juridique" ||
		ns[0].Notification.Canal != journal.CanalAbonnes || ns[1].Notification.Statut != journal.RemisePlanifiee || ns[0].Seq != 1 {
		t.Fatalf("notifications %+v / %+v", ns[0].Notification, ns[1].Notification)
	}
	a := b.appels(t)
	if a[0].Decision.Origine != journal.OrigineNotification || a[0].Decision.Effet != "refuser" || len(a[0].Remises) != 1 ||
		len(a[0].Decision.Marques) != 1 {
		t.Fatalf("appel %+v", a[0].Decision)
	}
	b.rejouer(t)

	// Remise effacée de l'appel, ou statut falsifié : écart.
	for nom, alterer := range map[string]func(es []journal.Entree){
		"remise effacée": func(es []journal.Entree) {
			for i := range es {
				es[i].Remises = nil
			}
		},
		"statut falsifié": func(es []journal.Entree) {
			for i := range es {
				for j := range es[i].Remises {
					es[i].Remises[j].Statut = journal.RemiseFaite
				}
				if es[i].Notification != nil {
					es[i].Notification.Statut = journal.RemiseFaite
				}
			}
		},
	} {
		es := b.entrees(t)
		alterer(es)
		rs, _ := rejeu.RejouerJournal(es, b.coffre, rejeu.Catalogue{b.jeu}, rejeu.Options{})
		if len(rs[0].Ecarts) == 0 {
			t.Errorf("%s : non détecté", nom)
		}
	}
}

// Avec un abonné (l'écran du juridique), la notification est remise
// avant la transmission et l'appel passe.
func TestNotification_RemiseAvantTransmission(t *testing.T) {
	b := monter(t, func(b *banc) { b.docJeu = jeuNotification })
	ch, fin := b.ecrivain.Abonner(64)
	defer fin()
	r := appelerAgent(t, b.agent(t), "banque-synthetique__attribuer_pret", map[string]any{"montant": 1.0, "siren": "1"})
	if r.IsError {
		t.Fatalf("%s", texte(r))
	}
	var ordre []journal.Type
	for len(ch) > 0 {
		e := <-ch
		if e.Type == journal.NotificationT || e.Type == journal.TransmissionT || e.Type == journal.AppelT {
			ordre = append(ordre, e.Type)
		}
		if e.Type == journal.NotificationT && (e.Notification.Statut != journal.RemiseFaite || e.Notification.Remis != 1) {
			t.Fatalf("remise %+v", e.Notification)
		}
	}
	if len(ordre) != 3 || ordre[0] != journal.NotificationT || ordre[1] != journal.TransmissionT {
		t.Fatalf("ordre %v", ordre)
	}
	b.rejouer(t)
}
