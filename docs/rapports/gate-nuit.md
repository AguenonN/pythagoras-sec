# Rapport de nuit — session principale, agrégateur `cmd/mcp-gate`

Branche `feat/gate`, worktree `~/cybercar/mcp_rogue-gate`. Rien n'est poussé, rien n'est fusionné, aucun `--force`.
Ce fichier n'est pas commité (il est tenu à jour à la racine du worktree, comme `nuit/`).

**En une ligne : les 6 étapes du brief sont faites, et le critère de réussite est atteint.** Deux backends de transports différents sont routés (stdio + HTTP streamable TLS), la provenance est tenue par l'engine, et le journal relu depuis le disque se rejoue sans écart. Le scénario passe aussi à travers le vrai binaire `cmd/mcp-gate`.

## 1. Fait

| Étape | Commit | Contenu |
|---|---|---|
| 1. `internal/registre` | `6bffb82` | format JSON, chargeur strict (erreurs en bloc), empreinte du registre, pins d'outils, vue `EntreeRegistre` pour `ControlesEnVigueur` |
| 2. `internal/backend` (stdio) | `7037c2b` | interface `Backend` du brief, transport stdio avec empreinte vérifiée **avant** lancement, comparaison stricte au catalogue épinglé, quarantaine sur `tools/list_changed`, refus sampling/elicitation **prouvé par un serveur JSON-RPC écrit à la main** qui ignore les capacités ; `internal/backendtest` (vrais serveurs MCP de test, compilés pendant le test) |
| 3. backend HTTP streamable | `c3144be` | pin SPKI du certificat feuille + `ServerName` imposé et vérifié, à chaque poignée de main ; ni proxy ni redirection ; testé derrière `httptest` TLS (conforme ; pin faux : zéro requête HTTP reçue ; nom non couvert ; catalogue non conforme ; quarantaine par `list_changed` en HTTP) |
| 4. `cmd/mcp-gate` + `internal/gate` | `4a24b89` | exposition `serveur__outil` depuis le registre, pipeline complet (les 8 étapes du brief, dans l'ordre du contrat de l'engine), scellement des réponses, contrôle du registre toujours actif, panne du journal ⇒ engine fermé ; 13 tests du pipeline (backends factices, vrai client MCP en mémoire), dont 7 exigent un rejeu sans écart, plus 1 test de la lecture de clé |
| 5. **Point de contrôle** | `6ca1e39` | **critère de réussite atteint** : `test/gate/gate_e2e_test.go`. Un gate devant `taux-officiel` en **stdio** (binaire compilé, empreinte vérifiée) et `banque-synthetique` en **HTTP streamable TLS** (SPKI épinglé). Prêt 1 routé en HTTP, taux routé en stdio, prêt 2 refusé : la décision cite `D-001 v1 (provenance_session, RGPD art. 22)`, l'instantané de session porte les deux consultations, et **le journal relu depuis le disque se rejoue sans écart** (`rejeu.RejouerJournal`). Vue avant/après sans D-001 : le prêt 2 passe de `refuser` à `autoriser`. Le même scénario passe **à travers le binaire `cmd/mcp-gate`** lancé en stdio. |
| 6. Imposteur | `1bb7259` | trois imposteurs, chacun refusé par la directive `source_attestee` qui le cible (fondement cité). **Binaire d'empreinte différente** : jamais lancé (prouvé par un fichier témoin), description épinglée exposée, la banque attestée reste routée. **Bon binaire au catalogue empoisonné** : lancé, mais sa description n'est jamais exposée. **Serveur HTTPS avec sa propre clé pour le même nom** : aucune requête HTTP ne lui parvient. Attestation en échec journalisée avec ce qui a été observé, aucun texte de l'imposteur au journal, rejeu sans écart. |
| en plus | `0a7c08e` | délai du backend : la consultation est `indisponible`, le verrou est rendu et l'appel suivant avance ; quarantaine en cours de session : journalisée, appels suivants refusés par D-003 (le motif de l'application nomme la quarantaine), l'outil apparu n'est jamais exposé |
| demande du matin | `5b71e4c` | `_meta.sous_agent` lu et journalisé dans l'entrée `appel` sous `sous_agent_declare` (déclaration non prouvée : ni `Decide`, ni état de session, ni serveur ne la voient ; l'appelant prouvé reste celui du transport). **Touche `internal/journal`** : un champ `omitempty` ajouté à `Entree`, sur demande explicite. |
| en plus | `24d0b05` | la session d'un serveur non attesté à la connexion est fermée tout de suite (le processus stdio de l'imposteur ne reste pas en vie) |

