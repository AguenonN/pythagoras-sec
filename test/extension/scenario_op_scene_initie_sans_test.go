package extension

import "testing"

// Rejoue hors ligne le scénario op-scene-initie-sans une fois enregistré
// (scripts/lancer-demo.sh scenario op-scene-initie-sans) : sans réseau ni Mistral, devant le
// gate ; vérifie la réponse, les directives citées, les sous-agents et les
// décisions attendus par demo/scenarios/op-scene-initie-sans.json, puis la chaîne du journal.
// Ignoré tant qu'aucun enregistrement n'existe.
func TestScenario_op_scene_initie_sans(t *testing.T) {
	RejouerScenario(t, "op-scene-initie-sans")
}
