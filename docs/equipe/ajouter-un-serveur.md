# Ajouter un serveur MCP

La procédure unique est dans **[`docs/demo/etendre.md`](../demo/etendre.md)**, recette 1 (« Un serveur de données synthétiques »), précédée des règles pour concevoir des outils gouvernables : une commande, `scripts/nouveau-serveur.sh <nom> <donnees.json> <outils.json> [etiquettes.json]`, sans écrire de Go ni toucher au registre à la main. Elle est testée à la lettre par `test/extension/`.

Pour un serveur qui n'est pas synthétique (API externe, enregistrements), le moteur commun ne suffit pas : prendre `servers/banque-synthetique` comme modèle et signaler le besoin.
