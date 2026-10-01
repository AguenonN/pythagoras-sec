# Rapport : session « gate-v2 », brancher le gate sur Decide v2

*Rapport de la nuit, complété après les réponses de Manu (retours 2 et 5).*

Branche `feat/gate-v2`, worktree `~/cybercar/mcp_rogue-gate-v2`, partie de
`782ff37`. Rien n'est fusionné ni poussé.

**Critère de réussite atteint** : les onze étapes du brief sont faites et
commitées, une par unité logique ; `go vet ./...` et `go test -race
-count=1 ./...` sont verts (tests d'intégration compris, avec le vrai
`mcp-gate` devant les vrais serveurs), ainsi que les 113 tests Python. Les
quatre scénarios de bout en bout du brief (week-end, top management, RH,
demandeur et rapport) passent, et chacun de leurs journaux est vérifié et
rejoué sans écart, par le paquet `rejeu` et par la commande `journal`.
Une directive v2 qui vise une étiquette ou un accès n'est plus refusée par
défaut.

## 1. Fait

| Étape | Commit | Contenu |
|---|---|---|
| 1 | 5d8e162 | Registre : `etiquettes` par serveur et par outil (validées, dans l'empreinte, omises si absentes : l'empreinte d'un registre v1 est inchangée), `Registre.Version()` (`id vN sha256:…`), `Serveur.EtiquettesOutil`. `registre-demo` lit `demo/etiquettes.json` (versionné) ; registre de démo passé en version 2. |
| 2 | 8d5426d | Gate : `VersionRegistre`, `Acces`, `EtiquettesCible`, `SousAgentDeclare` passés à `Decide` ; consultations complétées (étiquettes, version, source déclarée) avant `Decide` ; champs v2 écrits à l'entrée `appel` (`version_registre`, `acces`, `etiquettes_cible`, instantané complété, décision avec notifications / marques / validation / résultat / fondements). Contrôle des cibles au démarrage étendu aux directives v2 (il refusait tout jeu v2 : la cible v1 vide ne correspondait à aucun serveur). Rejeu : consultations reconstituées avec leurs compléments (nécessaire pour que les tests existants restent verts). |
| 3 | 0e1a9bb | Rejeu : `AppelDepuis` reconstitue l'appel en entier (registre, demandeur, sujets, sous-agent) ; le rejeu compare aussi motif, applications (résultat à trois valeurs), alertes, notifications, marques, validation. Tests : journal v2 rejoué sans écart ; huit altérations de champs v2 détectées ; aller-retour JSON. |
| 4 | 153f1b5 | `internal/annuaire` : document signé Ed25519 par une autorité dont la clé publique est fournie à part, identités fictives, groupes, rôles datés (`depuis`, `jusqu_a`), clé publique de chacun ; interface `Source` (implémentation `Local`, nom `annuaire-local-demo`) ; jetons JWT EdDSA d'identité du demandeur et d'approbation. Entrée `annuaire_charge` au démarrage (source, id, version, empreinte). `mcp-gate -annuaire -annuaire-autorite`. `mcp-approve annuaire-demo / identite / valider`. Source versionnée sans clé : `demo/annuaire-source.json`. |
| 5 | f8faadd | Demandeur : jeton d'identité (`MCP_GATE_JETON_DEMANDEUR`) vérifié au lancement ; à l'ouverture de session, groupes résolus à cette date et consignés dans `session_ouverte` (avec `source_identite` et la référence de l'annuaire), fournis à `Decide` et à l'entrée `appel`. Sans jeton : demandeur absent, les directives qui le lisent sont indéterminées (refus). Rejeu : le demandeur de chaque appel doit être celui de `session_ouverte`. |
| 9 | 0386627 | Validation par rôle et reprise : la réponse d'attente porte `roles`, `quatre_yeux` (et `motif_jeton` si une reprise est refusée). À la reprise (même appel, `_meta.approvalToken` : un jeton ou une liste), vérification signature / période / appel visé / rôle à la date de validation / un jeton par rôle / valideurs distincts / valideur ≠ demandeur ; entrée `approbation` (acceptée ou refusée), puis transmission. Rejeu : contrôle `approbation` (entrée chaînée identique avant la transmission, règles, rôles revérifiés si `Options.Annuaire`). `docs/contrat-engine.md` §6 mis à jour. |
| 10 | c57aecc | `cmd/journal` : `verifier <fichier>` (chaîne HMAC, période, sessions et demandeurs, décisions, résultats, approbations, notifications, alertes, transmissions orphelines) et `rejouer <fichier> -directives <jeu> [<jeu2>] [-coffre] [-annuaire -annuaire-autorite]` (tableau appel par appel : journalisé / rejoué / écarts, motifs, détail des écarts ; avec deux jeux, vue avant / après et transitions). Sortie française, code 0 / 1 (écart ou chaîne rompue) / 2 (usage). |
| 8 | 998c5db | Notifications : une entrée `notification` par notification de la décision, écrite avant toute transmission, avec statut de remise (`remise`, `echec`, `planifiee`) ; canal de démo = abonnés du journal, compte exact (`EcrireRemise`) ; `si_echec: refuser` non remise → refus d'origine `notification` (`journal.ControleNotifications`, appelée par le gate et le rejeu) ; l'entrée `appel` porte ses `remises`, contrôlées au rejeu. Marques : journalisées dans la décision. `mcp-gate -notifications <fichier>` (écran de démo abonné) et `-horloge-depart` (horloge simulée, signalée par une alerte au journal). Contrat de l'engine §6 et §7. Jeux `demo/directives/weekend.json` et `top-management.json`. |
| 11 (a) | d320860 | `test/integration` : week-end avec le gate réel et l'horloge simulée, aux quatre horodatages limites (vendredi 23:59, samedi 00:00, dimanche 23:59, lundi 00:00, heure de Paris) : lecture autorisée, action refusée le week-end ; top management : attente par rôle, reprise avec le bon valideur, refus d'un valideur sans le rôle, d'un rôle échu, d'un valideur identique au demandeur ; notification du juridique sur l'écran abonné ; sans écran, refus d'origine notification. Chaque journal est vérifié et rejoué sans écart par `rejeu` et par la commande `journal` (annuaire compris). |
| 6 | 12d02e2 | Personnes concernées : `personnes_concernees` (pointeur JSON) par outil au registre (`demo/etiquettes.json` pour rh-synthetique) ; avant `Decide`, le gate lit le ou les identifiants et résout leurs groupes auprès de `referentiel-rh/groupes_salarie` seulement si ce serveur est attesté (délai 10 s par appel, 100 identifiants au plus) ; `Sujets` (empreinte + groupes) et `sujets_non_resolus` (empreintes) consignés dans l'entrée `appel`, jamais l'identifiant en clair. Introuvable = non fourni à `Decide` = indéterminé. Jeu `demo/directives/rh.json`. Test de bout en bout : contentieux → refus ; protégé → attente DRH ; introuvable → refus ; sans groupe → transmis ; sanction sur un identifiant seul. |
| 7 | 4dbd6f1 | Évènements : `directive.DecideEvenement` (même moteur que `Decide`, évènement en paramètre) ; chargeur : évènements admis à tout statut, feuilles et effets restreints à ce que l'évènement renseigne (schéma §4 mis à jour, évolution signalée). Gate : `ouverture_session` évaluée dans le crochet d'ouverture (refus → session fermée au premier tour, motif cité), `attestation_echouee` à chaque attestation en échec, `purge` par un crochet `Coffre.AvantPurge` ; décision, jeu et remises consignés dans l'entrée de l'évènement ; notifications remises comme pour un appel. `rejeu.RejouerEvenements`, affiché par `cmd/journal rejouer`. **Défaut préexistant corrigé** : la fermeture forcée d'une session (bornes `-max-consultations`, `-age-max`, et désormais ouverture refusée) appelait `ServerSession.Close` depuis le gestionnaire de l'appel, qui attend la fin de ce même gestionnaire : interblocage. Test ajouté (il échoue sans le correctif). |
| 11 (b) | 2cd92e2 | `test/integration` : demandeur du groupe `prestataires-externes` qui a reçu des données client (recherche-entreprises, étiquetée `donnees_client`) puis remet un rapport (`production_rapport`) → refus U-001 et notification remise au DPO ; sans données client, ou pour une analyste → transmis, pas de notification. Serveur fictif `test/integration/rapports` (outil `remettre_rapport`, catalogue épinglé lu par tools/list), ajouté au registre de test. Jeu `demo/directives/rapport.json`. Fermeture forcée : la connexion se ferme après un délai de grâce d'1 s (le SDK retire l'appel des requêtes en cours avant d'écrire sa réponse : une fermeture immédiate la coupait). |
| réponse du brief | 6495044 | Correctif signalé dans `internal/directive` : sous un quantificateur, `$racine/…` lit les arguments (feuilles `argument`, `sujet`, champ d'un quantificateur imbriqué) ; refusé hors quantificateur. Schéma §5.1 complété. |
| revue | d49f39f, 9c82f6f | Jetons d'approbation à **usage unique** (sinon une validation autorisait deux fois le même prêt dans les 15 min) ; le rejeu signale un jeton accepté pour plusieurs appels. Contrat mis à jour. |
| — | fd4e06f | gofmt d'un fichier de test. |
| retour 5 | bfbfff9 | `mcp-gate -journaux-precedents <motif>` : au démarrage, relecture (chaîne vérifiée, même clé) des entrées `approbation` acceptées des journaux désignés ; un jeton déjà consommé est refusé, avec une entrée `approbation` refusée « jeton déjà utilisé ». Journal précédent rompu ou illisible : démarrage refusé, avec un message qui le nomme. Test de bout en bout : deux processus `mcp-gate` successifs, même jeton, le second refuse ; et refus de démarrer sur un journal rompu. |
| retour 2 | f7eaf46 | `docs/contrat-engine.md` §8 : résolution des personnes concernées, et sa limite connue (pas d'entrée chaînée avant l'appel au résolveur). |
| avant fusion | commit « decision.duree_us » (dernier de la branche) | `decision.duree_us` sur chaque entrée `appel` : durée de l'évaluation, de l'entrée dans le pipeline (`Gate.Traiter`) jusqu'à la décision rendue (contrôles du registre et des notifications compris), en µs arrondies au supérieur. **Ajout `omitempty` dans `internal/journal`** (`Decision.DureeUs`), signalé ici. Le rejeu ne le compare jamais. Tests : présent sur chaque appel (gate et binaire réel en intégration) ; chaîne vérifiée ; journal rejoué sans écart, et sans écart non plus quand la durée est modifiée. |

## 2. Pas fait, ou fait autrement que le brief

- **`rejeu.RejouerEntree`** n'existe pas sous ce nom : la fonction est
  `rejeu.Rejouer` ; la reconstitution de l'appel est `rejeu.AppelDepuis`.
- **`scripts/lancer-demo.sh`** n'est pas modifié : une autre session le
  reprendra après la fusion. La démo Mistral tourne toujours avec
  `credit-demo.json`, sans annuaire. Pour essayer la v2 à la main, voir §6.
- **Entrée chaînée avant l'appel au résolveur RH** : pas faite (décision de
  Manu), documentée comme limite connue (contrat §8).
- **Compilateur et relecture (Python)** : ne connaissent ni les étiquettes
  du registre, ni les évènements, ni `$racine` (hors périmètre).
- **Écran de synthèse** : les notifications `synthese` sont consignées
  « planifiée », rien ne les regroupe encore.

## 3. Décisions prises seul

1. **Étiquettes d'un outil = étiquettes du serveur ∪ étiquettes de l'outil.**
   C'est ce que reçoivent `Appel.EtiquettesCible` et les consultations.
2. **Version du registre** = `"<id> v<version> <empreinte>"`. Vide tant que
   le registre n'est pas validé : Decide tranche alors par `si_indetermine`.
3. **`demo/etiquettes.json`** : un serveur ou un outil nommé inconnu du
   registre est une erreur (une faute de frappe laisserait une directive
   sans cible) ; une règle `*/outil` peut ne rien toucher, puisqu'aucun
   serveur de démo n'a encore d'outil `remettre_rapport`.
4. **Format d'étiquette** imposé par le registre : `^[a-z][a-z0-9_-]{0,63}$`.
5. **Compléments de consultation tenus par le gate**, pas par la session :
   `internal/session` n'est pas dans la liste de ce que je peux modifier. Le
   gate garde, par session et par seq, les étiquettes, la version du
   registre et la source de chaque consultation, pris dans l'entrée `appel`
   qui la journalise, et les ajoute à `tour.Etat()` avant `Decide`. Le
   rejeu les relit dans la même entrée : mêmes valeurs des deux côtés.
6. **Source « non_declaree » = pas de source pour Decide.** Le marqueur de
   l'engine n'est pas présenté comme une origine déclarée : la feuille
   `metadonnee_reponse` est indéterminée (donc refus par défaut).
7. **Sous-agent déclaré passé à `Decide`** : le schéma v2 le prévoit (feuille
   `sous_agent_declare`, qui ne sert qu'à resserrer). Le commentaire du gate
   qui disait le contraire est corrigé.
8. **Le rejeu compare désormais le détail de la décision** (motif,
   applications, alertes, sorties v2), pas seulement l'effet et
   l'empreinte transmise : une condition indéterminée présentée comme vraie
   ne changeait ni l'effet ni le motif. Un journal antérieur (sans
   `resultat` d'application) n'est pas comparé sur ce champ.
9. **Aucune clé au dépôt, même fictive** (règle commune). Ce qui est
   versionné est `demo/annuaire-source.json` (identités, groupes, rôles,
   sans clé). `mcp-approve annuaire-demo` tire les clés et écrit
   `demo/annuaire.json` signé et `demo/execution/cles/` (0600) ; les deux
   sont ignorés par Git (`/demo/annuaire.json`, `*.cle`). Les tests
   génèrent leur propre annuaire avec des clés neuves.
10. **Clé de l'autorité fournie à part** (`-annuaire-autorite`), jamais lue
    dans l'annuaire : une signature vérifiée avec la clé qu'elle apporte
    ne prouve rien.
11. **Jeton d'identité** : signé par le demandeur lui-même, ou par une
    identité qui détient le rôle `emetteur-identite` (un portail) à la date
    d'émission ; sinon n'importe quelle identité de l'annuaire pourrait
    se faire passer pour une autre. Durée de vie 12 h au plus.
12. **Jeton d'approbation** : signé par le valideur pour lui-même, audience
    distincte de celle du jeton d'identité, 15 min au plus, lié à
    `empreinte_transmise` ; le rôle est vérifié à `iat` (date de
    validation). Deux identités sous la même clé sont refusées au
    chargement.
13. **Code des jetons dans `internal/annuaire`, pas dans
    `internal/approval`** : les fichiers d'`approval` datent d'avant
    `baseline-pre-hackathon` ; je n'y touche pas, même si le brief
    l'autorise. `cmd/mcp-approve/main.go` (lui aussi d'avant la baseline,
    autorisé par le brief) ne gagne qu'un aiguillage vers les
    sous-commandes ; l'usage historique est inchangé.
14. **`internal/session` modifié a minima (seule entorse à la liste du
    brief)** : le brief demande que les groupes du demandeur soient
    consignés « dans `session_ouverte` », entrée que seul ce paquet écrit.
    J'ai ajouté un crochet `Config.AOuverture` (nil : comportement
    inchangé) qui complète l'entrée avant écriture et peut refuser
    l'ouverture (servira à l'évènement `ouverture_session`). Les champs
    `TS`, `Type`, `Session`, `Appelant` ne peuvent pas être changés par le
    crochet. Test ajouté dans `session_test.go`.
