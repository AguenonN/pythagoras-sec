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
