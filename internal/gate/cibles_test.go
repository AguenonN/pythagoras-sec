package gate

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"mcprogue/internal/backend"
	"mcprogue/internal/chaine"
	"mcprogue/internal/directive"
	"mcprogue/internal/journal"
	"mcprogue/internal/registre"
)

// jeuCible est un jeu d'une interdiction sur la cible donnée.
func jeuCible(version int, serveur, outil string) string {
	return `{"jeu": {"id": "gate-test", "version": ` + strconv.Itoa(version) + `, "date_effet": "2026-01-01T00:00:00Z", "auteur": "j"},
  "directives": [{"id": "D-X", "version": 1, "statut": "brouillon", "date_effet": "2026-01-01T00:00:00Z",
    "texte_fr": "t", "fondement": {"texte": "RGPD"}, "verrou": "interdiction",
    "cible": {"serveur": "` + serveur + `", "outil": "` + outil + `"}, "effet": "refuser"}]}`
}

func TestVerifierCibles(t *testing.T) {
	b := monter(t, nil)
	reg := b.g.cfg.Registre
	cas := []struct {
		serveur, outil string
		veut           string // "" : admis
	}{
		{"*", "*", ""},
		{"taux-officiel", "*", ""},
		{"taux-officiel", "get_taux", ""},
		{"*", "attribuer_pret", ""},
		{"inconnu", "*", "le serveur inconnu, absent du registre"},
		{"inconnu", "get_taux", "le serveur inconnu, absent du registre"},
		{"taux-officiel", "attribuer_pret", "l'outil taux-officiel/attribuer_pret, absent du registre"},
		{"*", "exfiltrer", "l'outil exfiltrer, qu'aucun serveur du registre n'expose"},
	}
	for _, c := range cas {
		j, err := directive.Charger([]byte(jeuCible(1, c.serveur, c.outil)))
		if err != nil {
			t.Fatal(err)
		}
		err = verifierCibles(*j, reg)
		switch {
		case c.veut == "" && err != nil:
			t.Errorf("%s/%s refusé : %v", c.serveur, c.outil, err)
		case c.veut != "" && (!errors.Is(err, ErrCibleHorsRegistre) || !strings.Contains(err.Error(), c.veut)):
			t.Errorf("%s/%s : %v", c.serveur, c.outil, err)
		}
	}
}