15. **Jeton du demandeur dans l'environnement** (`MCP_GATE_JETON_DEMANDEUR`),
    jamais sur la ligne de commande. Vérifié au lancement (un jeton refusé
    empêche le démarrage), puis revérifié à l'ouverture de session : s'il a
    expiré entre-temps, la session n'a pas de demandeur et le message de
    `session_ouverte` le dit.
16. **Horloge de session = horloge du gate** quand la configuration de
    session n'en donne pas : l'ouverture date la résolution du demandeur,
    elle doit suivre la même horloge que les appels.
17. **Valideur ≠ demandeur toujours**, pas seulement sous `quatre_yeux` :
    s'approuver soi-même n'est jamais une validation. Sous `quatre_yeux`
    sans demandeur résolu, refus (la règle ne peut pas être vérifiée).
18. **Un jeton par rôle requis, ni plus ni moins, valideurs distincts** :
    une même personne qui détient deux rôles requis ne peut pas valider
    deux fois. Sans rôle requis (v1 `approbation_humaine`), un seul jeton,
    de tout valideur de l'annuaire.
19. **Jetons refusés : l'appel reste en attente** (réponse `en_attente`
    avec `motif_jeton`), pas un refus : la décision de `Decide` est
    l'attente, et l'agent peut reprendre avec un bon jeton. Une entrée
    `approbation` refusée est écrite à chaque tentative.
