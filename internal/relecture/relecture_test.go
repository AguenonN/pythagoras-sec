package relecture

import (
	"flag"
	"os"
	"strings"
	"testing"
	"time"

	"mcprogue/internal/directive"
)

var maj = flag.Bool("update", false, "réécrire les fichiers attendus de testdata")

func charger(t *testing.T, chemin string) *directive.Jeu {
	t.Helper()
	b, err := os.ReadFile(chemin)
	if err != nil {
		t.Fatal(err)
	}
	jeu, err := directive.Charger(b)
	if err != nil {
		t.Fatal(err)
	}
	return jeu
}

// TestJeu_Attendu compare la relecture complète au fichier attendu, octet
// pour octet, sur plusieurs chargements et sous plusieurs fuseaux locaux.
func TestJeu_Attendu(t *testing.T) {
	cases := []struct{ nom, jeu, attendu string }{
		{"credit-pme", "../directive/testdata/credit-pme.json", "testdata/credit-pme.txt"},
		{"variantes", "testdata/variantes.json", "testdata/variantes.txt"},
	}
	for _, c := range cases {
		t.Run(c.nom, func(t *testing.T) {
			got := Jeu(charger(t, c.jeu))
			if *maj {
				if err := os.WriteFile(c.attendu, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(c.attendu)
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Fatalf("relecture différente de %s :\n--- obtenu\n%s\n--- attendu\n%s", c.attendu, got, want)
			}
			local := time.Local
			defer func() { time.Local = local }()
			for _, tz := range []*time.Location{time.UTC, time.FixedZone("UTC+13", 13*3600), time.FixedZone("UTC-10", -10*3600)} {
				time.Local = tz
				for i := 0; i < 3; i++ {
					if again := Jeu(charger(t, c.jeu)); again != got {
						t.Fatalf("relecture non déterministe (fuseau %s, passe %d)", tz, i)
					}
				}
			}
		})
	}
}

// TestDirective_CreditPME fixe, directive par directive, la phrase de règle
// que le juriste relit.
func TestDirective_CreditPME(t *testing.T) {
	jeu := charger(t, "../directive/testdata/credit-pme.json")
	cases := []struct{ id, veut string }{
		{"D-001", "Directive D-001, version 1, active (appliquée), fondée sur l'art. 22 RGPD (source : https://eur-lex.europa.eu/eli/reg/2016/679/oj).\n" +
			"Applicable aux appels à partir du 04/10/2026 à 08:00 UTC, sans date de fin ; sans échéance de relecture.\n" +
			"Verrou provenance_session. Pour tous les appelants : si la session a reçu des données du serveur taux-officiel, tout appel à attribuer_pret sur banque-synthetique exige une validation humaine.\n"},
		{"D-002", "Directive D-002, version 1, active (appliquée), fondée sur l'art. 22 RGPD.\n" +
			"Applicable aux appels à partir du 04/10/2026 à 08:00 UTC, sans date de fin ; à relire avant le 01/01/2027 à 00:00 UTC (passée cette date, la directive reste appliquée et une alerte est émise).\n" +
			"Verrou seuil_argument. Pour tous les appelants : tout appel à attribuer_pret sur banque-synthetique dont l'argument /montant est strictement supérieur à 500000 est refusé. Si cette condition ne peut pas être évaluée (argument absent ou de type inattendu), l'appel est refusé.\n"},
		{"D-003", "Directive D-003, version 1, active (appliquée), fondée sur l'art. 22 RGPD.\n" +
			"Applicable aux appels à partir du 04/10/2026 à 08:00 UTC, sans date de fin ; sans échéance de relecture.\n" +
			"Verrou source_attestee. Pour tous les appelants : tout appel à un outil du serveur taux-officiel est refusé si l'attestation de ce serveur au registre (identité, schémas épinglés) a échoué.\n"},
		{"D-004", "Directive D-004, version 1, active (appliquée), fondée sur l'art. 5 RGPD (source : https://eur-lex.europa.eu/eli/reg/2016/679/oj).\n" +
			"Applicable aux appels à partir du 04/10/2026 à 08:00 UTC, sans date de fin ; sans échéance de relecture.\n" +
			"Verrou champs_autorises_par_destination. Pour tous les appelants : tout appel à un outil du serveur recherche-entreprises ne transmet que le champ /siren (un champ autorisé couvre aussi tout son contenu) ; tout autre champ est retiré avant transmission.\n"},
	}
	if len(cases) != len(jeu.Directives) {
		t.Fatalf("%d directives dans le jeu, %d cas", len(jeu.Directives), len(cases))
	}
	for i, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			d := jeu.Directives[i]
			if d.ID != c.id {
				t.Fatalf("directive %d : %s, attendu %s", i, d.ID, c.id)
			}
			if got := Directive(d); got != c.veut {
				t.Errorf("obtenu :\n%s\nattendu :\n%s", got, c.veut)
			}
		})
	}
}

