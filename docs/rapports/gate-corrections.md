# Rapport — corrections (sous-agent déclaré, rechargement à chaud)

Branche `feat/gate`, worktree `~/cybercar/mcp_rogue-gate`. Rien n'est fusionné ni poussé, et `main` n'a pas été touché.
Ce fichier n'est pas commité, comme `RAPPORT_NUIT.md`.

## 1. Fait

| Point | Commit | Contenu |
|---|---|---|
| Sous-agent déclaré | `5b71e4c` | **Déjà fait avant cette mission** (demande précédente, identique) : il n'a pas été refait. `_meta.sous_agent` est lu et inscrit dans l'entrée `appel` sous `sous_agent_declare`. Il ne va ni à `Decide`, ni à l'état de session, ni au serveur ; l'appelant prouvé reste celui du transport. Test : deux sous-agents dans la même session ; le journal les distingue, la provenance reste commune (D-001 déclenche pour le second), et le rejeu est sans écart. |
| Retour arrière marqué | `d0eaca8` | Suite aux réponses de Manu : pas de changement d'identifiant à chaud et les deux garde-fous restent. Le retour à une version inférieure reste autorisé ; son entrée `jeu_charge` porte `retour_arriere: true`, `version_depart` et `version_arrivee`. Trois champs `omitempty` ajoutés à `journal.Entree`. Testé en processus (v1 → v2 → v1 → v2 : seule l'entrée du retour est marquée, 2 → 1) et via le binaire piloté par `SIGHUP`. |
| Rechargement à chaud | `bdfc90e` | `SIGHUP` recharge le jeu depuis le fichier `-directives`. Le nouveau jeu passe par le chargeur strict, avec bascule atomique, entrée `jeu_charge` et alerte en cas d'échec. Voir le détail ci-dessous. |

Chaque commit a passé `go vet ./...` et `go test -race ./...` en vert. Les tests du rechargement ont passé 10 fois de suite (`-count=10`), et le test `SIGHUP` du binaire 3 fois.

### Rechargement : ce qui est garanti et testé

- **Chargeur strict avant la bascule** (`directive.Charger`). En cas d'échec, l'ancien jeu reste actif et une entrée `alerte` est écrite (« rechargement du jeu refusé, `<id>` v`<n>` reste actif : … »). Cas testés : document illisible, invalide, d'un autre identifiant, version réécrite, fichier absent.
- **Entrée `jeu_charge`** : identifiant, version et empreinte, avec le message « rechargé à chaud, remplace `<id>` v`<n>` `<empreinte>` ». Elle est écrite *avant* la bascule ; si le journal refuse l'écriture, il n'y a pas de bascule et l'engine se ferme.
- **Bascule atomique entre deux appels.** Le jeu actif est tenu par un `atomic.Pointer`. Chaque appel lit le jeu une seule fois, juste avant `Decide`, et son entrée `appel` cite ce jeu. Test de concurrence : 4 sessions appellent pendant que les rechargements alternent v1/v2. Le test exige que les deux versions aient décidé, que chaque décision corresponde à la version citée, et que le rejeu soit sans écart.
- **Scénario demandé** (`TestRecharger_BasculeEtRetourArriere`, et le même via le binaire dans `TestRechargement_SIGHUP`) :
  - Pendant une session, on charge une v2 qui ajoute un refus (D-009, interdiction). L'appel suivant est décidé sous v2 et refusé, en citant D-009 et son fondement.
  - L'appel précédent reste rejouable sous v1. Sans v1 dans le catalogue, le rejeu dit « gate-test v1 introuvable », ce qui prouve que l'appel est bien lié à v1.
  - Retour arrière vers v1 : l'appel suivant est de nouveau autorisé.
  - Le rejeu du journal entier, avec le catalogue {v1, v2}, est sans écart.

## 1 bis. Revue de code (un commit par point)