20. **Jeton fourni pour un appel qui n'attend pas** : ignoré, avec une
    entrée `alerte`.
21. **Remise = au moins un abonné du journal atteint.** Un abonné saturé
    ne compte pas. Sans écran abonné, une notification `temps_reel` est en
    échec ; si elle est `si_echec: refuser`, l'appel est refusé (fermé par
    défaut). Pour la démo et les tests : `mcp-gate -notifications`.
22. **`synthese` + `si_echec: continuer` = planifiée**, pas remise
    maintenant (aucun écran de synthèse n'existe encore). `synthese` +
    `si_echec: refuser` est remise tout de suite : la règle exige une
    remise avant la transmission.
23. **Refus d'origine `notification`** : nouvelle origine dans le contrat
    de réponse (§6), qui cite la directive et son fondement ; s'applique
    même à une attente (l'appel ne partirait pas sans la notification).
    Un refus par directive ou par le registre garde la main.
24. **Horloge simulée** (`-horloge-depart`) : nécessaire pour tester le
    week-end avec le vrai binaire ; une entrée `alerte` le dit dans le
    journal. Elle avance au rythme réel.
25. **Les appels au résolveur des personnes concernées ne sont pas des
    consultations** : ils ne vont ni à l'état de session (ils fausseraient
    la provenance) ni au chaînage `transmission`. Ils partent avant toute
    entrée de l'appel ; leur résultat est consigné dans l'entrée `appel`.
    Si le journal tombait entre les deux, une requête au référentiel RH
    interne serait partie sans trace : risque jugé acceptable (serveur
    interne attesté, lecture seule), à confirmer (question 2).
26. **Le résolveur n'est interrogé que s'il est attesté** ; sinon tous les
    sujets sont non résolus (indéterminé, refus par défaut).
27. **Commit cdae9af amendé en 12d02e2** (local, jamais poussé) : il
    contenait un test rouge, mon enchaînement de commandes ne s'arrêtait
    pas sur l'échec. Corrigé avant toute autre chose.
28. **Évènements : feuilles et effets restreints au chargement** plutôt
    qu'indéterminés à l'évaluation : une feuille que l'évènement ne
    renseigne jamais (argument, consultation…) rendrait la directive
    toujours indéterminée, donc toujours déclenchée par défaut, ce qui a
    l'air de compter sans rien dire. `refuser` n'a d'objet qu'à
    l'ouverture ; une purge ou une attestation n'est pas « refusée ».
29. **Notifications d'ouverture écrites juste avant `session_ouverte`**
    (même session, seq 0) : le crochet s'exécute avant l'écriture de
    l'entrée d'ouverture. C'est la seule exception à « aucune entrée de la
    session ne précède `session_ouverte` », et elle ne porte que des
    notifications.
30. **Pas de décision consignée si aucune directive du jeu ne porte sur
    l'évènement** : les journaux sans directive d'évènement restent tels
    qu'avant.
31. **Serveur fictif `rapports` dans `test/integration/`** : aucun serveur de
    démo n'a d'outil `remettre_rapport`, et `servers/` n'est pas dans ce que
    le brief me permet de modifier. Conséquence : `demo/directives/rapport.json`
    ne se charge pas devant le registre de démo seul (le gate refuse une
    étiquette qu'aucun outil ne porte) ; il faut un outil
    `remettre_rapport` au registre (question 4).
32. **Délai de grâce de fermeture (1 s)** : pendant ce délai, la session
    est déjà fermée côté engine (tout appel y est refusé et journalisé
    comme tel) ; seule la connexion reste ouverte.
33. **Jetons d'approbation à usage unique** (ajout de revue) : non demandé
    par le brief, mais sans cela un même jeton valide 15 min permettait
    d'exécuter deux fois un appel identique.
34. **Horloge simulée et jetons** : avec `-horloge-depart`, les jetons sont
    vérifiés à l'heure simulée ; un jeton signé maintenant par
    `mcp-approve` peut y paraître futur ou expiré. Les tests signent à
    l'heure simulée.
35. **`-journaux-precedents` ne relit que les journaux désignés.** Un motif
    trop large, qui attrape aussi le coffre ou l'écran de notifications,
    empêche le démarrage : ces fichiers ne sont pas chaînés. C'est voulu
    (fermé par défaut) ; le message nomme le fichier fautif. Un motif qui
    ne désigne aucun fichier est admis (premier lancement).
36. **Motif de refus « jeton déjà utilisé (jeton N) »**, pour un jeton
    consommé dans ce processus comme dans un précédent.
37. **`decision.duree_us`** : mesurée sur l'horloge monotone du processus,
    jamais sur l'horloge de l'engine (qui peut être simulée) ; elle inclut
    l'attente du verrou de session, le dépôt au coffre, la résolution des
    personnes concernées et la remise des notifications, mais pas la
    transmission. Arrondie au supérieur, pour qu'une évaluation de moins
    d'une microseconde ne disparaisse pas sous `omitempty`. Elle est aussi
    présente sur l'entrée `transmission`, qui reprend la décision. Le rejeu
    n'est pas modifié : il compare la décision champ par champ (effet,
    origine, motif, applications, alertes, notifications, marques,
    validation), jamais l'objet entier.

## 4. Blocages

Aucun blocage externe : aucun appel réseau, aucun appel Mistral.

Deux défauts trouvés en route, corrigés et testés :

- **interblocage à la fermeture forcée d'une session** (préexistant, jamais
  testé) : `ServerSession.Close` appelé depuis le gestionnaire de l'appel
  attendait la fin de ce même gestionnaire ; un gate lancé avec
  `-max-consultations` ou `-age-max` se serait figé ;