Tous les commits ont passé `go vet ./...` et `go test -race ./...` en vert. Les nouvelles suites ont aussi passé 5 fois de suite (`-count=5`), sans instabilité.

**Périmètre respecté** : `git diff --stat 69e32ee..HEAD` ne contient **que des ajouts** (18 fichiers nouveaux). Aucun fichier existant n'est modifié : ni `internal/{directive,journal,session,rejeu,chaine,nommage}`, ni `mcp-proxy`, ni `internal/proxy`, ni rien d'antérieur au tag. Aucun binaire, secret ou `.env` n'est commité : les binaires de test sont compilés dans des répertoires temporaires, supprimés ensuite.

## 2. Pas fait (et pourquoi)

Rien de ce que le brief demandait n'est resté en plan. Suites possibles, hors brief :

- **HTTP + SPIFFE (jeudi)** : l'emplacement est prêt. Il y a `registre.HTTPSpiffe` et `ModeSpiffe` (validation `spiffe://`), ainsi qu'un `case registre.HTTPSpiffe` dans `backend.Connecter`. Il faudra lever le refus du chargeur (`registre.go`, `case HTTPSpiffe`) et écrire `connecterSpiffe` sur le modèle de `connecterHTTP`, avec `internal/identity`.
- **Reprise après validation avec jeton (vendredi)** : `attendre_validation` rend pour l'instant un refus avec `structuredContent = {statut: "en_attente", empreinte_transmise, directive, directives}`. La reprise devra viser `empreinte_transmise`.
- **Reconnexion d'un backend en quarantaine** : il n'y en a aucune. Une quarantaine dure jusqu'au redémarrage du gate. C'est le choix le plus prudent, mais il est peut-être trop dur en production.
- **Aucun essai avec un vrai agent Mistral** : ce n'était pas demandé. Le brief n'autorisait aucun secret ni aucun réseau autre que les registres de paquets, et aucun appel Mistral n'a été fait.

## 3. Décisions prises seul

Registre :
1. **Format du registre en JSON strict** (`DisallowUnknownFields`), sur le modèle de `directive.Charger`. Le paquet ne lit aucun fichier.
2. **Aucune attestation `aucune` n'est admise** au registre. Un serveur sans attestation serait de toute façon refusé à chaque appel : autant le refuser au chargement.
3. **`http-spiffe` est réservé mais refusé au chargement** (« pas encore disponible »). C'est fermé par défaut, plutôt qu'un transport qui échouerait à la connexion.
4. **Les accès des outils sont notés `read` / `action`** (pas `lecture`), pour coller au vocabulaire déjà utilisé par `directive.EntreeRegistre` et `toolpolicy`.
5. **Le chemin stdio doit être absolu**, jamais résolu dans le `PATH` : ce qui est haché est exactement ce qui est lancé.
6. **Le pin SPKI s'écrit `sha256:<hex>`** (pas en base64 façon HPKP), pour garder un format unique avec l'empreinte binaire.
7. **Le catalogue épinglé contient le schéma complet** (nom, description, `input_schema`), pas seulement un hash : c'est ce que l'engine expose, y compris pour un serveur non attesté. Le pin de comparaison est `toolpin.Hash` sur ces trois champs.

