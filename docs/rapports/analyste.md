# Rapport E : Analyste (`agents/analyste/`)

Branche `feat/analyste`, partie de `5b83cd5`. Rien de poussé ni fusionné.

## 1. Ce qui est fait

| Commit | Contenu |
|---|---|
| `b5bd9b6` | Détecteurs déterministes, couverture par les directives, regroupement en constats, journaux de test |
| `b08908f` | Rédaction par Mistral, écriture des propositions (contrats §6), mesure par rejeu, critère D-009, enregistrements |
| `221badd` | La mesure rejoue toutes les sessions passées, pas seulement celles du constat |
| `db892ba` | Rapport (alors `RAPPORT_E.md`), `RAPPORT_NUIT.md` |
| `d8e9180` | Rapport déplacé ici ; écarts au contrat §6 acceptés par Manu |
| `268743a` | Suite embeddings, étape 1 : corpus local des fondements (§7) |
| `f73f68e` | Suite embeddings, étape 2 : sessions similaires et fondements suggérés (§7) |
| (étape 3) | Suite, point 5 : détecteur « contournement d'un caviardage » (§7) |

**Critère de réussite : atteint, enregistré, rejouable hors ligne.**
- Le détecteur « source interne périmée » trouve DOS-FICTIF-002 (passage nominal : base interne consultée à l'entrée 2, trimestre 2026-Q3 ; taux officiel consulté à l'entrée 5, trimestre 2026-Q4 ; attribution transmise à l'entrée 7).
- Mistral rédige : « *Attribution d'un prêt : si une source interne de taux (banque-synthetique/get_taux_interne) est consultée, l'agent attend la validation d'un analyste-risques avant d'attribuer le prêt.* » Fondement : « Code monétaire et financier — à valider par le juriste, article L313-5-1 ».
- Repris tel quel et passé au service du compilateur (contrats §4), ce texte donne **D-009** : `attribuer_pret`, `session.a_consulte banque-synthetique/get_taux_interne` (critère `donnees_recues`), `attendre_validation` par `analyste-risques`. Une seule question (celle du code sur `si_indetermine`), à laquelle j'ai répondu `refuser`, la valeur par défaut.

**Les deux cas attendus en plus :**
- *Attribution à une entreprise non retrouvée au répertoire SIRENE* : nouveau détecteur `entreprise_non_retrouvee`. Il trouve DOS-FICTIF-004 (SIREN 000000000) dans les deux passages « consigne » : recherché à l'entrée 2, 0 résultat ; attribution présentée à l'entrée 5 ; seule D-002 l'a retenue, par hasard. Il distingue aussi le cas « jamais recherché ». Proposition écrite (voir 4 pour sa limite).
- *Décision fondée sur la base interne sans consultation du taux officiel* : couverte par le même détecteur « source interne périmée ». Quand aucun taux officiel n'est consulté, la référence est le trimestre en vigueur à la date de l'action (passage « sans D-009 », DOS-FICTIF-003, attribution **autorisée sans validation**). Les deux variantes donnent une seule proposition, et c'est elle qui retrouve D-009.

**Détecteurs** (`detecteurs.py`, sans modèle) : source interne périmée, action sans contrôle humain au-delà d'un montant, données personnelles puis action vers une destination `externe`, entreprise non retrouvée au répertoire SIRENE. Chaque occurrence est datée et sourcée (session, `seq`, outil, décision, directives appliquées). Une occurrence déjà traitée par une directive appliquée à l'appel est écartée : par exemple, le passage « avec D-009 » ne ressort pas. Un constat que le jeu courant (`demo/directives/demo.json`) couvre n'est pas proposé.

**Propositions écrites** sur les 6 passages, 3 au total, dans `demo/execution/propositions/`. Copies dans `agents/analyste/exemples/propositions/`.

**Mesure** (`python -m analyste mesurer <id>`) :
- avant la décision du juriste ou avant la compilation : les sessions concernées seulement ;
- proposition rejetée : rien à mesurer ;
- après compilation : rejeu de toutes les sessions avec une copie temporaire du jeu candidat, où la directive ajoutée est active depuis le 01/01/2000. Pour D-009 : **1 appel change sur 6 sessions** (passage « sans D-009 », entrée 5 : `autoriser` devient `attendre_validation`).

**Tests** : 38, hors ligne (`agents/analyste/tests/`). La suite existante `agents/tests` passe aussi (191 tests). Aucun fichier Go n'a été modifié.

**Budget Mistral : 13 appels réels sur 15.** 12 pour la rédaction (4 itérations du prompt, dont 3 appels perdus sur une modification qui ne s'était pas appliquée) et 1 pour le compilateur (`mistral-medium-latest`, son modèle par défaut).

## 2. Ce qui n'est pas fait, et pourquoi

- **Les deux autres propositions n'ont pas été passées au compilateur** (budget : 2 appels restants, gardés en réserve).
  - « Montant ≥ 100 000 € » : le texte ajoute « et qu'aucune validation humaine n'a été enregistrée », ce qui provoquera probablement une question.
  - « Entreprise non retrouvée » : le fait n'est pas observable par l'engine (voir 4). On s'attend à `non_observable`, ce qui ferait une scène honnête.
- **Fondements discutables** sur ces deux propositions : `mistral-small` cite L562-4 (gels des avoirs) malgré la consigne. Ils sont marqués « à valider par le juriste ». Je n'ai pas relancé, faute de budget.
- **Le détecteur « données personnelles vers l'extérieur » n'a aucun cas réel** : aucun outil d'action étiqueté `externe` dans le registre de démo. Il est testé sur des sessions écrites à la main.
- **Lecture des propositions par la console non vérifiée en vrai.** Elle est vérifiée seulement contre le type Go `console.Proposition` lu dans le code (voir 3).

## 3. Décisions prises seul

1. **Journaux de la session A absents de la machine.** Les sessions `20261001-…` / `20261002-09…` de `docs/demo/passages.md` ne sont dans aucun worktree. Je les ai régénérées **hors ligne** à partir des enregistrements versionnés : `scripts/lancer-demo.sh scenario <nom> --mode rejeu`, pour les 6 scénarios, dans mon worktree.
   - Sans réseau, sans Mistral. Le registre local des gels a été copié depuis le checkout principal pour éviter un téléchargement hors du périmètre réseau.
   - Les journaux sont scellés avec une **clé de test (64 zéros)**, notée comme telle dans les tests. Ce n'est pas un secret : `lancer-demo.sh` l'utilise déjà pour son essai à blanc.
   - Ils sont figés dans `agents/analyste/tests/donnees/sessions/` (340 Ko, entreprises et dossiers fictifs, aucune personne physique).
   - « avec D-009 » s'arrête en erreur à l'appel 8, comme documenté dans `passages.md`. Les appels 1 à 7 suffisent.
2. **Le trimestre de la base interne n'est pas dans le journal.** `banque-synthetique` ne déclare pas de `structuredContent.source`, donc l'origine est `non_declaree` et il n'y a pas de date. Le détecteur le lit dans le fichier que le serveur sert (`servers/taux-officiel/instantane_trimestre_precedent.json`, `trimestre_instantane`), déclaré dans `agents/analyste/sources.json`, et le constat cite ce fichier.
3. **« Non retrouvée au répertoire » n'est pas dans le journal non plus** : seuls le statut `ok` et l'empreinte de la réponse y sont. Le détecteur relit la réponse datée que `recherche-entreprises` a servie (`demo/scenarios/enregistrements/serveurs/recherche-entreprises/`), reconnue par le même SIREN (lu au coffre) et la même date que la `source.date` du journal. Si la réponse n'est pas relisible, il n'y a pas de constat ; le cas est listé dans `non_verifiables`.
4. **`constat.detail` est une chaîne**, parce que la console lit `Detail string` (`internal/console/propositions.go`) : un objet rendrait la proposition illisible et la console l'ignorerait en silence. Les occurrences structurées vont dans un champ en plus, `constat.occurrences`, que la console ignore. **Écart au contrat §6 : accepté par Manu le 02/10.**
5. **Mesures hors du dossier des propositions** (`demo/execution/analyste/mesures/<id>.json`). La console lit tout `*.json` de `propositions/` (hors `.decision.json`) comme une proposition : un fichier `<id>.mesure.json` y serait apparu comme une proposition fantôme. **Écart au contrat §6 : accepté par Manu le 02/10.**
6. **Identifiants stables** : `A-<type>-<serveur>.<outil>`. Une proposition existante n'est jamais réécrite, et un risque qui a déjà une `.decision.json` (rejet compris) n'est jamais reproposé. Écriture atomique sans écrasement (`os.link`). L'Analyste n'écrit aucune décision.
7. **Seuil de montant : 100 000 €** par défaut (`--seuil-montant`). J'ai lu « au-delà d'un montant observé » comme : action d'un montant au moins égal au seuil, transmise sans validation humaine ni directive appliquée. Un seuil existant (D-005, plus de 2 M€) ne compte comme couverture que s'il retient le plus petit montant relevé.
8. **Ce que voit Mistral** : seulement la vue du constat (type, action, sessions, `seq`, dates, trimestres, effets, directives), plus un contexte tiré du **journal et du jeu, jamais du coffre** :
   - les rôles qui ont déjà validé l'action ;
   - les directives qui visent l'action, avec leurs fondements ;
   - le fait observable commun ;
   - la limite d'observation.
   Ni `detail`, ni SIREN, ni montant, ni dossier. Le code refuse une rédaction qui citerait une session inventée ou recopierait une valeur du coffre. Il ajoute « à valider par le juriste » et complète l'explication avec les sessions qu'elle ne cite pas.
9. **Le compilateur est appelé tel quel** (`compilateur.service.Service`), avec un client dont les enregistrements vont dans `agents/analyste/enregistrements/compilateur/` (mon périmètre, pas celui de B) et dont les appels sont comptés dans mon budget.
10. **Couverture par lecture des directives** (ce sur quoi porte la condition), pas par évaluation. C'est simple et prudent : au pire, l'Analyste propose un risque que le juriste jugera déjà couvert.
11. **Reprises dédoublonnées** : une attribution en attente puis reprise avec jeton compte une seule fois. C'est la dernière présentation qui est retenue.

## 4. Blocages rencontrés

- Une modification du prompt ne s'est pas appliquée (assertion de mon script d'édition) : 3 appels dépensés pour rien. Rattrapé en ne rédigeant ensuite que le constat utile (`--constat`).
- **Limite de fond** : l'engine ne lit pas le contenu des réponses. La directive « entreprise non retrouvée » n'est donc pas applicable aujourd'hui. L'Analyste le dit dans l'explication de la proposition.

## 5. Questions pour Manu (par urgence)

1. ~~**Contrat §6** : `detail` en chaîne plus `occurrences`, et le dossier `demo/execution/analyste/mesures/`~~ : **tranché par Manu le 02/10, les deux écarts sont acceptés.**
2. **Rendre « entreprise introuvable » observable** : `recherche-entreprises` pourrait déclarer `source.origine: "aucune"` (déjà au vocabulaire du gate : « aucune donnée servie ») quand le SIREN est introuvable. Une directive en `metadonnee_reponse` deviendrait alors possible, et je pourrais retirer la relecture des enregistrements.
3. **Dater la base interne** : `banque-synthetique/get_taux_interne` pourrait déclarer `structuredContent.source` avec une date de fin de trimestre. Le détecteur n'aurait plus besoin de `sources.json`.
4. Faut-il passer les deux autres propositions au compilateur (environ 2 à 4 appels medium) ?
5. Le compilateur a rendu le fondement de D-009 **sans** la mention « à valider par le juriste » (il ne garde que « Code monétaire et financier », L313-5-1). À voir côté B si la mention doit survivre.

## 6. Vérifier en 5 minutes

```sh
cd agents
# 1. Tests (hors ligne, jamais d'appel réel)
env -u MISTRAL_API_KEY python3 -m unittest discover -s analyste/tests -t .

# 2. Constats, sans modèle, sur les journaux figés
python3 -m analyste constats --sessions analyste/tests/donnees/sessions | python3 -c \
  "import json,sys; d=json.load(sys.stdin); [print(c['detail'],'\n') for c in d['constats']]"

# 3. Propositions (réponses enregistrées), dans un dossier jetable
T=$(mktemp -d); env -u MISTRAL_API_KEY python3 -m analyste proposer \
  --sessions analyste/tests/donnees/sessions --sortie $T; ls $T

# 4. Critère : texte proposé → compilateur → D-009 (rejeu)
env -u MISTRAL_API_KEY python3 -m analyste.critere | tail -5      # "conforme_d009": true
```

La mesure par rejeu demande `bin/journal`, construit par `scripts/lancer-demo.sh`. Le test `test_rejeu` est sauté s'il manque.

## 7. Suite du vendredi 17 h 35 : embeddings Mistral

Les embeddings servent à **proposer**, jamais à décider. Ils n'ajoutent que deux champs facultatifs à une proposition. Aucun constat n'est créé ni écarté par un score, et rien n'entre dans l'engine. Un test vérifie que les constats et les textes proposés sont identiques avec ou sans embeddings.

### Fait

**Étape 1, corpus local** (`agents/analyste/corpus/`, `python -m analyste.corpus`, commit `268743a`).
- Contenu : CMF L313-5-1 ; Code de la consommation L314-6, dans sa version en vigueur au 02/10/2026 (`LEGIARTI000032303335`, abrogation différée au 20/11/2026) ; RGPD articles 5, 22 et 32.
- Provenance : texte public rendu **par les serveurs MCP `legifrance` et `eurlex` du projet**, en `REJEU` sur leurs enregistrements, car aucun identifiant PISTE n'est dans l'environnement. Pas de donnée personnelle : le filtre d'`eurlex` retire les signataires.
- Chaque article garde sa source (serveur, outil, arguments, date, `sha256`) et un lien public.
- L'empreinte du corpus est consignée dans `index.json` (`sha256:1983a756…`) et vérifiée au chargement : un corpus retouché à la main ne se charge pas. Reconstruit à l'identique en rejeu (test).
- **Indisponibles, consignés et jamais écrits de mémoire** : CMF L562-4 (D-001), Code du travail L1121-1 (D-006) et L2411-1 (D-007). Aucun enregistrement, et PISTE serait nécessaire. Les politiques internes de D-003 et D-005 ne sont pas publiques : hors corpus.

**Étape 2, sessions similaires et fondements suggérés.**
- **Client d'embeddings** (`plongements.py`, `mistral-embed`) : un enregistrement par texte (`enregistrements/plongements/<sha256(modèle, texte)>.json`). En réel, les textes manquants partent en un seul lot ; en rejeu (défaut et tests), un texte inconnu lève `EnregistrementAbsent`. Le corpus est embarqué une fois.
- **`sessions_similaires`** : jusqu'à 3 sessions passées terminées, hors celles qui fondent le constat, avec `score`, `proche_de` et `score_constat`, au-dessus d'un **seuil de 0,95**.
- **`fondements_suggeres`** : les 2 articles du corpus les plus proches du texte proposé, avec `score`, `lien` et la mention « suggestion — à valider par le juriste ».
- **Pour la fusion (contrat §6)** : ces deux champs sont facultatifs. La console les ignore tant qu'elle ne les affiche pas : son `json.Unmarshal` ignore les champs inconnus. `python -m analyste proposer --sans-plongements` s'en passe. Si un embedding manque, la proposition est écrite quand même, sans ces champs, et un avertissement est émis.
- **Trois parcours hors crédit**, pour que le seuil ait un sens : RH prime refusée par D-006, fiche RH consultée, rapport d'un prestataire refusé par D-008. Ils ont été joués devant le vrai gate et les vrais serveurs avec des réponses de modèle **écrites à la main** (`agents/analyste/scenarios-scriptes/`, aucun appel Mistral), comme la répétition du 01/10, puis figés dans les journaux de test : il y en a désormais 9.

Résultats sur les 9 sessions (rejouables hors ligne, copies dans `exemples/propositions/`) :

| Proposition | Sessions similaires (score) | Fondements suggérés (score) |
|---|---|---|
| source interne périmée (D-009) | base-interne-avec-d009 (0,994), consigne-sans-d001 (0,990), consigne-avec-d001 (0,990) | Conso L314-6 (0,818), CMF L313-5-1 (0,817) |
| action sans contrôle humain | base-interne-avec-d009 (0,994), consigne ×2 (0,976) | Conso L314-6 (0,824), RGPD art. 22 (0,811) |
| entreprise non retrouvée | nominal (0,990), top-management (0,985), base-interne-avec-d009 (0,981) | CMF L313-5-1 (0,807), Conso L314-6 (0,803) |

**Budget de la suite : 3 appels réels sur 40, tous `mistral-embed`.** Le premier a confirmé que le modèle est disponible pour la clé. Les deux autres ont servi à calibrer : les trois parcours hors crédit, puis une forme de texte rejetée. Les vecteurs devenus inutiles ont été retirés ; il reste 19 vecteurs enregistrés.

### Décisions prises seul (suite)

1. **On compare la session à la session, pas au constat.** L'embedding du constat est calculé, mais il sépare mal les parcours : de 0,79 à 0,88 contre les 9 sessions, et les sessions qui fondent le constat ne sortent même pas en tête. Le score qui ordonne est donc le plus grand cosinus entre une session et l'une des sessions fondatrices, deux textes du même genre (demande et appels du journal) : 0,97 à 0,99 pour les décisions de crédit, 0,92 à 0,94 pour la remise d'un rapport sur un dossier de crédit, 0,85 à 0,88 pour les parcours RH. Le score contre le constat reste affiché (`score_constat`) à titre indicatif. C'est une lecture de « à partir du texte du constat et des appels en cause » ; à reprendre si Manu tient à la comparaison au constat.
2. **Seuil fixé à 0,95** sur ces scores : il cite les décisions de crédit et écarte les parcours RH et la remise de rapport. Il est fixé dans le code (`similarite.SEUIL_SESSIONS`) et vérifié par un test. Il ne repose que sur 9 sessions et devra être revu sur un historique réel plus large.
3. **Textes plongés, jamais de valeur du coffre.**
   - Une session : la demande (`etat.json`, déjà envoyée au modèle par l'orchestrateur) et la suite des appels (outil, accès, décision, directives, validation, résultat).
   - Un constat : la nature du risque, le fait observable et les appels en cause.
   - Un article : code, numéro et texte.
   Un test le vérifie (une valeur du coffre n'apparaît que si la demande la contenait déjà).
4. **Budget** : à partir de la suite, toute rédaction réelle (`redaction.client`) et le critère comptent dans le budget de la suite (`agents/.etat/analyste-suite`, 40) ; le premier budget (15) s'était arrêté à 13.
5. **Propositions déjà écrites non enrichies** : une proposition n'est jamais réécrite. Les trois déjà dans `demo/execution/propositions/` n'ont donc pas ces champs. Elles les auront si l'on vide ce dossier (de démo, hors Git) avant de relancer `proposer`.
6. **L314-6** relève du Code de la consommation, pas du CMF : il est classé ainsi dans le corpus.

**Étape 3, détecteur « contournement d'un caviardage »** (`detecteurs.contournement_caviardage`).
- **Le fait détecté** : dans une session, une directive appliquée a retiré des champs d'un appel (`champs_retires` du journal). Plus loin, un **autre** outil porte une étiquette **propre** à l'outil caviardé (pas une étiquette de tout son serveur, lue dans `demo/etiquettes.json`). Il est appelé avec données reçues, sans caviardage et sans directive qui le retienne.
- **Testé sur une session construite à la main** (`tests/donnees/sessions-construites/`), comme le brief le demande avant la fusion n° 3 : `lister_salaries` caviardé par D-023 (`/inclure_salaires`), puis `consulter_fiche` (étiquette `salaire`) aux entrées 2, 3 et 4. `consulter_salarie`, qui ne partage que `donnees_personnelles`, n'est pas retenu.
- **Vérifié en lecture seule sur les vrais journaux de la session secteurs** (`mcp_rogue-secteurs/demo/execution/sessions/`, 79 sessions) : le détecteur trouve **exactement** les deux sessions piège D-023 (`20261002-172413-secteur-d023-piege` et son rejeu) et aucune session propre. Ces journaux ne sont pas copiés ici : ils le seront après la fusion n° 3.
- **Proposition rédigée** par Mistral : « *Si un appel à l'outil rh-synthetique/lister_salaries a été fait dans la session et que l'appel suivant porte l'étiquette salaire sans caviardage, alors demander une validation humaine au rôle responsable-rh avant d'autoriser l'accès aux données.* »
  - Fondements suggérés : RGPD articles 22 et 5.
  - Aucune session similaire : la session construite est seule dans son dossier.
- **Couverture** : une directive active qui caviarde ou refuse déjà l'outil du contournement le ferme, et la proposition n'est alors pas faite (test).
- **Pour la fusion** : le brief parle d'un champ « retiré d'une réponse », mais D-023 caviarde un **argument** (la demande des salaires) ; c'est ce que le journal consigne et ce que lit le détecteur.

**Budget de la suite, à jour : 7 appels réels sur 40.**
- 4 `mistral-embed` : disponibilité, deux calibrations, nouveaux textes.
- 3 rédactions : contournement, deux versions.
Le premier jet de la proposition de contournement reposait sur un fait non observable (« a caviardé des champs ») et proposait `analyste-risques`, un rôle sans rapport avec la RH. Plutôt que de modifier le prompt commun, ce qui aurait invalidé les trois rédactions enregistrées dont celle qui donne D-009, le contexte de ce constat cite désormais la directive contournée (texte, cible, effets) et un fait observable. Le premier jet a été retiré des enregistrements.

### Pas fait

- La proposition de contournement n'est pas passée au compilateur. Dans sa forme, la condition « l'appel suivant porte l'étiquette salaire » relève du déclencheur (étiquette de la cible), et « sans caviardage » est redondant ; le juriste la reprendra probablement modifiée.
- Les propositions déjà écrites dans `demo/execution/propositions/` ne sont pas enrichies (voir décision 5).

### Questions pour Manu (suite)

1. Comparer les sessions **entre elles** plutôt qu'au constat (décision 1) : cela vous convient-il ?
2. Faut-il un historique plus large que 9 sessions pour revoir le seuil de 0,95 avant le jour J ?
3. Après la fusion n° 3 : copier les deux sessions piège D-023 de la session secteurs dans les journaux de test.

### Vérifier la suite en 5 minutes

```sh
cd agents
env -u MISTRAL_API_KEY python3 -m unittest discover -s analyste/tests -t .          # 63 tests, sans réseau
T=$(mktemp -d); env -u MISTRAL_API_KEY python3 -m analyste proposer --sessions analyste/tests/donnees/sessions --sortie $T
python3 -c "import json,glob; [print(p['id'], p['sessions_similaires'], [f['id'] for f in p['fondements_suggeres']]) for p in map(lambda f: json.load(open(f)), glob.glob('$T/*.json'))]"
python3 -m analyste constats --sessions analyste/tests/donnees/sessions-construites   # contournement D-023
python3 -m analyste constats --sessions ../../mcp_rogue-secteurs/demo/execution/sessions | grep -c contournement   # lecture seule
```
