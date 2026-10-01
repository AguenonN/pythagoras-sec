# Schéma de directive v2

Statut : **figé**. Ce document est le contrat entre le juriste (qui écrit la
directive), le compilateur (qui la met sous cette forme), la relecture (qui
la lui rend en français), l'engine (qui fournit les entrées de `Decide`) et
le rejeu. Toute évolution passe par une v3, jamais par une retouche silencieuse.

Types Go : `internal/directive/schema.go` (enveloppe, v1) et
`internal/directive/schema_v2.go` (v2). Chargeur : `directive.Charger`.

## 1. Pourquoi une v2

La v1 impose **un verrou parmi six** et **un seul effet** par directive. La v2
garde l'enveloppe juridique et remplace la logique par :

- un **déclencheur** (quel évènement, quelle cible) ;
- une **condition** en arbre composable (`et`, `ou`, `non`, quantificateurs,
  feuilles d'une liste fermée), évaluée en logique à trois valeurs ;
- une **liste d'effets**.

Un jeu v1 reste accepté tel quel : chaque directive v1 est traduite en v2
(§9) et évaluée par le même moteur.

## 2. Document

Un jeu garde sa forme : `{"jeu": {…}, "directives": [ … ]}`. Le bloc `jeu`
(`id`, `version`, `date_effet`, `auteur`, `empreinte`, `signature`) est
inchangé. L'empreinte reste `sha256` du tableau `directives[]` en JSON
canonique (clés triées), calculée sur la forme écrite de chaque directive.

**Discriminant, par directive** : une directive qui porte la clé
`declencheur` est une directive v2 ; sinon c'est une directive v1 (clé
`verrou`). Porter les deux est refusé. Un jeu peut mêler v1 et v2.

Exemple complet :

```json
{
  "id": "D-101",
  "version": 1,
  "statut": "active",
  "date_effet": "2026-10-01T00:00:00Z",
  "revision_avant": "2027-10-01T00:00:00Z",
  "texte_fr": "Aucune action d'agent le week-end, heure de Paris.",
  "fondements": [{"texte": "Règlement intérieur", "article": "12"}],
  "declencheur": {"evenement": "appel_outil", "cible": {"serveur": "*", "outil": "*", "acces": "action"}},
  "condition": {"temps": {"jour_semaine": ["samedi", "dimanche"], "fuseau": "Europe/Paris"}},
  "effets": [{"effet": "refuser"}, {"effet": "notifier", "destinataire_role": "juridique", "rythme": "synthese", "si_echec": "continuer"}]
}
```

## 3. Enveloppe

| Champ | Obligatoire | Règle |
|---|---|---|
| `id` | oui | unique dans le jeu |
| `version` | oui | entier ≥ 1 |
| `statut` | oui | `brouillon \| observation \| active \| retiree` ; seules `active` et `observation` sont évaluées, `observation` journalise sans appliquer |
| `date_effet` | oui | RFC 3339 ; non-rétroactivité : une directive n'est évaluée que si `date_effet` ≤ horodatage de l'appel |
| `revision_avant` | non | RFC 3339, postérieure à `date_effet` ; passée, la directive **reste appliquée** et une alerte est émise |
| `texte_fr` | oui | le texte du juriste, **intact**, jamais réécrit |
| `fondements` | oui | liste non vide de `{texte, article?, url?}`, `texte` obligatoire ; le premier est le fondement cité par le motif et par la réponse à l'agent |
| `perimetre` | non | `{appelants: [SPIFFE ID \| "*"]}` non vide ; un appelant non prouvé tombe sous toute directive à périmètre |
| `si_indetermine` | non | `refuser` (défaut) \| `ignorer` ; s'applique **à la racine uniquement** (§7) |
| `si_indetermine_valide_par` | si `ignorer` | nom du juriste qui a validé `ignorer` ; interdit sinon |
| `declencheur` | oui | §4 |
| `condition` | non | un nœud (§5) ; absente, la condition vaut **vrai** |
| `effets` | oui | liste non vide (§6) |

Une directive v2 n'a ni `fondement` (singulier), ni `verrou`, ni `cible`,
ni `provenance`, ni `champs`, ni `effet` : ces clés sont refusées comme
inconnues.

## 4. Déclencheur

```json
{"evenement": "appel_outil", "cible": {"serveur": "banque", "outil": "*", "etiquette": "production_rapport", "acces": "action"}}
```