Backend :
8. **La comparaison du catalogue est stricte.** Tout outil épinglé doit être annoncé à l'identique, et **un outil annoncé hors registre fait aussi échouer l'attestation** (fermé par défaut ; un outil qui apparaît est le « rug-pull » décrit par `toolpin`). Voir la question 2.
9. **Le détail d'une attestation en échec ne cite jamais une chaîne fournie par le serveur**, seulement des noms venant du registre et des nombres. Ce détail remonte au journal et dans le motif rendu à l'agent, et un serveur hostile ne doit pas pouvoir y glisser du texte. La cause brute (corps HTTP, etc.) ne va que dans l'erreur rendue à l'opérateur, sur stderr.
10. **Sur `tools/list_changed`, le backend est suspendu pendant la revérification** : attestation dégradée, appels refusés. On ne laisse pas passer d'appels pendant qu'on liste. Si le catalogue est conforme, le backend redevient attesté. En cas de dérive, ou si la liste est impossible, la quarantaine est définitive jusqu'à reconnexion. Chaque changement donne une entrée `attestation` au journal.
11. **Le serveur stdio est lancé avec un environnement vide** : les secrets de l'engine (`MISTRAL_API_KEY`, `PISTE_*`) ne passent pas aux serveurs. Voir la question 3.
12. **Le middleware multi-aller-retour (SEP-2322) du go-sdk est désactivé**, et aucune capacité client n'est annoncée (`Capabilities: &ClientCapabilities{}`, donc pas même `roots`). Un résultat `input_required` est une erreur (`ErrEntreeDemandee`).
13. **Limite connue (TOCTOU)** : entre le hachage du binaire et l'exec, un fichier remplaçable pourrait changer. C'est la même limite que `mcp-proxy`. Mitigation documentée : répertoire inscriptible par l'opérateur seul.
14. **En TLS, la chaîne de certification n'est pas vérifiée : le pin SPKI la remplace** (`InsecureSkipVerify` + `VerifyConnection`, qui épingle la clé *et* vérifie le nom imposé). Une AC compromise ne suffit donc pas. En contrepartie, un renouvellement de clé côté serveur exige de mettre à jour le registre.
15. **Un serveur non attesté à la connexion voit sa session fermée tout de suite**, son processus stdio compris.

Gate :
16. **La logique du gate est dans `internal/gate`.** `cmd/mcp-gate` n'est qu'un `main` (flags, clé, fichiers), ce qui permet aux tests de bout en bout de piloter le vrai gate dans le processus.
17. **Le contrôle du registre s'ajoute à `source_attestee`.** Si l'attestation est invalide et qu'aucune directive `source_attestee` ne cible le serveur, l'appel est **quand même refusé**. L'entrée `appel` porte la décision de `Decide` telle quelle, avec `resultat.statut = non_transmis` et `message = "refusé par le registre : serveur non attesté"`. Le rejeu reste sans écart. Sans ce contrôle, un imposteur non ciblé par une directive aurait reçu les appels. Le contrôle passe aussi avant `attendre_validation` : on ne met pas en attente un appel vers un imposteur. Voir la question 4.
18. **Dépôt au coffre avant `Decide`** (ordre du brief), avec les seuls arguments d'origine. Les arguments transmis ne sont pas déposés : `Decide` étant déterministe, le rejeu les recalcule.
19. **Arguments décodés avec `json.Unmarshal` standard** (nombres en `float64`, sans `UseNumber`), comme à la relecture du coffre. L'empreinte d'origine se recalcule donc à l'identique au rejeu.
20. **Scellement.** Chaque texte du serveur passe par `envelope.Seal`. `structuredContent` est sérialisé puis scellé en texte, jamais transmis en clair. Le contenu non textuel (image, audio, ressource) est **retiré** et remplacé par une note de l'engine. Le `_meta` du serveur n'est pas repris. Les outils exposés n'ont pas d'`outputSchema`, donc la réponse reste conforme. Les descriptions exposées passent par `envelope.Defang`.
21. **Le `_meta` de l'agent n'est pas transmis au serveur** (`meta = nil`) : c'est un canal contrôlé par l'agent que rien ne filtre.
22. **Journal ou coffre en panne ⇒ engine fermé.** La réponse d'un appel déjà transmis est retenue (pas de donnée sans preuve), et tous les appels suivants sont refusés jusqu'au redémarrage. Même chose si l'ouverture de session échoue pour une raison inattendue.
23. **Le suivi de fin de session démarre au premier appel**, et non via `InitializedHandler` : avec le protocole 2026-07-28, qui n'a plus d'`initialize`, ce gestionnaire n'est jamais appelé.
24. **Classement des retours.** Une erreur JSON-RPC du serveur ou une demande d'entrée donne `erreur`. `IsError` donne aussi `erreur`, et le texte, scellé, est rendu à l'agent. Toute autre erreur (transport, délai) donne `indisponible`, avec un message générique à l'agent. `empreinte_reponse` vaut `sha256:` du JSON canonique de la réponse brute du serveur.
25. **Annotations des outils** : `destructiveHint` pour les outils `action`, `readOnlyHint` pour les outils `read`. Elles viennent du registre, jamais du serveur.
26. **Clé du journal** : variable `MCP_GATE_CLE_JOURNAL` (hexadécimal, ≥ 32 octets), jamais un fichier ni un flag. Aucun message d'erreur ne la cite (c'est testé).
27. **Motif rendu à l'agent pour `attendre_validation`** : `structuredContent.directive` porte la première directive appliquée (dans l'ordre du jeu), et `directives` les porte toutes.
28. **`RAPPORT_NUIT.md` n'est pas commité** : c'est un fichier de travail pour le matin, pas du code.

