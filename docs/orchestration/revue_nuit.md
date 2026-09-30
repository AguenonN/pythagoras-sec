# Revue indépendante des trois branches de nuit — LECTURE SEULE

Tu es relecteur. Tu n'as écrit aucune de ces branches. **Tu ne modifies aucun fichier, tu ne commites rien, tu ne fusionnes rien.** Tu peux lire, compiler et lancer les tests.

## Contexte

Trois sessions ont travaillé cette nuit, chacune dans son worktree, à partir du commit `69e32ee` :

| Branche | Worktree | Brief |
|---|---|---|
| `feat/gate` | `~/cybercar/mcp_rogue-gate` | `nuit/01_session_principale_agregateur.md` |
| `feat/donnees` | `~/cybercar/mcp_rogue-donnees` | `nuit/02_session_donnees.md` |
| `feat/agents` | `~/cybercar/mcp_rogue-agents` | `nuit/03_session_agents_hors_ligne.md` |

Règles communes : `nuit/00_regles_communes.md` (présent dans chaque worktree). Contrat de l'engine : `docs/contrat-engine.md`. Chaque worktree contient un `RAPPORT_NUIT.md` écrit par la session.

Principe du produit à ne jamais perdre de vue : **aucun LLM sur le chemin d'exécution** ; `Decide` est pure ; la provenance est tenue par l'engine, jamais déclarée par l'agent ; tout est fermé par défaut.

## Ta mission

Pour chaque branche, **vérifie le rapport contre le code**. Un rapport peut affirmer plus que ce que le code fait. Ne te contente pas de lire les tests : vérifie qu'ils testent vraiment ce qu'ils prétendent (assertions présentes, cas négatifs réels, pas de test qui passe à vide).

### `feat/gate`
1. Le pipeline de `tools/call` suit-il l'ordre de `docs/contrat-engine.md` (session → coffre → `Decide` → refus `non_transmis` ou transmission → scellement → entrée `appel` avec l'horodatage passé à `Decide` → `Terminer`) ? Cite les lignes.
2. `Terminer` est-il appelé sur **tous** les chemins, y compris erreur et panique du backend ?
3. Un serveur non attesté peut-il, par un chemin quelconque, faire exposer sa propre description d'outil à l'agent ?
4. Le test de point de contrôle rejoue-t-il vraiment le journal **relu depuis le disque** avec `internal/rejeu`, et échouerait-il si une entrée était altérée ?
5. Le rapport affirme que seuls des fichiers ont été ajoutés : vérifie.
6. Une panne d'écriture du journal fait-elle refuser l'appel **avant** transmission ?

### `feat/donnees`
1. Chaque client HTTP est-il limité à l'hôte officiel de son API, redirections comprises ?
2. Les enregistrements contiennent-ils un secret, un jeton, un en-tête d'autorisation, ou des données de **personnes physiques** (registre des gels) ?
3. Le mode `REJEU` est-il réellement sans réseau (vérifie dans le code, pas seulement dans les tests) ?
4. Une réponse servie depuis l'enregistrement le signale-t-elle toujours dans le résultat de l'outil ?
5. `schema.json` est-il bien la sortie brute de `tools/list` ?
6. **Calendrier** : le seuil de l'usure du 4e trimestre 2026 s'applique à partir du 1er octobre 2026. Le jour de la démo (4 octobre 2026), l'instantané « trimestre précédent » doit contenir le **3e trimestre**, et `get_taux_usure` doit servir le **4e**. Vérifie ce que contiennent réellement les fichiers et ce que sert le serveur pour une `date_reference` du 4 octobre.

### `feat/agents`
1. Aucune sortie de Mistral n'est-elle acceptée sans passer par `directive verifier` (le chargeur strict en Go) ?
2. Le statut produit est-il toujours `brouillon` ?
3. `si_indetermine: ignorer` peut-il être produit sans que le texte du juriste le demande ?
4. La relecture est-elle déterministe (aucune date système, aucun ordre de dictionnaire non trié) ?
5. Les enregistrements Mistral contiennent-ils la clé API ou des en-têtes ?
6. Les noms de serveurs utilisés par le compilateur correspondent-ils à ceux des serveurs de `feat/donnees` et du registre de `feat/gate` ?

### Transversal
- Conflits prévisibles à la fusion des trois branches dans `main` (fichiers touchés par plusieurs branches).
- Incohérences de nommage entre les branches.
- Tout écart aux règles communes.

## Format de ton rapport

Écris `REVUE_NUIT.md` **dans ta réponse** (pas dans un fichier) :

1. **Verdict par branche** : fusionnable tel quel / fusionnable après corrections / à ne pas fusionner.
2. **Constats**, chacun avec : gravité (bloquant / important / mineur), branche, fichier:ligne, ce qui est affirmé, ce qui est constaté.
3. **Affirmations des rapports non confirmées par le code.**
4. **Corrections à faire avant fusion**, par branche, dans l'ordre.
5. **Ordre de fusion recommandé** et conflits attendus.
