# Agents hors ligne

## Compilateur de directives

Le juriste écrit une directive en français ; Mistral la propose sous forme
structurée ou pose des questions ; le code vérifie (rien d'inventé, puis le
chargeur strict Go via `directive verifier`) ; `directive relire` produit la
relecture, sans modèle. Toute directive produite est en `brouillon`.

Aucune dépendance tierce (bibliothèque standard, urllib), Go ≥ 1.25 pour
construire `cmd/directive`.

```sh
cd agents
python3 -m venv .venv
.venv/bin/python -m unittest discover -s tests -t .      # rejeu, jamais d'appel réel

.venv/bin/python -m compilateur \
  --texte "En application de l'art. 22 RGPD, …" \
  --jeu credit-pme --catalogue compilateur/exemples/catalogue-credit-pme.json \
  --auteur juriste@cabinet.fr --date-effet 2026-10-04 \
  [--reponses compilateur/exemples/reponses-d001.json] [--mode reel]
```

`--mode rejeu` (défaut) n'utilise que `compilateur/enregistrements/` ;
`--mode reel` complète par l'API (`MISTRAL_API_KEY`), au plus une requête par
seconde et `MISTRAL_MAX_APPELS` (40) appels, compteur dans `.etat/`.
