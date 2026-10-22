# Console Pythagoras sur Aiven Runtime (lecture seule)

La console tourne sur la boucle locale du conteneur ; `proxy.py` l'expose sur
le port 8080 en ne transmettant que GET et HEAD avec l'hôte local. Valider,
compiler ou lancer une demande reste impossible depuis Internet : ces actions
se font sur la console de la banque.

Les données affichées sont un instantané des passages de démonstration
(journaux signés, données fictives) : `scripts/instantane-runtime.sh`, puis
commit de `deploy/console-runtime/instantane/`. Aucune clé privée n'y est copiée.

Runtime : contexte de build `/`, Containerfile `/deploy/console-runtime/Dockerfile`,
port 8080, variable secrète `MCP_GATE_CLE_JOURNAL` (clé HMAC des journaux de
démonstration, pour que la console vérifie les chaînes).
