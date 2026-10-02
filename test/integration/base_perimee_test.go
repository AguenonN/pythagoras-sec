package integration

import (
	"testing"

	"mcprogue/internal/journal"
)

// cheminDirectivesVersion2 est la version 2 du jeu de test credit-demo, qui
// ajoute D-005.
const cheminDirectivesVersion2 = "test/integration/testdata/credit-demo-v2.json"

// basePerimee joue le scénario « base périmée » : consultation de la base
// interne (T3 périmé), puis attribution du prêt, avec le jeu donné.
func basePerimee(t *testing.T, directives string) (attribution map[string]any, erreur bool, appels []journal.Entree) {
	t.Helper()
	b := demarrerAvec(t, lancement{registre: cheminRegistre, jeu: directives})
	r := b.appeler(t, "banque-synthetique__get_taux_interne", map[string]any{"categorie": "decouvert-societes-non-financieres"})
	if r.IsError {
		t.Fatalf("get_taux_interne : %s", texte(r))
	}
	contientTout(t, "get_taux_interne", texte(r), `"trimestre":"2026-Q3"`)
	r = b.appeler(t, "banque-synthetique__attribuer_pret", map[string]any{
		"dossier_id": "DOS-FICTIF-002", "siren": "382891752", "montant": 400000,
	})
	// Autorisée, l'attribution rend le texte du serveur sans structuredContent.
	attribution, _ = r.StructuredContent.(map[string]any)
	erreur = r.IsError
	_, appels = b.verifier(t)
	if len(appels) != 2 {
		t.Fatalf("%d appels", len(appels))
	}
	return attribution, erreur, appels
}

// Base périmée avec la version 2 du jeu : la consultation de
// get_taux_interne (D-005) met l'attribution en attente, alors que la
// version 1 l'autorise.
func TestBasePerimee_JeuVersion2(t *testing.T) {
	sc, erreur, appels := basePerimee(t, cheminDirectives)
	if erreur || appels[1].Decision.Effet != "autoriser" || appels[1].Resultat.Statut != journal.StatutOK {
		t.Fatalf("version 1 : attribution non autorisée : %v %+v", sc, appels[1].Decision)
	}

	sc, erreur, appels = basePerimee(t, cheminDirectivesVersion2)
	if !erreur || sc["statut"] != "en_attente" || sc["directive"] != "D-005" || sc["version"] != float64(1) {
		t.Fatalf("version 2 : attribution non mise en attente : %v", sc)
	}
	a := appels[1]
	if a.Decision.Effet != "attendre_validation" || a.Resultat.Statut != journal.StatutNonTransmis {
		t.Fatalf("version 2 : appel 2 : %+v, statut %s", a.Decision, a.Resultat.Statut)
	}
	d005 := false
	for _, app := range a.Decision.Applications {
		if app.ID == "D-005" && app.Appliquee {
			d005 = true
		}
	}
	if !d005 {
		t.Fatalf("D-005 absente des applications : %+v", a.Decision.Applications)
	}
}
