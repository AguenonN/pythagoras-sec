# Rapport de nuit — Session H « opération » (fusion-acquisition ATLAS)

Branche : `feat/operation` (worktree). Engine **non modifié**. Tout est **fictif**.

## Compteur Mistral

- Budget brief : 150 ; plafond des règles communes : **40 appels réels** (les règles
  communes priment, §« Coûts »). **Décision prise seul** : je retiens le plafond le plus
  prudent, **40**, et je compte chaque appel réel ci-dessous.
- Appels réels effectués : **0** à ce stade.

## 1. Ce qui est fait

- **Quatre serveurs gouvernables créés par le kit** (`scripts/nouveau-serveur.sh`, hors ligne) :
  - `data-room` : `lister_documents`, `lire_document` (étiqueté `information-privilegiee`,
    `projet-atlas`), `partager_document` (destination `interne`/`externe`, étiqueté
    `transfert_donnees`). 11 documents fictifs (10 ATLAS + 2 publics).
  - `messagerie` : `rediger_message` (lecture de gabarits), `envoyer_message`
    (destination `interne`/`externe`, destinataire, objet, corps, pièces).
  - `bourse` : `consulter_cours`, `passer_ordre` (titre, sens, quantité, pour le compte de) ;
    titres fictifs VENTOUX, BIOLYS, ALPHA, OMEGA.
  - `communication` : `preparer_communique`, `publier_communique` (canal).
  - `go vet ./...` et `go test ./test/extension/` + tests des 4 serveurs **verts**.
- **Identités ATLAS** ajoutées à `demo/annuaire-source.json` (toutes `@exemple.test`, fictives) :
  `nora.ventoux` (directrice juridique, rôle `direction-juridique`), `theo.ventoux` (conformité,
  rôle `conformite`), `victor`/`wassila.ventoux` (groupe `inities-atlas`), `yanis.ventoux`
  (non initié), `adrien`/`chloe.arceau` (groupe `equipe-atlas-cabinet`), `damien.arceau`
  (cabinet mais hors équipe ATLAS). Groupes : `inities-atlas`, `equipe-atlas-cabinet`,
  `conformite`, `direction-juridique`.
- **Deux sous-agents neutres** (`agents/orchestrateur/sous_agents/`) : `assistant-salarie`
  (data-room, messagerie, bourse) et `equipe-deal` (data-room, messagerie, communication).
  Aucune règle dans les consignes. `python3 -m orchestrateur sous-agents` les liste.
  Tests Python **verts** (`test_orchestrateur`, `test_jeux`, `test_secteurs`).

## 2. Ce qui n'est pas fait, et pourquoi

## 3. Décisions prises seul (avec justification)

- **Plafond Mistral 40** (voir ci-dessus) : les règles communes priment sur le brief.
- **`faux_gate.py` et un test du kit étendus** (`agents/orchestrateur/tests/`) : le double de
  gate des tests et le test `test_les_sous_agents_de_la_demo_se_chargent_devant_le_catalogue`
  listent en dur les outils/serveurs connus et l'ensemble exact des sous-agents de la démo.
  Ajouter des sous-agents (ce que le brief demande) casse ces tests tant que les 4 nouveaux
  serveurs n'y figurent pas. Le catalogue figé (`agents/compilateur/exemples`, **interdit**)
  ne peut pas les inclure. **Contournement** (même motif que le bloc SECTEURS déjà présent) :
  j'ai réuni les noms d'outils de l'opération dans `faux_gate.py` (bloc `OPERATION`) et dans
  ce test, **sans toucher au catalogue figé**. Preuve pour dimanche : le kit n'offre pas de
  mécanisme pour qu'un nouveau serveur soit reconnu par les tests figés sans édition manuelle.

## 4. Blocages rencontrés

## 5. Questions pour Manu (par urgence)

## 6. Vérifier en 5 minutes

```sh
# depuis la racine du worktree
go vet ./...
go test ./test/extension/        # le kit : serveurs, sous-agents, directive, scénarios (hors ligne)
go run ./cmd/directive verifier < demo/scenarios/jeux/operation.json
```