| `evenement` | `cible` | Évalué par |
|---|---|---|
| `appel_outil` | obligatoire, au moins une clé | `Decide`, à chaque `tools/call` |
| `ouverture_session` | interdite | `DecideEvenement`, à l'ouverture de chaque session |
| `attestation_echouee` | facultative, `serveur` seul | `DecideEvenement`, à chaque attestation en échec (connexion ou changement) |
| `purge` | interdite | `DecideEvenement`, à chaque purge du coffre |

*Évolution signalée (session gate-v2)* : tant que ces évènements n'avaient
pas d'évaluateur, le chargeur ne les admettait qu'au statut `brouillon`.
Ils en ont un désormais (`internal/directive/evenement.go`), et le
chargeur les admet à tout statut, restreints à ce que l'évènement
renseigne :

| `evenement` | Feuilles admises (sous `et`, `ou`, `non` ; aucun quantificateur) | Effets admis |
|---|---|---|
| `ouverture_session` | `appelant`, `demandeur`, `temps` | `refuser` (la session n'est pas ouverte), `notifier`, `marquer` |
| `attestation_echouee` | `attestation`, `temps` | `notifier` (`si_echec: continuer`), `marquer` |
| `purge` | `temps` | `notifier` (`si_echec: continuer`), `marquer` |

La décision est consignée dans l'entrée qui journalise l'évènement
(`session_ouverte`, `attestation`, `purge`) avec le jeu cité et les
remises ; `rejeu.RejouerEvenements` la rejoue.

Clés de `cible` (toutes facultatives, combinées en **et**) :

| Clé | Valeur | Source |
|---|---|---|
| `serveur` | nom logique du registre, ou `*` | l'appel |
| `outil` | nom d'outil, ou `*` | l'appel |
| `etiquette` | une étiquette du registre | `Appel.EtiquettesCible` |
| `acces` | `read \| action` | `Appel.Acces` (grant du registre) |

`etiquette` et `acces` viennent du registre. Si l'appelant de `Decide` ne
fournit pas de version de registre (`Appel.VersionRegistre` vide) ou pas
d'accès, la correspondance de cible est **indéterminée** : elle est combinée
en **et** (Kleene) avec la condition, et `si_indetermine` tranche.

## 5. Condition : un arbre

Chaque nœud est un objet à **exactement une clé**.

### 5.1 Nœuds

| Nœud | Forme | Valeur |
|---|---|---|
| `et` | `{"et": [n1, n2, …]}`, au moins 1 enfant | Kleene ET |
| `ou` | `{"ou": [n1, n2, …]}`, au moins 1 enfant | Kleene OU |
| `non` | `{"non": n}` | Kleene NON |
| `existe` | `{"existe": {"champ": "/ptr", "condition": n}}` | OU sur les éléments du tableau ; tableau vide : faux |
| `tous` | `{"tous": {"champ": "/ptr", "condition": n}}` | ET sur les éléments du tableau ; tableau vide : vrai |

Quantificateurs :

- `champ` est un pointeur JSON (RFC 6901) vers un **tableau** des arguments
  (ou de l'élément courant, s'il est imbriqué dans un autre quantificateur).
- Champ absent, qui n'est pas un tableau, ou tableau de plus de 10 000
  éléments : **indéterminé**.
- Dans la sous-condition, les pointeurs des feuilles `argument` et `sujet`
  sont **relatifs à l'élément courant**, et le pointeur vide `""` désigne
  l'élément lui-même. Hors quantificateur, un pointeur commence par `/`.

### 5.2 Bornes

Profondeur **6** au plus (la racine est à la profondeur 1, un enfant de
quantificateur à la profondeur du quantificateur + 1), **64 nœuds** au plus
(feuilles comprises). Au-delà : refus au chargement.

### 5.3 Feuilles (liste fermée)

| Feuille | Forme | Source | Indéterminé quand |
|---|---|---|---|
| `argument` | `{champ, op, valeur}` | l'appel (arguments après les caviardages des directives précédentes) | voir 5.4 |
| `session.a_consulte` | `{serveur \| outil \| etiquette, critere}` | état de session tenu par l'engine | `etiquette` seulement : aucune consultation ne l'a, et l'une des consultations retenues a des étiquettes inconnues |
| `appelant` | `{egal \| prefixe}` | le transport (identité SPIFFE ou `local-process`) | appelant non prouvé (vide) |
| `sous_agent_declare` | `{egal \| dans}` | `_meta.sous_agent`, déclaratif | jamais (rien déclaré : faux) |
| `demandeur` | `{identite: {egal \| dans}}` ou `{groupe}` | annuaire, résolu à l'ouverture de session, consigné | demandeur non résolu |
| `attestation` | `{verifiee: bool, mode?}` | recalculée par `Attestation.Valide`, jamais crue | jamais |
| `temps` | `{jour_semaine?, heure?, date?, fuseau}` | horodatage de l'appel | horodatage nul |
| `etiquette_cible` | `{contient}` | registre | pas de version de registre |
| `metadonnee_reponse` | `{serveur \| etiquette, champ, op, valeur}` | consultations consignées | voir 5.5 |
| `sujet` | `{groupe, champ?}` | résolveur attesté, consigné | identifiant illisible ou non résolu |

Détails :

- **`session.a_consulte`** : exactement une clé parmi `serveur` (nom
  logique), `outil` (`"serveur/outil"`), `etiquette` ; `critere` obligatoire :
  `transmis` (tout appel parti, quel que soit le résultat) ou
  `donnees_recues` (résultat `ok`). La négation (« sans consultation de »)
  passe par `non`. Vrai si une consultation retenue correspond.
- **`appelant`** : exactement une clé ; `egal` compare l'identité entière,
  `prefixe` son début (`spiffe://lab/agents/`).
- **`sous_agent_declare`** : exactement une clé ; `dans` est une liste non
  vide. C'est une déclaration de l'agent, non prouvée : à n'utiliser que
  pour resserrer, jamais pour accorder.
- **`demandeur`** : exactement une forme. `{"identite": {"egal": "…"}}`,
  `{"identite": {"dans": ["…"]}}` ou `{"groupe": "…"}` (appartenance).
- **`attestation`** : `verifiee` obligatoire ; vrai si `Valide()` vaut
  `verifiee` et, si `mode` est donné, si le mode du registre est `mode`
  (`spiffe | tls-pin | binaire | aucune`).
- **`temps`** : `fuseau` **obligatoire**, nom IANA de la liste embarquée
  (§8.2) ; au moins un critère parmi :
  - `jour_semaine` : liste non vide de `lundi … dimanche`, jour local ;
  - `heure` : `{"de": "HH:MM", "a": "HH:MM"}`, intervalle local `[de, a[` ;
    si `de` > `a`, l'intervalle passe minuit ; `de` = `a` est refusé ;
  - `date` : `{"op": > >= < <= == !=, "valeur": "AAAA-MM-JJ"}`, date locale.
  Les critères présents sont combinés en **et**.
- **`etiquette_cible`** : vrai si les étiquettes du serveur cible (registre)
  contiennent `contient`.
- **`sujet`** : attribut de la personne concernée. L'identifiant du sujet
  est lu au pointeur `champ` (relatif à l'élément courant dans un
  quantificateur ; `""` ou absent : l'élément lui-même ; hors quantificateur,
  `champ` est obligatoire et commence par `/`). L'identifiant est une chaîne
  ou un entier JSON. `Decide` le hache (`EmpreinteSujet`, sha256) et le
  cherche dans `Appel.Sujets` : **l'identifiant en clair n'est jamais
  consigné**, seule son empreinte l'est, avec les groupes.

### 5.4 Feuille `argument`

`champ` : pointeur JSON. Opérateurs :

| `op` | `valeur` | Vrai si | Indéterminé si |
|---|---|---|---|
| `>` `>=` `<` `<=` | nombre | comparaison **décimale exacte** | champ absent, non numérique |
| `==` `!=` | nombre, chaîne ou booléen | égalité (nombres : décimale exacte) | champ absent, type différent |
| `dans` | liste non vide de scalaires | égal à un élément | champ absent |
| `prefixe` | chaîne non vide | la chaîne commence par `valeur` | champ absent, non chaîne |
| `present` | — | le champ existe | jamais |
| `absent` | — | le champ n'existe pas | jamais |

Nombres : la valeur de la directive et celle de l'argument sont lues comme
décimaux exacts (`json.Number`, ou la représentation décimale la plus courte
d'un `float64`), jamais comparées en `float64`. Un nombre dont l'exposant
dépasse ±1 000 ou la mantisse 1 000 chiffres est indéterminé (et refusé
dans une directive). Aucune expression régulière nulle part.

### 5.5 Feuille `metadonnee_reponse`

Porte sur les consultations de la session **avec données reçues** (`ok`) dont
le serveur est `serveur`, ou dont les étiquettes contiennent `etiquette`
(exactement une des deux clés).

| `champ` | `op` | `valeur` |
|---|---|---|
| `source.origine` | `== != dans prefixe` | chaîne (liste pour `dans`) |
| `source.date` | `> >= < <= == !=` | instant RFC 3339 |

Vrai si une consultation retenue a une métadonnée qui satisfait ; sinon
indéterminé si l'une d'elles n'a pas la métadonnée, a une date illisible,
ou (pour `etiquette`) a des étiquettes inconnues ; sinon faux (y compris
aucune consultation retenue).

## 6. Effets

`effets` est une liste non vide d'objets `{"effet": …, paramètres}`.
Chaque paramètre n'est admis que pour son effet.

| `effet` | Paramètres | Sens |
|---|---|---|
| `refuser` | — | l'appel n'est pas transmis |
| `attendre_validation` | `par_role` (facultatif), `quatre_yeux` (bool, facultatif) | l'appel attend une validation humaine |
| `caviarder` | `champs_autorises` (pointeurs, non vide), `mode: caviarder \| refuser` | les champs hors liste sont retirés (`caviarder`) ou l'appel est refusé (`refuser`) |
| `notifier` | `destinataire_role` (obligatoire), `rythme: temps_reel \| synthese` (obligatoire), `si_echec: refuser \| continuer` (obligatoire) | une notification est à émettre |
| `marquer` | `etiquette` (obligatoire) | l'appel est marqué |

Règles de la liste :

- `refuser`, `attendre_validation`, `caviarder` : au plus une fois chacun ;
  `refuser` ne se combine ni avec `attendre_validation` ni avec
  `caviarder` (ils seraient sans effet, et une clé sans effet a l'air de
  compter).
- `caviarder` exige une cible nommée (`serveur` différent de `*`, ou
  `etiquette`). Un caviardage qui ne retire rien ne déclenche rien.
- `notifier` et `marquer` peuvent se répéter ; deux effets identiques sont
  refusés.

## 7. Sémantique

### 7.1 Logique à trois valeurs (Kleene forte)

V = vrai, I = indéterminé, F = faux.

| `non` | |
|---|---|
| V | F |
| I | I |
| F | V |

| `et` | V | I | F |
|---|---|---|---|
| **V** | V | I | F |
| **I** | I | I | F |
| **F** | F | F | F |

| `ou` | V | I | F |
|---|---|---|---|
| **V** | V | V | V |
| **I** | V | I | I |
| **F** | V | I | F |

`et` et `ou` à n enfants sont les repliements de ces tables (ordre F < I < V :
`et` = minimum, `ou` = maximum, `non` = symétrie). `existe` est le `ou` sur les
éléments, `tous` le `et`.

### 7.2 Résultat d'une directive

Pour une directive `active` ou `observation`, en vigueur, dont l'évènement
est celui évalué :

1. **cible** : V, F ou I (§4). F : la directive n'est pas évaluée pour cet
   appel (elle n'apparaît pas dans les applications).
2. **périmètre** : appelant hors périmètre : non évaluée ; appelant non
   prouvé : évaluée, avec une note.
3. **résultat** = `et(cible, condition)` ; condition absente = V.
4. `si_indetermine` s'applique **à ce résultat uniquement** (jamais à un
   sous-arbre) :
   - V : déclenchée ;
   - F : non déclenchée ;
   - I et `refuser` (défaut) : déclenchée, et l'effet de la directive est
     **forcé à `refuser`** ; les autres effets (notifier, marquer, caviarder)
     s'appliquent aussi ;
   - I et `ignorer` : non déclenchée, avec une note citant le juriste.
5. Déclenchée et `active` : appliquée. Déclenchée et `observation` :
   journalisée, jamais appliquée (pas de caviardage, pas de notification,
   pas de marque).

### 7.3 Précédence

`refuser` > `attendre_validation` > `caviarder` > `autoriser`. L'effet d'une
directive est le plus fort de sa liste (`caviarder` en mode `refuser` compte
comme `refuser` s'il retire quelque chose) ; l'effet de la décision est le
plus fort des directives appliquées. `notifier` et `marquer` se
**cumulent** : toute directive appliquée émet les siens, quel que soit
l'effet final.

Le caviardage des directives appliquées est cumulé dans l'ordre du jeu ; les
conditions d'une directive voient les arguments déjà caviardés par les
directives précédentes (ce qui est évalué est ce qui serait transmis).

### 7.4 Ordre

Les directives sont évaluées **dans l'ordre du jeu**, qui est l'ordre
canonique : il est fixé par le document haché dans l'empreinte, et le
contrat de l'engine (§6) cite « la première directive appliquée qui refuse,
dans l'ordre du jeu ». Les notifications, les marques et les rôles de
validation de la décision sont **triés**. Aucun parcours de `map` n'est
fait sans tri.

## 8. `Decide` v2

`Decide(jeu, etat, appel) Decision` reste une fonction pure : ni horloge,
ni I/O, ni lecture du système. Tout vient de l'appelant.

### 8.1 Entrées

`EtatSession` (inchangé) : identifiant, appelant prouvé, consultations.
Chaque `Consultation` porte en plus, facultativement :
`Etiquettes` (du registre au moment de la consultation),
`VersionRegistre` (vide : étiquettes inconnues) et `Source`
(`{Origine, Date}` déclarés par le serveur).

`Appel`, champs ajoutés :

| Champ | Type | Source |
|---|---|---|
| `Acces` | `read \| action` | grant du registre |
| `EtiquettesCible` | `[]string` | registre |
| `VersionRegistre` | `string` | registre ; vide : entrées du registre inconnues |
| `Demandeur` | `*Demandeur{Identite, Groupes}` | annuaire, à l'ouverture de session ; nil : non résolu |
| `Sujets` | `[]Sujet{Empreinte, Groupes}` | résolveur attesté ; `Empreinte = EmpreinteSujet(id)` |
| `SousAgentDeclare` | `string` | `_meta.sous_agent` |

### 8.2 Fuseaux

Chargés **uniquement** depuis une copie figée de la base IANA embarquée dans
le dépôt (`internal/directive/fuseaux/`, version dans `VERSION`), par
`time.LoadLocationFromTZData`. Jamais depuis le système ni depuis `TZ`.
Liste fermée : `Fuseaux()`. Un fuseau hors liste est refusé au chargement.

### 8.3 Sortie

`Decision` : effet retenu, arguments transmis, champs retirés, empreintes
d'origine et transmise, **toutes** les directives évaluées
(`Applications`, avec `Resultat` ∈ `vrai | faux | indetermine`), alertes,
`Notifications` (triées), `Marques` (triées), `Validation`
(`Roles` triés, `QuatreYeux`) quand l'effet est `attendre_validation`,
`VersionRegistre` recopiée, motif.

Précisions (étape 2, sans changement de forme) : `Validation.Roles` réunit
les `par_role` des directives appliquées dont l'effet est
`attendre_validation` ; chaque rôle listé doit valider, et `QuatreYeux` vaut
si l'une d'elles le demande. Le motif d'une directive v2 cite
`id vN (fondement)` ; celui d'une v1 traduite reste `id vN (verrou,
fondement)`.

### 8.4 Motifs

Aucun motif ne cite une valeur d'argument, ni une clé reçue, ni un
identifiant de sujet : seulement les champs, opérateurs et valeurs **de la
directive**, des nombres et des positions d'éléments.

### 8.5 Journal

Champs `omitempty` ajoutés au journal (`internal/journal`) pour que le rejeu
retrouve les entrées : sur l'entrée `appel`, `version_registre`, `acces`,
`etiquettes_cible`, `demandeur`, `sujets` (empreintes et groupes) ; sur une
consultation, `etiquettes`, `version_registre`, `source` ; sur la décision,
`notifications`, `marques`, `validation` ; sur une application, `resultat`
et `fondements`. Un journal v1 se relit à l'identique.

## 9. Conversion v1 → v2

Au chargement comme dans `Decide`, chaque directive v1 est traduite
(`ConvertirV1`). L'enveloppe est reprise (`fondement` → `fondements: [fondement]`),
le déclencheur est `{evenement: appel_outil, cible: {serveur, outil}}`.

| Verrou v1 | Condition v2 | Effets v2 |
|---|---|---|
| `interdiction` | aucune | `[refuser]` |
| `seuil_argument` | `et` des conditions (`in` = `dans`) | `[effet]` |
| `provenance_session` | `ou` de `session.a_consulte` (chaque serveur, puis chaque outil, avec le critère) | `[effet]` |
| `approbation_humaine` | aucune | `[attendre_validation]` |
| `champs_autorises_par_destination` | aucune | `[caviarder {champs_autorises, mode}]` |
| `source_attestee` | `attestation {verifiee: false}` | `[refuser]` |

La traduction garde le verrou d'origine pour le journal et les motifs : une
décision v1 rejouée après passage en v2 est identique (effet, arguments,
empreintes, applications, motifs). Seule différence voulue : les nombres
sont comparés en décimal exact, là où la v1 passait par `float64`.

## 10. Chargeur strict

- Champs inconnus refusés à tous les niveaux (enveloppe, déclencheur, cible,
  nœuds, feuilles, effets).
- Un nœud qui porte zéro ou plusieurs clés est refusé.
- Bornes de profondeur et de taille ; fuseau obligatoire et connu pour
  `temps` ; jours, heures, dates, pointeurs, opérateurs, types de valeurs,
  noms logiques, rôles, étiquettes non vides contrôlés.
- Toutes les erreurs sont rendues **en bloc**, une ligne chacune, préfixée
  par la directive et le chemin du nœud (`condition.et[1].argument`).