| Point | Commit | Contenu |
|---|---|---|
| Versions sur toute bascule | `d16a70f` | Réponse de Manu : `version_depart` et `version_arrivee` sur toute bascule à chaud ; `retour_arriere` marque toujours une version d’arrivée inférieure ; rien sur le `jeu_charge` de démarrage. |
| 1. Panne du journal | `16bc3d8` | Entrée chaînée `transmission` (session, seq, serveur, outil, empreinte transmise, décision) écrite avant `Tour.Transmettre` ; si elle échoue : refus, rien ne part, engine fermé. Le rejeu la contrôle (`ControleTransmission` : une et une seule avant tout appel transmis, identique sur serveur/outil/empreinte/effet, aucune pour un `non_transmis`) et `rejeu.TransmissionsSansAppel` signale les appels partis sans résultat journalisé. `docs/contrat-engine.md` §4. Le test de panne exige désormais **un seul** appel transmis (celui d’avant la panne). **Touche `internal/journal`** (type `transmission`) et **`internal/rejeu`** (contrôle, fixture de test), sur demande. Un journal antérieur à ce commit, sans entrées `transmission`, présente désormais des écarts au rejeu. |
| 2. Provenance réelle | `a6b1eca` | Plus de `Origine: "live"` codé en dur : `resultat.source` reprend `structuredContent.source.origine` et `.date` du résultat, au format des serveurs de `feat/donnees` (`direct`, `rejeu`, `aucune` ; date RFC 3339), sinon `non_declaree`. Valeur venue du serveur donc bornée : origine hors vocabulaire → `non_declaree`, date illisible → omise. |
| 3. Cibles des directives | `5f1da4d` | Au démarrage (rien n’est connecté) et à chaque rechargement (ancien jeu actif, alerte), refus de tout jeu dont une directive, **quel que soit son statut**, vise un serveur absent du registre ou un outil que ce serveur n’expose pas. Jokers admis ; un outil nommé sous un serveur joker doit exister sur au moins un serveur. Erreurs rendues en bloc. Les **provenances** (`provenance.serveurs` / `outils`) ne sont pas contrôlées : ce ne sont pas des cibles (voir questions). |
| 4. Refus par le registre | `191b501` | Le refus d’un serveur non attesté est journalisé `decision.effet = refuser`, `decision.origine = registre`, motif du registre. La règle est dans un paquet partagé `internal/controle` (`controle.Registre`), appelé par le gate **et** par le rejeu. Une directive qui refuse déjà garde la main (origine vide). Le rejeu recalcule le contrôle depuis l’attestation journalisée et ajoute le contrôle `statut` : un refus n’est jamais transmis, une autorisation l’est toujours, une attente seulement avec une approbation. `docs/contrat-engine.md` §5. **Touche `internal/journal`** (champ `Decision.Origine`) et `internal/rejeu` (la fixture des tests porte désormais une approbation sur l’appel en attente qu’elle transmet). |
| 5. Environnement stdio | `ac65a32` | `commande.env` au registre : **noms** de variables transmises au serveur, avec leur valeur dans l’environnement de l’engine au lancement. Une entrée contenant `=` est refusée **sans être citée** (elle porte peut-être une valeur) ; nom invalide ou en double refusé. Variable nommée mais absente : non transmise, seul son nom est tracé. Test : de deux variables posées, seule celle nommée arrive (vérifié par nom et longueur de valeur, jamais par valeur) ; sans liste, rien ne passe. |
| 6. Journal altéré | `3e19b2b` | `test/gate/alteration_test.go` : journal du scénario relu depuis le disque, honnête puis falsifié. Refus maquillé en autorisation (prev/mac inchangés) : `journal.Verifier` échoue, le rejeu nomme deux altérations (`statut`, `decision`). Entrée `appel` supprimée : la vérification échoue, la transmission devient orpheline, l’appel suivant ne retrouve plus son état (`etat`). |
| 7. Contrat de réponse | `46c594f` | Refus : `isError: true`, `structuredContent = {statut: "refuse", origine, directive, version, fondement, motif}` ; attente : `{statut: "en_attente", empreinte_transmise, directive, version}` (types `gate.Refus`, `gate.Attente`). Trois origines de refus : `directive` (première directive appliquée qui refuse), `registre` (id et version du registre), `engine` (vides, fondement « Contrat de l’engine »). Une source injoignable n’est pas un refus : `isError` sans structure. `docs/contrat-engine.md` §6. |
| 8. Provenances hors registre | `fbbad22` | Réponse de Manu : une directive dont `provenance.serveurs` ou `provenance.outils` nomme un serveur ou un outil absent du registre est refusée au démarrage et au rechargement, exactement comme une cible (même erreur, même alerte, toutes statuts confondus). |
| 9. Forme des échecs | `044657e` | Réponse de Manu : échec = `isError: true`, `structuredContent = {statut: "indisponible", serveur, outil, motif}` (noms logiques, motif de l’engine). Couvre la source indisponible (journal `indisponible`) et l’erreur de protocole (journal `erreur`). L’agent distingue refus, attente, échec et outil qui décline (réponse scellée, sans structure). Champ `origine` des refus conservé. `docs/contrat-engine.md` §6 (tableau des quatre cas). |
<!-- fin revue -->

