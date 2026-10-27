# Attester un artefact né il y a 2 ms

Question posée telle quelle : *comment attester, au niveau de l'OAuth d'Entra ID
ou d'un SSO, un code généré 2 ms plus tôt ?*

Réponse courte : on ne l'attestera jamais **auprès** de l'IdP, et il ne faut pas
essayer. L'IdP continue d'authentifier ce qu'il sait authentifier — des identités
enregistrées à l'avance — et l'artefact éphémère n'en obtient aucune. Il obtient
une **empreinte**, un **périmètre** et un **jeton interne borné par les deux**.

---

## 1. Pourquoi la question, posée ainsi, n'a pas de solution

Un IdP OAuth/OIDC délivre un jeton à un `client_id` enrôlé au préalable :
l'enregistrement d'application précède l'émission, par construction. Un
descripteur produit il y a 2 ms n'est enrôlé nulle part, et le temps d'écrire son
enregistrement dans l'annuaire, l'appel qu'il devait servir est passé.

L'issue apparente — enrôler le connecteur à la volée — est la pire des trois
options disponibles :

- elle exige un droit d'écriture sur l'annuaire d'identité **depuis le plan de
  génération**, c'est-à-dire exactement depuis le composant dont on cherche à
  borner le rayon d'action ;
- le cycle de vie d'un enregistrement d'application (minutes à heures, purge
  manuelle) est de trois ordres de grandeur supérieur à la durée de vie de
  l'artefact : l'annuaire se remplit de principals orphelins qui restent
  valides ;
- le consentement associé est accordé à un `client_id`, pas à une procédure, donc
  il survit au descripteur qui l'a motivé.

On abandonne donc l'idée que l'artefact soit un sujet d'identité.

## 2. Trois identités, pas une

| Identité | Nature | Qui l'émet | Durée de vie |
|---|---|---|---|
| `I_humain` | OIDC / Entra ID, porte l'habilitation | l'IdP de l'organisation | session |
| `I_workload` | SVID SPIFFE du proxy et de l'exécuteur | SPIRE (`internal/identity`) | rotation courte |
| `I_artefact` | **pas une identité** : une empreinte | personne — elle se calcule | celle du descripteur |

Les deux premières sont stables, pré-enregistrées, et c'est tout ce que l'IdP a
jamais besoin de connaître. La troisième n'a ni enrôlement, ni secret, ni
consentement : un `sha256` sur la forme canonique du descripteur.

La question « l'IdP atteste-t-il l'artefact ? » devient : **le proxy, lui-même
authentifié par l'IdP et porteur d'un SVID, accepte-t-il de frapper un jeton
d'action dont la portée est celle que l'attestation de l'artefact déclare ?**
Celle-là a une réponse mécanique.

## 3. Les deux plans de jeton

Le secret du fournisseur (Uber, Outlook, Instagram) **ne quitte jamais le
proxy**. Le descripteur en cours d'exécution ne le voit pas, ne le stocke pas, et
ne peut pas l'exfiltrer puisqu'il ne le détient pas. Le proxy est l'unique sortie
réseau et attache le secret au dernier moment, par opération.

Ce que l'exécuteur détient, c'est un JWT interne lié à `(session, empreinte
d'artefact, E_classe)`. L'échange suit RFC 8693 : `I_humain` est échangé contre un
jeton d'action, l'artefact figurant en claim `act` — le délégué agissant pour le
principal. `internal/approval.Actor` modélise déjà ce claim, et
`approval.TokenClaims` lie déjà un jeton à **un appel exact** plutôt qu'à une
permission permanente ; c'est la liaison qu'il faut étendre de « un appel » à
« une empreinte de descripteur ».

Conséquence directe : un jeton volé en cours d'exécution ne vaut rien hors de son
artefact. Il nomme une empreinte qui n'est plus celle du descripteur courant.

## 4. Ce que l'attestation contient — et ce qu'elle ne contient pas

Enveloppe DSSE, prédicat in-toto.

```
subject   : sha256 de la forme canonique du descripteur
predicate :
  e_classe:
    domaines : ["api.uber.com"]        # triés, minuscules
    palier   : transaction
  generateur : identifiant de version du modèle
  etalon     : sha256 de la spec OpenAPI utilisée comme référence
```

N'y figurent **jamais** : le plafond, l'adresse de livraison, le demandeur, la
session. C'est `EInstance`, et elle se vérifie mécaniquement à l'exécution.

La raison est économique, pas esthétique. Si `EInstance` entrait dans le prédicat,
deux utilisateurs commandant chez la même enseigne produiraient deux empreintes
distinctes, la clé de cache se fragmenterait jusqu'à l'unicité, et le cache
mourrait — avec lui la seule chose qui rend le premier appel coûteux acceptable.
D'où la règle : **l'admission peut être approximative parce que l'exécution est
exacte.** Le palier est arrondi au minimum qui couvre la demande ; chaque effet,
lui, est tranché à l'unité au moment où il se produit.

DSSE signe l'énoncé entier via PAE : inclure le périmètre dans le prédicat ne
coûte donc aucune signature supplémentaire.

## 5. La chaîne de vérification, étape par étape

Chaque étape échoue fermée. Un `Indetermine` à n'importe quel rang produit un
refus, pas une tentative dégradée.

1. **L'humain s'authentifie.** OIDC auprès de l'IdP. Le proxy obtient
   `I_humain` et l'habilitation qui y est attachée. Rien de nouveau.
2. **Le plan de génération produit un descripteur.** Il tourne ailleurs que le
   plan d'exécution — rayon d'action différent — et ne détient aucun secret de
   fournisseur.
