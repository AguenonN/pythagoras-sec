# Brief — Session principale du soir : schéma de directive v2 et `Decide` v2

Lis d'abord `docs/orchestration/00_regles_communes.md` et `docs/contrat-engine.md`.

## Pourquoi

Le schéma v1 impose **un verrou parmi six** et **un seul effet** par directive. Les verrous ne se combinent pas. Le juriste ne peut donc pas exprimer, par exemple :
- « pas d'attribution de prêt **sans** consultation du registre des gels » (négation de provenance) ;
- « validation si montant > 200 000 € **et** source officielle consultée » (combinaison) ;
- « aucune action d'agent **le week-end**, heure de Paris » (temps) ;
- « décision réservée au **directeur des risques**, quatre yeux, et notification au juridique » (effets multiples, rôle) ;
- « si **l'utilisateur** est membre du groupe X, pas de rapport fondé sur des **données client** » (demandeur, étiquettes) ;
- « aucune décision RH si **l'un des salariés concernés** est dans le groupe Y » (attribut de la personne concernée, quantificateur).

La v2 garde l'**enveloppe juridique** et remplace la logique par un **arbre composable** et une **liste d'effets**.

## Périmètre de cette session

Tu es autorisé à modifier `internal/directive`, et, uniquement pour y ajouter des champs `omitempty`, `internal/journal`. Ne modifie ni le gate, ni le compilateur, ni la relecture : d'autres sessions s'en chargeront **à partir de ton schéma figé**.

Worktree `~/cybercar/mcp_rogue-v2`, branche `feat/v2`, partie de `main` **après** la fusion de `feat/integration` si elle est faite, sinon de `main`. Un commit par étape, `go vet` et `go test -race ./...` verts, **y compris tous les tests existants**.

## Étape 1 — Le schéma figé (à commiter seul, en premier)

`docs/schema-directive-v2.md` + types Go + chargeur strict. **Arrête-toi après ce commit et écris dans le rapport « schéma figé : <commit> »** : d'autres sessions démarreront dessus.

### Enveloppe (inchangée)
`id`, `version`, `statut` (`brouillon | observation | active | retiree`), `date_effet`, `revision_avant`, `texte_fr` (intact), `fondements` (liste de `{texte, article, url}`), `perimetre`, `si_indetermine` (`refuser` par défaut ; `ignorer` seulement avec `si_indetermine_valide_par`).

### Déclencheur
`{ "evenement": "appel_outil" | "ouverture_session" | "attestation_echouee" | "purge", "cible": { "serveur" | "outil" | "etiquette" | "acces": "read"|"action" } }`. Jokers `*` admis sur serveur et outil.

### Condition : un arbre
- Nœuds : `et` (liste), `ou` (liste), `non` (un enfant).
- Quantificateurs sur un tableau d'arguments : `existe` et `tous`, avec `champ` (pointeur JSON vers le tableau) et une sous-condition évaluée sur chaque élément.
- **Bornes** : profondeur 6, 64 nœuds au plus. Au-delà : refus au chargement.
- Feuilles, **liste fermée** :

| Feuille | Forme | Source de la valeur |
|---|---|---|
| `argument` | `{champ, op, valeur}`, ops `> >= < <= == != dans present absent prefixe` | L'appel |
| `session.a_consulte` | `{serveur | outil | etiquette, critere: transmis | donnees_recues}` ; la négation passe par `non` | État de session tenu par l'engine |
| `appelant` | `{egal | prefixe}` (identité SPIFFE ou `local-process`) | Le transport |
| `sous_agent_declare` | `{egal | dans}` | `_meta`, posé par le code d'orchestration |
| `demandeur` | `{identite: egal | dans} | {groupe}` | **Résolu à l'ouverture de session** par l'annuaire, consigné au journal (fourni en entrée à `Decide`) |
| `attestation` | `{verifiee: bool, mode}` | L'engine (recalculée, jamais crue) |
| `temps` | `{jour_semaine: dans [...], heure: {de, a}, date: {op, valeur}, fuseau}` ; fuseau **obligatoire** | Horodatage de l'appel |
| `etiquette_cible` | `{contient}` | Le registre (fourni en entrée) |
| `metadonnee_reponse` | `{serveur | etiquette, champ: "source.origine" | "source.date", op, valeur}` sur les consultations de la session | Consultations consignées |
| `sujet` | `{groupe}` : attribut de la personne concernée | Résolveur attesté, fourni en entrée, consigné |