## 2. Pas fait

Rien de ce qui était demandé.

## 3. Décisions prises seul

1. **Un signal (`SIGHUP`) plutôt qu'une surveillance du fichier.**
   - La bascule doit être un acte explicite de l'opérateur, une fois le jeu validé par le juriste. Elle ne doit pas être l'effet d'une sauvegarde d'éditeur ou d'un fichier à moitié écrit : une surveillance lirait ces états intermédiaires et produirait des alertes parasites, voire une bascule sur une version non voulue.
   - C'est aussi la convention Unix pour « relis ta configuration ».
   - Aucune dépendance n'est ajoutée (pas de `fsnotify` au `go.mod`).
   - La bonne pratique pour publier le fichier est de l'écrire à côté puis de le renommer, comme le fait le test.
2. **L'écoute de `SIGHUP` commence au lancement du processus**, avant la connexion aux backends : sinon, un `SIGHUP` reçu pendant le démarrage tuerait le gate (action par défaut du signal).
3. **Le jeu rechargé doit garder l'identifiant du jeu actif** (`ErrJeuAutreID`). Un rechargement met à jour la politique, il ne la remplace pas par une autre. Voir la question 1.
4. **Une version déjà chargée doit revenir à l'identique** (`ErrVersionReecrite`). Le rejeu retrouve un jeu par (id, version) ; deux contenus sous la même version rendraient le journal ambigu. C'est ce qui permet le retour arrière en toute sûreté. Aucune contrainte d'ordre n'est imposée : on peut recharger une version inférieure.
5. **Un jeu identique au jeu actif ne bascule rien et n'écrit rien au journal** (une trace sur stderr seulement).
6. **Un appel en vol pendant une bascule** garde le jeu qu'il a lu, et son entrée `appel` peut donc être écrite *après* le `jeu_charge` de la nouvelle version. C'est sans conséquence pour le rejeu, puisque chaque entrée cite son jeu. J'ai préféré ne pas bloquer les rechargements derrière un appel lent au backend (jusqu'à 30 s).
7. **Message d'alerte tronqué à 4000 caractères** : le chargeur rend toutes ses erreurs en bloc, et une ligne de journal est bornée.
8. **Les alertes d'échéance de relecture** du nouveau jeu sont écrites au rechargement, comme au démarrage.
9. **Sous-agent : valeur illisible ignorée.** Une valeur qui n'est pas une chaîne UTF-8 de 1 à 128 octets sans caractère de contrôle n'est pas journalisée ; une alerte le signale sans citer la valeur. L'appel suit son cours.
10. **Sous-agent : `internal/journal` touché**, un seul champ `omitempty` ajouté à `Entree`, sur demande explicite. Les journaux existants se vérifient et se rejouent à l'identique.

Revue de code :