// TestRegle_Variantes couvre chaque branche du gabarit sur une directive
// minimale ; la phrase de règle est comparée en entier.
func TestRegle_Variantes(t *testing.T) {
	cases := []struct{ nom, directive, veut string }{
		{"interdiction partout", `"verrou": "interdiction", "cible": {"serveur": "*", "outil": "*"}, "effet": "refuser"`,
			"Pour tous les appelants : tout appel d'outil, sur tout serveur, est refusé, sans condition."},
		{"approbation outil sur tout serveur", `"verrou": "approbation_humaine", "cible": {"serveur": "*", "outil": "virer"}, "effet": "attendre_validation"`,
			"Pour tous les appelants : tout appel à virer, sur tout serveur, exige une validation humaine."},
		{"périmètre nommé", `"verrou": "approbation_humaine", "perimetre": {"appelants": ["spiffe://lab/a", "spiffe://lab/b"]}, "cible": {"serveur": "banque-synthetique", "outil": "virer"}, "effet": "attendre_validation"`,
			"Pour les appelants spiffe://lab/a, spiffe://lab/b, et pour tout appelant dont l'identité n'est pas prouvée : tout appel à virer sur banque-synthetique exige une validation humaine."},
		{"périmètre étoile", `"verrou": "approbation_humaine", "perimetre": {"appelants": ["*"]}, "cible": {"serveur": "banque-synthetique", "outil": "virer"}, "effet": "attendre_validation"`,
			"Pour tous les appelants : tout appel à virer sur banque-synthetique exige une validation humaine."},
		{"seuil plusieurs conditions", `"verrou": "seuil_argument", "cible": {"serveur": "banque-synthetique", "outil": "virer"}, "condition": [{"champ": "/montant", "op": ">=", "valeur": 1000.5}, {"champ": "/pays", "op": "in", "valeur": ["FR", "BE"]}, {"champ": "/urgent", "op": "==", "valeur": true}], "effet": "attendre_validation"`,
			"Pour tous les appelants : tout appel à virer sur banque-synthetique dont l'argument /montant est supérieur ou égal à 1000.5 et dont l'argument /pays vaut l'une des valeurs « FR », « BE » et dont l'argument /urgent est égal à vrai exige une validation humaine. Si l'une de ces conditions ne peut pas être évaluée (argument absent ou de type inattendu), l'appel est refusé."},
		{"seuil opérateurs restants", `"verrou": "seuil_argument", "cible": {"serveur": "banque-synthetique", "outil": "virer"}, "condition": [{"champ": "/a", "op": "<", "valeur": 1}, {"champ": "/b", "op": "<=", "valeur": 2}, {"champ": "/c", "op": "!=", "valeur": "x"}, {"champ": "/d", "op": "present"}, {"champ": "/e", "op": "absent"}], "si_indetermine": "refuser", "effet": "refuser"`,
			"Pour tous les appelants : tout appel à virer sur banque-synthetique dont l'argument /a est strictement inférieur à 1 et dont l'argument /b est inférieur ou égal à 2 et dont l'argument /c est différent de « x » et dont l'argument /d est présent et dont l'argument /e est absent est refusé. Si l'une de ces conditions ne peut pas être évaluée (argument absent ou de type inattendu), l'appel est refusé."},
		{"seuil indéterminé ignoré", `"verrou": "seuil_argument", "cible": {"serveur": "banque-synthetique", "outil": "virer"}, "condition": [{"champ": "/montant", "op": ">", "valeur": 10}], "si_indetermine": "ignorer", "si_indetermine_valide_par": "Me Dupont", "effet": "refuser"`,
			"Pour tous les appelants : tout appel à virer sur banque-synthetique dont l'argument /montant est strictement supérieur à 10 est refusé. Si cette condition ne peut pas être évaluée (argument absent ou de type inattendu), la directive ne s'applique pas à cet appel (choix validé par Me Dupont)."},
		{"provenance transmis, serveurs et outils", `"verrou": "provenance_session", "cible": {"serveur": "banque-synthetique", "outil": "*"}, "provenance": {"serveurs": ["taux-officiel"], "outils": ["legifrance/lire_article"], "critere": "transmis"}, "effet": "refuser"`,
			"Pour tous les appelants : si la session a déjà transmis un appel à destination du serveur taux-officiel ou de l'outil lire_article du serveur legifrance (quel qu'en soit le résultat), tout appel à un outil du serveur banque-synthetique est refusé."},
		{"champs en refus", `"verrou": "champs_autorises_par_destination", "cible": {"serveur": "recherche-entreprises", "outil": "verifier_entreprise"}, "champs": {"autorises": ["/siren", "/adresse"], "mode": "refuser"}, "effet": "refuser"`,
			"Pour tous les appelants : tout appel à verifier_entreprise sur recherche-entreprises ne peut contenir que les champs /siren, /adresse (un champ autorisé couvre aussi tout son contenu) ; un appel qui en contient un autre est refusé."},
	}
	for _, c := range cases {
		t.Run(c.nom, func(t *testing.T) {
			doc := `{"jeu": {"id": "t", "version": 1, "date_effet": "2026-10-04T08:00:00Z", "auteur": "j"}, "directives": [{"id": "D-1", "version": 1, "statut": "brouillon", "date_effet": "2026-10-04T08:00:00Z", "texte_fr": "t", "fondement": {"texte": "RGPD"}, ` + c.directive + `}]}`
			jeu, err := directive.Charger([]byte(doc))
			if err != nil {
				t.Fatal(err)
			}
			if got := regle(jeu.Directives[0]); got != c.veut {
				t.Errorf("obtenu :\n%s\nattendu :\n%s", got, c.veut)
			}
		})
	}
}

