package extension

import "testing"

// Rejoue hors ligne le scénario op-communique-temoin une fois enregistré
// (scripts/lancer-demo.sh scenario op-communique-temoin) : sans réseau ni Mistral, devant le
// gate ; vérifie la réponse, les directives citées, les sous-agents et les
// décisions attendus par demo/scenarios/op-communique-temoin.json, puis la chaîne du journal.
// Ignoré tant qu'aucun enregistrement n'existe.
func TestScenario_op_communique_temoin(t *testing.T) {
	RejouerScenario(t, "op-communique-temoin")
}
