# Brief de nuit — Session « Données » : serveurs MCP adaptateurs et rejeu

Lis d'abord `00_regles_communes.md` (joint).

## Contexte

Produit : le juriste écrit des directives ; l'engine `mcp_rogue` les applique aux appels d'outils MCP d'une chaîne d'agents Mistral qui travaillent sur des **données officielles réelles**. Ta mission : exposer ces données comme des **serveurs MCP**, avec une couche d'enregistrement et de rejeu pour que la démo tienne même si une API tombe.

Rien dans l'engine ne dépend de toi cette nuit, et tu ne dépends pas de l'engine : tes serveurs parlent MCP, c'est tout.

## Worktree

Branche `feat/donnees`, worktree `~/cybercar/mcp_rogue-donnees`. Code en **Go avec le go-sdk MCP**, un binaire par serveur, dans `servers/<nom>/`. Les noms logiques respectent `internal/nommage` (minuscules, chiffres, tirets).

**Réseau autorisé** : registres de paquets, `webstat.banque-france.fr`, `recherche-entreprises.api.gouv.fr`, `gels-avoirs.dgtresor.gouv.fr`, `publications.europa.eu`, `eur-lex.europa.eu`, `oauth.piste.gouv.fr`, `api.piste.gouv.fr`. Aucun autre.

**Secrets** : `PISTE_CLIENT_ID`, `PISTE_CLIENT_SECRET` dans l'environnement.

## La couche d'enregistrement et de rejeu : à faire en premier

Paquet `servers/internal/rejeu` (distinct de `internal/rejeu` de l'engine, ne pas confondre) :
- chaque réponse d'une API officielle est enregistrée : URL appelée (sans secret), date, empreinte SHA-256 du corps, corps ;
- trois modes par variable d'environnement : `DIRECT` (appelle et enregistre), `REJEU` (ne lit que l'enregistrement, **aucun réseau**), `MIXTE` (appelle, et bascule sur l'enregistrement en cas d'échec) ;
- toute réponse servie depuis l'enregistrement le dit dans le résultat de l'outil : `source.origine = "rejeu"`, avec la date de l'enregistrement. **On ne cache pas au jury qu'une donnée vient du cache** : sa fraîcheur doit rester visible ;
- une API indisponible produit un résultat d'outil explicite `indisponible`, jamais un plantage.

Enregistrements dans `servers/<nom>/enregistrements/`, versionnés dans git (ce sont des données publiques).

## Les serveurs, dans cet ordre

| Ordre | Nom logique | Source | Outils |
|---|---|---|---|
| 1 | `entreprises` | API Recherche d'entreprises (sans authentification, 7 req/s max, toi 1 req/s) | `rechercher_entreprise(siren)` → dénomination, forme, état, siège, activité |
| 2 | `gels-avoirs` | API Gels des avoirs, DG Trésor (JSON). La DG Trésor signale des erreurs de certificat sur certains environnements : si tu en as, note-le, n'installe rien, passe au suivant | `verifier_gel(nom)` → correspondances avec le registre, date de publication du registre |
| 3 | `taux-officiel` | Webstat, Banque de France : séries « Taux de l'usure » (publication trimestrielle). **Vérifie d'abord si l'API exige une clé** ; si oui et sans clé disponible, utilise l'export public des séries et note-le | `get_taux_usure(categorie)` → seuil en vigueur, trimestre, date de publication, identifiant de série |
| 4 | `eurlex` | CELLAR SPARQL (sans clé) + texte HTML par numéro CELEX | `rechercher_acte(celex)` → titre, date, statut ; `lire_article(celex, article)` → texte |
| 5 | `legifrance` | API Légifrance via PISTE (OAuth `client_credentials`, production) | `lire_article(identifiant)` → texte en vigueur, version, dates |

**Pour `legifrance`** : ce soir, les points d'accès « ping » ont renvoyé `500`, alors que l'authentification passe. Trouve un point d'accès **documenté** de consultation d'article dans la documentation officielle de l'API. S'il répond, construis l'adaptateur. S'il renvoie `500` après 3 tentatives espacées, **arrête ce serveur**, note-le comme bloqué, et n'y reviens pas cette nuit.

**Pour `eurlex`**, enregistre au minimum : DORA (règlement 2022/2554), RGPD (2016/679), AI Act (2024/1689), règlement Omnibus numérique 2026/1744. Vérifie les numéros CELEX sur la source plutôt que de les supposer.

## Données de démo à produire

- `servers/taux-officiel/instantane_trimestre_precedent.json` : la valeur **réelle** du seuil de l'usure du trimestre **précédent**, pour les mêmes catégories, avec sa date. Elle servira de « base interne périmée ». Nomme-la comme telle.
- Trois entreprises **réelles** (SIREN publics, entreprises actives, sans lien avec une affaire judiciaire) pour les demandes de crédit de démo.
- Une entrée **réelle** du registre des gels, pour le cas positif de filtrage. Choisis une entité (pas une personne physique) si possible.
- **Aucune donnée de personne physique réelle**, nulle part.

## Pour chaque serveur

- Schéma d'outils figé et exporté dans `servers/<nom>/schema.json` (il servira au registre et à l'épinglage).
- Empreinte SHA-256 du binaire compilé notée dans le rapport.
- Tests : en mode `REJEU` uniquement (aucun réseau dans les tests).
- Un test manuel en mode `DIRECT`, résultat noté dans le rapport.

## Critère de réussite de la nuit

Au moins `entreprises`, `gels-avoirs` et `taux-officiel` répondent en `DIRECT` **et** en `REJEU`, avec leurs enregistrements commités. `eurlex` et `legifrance` sont des bonus.

Le fichier `schema.json` de chaque serveur contient exactement la liste `tools` telle que renvoyée par la méthode MCP `tools/list` du serveur, sans transformation.