func TestStatutEtFondement(t *testing.T) {
	statuts := map[directive.Statut]string{
		directive.Brouillon:   "brouillon (non appliquée tant que le juriste ne l'a pas passée en observation ou active)",
		directive.Observation: "en observation (journalisée, non appliquée)",
		directive.Active:      "active (appliquée)",
		directive.Retiree:     "retirée (non appliquée)",
	}
	for s, veut := range statuts {
		if got := statut(s); got != veut {
			t.Errorf("statut(%s) = %q, attendu %q", s, got, veut)
		}
	}
	fondements := []struct {
		f    directive.Fondement
		veut string
	}{
		{directive.Fondement{Texte: "RGPD"}, "RGPD"},
		{directive.Fondement{Texte: "RGPD", Article: "22"}, "l'art. 22 RGPD"},
		{directive.Fondement{Texte: "RGPD", Article: "art. 22 §1"}, "l'art. 22 §1 RGPD"},
		{directive.Fondement{Texte: "Code civil", Article: "1240", URL: "https://x"}, "l'art. 1240 Code civil (source : https://x)"},
	}
	for _, c := range fondements {
		if got := fondement(c.f); got != c.veut {
			t.Errorf("fondement(%+v) = %q, attendu %q", c.f, got, c.veut)
		}
	}
}

func TestDate_UTC(t *testing.T) {
	paris := time.FixedZone("CEST", 2*3600)
	cases := []struct {
		t    time.Time
		veut string
	}{
		{time.Date(2026, 10, 4, 10, 0, 0, 0, paris), "04/10/2026 à 08:00 UTC"},
		{time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), "01/01/2027 à 00:00 UTC"},
		{time.Date(2027, 1, 1, 0, 0, 7, 0, time.UTC), "01/01/2027 à 00:00:07 UTC"},
	}
	for _, c := range cases {
		if got := date(c.t); got != c.veut {
			t.Errorf("date(%v) = %q, attendu %q", c.t, got, c.veut)
		}
	}
	if !strings.Contains(revision(&cases[1].t), "reste appliquée") {
		t.Error("l'échéance de relecture doit dire que la directive reste appliquée")
	}
}
