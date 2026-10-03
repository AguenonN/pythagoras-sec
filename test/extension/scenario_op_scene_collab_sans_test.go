package extension

import "testing"

// Rejoue hors ligne le scénario op-scene-collab-sans une fois enregistré
// (scripts/lancer-demo.sh scenario op-scene-collab-sans) : sans réseau ni Mistral, devant le
// gate ; vérifie la réponse, les directives citées, les sous-agents et les
// décisions attendus par demo/scenarios/op-scene-collab-sans.json, puis la chaîne du journal.
// Ignoré tant qu'aucun enregistrement n'existe.
func TestScenario_op_scene_collab_sans(t *testing.T) {
	RejouerScenario(t, "op-scene-collab-sans")
}
