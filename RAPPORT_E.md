# Rapport E : Analyste (`agents/analyste/`)

Branche `feat/analyste`, partie de `5b83cd5`. Rien de poussé ni fusionné.

## 1. Ce qui est fait

| Commit | Contenu |
|---|---|
| `b5bd9b6` | Détecteurs déterministes, couverture par les directives, regroupement en constats, journaux de test |
| `b08908f` | Rédaction par Mistral, écriture des propositions (contrats §6), mesure par rejeu, critère D-009, enregistrements |
| `221badd` | La mesure rejoue toutes les sessions passées, pas seulement celles du constat |
| (ce rapport) | `RAPPORT_E.md`, `RAPPORT_NUIT.md` |

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
4. **`constat.detail` est une chaîne**, parce que la console lit `Detail string` (`internal/console/propositions.go`) : un objet rendrait la proposition illisible et la console l'ignorerait en silence. Les occurrences structurées vont dans un champ en plus, `constat.occurrences`, que la console ignore. **Écart au contrat §6, signalé.**
5. **Mesures hors du dossier des propositions** (`demo/execution/analyste/mesures/<id>.json`). La console lit tout `*.json` de `propositions/` (hors `.decision.json`) comme une proposition : un fichier `<id>.mesure.json` y serait apparu comme une proposition fantôme. **Écart signalé.**
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

1. **Contrat §6** : valider `detail` en chaîne plus `occurrences` en champ supplémentaire, et le dossier `demo/execution/analyste/mesures/` (3.4 et 3.5). Sinon, la console doit accepter un objet, ou ignorer d'autres suffixes.
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
