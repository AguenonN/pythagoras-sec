package extension

import "testing"

// Rejoue hors ligne le scénario op-initie-piege une fois enregistré
// (scripts/lancer-demo.sh scenario op-initie-piege) : sans réseau ni Mistral, devant le
// gate ; vérifie la réponse, les directives citées, les sous-agents et les
// décisions attendus par demo/scenarios/op-initie-piege.json, puis la chaîne du journal.
// Ignoré tant qu'aucun enregistrement n'existe.
func TestScenario_op_initie_piege(t *testing.T) {
	RejouerScenario(t, "op-initie-piege")
}