- puis, une fois la fermeture rendue asynchrone, la **réponse du dernier
  appel était coupée** (EOF côté agent) : délai de grâce d'1 s.

Un commit (cdae9af) a été fait avec un test rouge, puis amendé localement
en 12d02e2 avant tout autre commit (décision 27).

## 5. Questions pour Manu

Toutes les questions de la nuit ont reçu réponse :

| Question | Réponse | Suite donnée |
|---|---|---|
| 1. Crochet `AOuverture` dans `internal/session` | accepté tel quel | — |
| 2. Entrée chaînée avant l'appel au résolveur RH | pas maintenant | limite connue, contrat §8 (f7eaf46) |
| 3. Valideur ≠ demandeur même sans `quatre_yeux` | oui, gardé | — |
| 4. `remettre_rapport` dans un serveur de démo | fait dans `feat/correctifs` | rien ici ; la fusion vérifiera que `rapport.json` se charge |
| 5. Unicité des jetons après redémarrage | à faire | `-journaux-precedents` (bfbfff9) |
| 6. Restriction des évènements au chargement | acceptée | — |
| 7. Annuaire dans `lancer-demo.sh` | autre session, après la fusion | rien ici |

Nouvelle question, non urgente : `-journaux-precedents` suppose que tous
les journaux relus ont la même clé HMAC que le journal courant. Un journal
écrit avec une autre clé est vu comme rompu et empêche le démarrage. Si la
démo change de clé d'une exécution à l'autre, il faudra soit garder la
clé, soit fournir une clé par journal.