3. **Le chargeur strict refuse ou accepte.** Champ hors schéma, `operation`
   absente de la spec étalon, `id` dupliqué, `choix` non exhaustif,
   `reprise.max > 0` sur une étape irréversible : refus désigné par sa position.
   Aucun modèle n'intervient ici.
4. **Le périmètre se canonise.** `EClasse.Canoniser()` rend l'empreinte. Un
   périmètre qui ne se canonise pas est non évaluable, donc refusé.
5. **L'attestation est signée et le couple est mis en cache.** Clé = empreinte
   de `EClasse`, **égalité stricte**. Jamais d'inclusion : raisonner sur
   l'emboîtement des périmètres au moment de la réutilisation est précisément
   l'endroit où les failles se logent.
6. **Le proxy frappe le jeton d'action.** Il vérifie dans l'ordre : la signature
   DSSE, que `subject` est bien l'empreinte du descripteur qu'il tient en main,
   que `e_classe.palier` couvre l'effet demandé, que l'habilitation de
   `I_humain` atteint ce palier, et que `EInstance` (plafond, adresse, session)
   satisfait les bornes. Les quatre premières portent sur des données signées,
   la cinquième sur l'état de la session.
7. **Chaque effet est décidé au moment où il se produit.** `Decide` est pure et
   monotone : les verdicts ne peuvent que durcir. `transaction` exige la
   validation humaine. Le journal est chaîné par HMAC (`internal/chaine`) et
   suffit à rejouer la décision à l'identique.

`Administration` n'est jamais atteint par arrondi. Il exige sa propre admission
et une validation humaine explicite.

## 6. Ce que le dépôt porte déjà, et ce qui manque

| Paquet | Ce qu'il fait aujourd'hui | Ce qu'il devient |
|---|---|---|
| `internal/identity` | SVID SPIFFE, mTLS, liste blanche d'appelants exacte | inchangé — c'est `I_workload` |
| `internal/toolpin` | `Hash(name, description, inputSchema)` d'un outil statique, verrouillé dans `tools.lock.json`, mutation = outil désactivé | le même geste sur la forme canonique du descripteur : c'est `subject` |
| `internal/approval` | JWT lié à un appel exact, claim `act` RFC 8693, TTL 15 min, algorithmes dérivés de la clé | liaison étendue à l'empreinte d'artefact |
| `internal/chaine` | JSONL chaîné par HMAC-SHA256, forme canonique | inchangé — c'est le journal de l'étape 7 |
| `internal/directive` | `Decide` pure, trivalente, monotone, sous test de pureté | gagne un paramètre `verdicts` |

Manquent entièrement : `internal/descripteur` (schéma et chargeur),
`internal/perimetre` (`EClasse`, `EInstance`, `Canoniser`), `internal/effet`
(l'échelle des classes), `internal/executeur` (l'interpréteur), et la signature
DSSE elle-même. Voir `docs/plan-construction.md`.

`toolpin` est l'indice le plus net que l'approche tient : le mécanisme qui épingle
un outil statique contre le *rug-pull* est, au geste près, celui qui épingle un
descripteur contre sa propre réécriture. Ce qui change, c'est que l'épinglage ne
se fait plus une fois pour toutes à l'enrôlement, mais à chaque naissance.

## 7. Les objections, et ce qu'elles valent

**« Une attestation de provenance ne prouve pas que le descripteur est sûr. »**
Exact, et elle ne le prétend pas. Elle prouve deux choses seulement : que
l'artefact est sorti de notre générateur, et qu'il a déclaré **ce périmètre-là**
avant de s'exécuter. La sûreté ne vient pas de la provenance, elle vient de ce
que l'exécution est bornée par le périmètre déclaré et que chaque effet est
tranché à l'unité. La provenance rend la portée **opposable** ; c'est tout ce
qu'on lui demande.

**« L'attestation ne vaut que la rigueur du générateur. »** Oui, et c'est pour
cela que le générateur n'est jamais le juge : poids partagés, angles morts
partagés. Les contrôles ne lisent pas le descripteur en tant que texte, ils
lisent des faits extraits de sa forme canonique, et le plancher déterministe
tranche avant tout appel de modèle — méthode ≠ GET ⇒ au moins `ecriture`, corps
portant un montant ⇒ au moins `transaction`, domaine hors enveloppe ⇒ refus
immédiat sans appel de modèle.

**« Avec une spec publiée, un MCP statique offrirait les mêmes garanties. »**
Partiellement vrai, et il faut le dire plutôt que l'esquiver : quatre des cinq
contrôles de l'état de l'art redeviennent atteignables. Ce qui ne l'est pas :
l'unité d'autorisation n'est plus l'outil mais la **composition** — c'est la
procédure entière qui porte un palier, pas chaque appel isolément. Et surtout la
base de confiance diffère. Un serveur MCP statique a été relu par quelqu'un ; un
connecteur généré n'a personne derrière lui. Le moteur ne fait rien d'autre que
remplacer cette relecture par du mécanisme.

## 8. Non tranché

- La forme canonique exacte du descripteur avant hachage : tri des clés à tous
  les niveaux, normalisation des nombres, traitement des champs absents contre
  champs nuls. Deux implémentations qui divergent d'un espace produisent deux
  empreintes et tuent le cache.
- Où vit la racine de confiance des signatures DSSE en profil sur site :
  Fulcio/Rekor suppose un service joignable, ce que la contrainte de déploiement
  n° 1 interdit à l'exécuteur.
- Révocation : l'empreinte d'un descripteur jugé mauvais après coup doit pouvoir
  être refusée sans attendre l'expiration du cache. Liste de refus locale,
  probablement, mais sa distribution reste à décider.
