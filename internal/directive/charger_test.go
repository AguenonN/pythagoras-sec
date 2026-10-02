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

const base = `"id": "D-1", "version": 1, "statut": "active", "date_effet": "2026-10-04T08:00:00Z",
	"texte_fr": "t", "fondement": {"texte": "RGPD", "article": "22"}`

func TestCharger_Refus(t *testing.T) {
	cases := []struct{ nom, doc, veut string }{
		{"champ inconnu", doc(`{` + base + `, "verrou": "interdiction", "cible": {"serveur": "*", "outil": "*"}, "effet": "refuser", "bonus": 1}`), "unknown field"},
		{"interdiction avec condition", doc(`{` + base + `, "verrou": "interdiction", "cible": {"serveur": "*", "outil": "*"}, "condition": [{"champ": "/a", "op": "present"}], "effet": "refuser"}`), "n'admet pas de condition"},
		{"seuil sans condition", doc(`{` + base + `, "verrou": "seuil_argument", "cible": {"serveur": "*", "outil": "*"}, "effet": "refuser"}`), "exige au moins une condition"},
		{"provenance_session sans provenance", doc(`{` + base + `, "verrou": "provenance_session", "cible": {"serveur": "*", "outil": "*"}, "effet": "refuser"}`), "exige une provenance"},
		{"approbation avec provenance", doc(`{` + base + `, "verrou": "approbation_humaine", "cible": {"serveur": "*", "outil": "*"}, "provenance": {"serveurs": ["x"]}, "effet": "attendre_validation"}`), "n'admet pas de provenance"},
		{"contradiction effet / champs.mode", doc(`{` + base + `, "verrou": "champs_autorises_par_destination", "cible": {"serveur": "partenaire", "outil": "*"}, "champs": {"autorises": ["/siren"], "mode": "caviarder"}, "effet": "refuser"}`), "exige effet caviarder"},
		{"contradiction inverse", doc(`{` + base + `, "verrou": "champs_autorises_par_destination", "cible": {"serveur": "partenaire", "outil": "*"}, "champs": {"autorises": ["/siren"], "mode": "refuser"}, "effet": "caviarder"}`), "exige effet refuser"},
		{"champs sans serveur nommé", doc(`{` + base + `, "verrou": "champs_autorises_par_destination", "cible": {"serveur": "*", "outil": "*"}, "champs": {"autorises": ["/siren"], "mode": "caviarder"}, "effet": "caviarder"}`), "exige un serveur nommé"},
		{"source_attestee effet attendre", doc(`{` + base + `, "verrou": "source_attestee", "cible": {"serveur": "taux", "outil": "*"}, "effet": "attendre_validation"}`), "non admis"},
		{"approbation effet refuser", doc(`{` + base + `, "verrou": "approbation_humaine", "cible": {"serveur": "*", "outil": "*"}, "effet": "refuser"}`), "non admis"},
		{"effet autoriser jamais admis", doc(`{` + base + `, "verrou": "interdiction", "cible": {"serveur": "*", "outil": "*"}, "effet": "autoriser"}`), "non admis"},
		{"ignorer sans validation nommée", doc(`{` + base + `, "verrou": "seuil_argument", "cible": {"serveur": "*", "outil": "*"}, "condition": [{"champ": "/m", "op": ">", "valeur": 1}], "si_indetermine": "ignorer", "effet": "refuser"}`), "exige si_indetermine_valide_par"},
		{"valide_par sans ignorer", doc(`{` + base + `, "verrou": "seuil_argument", "cible": {"serveur": "*", "outil": "*"}, "condition": [{"champ": "/m", "op": ">", "valeur": 1}], "si_indetermine_valide_par": "j", "effet": "refuser"}`), "sans si_indetermine: ignorer"},
		{"séparateur dans le nom logique", doc(`{` + base + `, "verrou": "interdiction", "cible": {"serveur": "taux__officiel", "outil": "*"}, "effet": "refuser"}`), "séparateur"},
		{"majuscule dans le nom logique", doc(`{` + base + `, "verrou": "interdiction", "cible": {"serveur": "Taux", "outil": "*"}, "effet": "refuser"}`), "invalide"},
		{"signature non vérifiable", `{"jeu": {"id": "t", "version": 1, "date_effet": "2026-10-04T08:00:00Z", "auteur": "j", "signature": "ed25519:x"}, "directives": [{` + base + `, "verrou": "interdiction", "cible": {"serveur": "*", "outil": "*"}, "effet": "refuser"}]}`, "signature"},
		{"empreinte fausse", `{"jeu": {"id": "t", "version": 1, "date_effet": "2026-10-04T08:00:00Z", "auteur": "j", "empreinte": "sha256:00"}, "directives": [{` + base + `, "verrou": "interdiction", "cible": {"serveur": "*", "outil": "*"}, "effet": "refuser"}]}`, "ne correspond pas"},
		{"id en double", `{` + meta + `, "directives": [{` + base + `, "verrou": "interdiction", "cible": {"serveur": "*", "outil": "*"}, "effet": "refuser"}, {` + base + `, "verrou": "interdiction", "cible": {"serveur": "*", "outil": "*"}, "effet": "refuser"}]}`, "en double"},
		{"opérateur d'ordre avec chaîne", doc(`{` + base + `, "verrou": "seuil_argument", "cible": {"serveur": "*", "outil": "*"}, "condition": [{"champ": "/m", "op": ">", "valeur": "1"}], "effet": "refuser"}`), "exige une valeur numérique"},
		{"present avec valeur", doc(`{` + base + `, "verrou": "seuil_argument", "cible": {"serveur": "*", "outil": "*"}, "condition": [{"champ": "/m", "op": "present", "valeur": 1}], "effet": "refuser"}`), "ne prend pas de valeur"},
		{"in sans liste", doc(`{` + base + `, "verrou": "seuil_argument", "cible": {"serveur": "*", "outil": "*"}, "condition": [{"champ": "/m", "op": "in", "valeur": 1}], "effet": "refuser"}`), "exige une liste"},
		{"opérateur inconnu", doc(`{` + base + `, "verrou": "seuil_argument", "cible": {"serveur": "*", "outil": "*"}, "condition": [{"champ": "/m", "op": "~", "valeur": 1}], "effet": "refuser"}`), "opérateur"},
		{"pointeur sans /", doc(`{` + base + `, "verrou": "seuil_argument", "cible": {"serveur": "*", "outil": "*"}, "condition": [{"champ": "m", "op": ">", "valeur": 1}], "effet": "refuser"}`), "pointeur JSON"},
		{"provenance vide", doc(`{` + base + `, "verrou": "provenance_session", "cible": {"serveur": "*", "outil": "*"}, "provenance": {}, "effet": "refuser"}`), "sans serveur ni outil"},
		{"provenance critère inconnu", doc(`{` + base + `, "verrou": "provenance_session", "cible": {"serveur": "*", "outil": "*"}, "provenance": {"serveurs": ["taux"], "critere": "x"}, "effet": "refuser"}`), "critere"},
		{"provenance outil mal formé", doc(`{` + base + `, "verrou": "provenance_session", "cible": {"serveur": "*", "outil": "*"}, "provenance": {"outils": ["taux"]}, "effet": "refuser"}`), "serveur/outil"},
		{"revision_avant antérieure", doc(`{` + base + `, "revision_avant": "2026-01-01T00:00:00Z", "verrou": "interdiction", "cible": {"serveur": "*", "outil": "*"}, "effet": "refuser"}`), "revision_avant"},
		{"périmètre invalide", doc(`{` + base + `, "perimetre": {"appelants": ["alice"]}, "verrou": "interdiction", "cible": {"serveur": "*", "outil": "*"}, "effet": "refuser"}`), "SPIFFE"},
		{"périmètre vide", doc(`{` + base + `, "perimetre": {"appelants": []}, "verrou": "interdiction", "cible": {"serveur": "*", "outil": "*"}, "effet": "refuser"}`), "vide"},
		{"verrou inconnu", doc(`{` + base + `, "verrou": "magie", "cible": {"serveur": "*", "outil": "*"}, "effet": "refuser"}`), "verrou"},
		{"statut inconnu", doc(`{"id": "D-1", "version": 1, "statut": "x", "date_effet": "2026-10-04T08:00:00Z", "texte_fr": "t", "fondement": {"texte": "RGPD"}, "verrou": "interdiction", "cible": {"serveur": "*", "outil": "*"}, "effet": "refuser"}`), "statut"},
		{"fondement manquant", doc(`{"id": "D-1", "version": 1, "statut": "active", "date_effet": "2026-10-04T08:00:00Z", "texte_fr": "t", "fondement": {}, "verrou": "interdiction", "cible": {"serveur": "*", "outil": "*"}, "effet": "refuser"}`), "fondement"},
		{"jeu vide", `{` + meta + `, "directives": []}`, "aucune directive"},
		{"date non RFC3339", doc(`{"id": "D-1", "version": 1, "statut": "active", "date_effet": "04/10/2026", "texte_fr": "t", "fondement": {"texte": "RGPD"}, "verrou": "interdiction", "cible": {"serveur": "*", "outil": "*"}, "effet": "refuser"}`), "illisible"},
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
	if jeu.Directives[0].Provenance.Critere != DonneesRecues {
		t.Errorf("le critère par défaut doit être donnees_recues")
	}
	// Revalider un jeu déjà normalisé rend la même empreinte.
	e := jeu.Meta.Empreinte
	if err := Valider(jeu); err != nil || jeu.Meta.Empreinte != e {
		t.Fatalf("revalidation: %v, empreinte %s → %s", err, e, jeu.Meta.Empreinte)
	}
	// Toutes les erreurs remontent ensemble.
	_, err = Charger([]byte(doc(`{"id": "", "version": 0, "statut": "x", "date_effet": "2026-10-04T08:00:00Z", "texte_fr": "", "fondement": {}, "verrou": "interdiction", "cible": {"serveur": "*", "outil": "*"}, "effet": "refuser"}`)))
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
