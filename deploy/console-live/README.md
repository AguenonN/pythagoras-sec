# Console Pythagoras « live » sur Aiven Runtime

Les vrais agents tournent dans le conteneur : orchestrateur et sous-agents
Mistral, `mcp-gate`, les 12 serveurs MCP. Une demande tapée dans « Advisor
desk » lance un vrai passage (jeu avec D-009, via `scripts/lanceur-demo-d009.sh`).

- Build (contexte `/`, Containerfile `/deploy/console-live/Dockerfile`) :
  `scripts/lancer-demo.sh preparer` (serveurs, gate, registre, annuaire et clés
  fictives, registre des gels du jour téléchargé) ; aucun secret.
- Secrets Runtime : `MCP_GATE_CLE_JOURNAL`, `MISTRAL_API_KEY`, `DEMO_MOT_DE_PASSE`.
- Accès : mot de passe HTTP Basic sur toute l'URL (utilisateur quelconque).
  Sans `DEMO_MOT_DE_PASSE`, lecture seule.
- Plan : `startup-100-2048` au moins (1 vCPU).
- Les sessions du conteneur sont éphémères : un redéploiement les efface
  (l'instantané de `deploy/console-runtime/instantane` est recopié au build).