// Au démarrage, un jeu qui vise une cible hors registre ne démarre rien,
// même si la directive fautive n'est qu'un brouillon.
func TestDemarrer_CibleHorsRegistre(t *testing.T) {
	b := monter(t, nil)
	j, err := directive.Charger([]byte(jeuCible(1, "recherche-entreprises", "*")))
	if err != nil {
		t.Fatal(err)
	}
	coffre, err := journal.OuvrirCoffre(filepath.Join(t.TempDir(), "c.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer coffre.Fermer()
	connectes := 0
	_, err = Demarrer(context.Background(), Config{
		Registre: b.g.cfg.Registre, Jeu: *j, Journal: journal.Nouveau(chaine.Nouvelle(&tampon{}, cle)), Coffre: coffre,
		Connecter: func(context.Context, registre.Serveur, backend.Options) (backend.Backend, error) {
			connectes++
			return b.taux, nil
		},
	})
	if !errors.Is(err, ErrCibleHorsRegistre) || !strings.Contains(err.Error(), "recherche-entreprises") {
		t.Fatalf("erreur %v", err)
	}
	if connectes != 0 {
		t.Fatal("aucun backend ne doit être connecté pour un jeu refusé")
	}
}

// Au rechargement, un jeu qui vise une cible hors registre est refusé :
// l'ancien reste actif, une alerte est écrite.
func TestRecharger_CibleHorsRegistre(t *testing.T) {
	b := monter(t, nil)
	avant := b.g.Jeu().Meta.Empreinte
	_, err := b.g.Recharger([]byte(jeuCible(2, "taux-officiel", "get_taux_usure")))
	if !errors.Is(err, ErrCibleHorsRegistre) || b.g.Jeu().Meta.Empreinte != avant {
		t.Fatalf("%v", err)
	}
	for _, e := range b.entrees(t) {
		if e.Type == journal.AlerteT && strings.Contains(e.Message, "taux-officiel/get_taux_usure, absent du registre") {
			return
		}
	}
	t.Fatal("pas d'alerte")
}

// jeuProvenance est un jeu d'une directive provenance_session sur une cible
// du registre, dont la provenance attend les serveurs et outils donnés.
func jeuProvenance(version int, serveurs, outils []string) string {
	q := func(xs []string) string {
		if len(xs) == 0 {
			return "[]"
		}
		return `["` + strings.Join(xs, `", "`) + `"]`
	}
	return `{"jeu": {"id": "gate-test", "version": ` + strconv.Itoa(version) + `, "date_effet": "2026-01-01T00:00:00Z", "auteur": "j"},
  "directives": [{"id": "D-P", "version": 1, "statut": "retiree", "date_effet": "2026-01-01T00:00:00Z",
    "texte_fr": "t", "fondement": {"texte": "RGPD"}, "verrou": "provenance_session",
    "cible": {"serveur": "banque-synthetique", "outil": "attribuer_pret"},
    "provenance": {"serveurs": ` + q(serveurs) + `, "outils": ` + q(outils) + `}, "effet": "refuser"}]}`
}

// Une provenance qui nomme un serveur ou un outil absent du registre est
// refusée comme une cible : la règle ne se déclencherait jamais. Même pour
// une directive retirée.
func TestVerifierCibles_Provenance(t *testing.T) {
	b := monter(t, nil)
	reg := b.g.cfg.Registre
	cas := []struct {
		serveurs, outils []string
		veut             []string // vide : admis
	}{
		{[]string{"taux-officiel"}, nil, nil},
		{nil, []string{"taux-officiel/get_taux"}, nil},
		{[]string{"taux-officiel"}, []string{"banque-synthetique/attribuer_pret"}, nil},
		{[]string{"gels-avoirs"}, nil, []string{"le serveur gels-avoirs, absent du registre"}},
		{nil, []string{"taux-officiel/get_taux_usure"}, []string{"l'outil taux-officiel/get_taux_usure, absent du registre"}},
		{nil, []string{"legifrance/lire_article"}, []string{"l'outil legifrance/lire_article, absent du registre"}},
		{[]string{"eurlex", "taux-officiel"}, []string{"rh/consulter_salarie"},
			[]string{"le serveur eurlex", "l'outil rh/consulter_salarie"}},
	}
	for _, c := range cas {
		j, err := directive.Charger([]byte(jeuProvenance(1, c.serveurs, c.outils)))
		if err != nil {
			t.Fatal(err)
		}
		err = verifierCibles(*j, reg)
		if len(c.veut) == 0 {
			if err != nil {
				t.Errorf("%v %v refusé : %v", c.serveurs, c.outils, err)
			}
			continue
		}
		if !errors.Is(err, ErrCibleHorsRegistre) {
			t.Errorf("%v %v : %v", c.serveurs, c.outils, err)
			continue
		}
		for _, v := range c.veut {
			if !strings.Contains(err.Error(), v) {
				t.Errorf("%v %v : %q ne dit pas %q", c.serveurs, c.outils, err, v)
			}
		}
	}
}

// Au démarrage comme au rechargement, exactement comme une cible.
func TestProvenanceHorsRegistre_DemarrageEtRechargement(t *testing.T) {
	b := monter(t, nil)
	j, err := directive.Charger([]byte(jeuProvenance(1, []string{"gels-avoirs"}, nil)))
	if err != nil {
		t.Fatal(err)
	}
	coffre, err := journal.OuvrirCoffre(filepath.Join(t.TempDir(), "c.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer coffre.Fermer()
	_, err = Demarrer(context.Background(), Config{
		Registre: b.g.cfg.Registre, Jeu: *j, Journal: journal.Nouveau(chaine.Nouvelle(&tampon{}, cle)), Coffre: coffre,
		Connecter: func(context.Context, registre.Serveur, backend.Options) (backend.Backend, error) {
			t.Fatal("aucun backend ne doit être connecté pour un jeu refusé")
			return nil, nil
		},
	})
	if !errors.Is(err, ErrCibleHorsRegistre) || !strings.Contains(err.Error(), "provenance le serveur gels-avoirs") {
		t.Fatalf("démarrage : %v", err)
	}

	avant := b.g.Jeu().Meta.Empreinte
	_, err = b.g.Recharger([]byte(jeuProvenance(2, nil, []string{"taux-officiel/get_taux_usure"})))
	if !errors.Is(err, ErrCibleHorsRegistre) || b.g.Jeu().Meta.Empreinte != avant {
		t.Fatalf("rechargement : %v", err)
	}
	for _, e := range b.entrees(t) {
		if e.Type == journal.AlerteT && strings.Contains(e.Message, "provenance l'outil taux-officiel/get_taux_usure") {
			return
		}
	}
	t.Fatal("pas d'alerte")
}
