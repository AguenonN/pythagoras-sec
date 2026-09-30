# Règles communes aux trois sessions de nuit

Ces règles s'appliquent à toi, quelle que soit ta mission. Elles priment sur toute autre considération.

## Autonomie

- **Personne ne te répondra avant demain matin.** Ne pose aucune question et n'attends aucune validation.
- Face à une décision non couverte par ton brief : choisis l'option **la plus prudente** (fermée par défaut, la moins irréversible, la plus simple à défaire), **note-la** dans ton rapport avec ta justification, et continue.
- Si un élément est bloqué (API en panne, dépendance manquante), passe à l'élément suivant de ta liste. Ne boucle jamais plus de 3 fois sur le même problème.
- Si plus rien n'est faisable, écris ton rapport et arrête-toi.

## Dépôt

- Tu travailles **uniquement dans ton worktree et ta branche**. Ne touche jamais à `main`, ne fusionne rien, ne pousse rien, n'utilise jamais `--force`.
- Ne modifie **aucun fichier commité avant le tag `baseline-pre-hackathon`**, ni les paquets `internal/directive`, `internal/journal`, `internal/session`, `internal/rejeu`, `internal/chaine`, `internal/nommage`, sauf si ton brief l'autorise explicitement.
- Lis `docs/contrat-engine.md` avant d'écrire du code qui touche au journal ou aux sessions.
- **Commits petits et fréquents**, un par unité logique, message clair. Avant chaque commit : `go vet ./...` et `go test -race ./...` verts pour le Go ; tests Python verts pour le Python.
- Un commit ne contient jamais : secret, clé, jeton, fichier `.env`, `__pycache__`, binaire compilé, données réelles de personne physique.

## Sécurité de la machine

- Pas de `sudo`. Pas de suppression en dehors de ton worktree. Pas d'installation globale : dépendances Go via les modules, dépendances Python dans un environnement virtuel du worktree.
- Réseau autorisé : les registres de paquets, et **uniquement** les domaines que ton brief liste. Aucun autre.

## Secrets

- Les secrets sont dans l'environnement (`MISTRAL_API_KEY`, `PISTE_CLIENT_ID`, `PISTE_CLIENT_SECRET`). Lis-les depuis l'environnement, jamais depuis un fichier que tu créerais.
- **N'affiche jamais un secret**, même partiellement, ni dans la sortie, ni dans un log, ni dans un test, ni dans ton rapport.

## Coûts et courtoisie envers les API

- API officielles : **au plus 1 requête par seconde** par API, 3 tentatives maximum avec attente croissante, et **mise en cache** de toute réponse déjà obtenue.
- Mistral : modèle `mistral-small-latest`, `temperature` 0, **au plus 40 appels réels sur toute la nuit**. Les tests automatisés utilisent des réponses enregistrées, jamais l'API réelle.

## Rapport du matin

À la racine de ton worktree, tiens à jour `RAPPORT_NUIT.md`, mis à jour **après chaque commit** (pas seulement à la fin) :

1. Ce qui est fait (avec les identifiants de commit).
2. Ce qui n'est pas fait, et pourquoi.
3. **Décisions prises seul**, avec justification.
4. Blocages rencontrés.
5. Questions pour Manu, classées par urgence.
6. Comment vérifier ton travail en 5 minutes (commandes exactes).