## 6. Vérifier en 5 minutes

```sh
cd ~/cybercar/mcp_rogue-gate-v2
git log --oneline 782ff37..HEAD
go vet ./... && go test -race -count=1 ./...          # ~1 min
(cd agents && python3 -m unittest discover -s tests -t .)

# Les quatre scénarios de bout en bout, gate réel, en clair :
go test -count=1 -v ./test/integration -run 'WeekEnd|TopManagement|RH_|Demandeur_Rapport|JetonConsomme' 2>&1 | grep -E '^(=== RUN|--- |ok|FAIL)'

# Ce que voit un humain : un journal produit, vérifié et rejoué.
go test -count=1 -v ./cmd/journal -run TestVerifierEtRejouer
```

Essai à la main (annuaire fictif, clés hors Git) :

```sh
go run ./cmd/mcp-approve annuaire-demo                # demo/annuaire.json + demo/execution/cles/
export MCP_GATE_JETON_DEMANDEUR=$(go run ./cmd/mcp-approve identite -signataire claire.analyste@exemple.test)
# puis mcp-gate ... -directives demo/directives/top-management.json \
#   -annuaire demo/annuaire.json -annuaire-autorite demo/execution/cles/autorite.pub \
#   -notifications demo/execution/notifications.jsonl
# reprise : go run ./cmd/mcp-approve valider -valideur diane.risques@exemple.test \
#   -role directeur-risques -empreinte <empreinte_transmise>
# journal : MCP_GATE_CLE_JOURNAL=… go run ./cmd/journal rejouer demo/execution/journal.jsonl \
#   -directives demo/directives/top-management.json
```

À lire en premier : `docs/contrat-engine.md` (§6 reprise, §7
notifications), `internal/gate/gate.go` (`Traiter`), `internal/gate/validation.go`,
`internal/rejeu/rejeu.go` (`sortiesV2`, `controlerApprobation`,
`controlerRemises`), `internal/annuaire/jeton.go`.
