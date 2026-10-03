package extension

import "testing"

// Rejoue hors ligne le scénario op-divulgation-piege une fois enregistré
// (scripts/lancer-demo.sh scenario op-divulgation-piege) : sans réseau ni Mistral, devant le
// gate ; vérifie la réponse, les directives citées, les sous-agents et les
// décisions attendus par demo/scenarios/op-divulgation-piege.json, puis la chaîne du journal.
// Ignoré tant qu'aucun enregistrement n'existe.
func TestScenario_op_divulgation_piege(t *testing.T) {
	RejouerScenario(t, "op-divulgation-piege")
}
