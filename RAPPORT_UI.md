# RAPPORT_UI — session « ui »

Branche `feat/ui`. Console locale `cmd/console` + `internal/console` : un binaire, tout embarqué (HTML, CSS, JS), écoute uniquement sur la boucle locale, **lecture seule**, aucune ressource externe (test automatisé `TestAucuneRessourceExterne`, en-tête CSP `default-src 'self'`).

## 1. Fait
| Commit | Contenu |
|---|---|
| `6ea6704` | Option de conservation des journaux de test : `MCP_CONSERVER_JOURNAUX=<dir> go test ./test/integration/` copie `journal.jsonl` et `coffre.jsonl` de chaque scénario dans `<dir>/<scénario>/` (seule modification hors `cmd/console` et `internal/console`). |
| `b8c34c5` | Console : les six écrans (1 à 4 complets ; 5 et 6 en lecture seule), vérification de chaîne à chaque affichage, tests. |

**Critère de réussite** : écrans 1 à 4 fonctionnels sur les journaux réels produits par le gate (4 scénarios d'intégration : nominal, seuil, attaque, base périmée), chaîne vérifiée et affichée, rien chargé depuis le réseau. Vérifié en interrogeant l'API de la console sur ces journaux ; `go vet ./...` et `go test -race ./...` verts.

## 2. Pas fait / limites
- **Aucune capture d'écran réelle** : pas de navigateur dans l'environnement. Le JS a passé `node --check`, les API et les vues sont testées en Go, mais **le rendu visuel n'a jamais été vu**. À regarder en premier demain matin (section 6).
- Écran 6 : le compilateur de texte libre n'existe pas dans le dépôt. La zone de texte relit un **jeu JSON** (chargeur strict + relecture) ; un texte libre reçoit un message qui le dit, pas une directive inventée. Bouton d'activation désactivé.
- Écran 5 : la reprise d'un appel retenu n'existe pas ; boutons désactivés avec note. « Rôles requis » : lus dans `decision.validation` du journal, affichés « non précisés » quand la directive n'en donne pas.
- Annuaire : aucun paquet annuaire dans le dépôt ; le demandeur est lu dans `session_ouverte` / `appel` (`demandeur`) s'il est journalisé, sinon « non identifié ».
- « Réponse finale » : le journal ne garde que l'empreinte de la réponse d'un serveur. La console dit ce que l'agent a reçu, reconstitué d'après la décision (motif pour un refus/attente ; « réponse remise à l'agent » pour un succès). Le texte exact n'est pas disponible.
- Le suivi en continu est un sondage toutes les 2 s (relecture complète du fichier + vérification de chaîne), pas une lecture incrémentale de fin de fichier : plus simple, et la vérification complète à chaque affichage est ce que demande le brief. Suffisant pour des journaux de démo ; à revoir au-delà de quelques dizaines de milliers de lignes.

## 3. Décisions prises seul
- **Clé des journaux de test** : `cleTest` (chaîne sans valeur, déjà dans le test d'intégration). La console lit sa clé dans `MCP_GATE_CLE_JOURNAL` (hex), comme le gate, jamais dans un fichier. Sans clé : bannière rouge « chaîne non vérifiable », rien n'est tenu pour prouvé (choix le plus prudent).
- **Bannière de chaîne** : rouge dès la première anomalie ; les appels à partir de l'entrée fautive sont marqués « entrée non fiable » en rouge. Une dernière ligne sans saut de ligne (en cours d'écriture par le gate) est ignorée, pas signalée.
- **Coût de la règle** : le temps ajouté par la règle n'est pas journalisé. Je le mesure en rejouant chaque décision (`rejeu.Rejouer`, `Decide` est pure) : ordre de grandeur (quelques dizaines de µs), affiché comme tel. Le temps d'attente humaine n'est pas mesuré (dit à l'écran). Validations, refus, caviardages sont comptés depuis le journal.
- **Vitrine** : relecture via le paquet `internal/relecture` (la fonction même qu'appelle `cmd/directive relire`), pas via un sous-processus. La décision rejouée montrée est d'abord une décision où la directive s'est déclenchée, à défaut une où elle a été examinée.
- **Avant / après** : utilise `rejeu.Comparer` (transitions et lignes) ; un second rejeu sert uniquement à afficher le motif « après ». Un jeu variante `credit-demo` v2 « plus stricte » (D-001 refuse au lieu d'attendre ; D-002 à 50 000 €) est **intégré à la console** (`internal/console/exemples/`) pour qu'il y ait toujours deux jeux ; `credit-demo-v1.json` y est une copie de `demo/directives/credit-demo.json` (dédoublonnée par empreinte quand le vrai fichier est fourni par `-jeux`).
- **Arguments affichés** : uniquement à l'écran 5 (la requête exacte à valider est justement le besoin), jamais ailleurs. Ils viennent du coffre (arguments transmis s'ils existent). Les serveurs `127.0.0.1` seulement, contrôle de l'en-tête `Host` (anti-rebinding).
- Journaux d'exemple conservés **hors du dépôt** (répertoire de travail), pas dans `demo/`.
- Une dépendance de rejeu : un appel dont le jeu n'est pas fourni n'arrête pas l'écran (il est exclu ou signalé), contrairement à `RejouerJournal`.

## 4. Blocages
Aucun bloquant. `scripts/lancer-demo.sh` n'a pas été utilisé (il demande le réseau et des clés) ; les journaux viennent de `test/integration`, qui tourne sans réseau.

## 5. Questions pour Manu
1. (Urgent, 2 min) Le rendu : lancer la console (section 6) et dire ce qui choque visuellement — je n'ai pas pu le voir.
2. Faut-il que le gate journalise la durée de `Decide` (et l'attente de validation) pour que « temps ajouté » soit une mesure et non un rejeu ?
3. Faut-il journaliser le texte de la réponse rendue à l'agent (refus/attente) pour l'afficher tel quel au conseiller ?
4. Annuaire et compilateur : où seront-ils, pour brancher les écrans 5 et 6 ?

## 6. Vérification en 5 minutes
```bash
cd ~/cybercar/mcp_rogue-ui            # worktree de la session ui
go vet ./... && go test -race ./internal/console/ ./cmd/... ./test/integration/

# Journaux réels produits par le gate (4 scénarios) :
MCP_CONSERVER_JOURNAUX=/tmp/journaux-demo go test -count=1 ./test/integration/

# Console (la clé ci-dessous est celle, sans valeur, du test d'intégration) :
export MCP_GATE_CLE_JOURNAL=$(printf 'integ-integ-integ-integ-integ-integ-' | xxd -p | tr -d '\n')
go run ./cmd/console -donnees /tmp/journaux-demo -jeux demo/directives
# puis ouvrir http://127.0.0.1:8088/

# Voir le rouge : falsifier un journal copié et le relancer
cp -r /tmp/journaux-demo/testattaque /tmp/journaux-demo/falsifie
sed -i '0,/"motif":"refusé/s//"motif":"autorisé/' /tmp/journaux-demo/falsifie/journal.jsonl
```
Essayer aussi sans `MCP_GATE_CLE_JOURNAL` (bannière rouge « non vérifiable »), et couper le réseau : rien ne doit changer.

## 7. Structure des pages (captures texte, d'après les données réelles du scénario `testnominal`)

Toutes les pages : bandeau bleu « Console de supervision des agents » + onglets *Journal et chaîne · Poste conseiller · Règles · Avant / après · File du superviseur · Rédaction* ; sous le bandeau, la **bannière de chaîne** :
`✔ Chaîne du journal vérifiée : 19 entrées, aucune altération.` (vert) ou, pour un journal falsifié :
`✖ CHAÎNE ROMPUE — chaine: ligne 12 altérée : … Les entrées à partir de ce point ne sont pas fiables (11 entrée(s) vérifiée(s) sur 15).` (rouge, appels suivants marqués « entrée non fiable »).

**1. Journal et chaîne** — sélecteurs Journal / Session, puis :
```
[Qui demande, qui agit]  Demandeur: non identifié → Agent (identité prouvée): local-process → Sous-agents déclarés: aucun
  Règles appliquées : jeu « credit-demo », version 1.   Serveurs rencontrés : recherche-entreprises (attesté) · gels-avoirs (attesté) · taux-officiel (attesté) · banque-synthetique (attesté)
[Appel n° 1  Caviardé]  Sous-agent: — → Appel: rechercher_entreprise (champs retirés : /dirigeant, /note, /raison_sociale) → Serveur: recherche-entreprises (Attesté — programme vérifié) → Décision: Caviardé, appel transmis
    autorisé après caviardage de 3 champ(s) par D-004 v1 …      • Directive D-004, version 1 — appliquée (caviardé) ; Fondement : RGPD…, art. 5 ; texte du juriste
[Appel n° 3  Autorisé]  … get_taux_usure → taux-officiel (Attesté)   « Cette consultation a fondé la décision de : appel n° 4 »
[Appel n° 4  En attente de validation]  … attribuer_pret → banque-synthetique → Décision: En attente de validation, appel non transmis
    • Directive D-001, version 1 — appliquée (en attente de validation) ; Fondement : RGPD…, art. 22 ; Motif : la session a consulté taux-officiel (ok)
    Décision fondée sur : [consultation n° 3 (taux-officiel / get_taux_usure)] — directive D-001   (lien cliquable → saute à la carte n° 3)
```
Scénario `testattaque` : l'appel à `taux-officiel` porte le badge rouge plein **Source non attestée**, le nœud Serveur est encadré de rouge « NON attesté ».

**2. Poste conseiller** — frise horizontale, un jalon par appel (outil, serveur, badge), suivi en continu (point clignotant, sondage 2 s) ; clic sur un jalon = détail. Puis « Réponse finale » : *Dernier appel (banque-synthetique / attribuer_pret) : retenu, en attente de validation humaine. L'agent n'a reçu aucune donnée. …* ; puis « Coût de la règle » : `1 validation(s) · 0 refusé(s) · 1 avec champs retirés · ≈ 66 µs de temps moyen ajouté · 13 ms chez les serveurs`.

**3. Règles** — sélecteur de jeu ; par directive : texte du juriste (encadré), fondement, relecture (texte du gabarit), puis « Une décision réelle, rejouée » : *Appel n° 4 … Décision du journal : En attente de validation — décision rejouée : En attente de validation — identique*.

**4. Avant / après** — sélecteurs Journal / Avant / Après ; transitions `Autorisé→Autorisé ×2`, `Caviardé→Caviardé ×1`, **`En attente de validation → Refusé ×1`** (surlignée en jaune) ; tableau des appels, ligne de l'appel n° 4 surlignée avec le motif « après ».

**5. File du superviseur** — cartes des appels en attente (directive, fondement, rôles, requête exacte en JSON) ; boutons « Valider / Refuser » désactivés, note en tête.

**6. Rédaction** — zone de texte, bouton « Relire » (actif), relecture en français d'un jeu JSON valide ou liste d'erreurs du chargeur ; « Activer cette directive » désactivé, note en tête.
