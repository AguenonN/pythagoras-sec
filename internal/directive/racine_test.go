package directive

import (
	"strings"
	"testing"
	"time"
)

// Correctif signalé (session gate-v2) : sous un quantificateur, $racine/…
// lit les arguments et non l'élément courant.
func TestRacine_SousQuantificateur(t *testing.T) {
	doc := `{"jeu": {"id": "racine", "version": 1, "date_effet": "2026-09-01T00:00:00Z", "auteur": "j"},
	 "directives": [{"id": "Q-1", "version": 1, "statut": "active", "date_effet": "2026-09-01T00:00:00Z",
	  "texte_fr": "Une ligne au-dessus du plafond du dossier est refusée.", "fondements": [{"texte": "x"}],
	  "declencheur": {"evenement": "appel_outil", "cible": {"serveur": "s", "outil": "o"}},
	  "condition": {"existe": {"champ": "/lignes", "condition": {"argument": {"champ": "/montant", "op": ">", "valeur": 100}}}},
	  "effets": [{"effet": "refuser"}]}]}`
	// La valeur seuil vient de la racine : on compare en deux temps, la
	// ligne doit dépasser 100 ET le dossier doit être « sensible ».
	doc = strings.Replace(doc, `{"argument": {"champ": "/montant", "op": ">", "valeur": 100}}`,
		`{"et": [{"argument": {"champ": "/montant", "op": ">", "valeur": 100}}, {"argument": {"champ": "$racine/categorie", "op": "==", "valeur": "sensible"}}]}`, 1)
	j, err := Charger([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	h := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	appel := func(cat any) Decision {
		args := map[string]any{"lignes": []any{map[string]any{"montant": 50.0}, map[string]any{"montant": 500.0}}}
		if cat != nil {
			args["categorie"] = cat
		}
		return Decide(*j, EtatSession{}, Appel{Serveur: "s", Outil: "o", Arguments: args, Horodatage: h})
	}
	if d := appel("sensible"); d.Effet != Refuser || d.Applications[0].Resultat != Vrai {
		t.Fatalf("sensible : %+v", d.Applications)
	}
	if d := appel("courant"); d.Effet != Autoriser {
		t.Fatalf("courant : %+v", d.Applications)
	}
	if d := appel(nil); d.Effet != Refuser || d.Applications[0].Resultat != Indetermine {
		t.Fatalf("absent : %+v", d.Applications)
	}

	// Hors quantificateur, ou mal formé : refusé au chargement.
	seul := `{"jeu": {"id": "racine", "version": 1, "date_effet": "2026-09-01T00:00:00Z", "auteur": "j"},
	 "directives": [{"id": "Q-2", "version": 1, "statut": "active", "date_effet": "2026-09-01T00:00:00Z",
	  "texte_fr": "t", "fondements": [{"texte": "x"}],
	  "declencheur": {"evenement": "appel_outil", "cible": {"serveur": "s"}},
	  "condition": {"argument": {"champ": "$racine/categorie", "op": "present"}},
	  "effets": [{"effet": "refuser"}]}]}`
	if _, err := Charger([]byte(seul)); err == nil || !strings.Contains(err.Error(), "sous un quantificateur") {
		t.Fatalf("$racine hors quantificateur : %v", err)
	}
	vide := strings.Replace(doc, `$racine/categorie`, `$racine`, 1)
	if _, err := Charger([]byte(vide)); err == nil {
		t.Fatal("$racine sans pointeur accepté")
	}
}
