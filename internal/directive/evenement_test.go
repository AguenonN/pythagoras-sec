package directive

import (
	"strings"
	"testing"
	"time"
)

const jeuEvenements = `{
  "jeu": {"id": "evenements", "version": 1, "date_effet": "2026-09-01T00:00:00Z", "auteur": "juriste@cabinet.fr"},
  "directives": [
    {"id": "E-001", "version": 1, "statut": "active", "date_effet": "2026-09-01T00:00:00Z",
     "texte_fr": "Pas de session pour un prestataire externe le week-end ; le RSSI est prévenu.",
     "fondements": [{"texte": "Politique de sécurité", "article": "3"}],
     "declencheur": {"evenement": "ouverture_session"},
     "condition": {"et": [{"demandeur": {"groupe": "prestataires-externes"}}, {"temps": {"jour_semaine": ["samedi", "dimanche"], "fuseau": "Europe/Paris"}}]},
     "effets": [{"effet": "refuser"}, {"effet": "notifier", "destinataire_role": "rssi", "rythme": "temps_reel", "si_echec": "continuer"}]},
    {"id": "E-002", "version": 1, "statut": "active", "date_effet": "2026-09-01T00:00:00Z",
     "texte_fr": "Toute attestation de source officielle en échec est signalée au RSSI.",
     "fondements": [{"texte": "Politique de sécurité", "article": "4"}],
     "declencheur": {"evenement": "attestation_echouee", "cible": {"serveur": "taux-officiel"}},
     "effets": [{"effet": "notifier", "destinataire_role": "rssi", "rythme": "temps_reel", "si_echec": "continuer"}, {"effet": "marquer", "etiquette": "source-compromise"}]},
    {"id": "E-003", "version": 1, "statut": "active", "date_effet": "2026-09-01T00:00:00Z",
     "texte_fr": "Toute purge du coffre est signalée au DPO dans la synthèse.",
     "fondements": [{"texte": "RGPD", "article": "5"}],
     "declencheur": {"evenement": "purge"},
     "effets": [{"effet": "notifier", "destinataire_role": "dpo", "rythme": "synthese", "si_echec": "continuer"}]}
  ]
}`

func TestDecideEvenement(t *testing.T) {
	j, err := Charger([]byte(jeuEvenements))
	if err != nil {
		t.Fatal(err)
	}
	samedi := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
	ext := &Demandeur{Identite: "b", Groupes: []string{"prestataires-externes"}}

	d := DecideEvenement(*j, EntreeEvenement{Evenement: EvOuvertureSession, Horodatage: samedi, Demandeur: ext})
	if d.Effet != Refuser || len(d.Notifications) != 1 || d.EmpreinteOrigine != "" || !strings.Contains(d.Motif, "E-001") {
		t.Fatalf("ouverture le samedi : %+v", d)
	}
	if d := DecideEvenement(*j, EntreeEvenement{Evenement: EvOuvertureSession, Horodatage: samedi.Add(48 * time.Hour), Demandeur: ext}); d.Effet != Autoriser {
		t.Fatalf("ouverture le lundi : %+v", d)
	}
	// Demandeur non résolu : indéterminé, refus par défaut.
	if d := DecideEvenement(*j, EntreeEvenement{Evenement: EvOuvertureSession, Horodatage: samedi}); d.Effet != Refuser || !d.Applications[0].Indetermine {
		t.Fatalf("sans demandeur : %+v", d)
	}

	d = DecideEvenement(*j, EntreeEvenement{Evenement: EvAttestationEchouee, Horodatage: samedi, Serveur: "taux-officiel"})
	if d.Effet != Autoriser || len(d.Notifications) != 1 || len(d.Marques) != 1 || len(d.Applications) != 1 {
		t.Fatalf("attestation : %+v", d)
	}
	if d := DecideEvenement(*j, EntreeEvenement{Evenement: EvAttestationEchouee, Horodatage: samedi, Serveur: "eurlex"}); len(d.Applications) != 0 {
		t.Fatalf("autre serveur : %+v", d)
	}
	d = DecideEvenement(*j, EntreeEvenement{Evenement: EvPurge, Horodatage: samedi})
	if len(d.Notifications) != 1 || d.Notifications[0].DestinataireRole != "dpo" || len(d.Applications) != 1 {
		t.Fatalf("purge : %+v", d)
	}
	// Un appel d'outil n'évalue jamais une directive d'évènement.
	if d := Decide(*j, EtatSession{}, Appel{Serveur: "taux-officiel", Outil: "x", Horodatage: samedi}); len(d.Applications) != 0 {
		t.Fatalf("appel : %+v", d)
	}
	if d := DecideEvenement(*j, EntreeEvenement{Evenement: EvAppelOutil}); d.Effet != Refuser {
		t.Fatal("appel_outil n'est pas un évènement")
	}
}
