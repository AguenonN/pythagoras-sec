package gate

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/internal/annuaire"
	"mcprogue/internal/directive"
	"mcprogue/internal/journal"
	"mcprogue/internal/rejeu"
)

const jeuEvenements = `{
  "jeu": {"id": "gate-evenements", "version": 1, "date_effet": "2026-01-01T00:00:00Z", "auteur": "juriste@cabinet.fr"},
  "directives": [
    {"id": "E-001", "version": 1, "statut": "active", "date_effet": "2026-01-01T00:00:00Z",
     "texte_fr": "Pas de session pour un prestataire externe le week-end ; le RSSI est prévenu.",
     "fondements": [{"texte": "Politique de sécurité", "article": "3"}],
     "declencheur": {"evenement": "ouverture_session"},
     "condition": {"et": [{"demandeur": {"groupe": "prestataires-externes"}}, {"temps": {"jour_semaine": ["samedi", "dimanche"], "fuseau": "Europe/Paris"}}]},
     "effets": [{"effet": "refuser"}, {"effet": "notifier", "destinataire_role": "rssi", "rythme": "temps_reel", "si_echec": "continuer"}]},
    {"id": "E-002", "version": 1, "statut": "active", "date_effet": "2026-01-01T00:00:00Z",
     "texte_fr": "Toute attestation de la source de taux en échec est signalée au RSSI.",
     "fondements": [{"texte": "Politique de sécurité", "article": "4"}],
     "declencheur": {"evenement": "attestation_echouee", "cible": {"serveur": "taux-officiel"}},
     "effets": [{"effet": "notifier", "destinataire_role": "rssi", "rythme": "temps_reel", "si_echec": "continuer"}, {"effet": "marquer", "etiquette": "source-compromise"}]},
    {"id": "E-003", "version": 1, "statut": "active", "date_effet": "2026-01-01T00:00:00Z",
     "texte_fr": "Toute purge du coffre figure dans la synthèse du DPO.",
     "fondements": [{"texte": "RGPD", "article": "5"}],
     "declencheur": {"evenement": "purge"},
     "effets": [{"effet": "notifier", "destinataire_role": "dpo", "rythme": "synthese", "si_echec": "continuer"}]}
  ]
}`

func monterEvenements(t *testing.T, demandeur string, modifier func(*banc)) *banc {
	t.Helper()
	l, cles := annuaireTest(t)
	return monter(t, func(b *banc) {
		b.docJeu = jeuEvenements
		jeton, _ := annuaire.SignerIdentite(cles.Identites[demandeur], demandeur, demandeur, b.horloge, time.Hour)
		b.config = func(c *Config) { c.Annuaire, c.JetonDemandeur = l, jeton }
		if modifier != nil {
			modifier(b)
		}
	})
}

func rejouerEvenements(t *testing.T, b *banc, es []journal.Entree) []rejeu.RejeuEvenement {
	t.Helper()
	evs, err := rejeu.RejouerEvenements(es, rejeu.Catalogue{b.jeu})
	if err != nil {
		t.Fatal(err)
	}
	return evs
}

// Ouverture refusée : un prestataire externe un dimanche (l'horloge du
// banc est le dimanche 4 octobre 2026). Aucun appel ne passe.
func TestEvenement_OuvertureRefusee(t *testing.T) {
	b := monterEvenements(t, "bruno.prestataire@exemple.test", nil)
	r := appelerAgent(t, b.agent(t), "taux-officiel__get_taux", map[string]any{"devise": "EUR"})
	if !r.IsError || !strings.Contains(texte(r), "fermée") || len(b.taux.appels()) != 0 {
		t.Fatalf("%s", texte(r))
	}
	o := ouverture(t, b)
	if o.Decision == nil || o.Decision.Effet != "refuser" || o.Jeu == nil || len(o.Remises) != 1 {
		t.Fatalf("session_ouverte %+v", o)
	}
	fermee := false
	for _, e := range b.entrees(t) {
		fermee = fermee || (e.Type == journal.SessionFermee && strings.Contains(e.Message, "ouverture refusée") && strings.Contains(e.Message, "E-001"))
	}
	if !fermee {
		t.Fatal("pas de session_fermee citant le refus")
	}
	evs := rejouerEvenements(t, b, b.entrees(t))
	if len(evs) != 1 || len(evs[0].Ecarts) != 0 || evs[0].Effet != "refuser" {
		t.Fatalf("rejeu %+v", evs)
	}
	// Groupes du demandeur falsifiés : l'ouverture rejouée diffère.
	es := b.entrees(t)
	for i := range es {
		if es[i].Type == journal.SessionOuverte {
			es[i].Demandeur.Groupes = []string{"analystes-credit"}
		}
	}
	if evs := rejouerEvenements(t, b, es); len(evs[0].Ecarts) == 0 {
		t.Fatal("falsification non détectée")
	}
}