## 4. Blocages

Aucun. Le backend HTTP a fonctionné du premier coup : le plan B « deux backends stdio » n'a pas servi.

Un seul faux départ, réglé en un essai : à la fermeture de stdin, le go-sdk rend `server is closing: EOF` sans que `errors.Is(err, io.EOF)` réponde vrai. Le serveur de test et `gate.Servir` testent donc le suffixe du message.

## 5. Questions pour Manu (par urgence)

1. **Urgent : appelant `local-process` et périmètres.** Le brief fixe l'appelant stdio à `local-process`. Or `Decide` n'applique une directive à périmètre qu'aux appelants listés (`spiffe://…` ou `*`). Le chargeur n'accepte que ces deux formes, donc `local-process` ne peut jamais être listé : **toute directive à périmètre restreint est ignorée pour l'agent stdio**, alors qu'un appelant `""` (non prouvé) y serait soumis par défaut. J'ai suivi le brief. Si ce n'est pas voulu, passer `Appelant: ""` dans `cmd/mcp-gate/main.go`, ou admettre `local-process` dans les périmètres.
2. **Important : catalogue strict.** Un serveur réel qui annonce un outil de plus que ceux épinglés est aujourd'hui **non attesté** (tous ses appels sont refusés). L'alternative moins stricte serait de ne simplement pas exposer l'outil en trop. Laquelle veux-tu ?
3. **Important : environnement des serveurs stdio.** Ils sont lancés sans aucune variable d'environnement. Un serveur qui a besoin d'un secret (par exemple PISTE pour Légifrance) ne pourra pas l'obtenir. Faut-il une liste blanche de variables au registre, par serveur (noms seulement, valeurs prises dans l'environnement du gate) ?
4. **Moyen : forme du refus par le registre.** Sans directive `source_attestee`, le refus d'un imposteur est journalisé avec la décision de `Decide` (par exemple `autoriser`), `non_transmis` et un `message`. C'est rejouable sans écart, mais un écran qui ne lirait que `decision.effet` se tromperait. Faut-il plutôt imposer au chargement du jeu une directive `source_attestee` par serveur du registre, ou un champ dédié dans l'entrée `appel` (ce qui toucherait `internal/journal`, que je n'ai pas touché) ?
5. **Moyen : motif rendu à l'agent.** `Decide` ne met dans `dec.Motif` que les directives et fondements, pas le détail de chaque application : l'agent lit « refusé par D-003 v1 (source_attestee, RGPD art. 5) », sans la raison de l'échec d'attestation, qui est au journal. Ça me paraît bien (moins de fuite), à confirmer.
6. **Faible : pin TLS sans chaîne.** Voir la décision 14. À confirmer pour la démo, où les serveurs réels auront probablement des certificats d'AC publique dont la clé tourne.

## 6. Vérifier en 5 minutes

```sh
cd ~/cybercar/mcp_rogue-gate
git log --oneline 69e32ee..HEAD                     # 8 commits de la nuit
git diff --stat 69e32ee..HEAD | tail -1             # 18 files changed, only insertions
go vet ./... && go test -race ./...                 # tout vert (~1 min)

# Le point de contrôle, en clair :
go test -race -count=1 -v -run PointDeControle ./test/gate/
# L'imposteur et la robustesse :
go test -race -count=1 -v -run 'Imposteur|Robustesse' ./test/gate/
```

À lire en premier : `internal/gate/gate.go` (fonction `Traiter`, le pipeline) et `test/gate/gate_e2e_test.go` (le scénario).

Pour lancer le binaire à la main : `go build -o /tmp/mcp-gate ./cmd/mcp-gate`, puis
`MCP_GATE_CLE_JOURNAL=<64 hex> /tmp/mcp-gate -registre R.json -directives J.json -journal j.jsonl -coffre c.jsonl`.
Le registre d'exemple est dans `internal/registre/testdata/lab.json`, mais ses chemins et ses pins sont fictifs.