11. **Entrée `transmission` écrite avec le `TS` de l'appel** (celui passé à `Decide`), verrou tenu, après le dépôt au coffre. Elle porte aussi l'appelant, le jeu et l'outil exposé, en plus des champs demandés.
12. **Un journal antérieur au commit `16bc3d8` ne se rejoue plus sans écart** : ses appels transmis n'ont pas d'entrée `transmission`. Aucun journal réel n'existait ; je n'ai pas ajouté de mode de compatibilité.
13. **Origines de provenance reprises du vocabulaire des serveurs de `feat/donnees`** (`direct`, `rejeu`, `aucune`, lues dans `structuredContent.source`), et non `live`. Une origine hors de ce vocabulaire devient `non_declaree` plutôt que d'être recopiée. La date est normalisée en UTC RFC 3339 ; illisible, elle est omise. Un appel transmis sans réponse porte aussi `non_declaree`.
14. **Les cibles sont contrôlées pour toutes les directives, brouillons et retirées compris** : une directive mal ciblée est une erreur de rédaction, quel que soit son statut.
15. **La règle du registre est dans un paquet neuf, `internal/controle`**, appelé par le gate et par le rejeu. La même fonction décide des deux côtés, sans modifier `internal/directive`.
16. **Contrôle `statut` au rejeu** : une attente transmise n'est cohérente qu'avec une `approbation` dans l'entrée. Cela prépare la reprise avec jeton de vendredi. La fixture des tests du rejeu, qui transmettait ses attentes « supposées approuvées », porte désormais cette approbation.
17. **Champ `origine` ajouté au refus** (`directive`, `registre`, `engine`), en plus des cinq champs demandés. Sans lui, un client ne distingue pas un refus du registre d'un refus de directive. Voir la question 5.
18. **`directive` et `version` d'un refus du registre** : identifiant et version du registre. Ceux d'un refus de l'engine sont vides (`""`, `0`), avec le fondement « Contrat de l'engine ».
19. **Une source injoignable ou une erreur de protocole n'est pas un refus.** Depuis `044657e`, c'est un échec structuré `statut: "indisponible"`. L'erreur de protocole y est rangée aussi (le journal la classe `erreur`), car l'engine n'en transmet rien.
20. **Variables d'environnement : une entrée de `commande.env` contenant `=` est refusée sans être citée** dans l'erreur, puisqu'elle porte peut-être une valeur secrète.

## 4. Blocages

Aucun.

## 5. Questions pour Manu

Réponses reçues et appliquées :
1. Pas de changement d'identifiant à chaud ; les deux garde-fous sont gardés.
2. Le retour arrière reste autorisé et il est marqué (commit `d0eaca8`).

3. `version_depart` et `version_arrivee` sur toute bascule à chaud (commit `d16a70f`).

4. Les provenances sont contrôlées comme les cibles (commit `fbbad22`).
5. Le champ `origine` des refus est gardé (rien à changer).
6. Les échecs ont une forme structurée `{statut: "indisponible", serveur, outil, motif}` (commit `044657e`).

Aucune question ouverte. La branche part en fusion : travail arrêté.

## 6. Vérifier en 5 minutes

```sh
cd ~/cybercar/mcp_rogue-gate
git log --oneline 5b71e4c~1..HEAD
go vet ./... && go test -race ./...
go test -race -count=1 -v -run 'Recharger|SousAgent' ./internal/gate/
go test -race -count=1 -v -run Rechargement ./test/gate/     # vrai binaire, vrais SIGHUP
# Revue de code :
go test -race -count=1 -v -run 'JournalEnPanne|ProvenanceDeclaree|Cible|Provenance|ControleRegistre|Contrat' ./internal/gate/
go test -race -count=1 -v -run 'Transmission|StatutEtEffet' ./internal/rejeu/
go test -race -count=1 -v -run Environnement ./internal/backend/
go test -race -count=1 -v -run Alteration ./test/gate/
```

À la main : lancer `mcp-gate`, modifier le fichier `-directives`, puis `kill -HUP <pid>` ; l'entrée `jeu_charge` apparaît au journal.
