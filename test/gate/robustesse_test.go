package gatee2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"mcprogue/internal/backendtest"
	"mcprogue/internal/journal"
	"mcprogue/internal/registre"
	"mcprogue/internal/session"
)

func registreSimple(t *testing.T) *registre.Registre {
	t.Helper()
	return &registre.Registre{
		Meta: registre.MetaRegistre{ID: "lab-robustesse", Version: 1, Auteur: "ops@exemple.test"},
		Serveurs: []registre.Serveur{
			backendtest.Stdio("taux-officiel", backendtest.Taux, serveurTest, empreinteServeur),
			banque(t),
		},
	}
}

// Un backend muet au-delà du délai : la consultation est indisponible, le
// verrou est rendu, et l'appel suivant de la même session avance.
func TestRobustesse_DelaiBackend(t *testing.T) {
	reg := registreSimple(t)
	if err := registre.Valider(reg); err != nil {
		t.Fatal(err)
	}
	b := demarrerAvec(t, reg, jeuImposteur, session.Config{DelaiBackend: 300 * time.Millisecond})
	debut := time.Now()
	r := appeler(t, b.cs, "taux-officiel__get_taux", map[string]any{"devise": "LENT"})
	if !r.IsError || !strings.Contains(texte(r), "indisponible") {
		t.Fatalf("%q", texte(r))
	}
	if d := time.Since(debut); d > 1500*time.Millisecond {
		t.Fatalf("le délai n'a pas borné l'appel : %v", d)
	}
	r = appeler(t, b.cs, "taux-officiel__get_taux", map[string]any{"devise": "EUR"})
	if r.IsError {
		t.Fatalf("%q", texte(r))
	}
	// Une consultation indisponible reste une consultation transmise, mais
	// pas des données reçues : la provenance (critère par défaut) ne
	// déclenche que sur le second appel, réussi.
	entrees := b.rejouer(t)
	var statuts []string
	for _, e := range entrees {
		if e.Type == journal.AppelT {
			statuts = append(statuts, e.Resultat.Statut)
		}
	}
	if len(statuts) != 2 || statuts[0] != journal.StatutIndisponible || statuts[1] != journal.StatutOK {
		t.Fatalf("statuts %v", statuts)
	}
}

// Un serveur attesté qui change son catalogue en cours de session : la
// quarantaine est journalisée, et ses appels suivants sont refusés par
// la directive d'attestation D-003.
func TestRobustesse_QuarantaineEnSession(t *testing.T) {
	reg := registreSimple(t)
	b := demarrer(t, reg, jeuImposteur)
	if r := appeler(t, b.cs, "taux-officiel__get_taux", map[string]any{"devise": "DERIVE"}); r.IsError {
		t.Fatalf("%q", texte(r))
	}
	fin := time.Now().Add(10 * time.Second)
	for b.g.Backend("taux-officiel").Attestation().Valide() {
		if time.Now().After(fin) {
			t.Fatal("pas de quarantaine")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// La revérification peut encore être en cours : on attend la
	// quarantaine proprement dite.
	for !strings.Contains(b.g.Backend("taux-officiel").Attestation().Detail, "quarantaine") {
		if time.Now().After(fin) {
			t.Fatal("pas de quarantaine")
		}
		time.Sleep(20 * time.Millisecond)
	}
	r := appeler(t, b.cs, "taux-officiel__get_taux", map[string]any{"devise": "EUR"})
	if !r.IsError || !strings.Contains(texte(r), "D-003 v1") {
		t.Fatalf("%q", texte(r))
	}
	// Le nouvel outil annoncé n'est pas exposé.
	res, err := b.cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range res.Tools {
		if strings.Contains(o.Name, "exfiltrer") {
			t.Fatal("l'outil apparu en cours de session est exposé")
		}
	}
	entrees := b.rejouer(t)
	quarantaine, motif := false, false
	for _, e := range entrees {
		if e.Type == journal.AppelT && e.Seq == 2 {
			for _, a := range e.Decision.Applications {
				motif = motif || (a.ID == "D-003" && a.Appliquee && strings.Contains(a.Motif, "quarantaine après tools/list_changed"))
			}
		}
		if e.Type == journal.AttestationT && e.Serveur == "taux-officiel" && e.Attestation != nil &&
			!e.Attestation.Valide && strings.Contains(e.Attestation.Detail, "quarantaine") {
			quarantaine = true
		}
	}
	if !quarantaine || !motif {
		t.Fatalf("quarantaine journalisée %v, citée par D-003 %v", quarantaine, motif)
	}
}

// demarrerAvec est demarrer avec une configuration de session choisie.
func demarrerAvec(t *testing.T, reg *registre.Registre, doc string, sc session.Config) *banc {
	t.Helper()
	ancien := configSession
	configSession = sc
	t.Cleanup(func() { configSession = ancien })
	return demarrer(t, reg, doc)
}
