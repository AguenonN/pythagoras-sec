# Agents hors ligne

## Compilateur de directives

Le juriste écrit une règle en français ; Mistral la propose sous forme d'un
jeu de directives (schéma v2, `docs/schema-directive-v2.md`), pose des
questions ou refuse un critère non observable ; le code vérifie (rien
d'inventé, puis le chargeur strict Go via `directive verifier`) ;
`directive relire` produit la relecture, sans modèle. Toute directive
produite est en `brouillon`. Le format v1 est retiré.

Aucune dépendance tierce (bibliothèque standard, urllib), Go ≥ 1.25 pour
construire `cmd/directive`. Modèle par défaut du compilateur :
`mistral-medium-latest` (retenu sur mesures, `RAPPORT_B.md`).

```sh
cd agents
python3 -m venv .venv
.venv/bin/python -m unittest discover -s tests -t .      # rejeu, jamais d'appel réel

# Service de la console (contrats §4) : une requête JSON, une réponse JSON.
.venv/bin/python -m compilateur.service [--mode reel] < requete.json
.venv/bin/python -m compilateur.service --parcours compilateur/exemples/service/weekend.json

# Scénarios enregistrés, un tour sans dialogue.
.venv/bin/python -m compilateur.scenarios_v2 rh
.venv/bin/python -m compilateur --texte "…" --jeu essai \
  --catalogue compilateur/exemples/catalogue-demo.json \
  --auteur juriste@cabinet.fr --date-effet 2026-10-04 [--reponses r.json] [--mode reel]
```

`compilateur/exemples/catalogue-demo.json` et `vocabulaire-demo.json` sont
générés (`go run ./cmd/registre-demo -catalogue …` et `-vocabulaire …` à la
racine) ; des tests échouent s'ils divergent de leurs sources. Les scénarios
enregistrés avant leur génération se rejouent sur des copies gelées
(`tests/donnees/catalogue-v2-enregistrements.json`,
`vocabulaire-v2-enregistrements.json`).

`--mode rejeu` (défaut) n'utilise que `compilateur/enregistrements/` ;
`--mode reel` complète par l'API (`MISTRAL_API_KEY`), au plus une requête par
seconde et `MISTRAL_MAX_APPELS` (40) appels, compteur dans `.etat/`.

## Démonstration

```sh
.venv/bin/python -m compilateur.demo --jeu credit-pme \
  --catalogue compilateur/exemples/catalogue-demo.json \
  --auteur juriste@cabinet.fr --sortie brouillon.json [--jeu-existant ../demo/directives/demo.json]
```

Demande le texte et la date d'effet, pose les questions, affiche la
relecture à côté du texte et écrit le jeu en `brouillon` (avec
`--jeu-existant`, la version n+1 de ce jeu).
