package integration

import (
	"testing"

	"mcprogue/internal/journal"
)

const cheminDirectivesV2 = "demo/directives/credit-demo-v2.json"

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

// Base périmée avec le jeu v2 : la provenance sur get_taux_interne (D-005)
// met l'attribution en attente, alors que le jeu v1 l'autorise.
func TestBasePerimeeJeuV2(t *testing.T) {
	sc, erreur, appels := basePerimee(t, cheminDirectives)
	if erreur || appels[1].Decision.Effet != "autoriser" || appels[1].Resultat.Statut != journal.StatutOK {
		t.Fatalf("jeu v1 : attribution non autorisée : %v %+v", sc, appels[1].Decision)
	}

	sc, erreur, appels = basePerimee(t, cheminDirectivesV2)
	if !erreur || sc["statut"] != "en_attente" || sc["directive"] != "D-005" || sc["version"] != float64(1) {
		t.Fatalf("jeu v2 : attribution non mise en attente : %v", sc)
	}
	a := appels[1]
	if a.Decision.Effet != "attendre_validation" || a.Resultat.Statut != journal.StatutNonTransmis {
		t.Fatalf("jeu v2 : appel 2 : %+v, statut %s", a.Decision, a.Resultat.Statut)
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
