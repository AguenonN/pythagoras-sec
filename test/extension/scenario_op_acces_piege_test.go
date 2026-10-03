package extension

import "testing"

// Rejoue hors ligne le scénario op-acces-piege une fois enregistré
// (scripts/lancer-demo.sh scenario op-acces-piege) : sans réseau ni Mistral, devant le
// gate ; vérifie la réponse, les directives citées, les sous-agents et les
// décisions attendus par demo/scenarios/op-acces-piege.json, puis la chaîne du journal.
// Ignoré tant qu'aucun enregistrement n'existe.
func TestScenario_op_acces_piege(t *testing.T) {
	RejouerScenario(t, "op-acces-piege")
}