// Ouverture autorisée (analyste), attestation en échec notifiée et
// marquée, purge notifiée dans la synthèse.
func TestEvenement_AttestationEtPurge(t *testing.T) {
	b := monterEvenements(t, "claire.analyste@exemple.test", func(b *banc) {
		b.taux.att = directive.Attestation{Mode: "binaire", Attendu: h64, Observe: "sha256:autre", Detail: "empreinte différente"}
	})
	if r := appelerAgent(t, b.agent(t), "banque-synthetique__attribuer_pret", map[string]any{"montant": 1.0, "siren": "1"}); r.IsError {
		t.Fatalf("%s", texte(r))
	}
	a := b.appels(t)[0]
	if _, err := b.coffre.PurgerEmpreintes(journal.DemandePurge{Motif: "conservation échue", Auteur: "dpo@exemple.test"}, a.Appel.EmpreinteOrigine); err != nil {
		t.Fatal(err)
	}
	var att, purge, ouv *journal.Entree
	es := b.entrees(t)
	for i := range es {
		switch {
		case es[i].Type == journal.AttestationT && es[i].Serveur == "taux-officiel":
			att = &es[i]
		case es[i].Type == journal.PurgeT:
			purge = &es[i]
		case es[i].Type == journal.SessionOuverte:
			ouv = &es[i]
		}
	}
	if att == nil || att.Decision == nil || len(att.Decision.Marques) != 1 || len(att.Remises) != 1 || att.Remises[0].Statut != journal.RemiseEchec ||
		att.Remises[0].Evenement != "attestation_echouee" {
		t.Fatalf("attestation %+v", att)
	}
	if purge == nil || purge.Decision == nil || len(purge.Remises) != 1 || purge.Remises[0].Statut != journal.RemisePlanifiee {
		t.Fatalf("purge %+v", purge)
	}
	if ouv == nil || ouv.Decision == nil || ouv.Decision.Effet != "autoriser" {
		t.Fatalf("ouverture %+v", ouv)
	}
	evs := rejouerEvenements(t, b, es)
	if len(evs) != 3 {
		t.Fatalf("%d évènements rejoués", len(evs))
	}
	for _, ev := range evs {
		if len(ev.Ecarts) != 0 {
			t.Fatalf("%s : %+v", ev.Type, ev.Ecarts)
		}
	}
	// Marque effacée de l'attestation : écart.
	for i := range es {
		if es[i].Type == journal.AttestationT && es[i].Decision != nil {
			es[i].Decision.Marques = nil
		}
	}
	n := 0
	for _, ev := range rejouerEvenements(t, b, es) {
		n += len(ev.Ecarts)
	}
	if n == 0 {
		t.Fatal("marque effacée non détectée")
	}
}

// Une borne de session atteinte ferme la connexion sans bloquer l'appel
// qui l'atteint (défaut corrigé : la fermeture attendait le gestionnaire
// de cet appel).
func TestFermetureForcee_SansInterblocage(t *testing.T) {
	b := monter(t, func(b *banc) {
		b.config = func(c *Config) { c.Session.MaxConsultations = 1 }
	})
	cs := b.agent(t)
	fait := make(chan struct{})
	go func() {
		defer close(fait)
		cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "taux-officiel__get_taux", Arguments: map[string]any{"devise": "EUR"}})
	}()
	select {
	case <-fait:
	case <-time.After(10 * time.Second):
		t.Fatal("interblocage à la fermeture forcée")
	}
}