### Effets : une liste
| Effet | Paramètres |
|---|---|
| `refuser` | — |
| `attendre_validation` | `par_role` (facultatif), `quatre_yeux` (bool) |
| `caviarder` | `champs_autorises` (pointeurs), `mode: caviarder | refuser` |
| `notifier` | `destinataire_role`, `rythme: temps_reel | synthese`, `si_echec: refuser | continuer` |
| `marquer` | `etiquette` |

### Sémantique
- **Précédence** : refuser > attendre_validation > caviarder > autoriser ; `notifier` et `marquer` se cumulent.
- **Logique à trois valeurs** (vrai / faux / indéterminé), Kleene forte : écris sa table de vérité dans `docs/schema-directive-v2.md`. `si_indetermine` s'applique **à la racine uniquement**.
- **Conversion v1 → v2 au chargement** : chacun des six verrous v1 se traduit en une directive v2. Un jeu v1 reste accepté tel quel.
- Chargeur **strict** : champs inconnus refusés, bornes, fuseau obligatoire pour `temps`, erreurs rendues en bloc.

## Étape 2 — `Decide` v2

Même principe : fonction pure, entrées explicites.
- Entrées ajoutées à l'appel : accès (`read | action`), étiquettes de la cible, demandeur (identité, groupes), attributs de la personne concernée, **version du registre**. Tout vient de l'appelant de `Decide`, rien n'est lu par `Decide`.
- Sortie : décision complète (effet retenu, arguments transmis, champs retirés, empreintes, **toutes** les directives évaluées avec leur résultat à trois valeurs, notifications à émettre, marques).
- Aucune valeur d'argument dans un motif (test existant à étendre).

### Les sept parades de déterminisme
1. Logique à trois valeurs écrite et testée cas par cas.
2. **Fuseau chargé uniquement depuis des données embarquées** (copie figée de la base des fuseaux, versionnée dans le dépôt, `time.LoadLocationFromTZData`), jamais depuis le système.
3. Nombres JSON comparés **exactement** (décimaux), jamais via `float64`.
4. Chaînes : égalité, préfixe, appartenance ; pas d'expression régulière.
5. Ordre canonique trié des directives, effets, notifications ; aucun parcours de `map` non trié.
6. Métadonnées de réponse lues dans les consultations fournies (consignées au journal).
7. Version du registre dans l'entrée d'appel du journal (champ `omitempty`).

### Les cinq tests
1. Garde de pureté étendue à tout `internal/directive`.
2. **Répétition** : mêmes entrées, 1 000 évaluations, une seule décision.
3. **Arbres générés aléatoirement** à graine fixe : aucune panique, aucune divergence entre deux évaluations.
4. Table de vérité de la logique à trois valeurs, cas par cas.
5. **Tous les journaux et jeux existants** (tests du gate, du rejeu, de l'intégration) **se rejouent sans écart** après passage en v2.

### Tests de scénarios obligatoires
- Politique de crédit : `non(a_consulte gels-avoirs)` → refus ; montant > 500 000 → refus ; `a_consulte taux-officiel` → attente ; caviardage `/siren` vers `recherche-entreprises`.
- **Week-end** : `2026-10-02T21:59:00Z` action → autorisé ; `2026-10-02T22:00:00Z` action → refusé ; même heure en lecture → autorisé ; `2026-10-04T21:59:00Z` → refusé ; `2026-10-04T22:00:00Z` → autorisé ; `2026-10-24T22:00:00Z` → refusé ; **résultats identiques quel que soit le fuseau de la machine** (`TZ=UTC` et `TZ=Asia/Tokyo`).
- **Top management** : montant > 2 000 000 → attente par rôle `directeur-risques`, quatre yeux, notification.
- **RH** : `existe` dans `/salarie_ids` un sujet du groupe `contentieux-en-cours` → refus ; sujet `salaries-proteges` et action → attente ; les deux → refus (précédence).
- **Demandeur** : demandeur du groupe X, session ayant reçu des données d'un serveur étiqueté `donnees_client`, cible étiquetée `production_rapport` → refus + notification.

## Critère de réussite
Étape 1 commitée et signalée « schéma figé » ; étape 2 verte avec les cinq tests et les scénarios ; aucun test existant cassé.

## Rapport
`RAPPORT_V2.md`, à jour après chaque commit.
