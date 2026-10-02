package directive

import (
	"os"
	"strings"
	"testing"
	"time"
)

func chargerTestdata(t *testing.T) (*Jeu, error) {
	t.Helper()
	b, err := os.ReadFile("testdata/credit-pme.json")
	if err != nil {
		t.Fatal(err)
	}
	return Charger(b)
}

const meta = `"jeu": {"id": "t", "version": 1, "date_effet": "2026-10-04T08:00:00Z", "auteur": "j@c.fr"}`

// doc construit un jeu d'une directive à partir d'un fragment JSON.
func doc(directive string) string {
	return `{` + meta + `, "directives": [` + directive + `]}`
}

// av2 construit une directive v2 de refus sans condition, avec des champs
// d'enveloppe en plus (fragment JSON) et la cible donnée.
func av2(enveloppe, serveur string) string {
	return `{"id": "D-1", "version": 1, "statut": "active", "date_effet": "2026-10-04T08:00:00Z",
	"texte_fr": "t", "fondements": [{"texte": "RGPD", "article": "22"}]` + enveloppe + `,
	"declencheur": {"evenement": "appel_outil", "cible": {"serveur": "` + serveur + `", "outil": "*"}},
	"effets": [{"effet": "refuser"}]}`
}

// argv2 construit une directive v2 dont la condition est une feuille
// argument (fragment JSON).
func argv2(argument string) string {
	return dv2(`{"argument": `+argument+`}`, refuser)
}

// Les refus de l'enveloppe, du jeu et des feuilles argument ; ceux des
// nœuds, déclencheurs et effets sont dans TestChargerV2_Refus.
func TestCharger_Refus(t *testing.T) {
	cases := []struct{ nom, doc, veut string }{
		{"effet autoriser jamais admis", doc(dv2("", `[{"effet": "autoriser"}]`)), "autoriser n'est pas un effet"},
		{"caviarder sans serveur nommé", doc(dv2("", `[{"effet": "caviarder", "champs_autorises": ["/siren"], "mode": "caviarder"}]`)), "cible nommée"},
		{"séparateur dans le nom logique", doc(av2("", "taux__officiel")), "séparateur"},
		{"majuscule dans le nom logique", doc(av2("", "Taux")), "invalide"},
		{"signature non vérifiable", `{"jeu": {"id": "t", "version": 1, "date_effet": "2026-10-04T08:00:00Z", "auteur": "j", "signature": "ed25519:x"}, "directives": [` + av2("", "*") + `]}`, "signature"},
		{"empreinte fausse", `{"jeu": {"id": "t", "version": 1, "date_effet": "2026-10-04T08:00:00Z", "auteur": "j", "empreinte": "sha256:00"}, "directives": [` + av2("", "*") + `]}`, "ne correspond pas"},
		{"id en double", `{` + meta + `, "directives": [` + av2("", "*") + `, ` + av2("", "*") + `]}`, "en double"},
		{"opérateur d'ordre avec chaîne", doc(argv2(`{"champ": "/m", "op": ">", "valeur": "1"}`)), "exige une valeur numérique"},
		{"present avec valeur", doc(argv2(`{"champ": "/m", "op": "present", "valeur": 1}`)), "ne prend pas de valeur"},
		{"dans sans liste", doc(argv2(`{"champ": "/m", "op": "dans", "valeur": 1}`)), "exige une liste"},
		{"in : ancienne écriture de dans", doc(argv2(`{"champ": "/m", "op": "in", "valeur": [1]}`)), `écrire "dans"`},
		{"opérateur inconnu", doc(argv2(`{"champ": "/m", "op": "~", "valeur": 1}`)), "opérateur"},
		{"pointeur sans /", doc(argv2(`{"champ": "m", "op": ">", "valeur": 1}`)), "pointeur JSON"},
		{"revision_avant antérieure", doc(av2(`, "revision_avant": "2026-01-01T00:00:00Z"`, "*")), "revision_avant"},
		{"périmètre invalide", doc(av2(`, "perimetre": {"appelants": ["alice"]}`, "*")), "SPIFFE"},
		{"périmètre vide", doc(av2(`, "perimetre": {"appelants": []}`, "*")), "vide"},
		{"statut inconnu", doc(strings.Replace(av2("", "*"), `"statut": "active"`, `"statut": "x"`, 1)), "statut"},
		{"jeu vide", `{` + meta + `, "directives": []}`, "aucune directive"},
		{"date non RFC3339", doc(strings.Replace(av2("", "*"), `"date_effet": "2026-10-04T08:00:00Z"`, `"date_effet": "04/10/2026"`, 1)), "illisible"},
	}
	for _, c := range cases {
		t.Run(c.nom, func(t *testing.T) {
			_, err := Charger([]byte(c.doc))
			if err == nil {
				t.Fatalf("accepté à tort")
			}
			if !strings.Contains(err.Error(), c.veut) {
				t.Fatalf("erreur %q, attendu une mention de %q", err, c.veut)
			}
		})
	}
}

func TestCharger_Accepte(t *testing.T) {
	jeu, err := chargerTestdata(t)
	if err != nil {
		t.Fatal(err)
	}
	if len(jeu.Directives) != 4 {
		t.Fatalf("%d directives", len(jeu.Directives))
	}
	if !strings.HasPrefix(jeu.Meta.Empreinte, "sha256:") {
		t.Fatalf("empreinte non calculée: %q", jeu.Meta.Empreinte)
	}
	if f := jeu.Directives[0].Arbre.AConsulte; f == nil || f.Serveur != "taux-officiel" || f.Critere != DonneesRecues {
		t.Errorf("D-001 : consultation de taux-officiel, données reçues, attendue : %+v", jeu.Directives[0].Arbre)
	}
	// Revalider un jeu déjà chargé rend la même empreinte.
	e := jeu.Meta.Empreinte
	if err := Valider(jeu); err != nil || jeu.Meta.Empreinte != e {
		t.Fatalf("revalidation: %v, empreinte %s → %s", err, e, jeu.Meta.Empreinte)
	}
	// Toutes les erreurs remontent ensemble.
	_, err = Charger([]byte(doc(`{"id": "", "version": 0, "statut": "x", "date_effet": "2026-10-04T08:00:00Z", "texte_fr": "", "fondements": [],
	"declencheur": {"evenement": "appel_outil", "cible": {"serveur": "*", "outil": "*"}}, "effets": [{"effet": "refuser"}]}`)))
	if err == nil || strings.Count(err.Error(), "directive:") < 4 {
		t.Fatalf("les défauts doivent être rendus en bloc: %v", err)
	}
}

func TestAlertes(t *testing.T) {
	r := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	j := jeu(interdire("I", func(d *Directive) { d.RevisionAvant = &r }))
	if a := j.Alertes(r.Add(-time.Hour)); len(a) != 0 {
		t.Errorf("alerte prématurée: %v", a)
	}
	if a := j.Alertes(r); len(a) != 1 || a[0].DirectiveID != "I" {
		t.Errorf("alerte attendue: %v", a)
	}
}
