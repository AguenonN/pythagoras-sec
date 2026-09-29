# Rapport de nuit — session « Données »

Branche `feat/donnees`, worktree `~/cybercar/mcp_rogue-donnees`. Rien n'a été poussé ni fusionné.

## 1. Fait

| Commit | Contenu |
|---|---|
| `7f7e928` | `servers/internal/rejeu` : enregistrement et rejeu (DIRECT / REJEU / MIXTE), tests sans réseau |
| `9e3bdc9` | Outillage : `servers/internal/lanceur` (main commun), `servers/internal/essai` (aides de test), `servers/exporter-schema` (écrit `result.tools` brut de `tools/list`, lu sur le stdio du binaire), `servers/appeler` (essai manuel d'un outil) |
| `c500c7a` | Serveur `entreprises` : `rechercher_entreprise(siren)`, 3 entreprises de démo enregistrées |
| `d8070b2` | Serveur `gels-avoirs` : `verifier_gel(nom)`, registre du 2026-09-29 enregistré (filtré) |
| `31ec0bc` | Serveur `taux-officiel` : `get_taux_usure(categorie, date_reference?)`, 10 séries enregistrées, `instantane_trimestre_precedent.json` |
| `6124862` | `rejeu` : chaque client limité à l'hôte de son API, redirections comprises (http→https) |
| `9726e1f` | Serveur `eurlex` (bonus) : `rechercher_acte(celex)`, `lire_article(celex, article)`, 4 actes enregistrés |

**Critère de réussite atteint** : `entreprises`, `gels-avoirs` et `taux-officiel` répondent en DIRECT et en REJEU, enregistrements commités.

### Serveurs

| Serveur | DIRECT | REJEU | SHA-256 du binaire (`go build -trimpath`, go1.25.0 linux/amd64) |
|---|---|---|---|
| `entreprises` | OK (3 SIREN, 2026-09-29 ~22:00 UTC) | OK | `15c53e7d1c68e3066ec2e8835876bc7f98df95c6cf8b5d58addbbf29b824d2c6` |
| `taux-officiel` | OK (10 catégories ; en vigueur 2026-Q3, suivant 2026-Q4 déjà publié le 2026-09-28) | OK | `427aa716b7ef574a88a430c243e42e26c29da296bb2fd4dea4127b9d8f0d31cd` |
| `eurlex` | OK (4 actes + 1 article chacun) | OK | `0644398e61ab045e3a9d19522fb6740b082e0d60b6f04df323bb62c71be8102b` |
| `gels-avoirs` | OK (« Rosoboronexport » → 1 correspondance, registre publié le 2026-09-29T15:47:57+02:00 ; 12 Mo, ~2 s) | OK | `b18d7b4dc4409ae306ecf7c79f8838f1377be9be8446cf4974d02c7701bcfe11` |

**Entreprises de démo** (actives, personnes morales, diffusion publique) :

| SIREN | Dénomination | Forme | Catégorie |
|---|---|---|---|
| 405750753 | TRICOTS SAINT-JAMES | SAS (5710) | ETI |
| 382891752 | BIOCOOP | SA coopérative de commerçants-détaillants (5560) | ETI |
| 508858230 | OPINEL DISTRIBUTION | SAS (5710) | PME |

**Cas positif de filtrage** : `JSC ROSOBORONEXPORT`, personne morale, `id_registre` 4493, fondements (UE) 2022/427 et 2023/571 (régime UE Ukraine, R (UE) 269/2014). `verifier_gel {"nom":"JSC ROSOBORONEXPORT"}` → correspondance `exacte`.

**Instantané périmé** (`servers/taux-officiel/instantane_trimestre_precedent.json`) : date de référence 2026-09-30, trimestre en vigueur 2026-Q3, instantané = **2026-Q2** (publié le 2026-03-27), 10 catégories. Ex. découverts aux sociétés non financières : 19,05 % (Q2) contre 19,00 % en vigueur (Q3) et 19,08 % publié pour Q4 ; crédit conso > 6 000 € : 8,61 % contre 8,56 %.

**Actes UE vérifiés sur CELLAR** (tous `en_vigueur`) : DORA `32022R2554` (14/12/2022) ; RGPD `32016R0679` (27/04/2016) ; AI Act `32024R1689` (13/06/2024), **`modifie_par: ["32026R1744"]`** ; `32026R1744` = règlement (UE) 2026/1744 du 8 juillet 2026 modifiant 2024/1689, 2018/1139 et 2023/1230 (« train de mesures omnibus numérique sur l'IA »). En rejeu, **tout article** de ces 4 actes est lisible (le texte entier est enregistré).

## 2. Pas fait (et pourquoi)

En cours : serveur `legifrance`.

## 3. Décisions prises seul

- **Mode par défaut = `REJEU`** quand `DONNEES_MODE` est absente (fermé par défaut : aucun réseau sans demande explicite). Une valeur inconnue est une erreur, pas un repli.
- **Nom de la variable** : `DONNEES_MODE` (le brief ne la nommait pas).
- **Statuts HTTP** : 404 → `introuvable` (pas de repli sur l'enregistrement en MIXTE : l'API a répondu). 400/422 → `requete_refusee`. 401/403 → `indisponible` sans nouvelle tentative. 429/5xx/réseau → 3 tentatives (attente 1 s puis 2 s), puis `indisponible`, et repli sur l'enregistrement en MIXTE.
- **Seules les réponses 2xx sont enregistrées.** Un enregistrement dont la clé ou l'empreinte ne correspond pas est refusé (`indisponible`), jamais servi.
- **Un enregistrement par requête** (clé = SHA-256 de méthode + URL nettoyée + corps). Un nouvel appel DIRECT l'écrase ; l'historique reste dans git.
- **Mémoïsation en mémoire 10 min** en DIRECT/MIXTE (règle « mise en cache de toute réponse déjà obtenue ») ; la réponse resservie garde `origine: direct` et **sa date d'obtention d'origine**.
- **Filtre avant enregistrement** : un serveur peut réduire un corps avant de le versionner (retrait des personnes physiques). L'enregistrement porte alors `filtre` et `sha256_origine` (empreinte du corps reçu), et la `source` d'une réponse rejouée affiche le filtre : un rejeu filtré est partiel et le dit.
- **`entreprises` ne restitue ni nom ni adresse d'un entrepreneur individuel** (nature juridique 1xxx) ni d'une unité en diffusion partielle, et ne les enregistre jamais. Requête `minimal=true&include=siege,complements` : pas de dirigeants dans la réponse, donc aucun nom de personne physique dans les enregistrements.
- **SIREN contrôlé localement** (9 chiffres, clé de Luhn) avant tout appel : `requete_refusee` sans réseau.
- **Répertoire d'enregistrements** : option `-enregistrements`, par défaut `servers/<nom>/enregistrements` relatif au répertoire courant (lancer les binaires depuis la racine du dépôt).
- **`gels-avoirs` : l'enregistrement est filtré.** Le registre complet contient 4 648 personnes physiques : il ne peut pas être versionné tel quel. L'enregistrement ne garde que les personnes morales (1 924) et navires (39), avec nom, alias (sans commentaire), fondements juridiques, références UE/ONU et numéro OMI. Motifs, adresses, téléphones, courriels, sites et identifiants sont retirés (ils peuvent nommer des personnes). `sha256_origine` garde l'empreinte du registre complet reçu. En rejeu, la réponse porte `source.filtre` et un `avertissement` : **une absence de correspondance en rejeu n'est pas probante**.
- **`gels-avoirs` en DIRECT renvoie aussi les correspondances de personnes physiques** (nom tel qu'au registre, nature, fondements), car c'est la fonction du filtrage. Rien de cela n'est enregistré ni commité. Voir question 2.
- **Correspondance** : noms normalisés (minuscules, sans accents, ponctuation → espace). `exacte` = mêmes mots dans le même ordre ; `partielle` = tous les mots cherchés présents dans le nom ou un alias. Pas de correspondance approchée (fautes de frappe) : plus simple à expliquer au jury, et un faux négatif possible, à dire. Au plus 20 correspondances rendues, total toujours donné.
- **Webstat : l'API exige une clé.** Le catalogue Opendatasoft répond sans clé mais le jeu `observations` renvoie 404 sans authentification. Le site embarque une clé dans son JavaScript public : **je ne l'ai pas utilisée** (ce n'est pas une clé qui nous a été donnée). J'utilise l'**export CSV public** du site : `https://webstat.banque-france.fr/export/csv/fr/catalog/mir1/<série>`, sans clé, une série par requête (l'export multi-séries renvoie 404).
- **Séries retenues** : les 10 séries trimestrielles (`MIR1.Q…`) « seuil de l'usure » non arrêtées (immobilier fixe < 10 / 10-20 / ≥ 20 ans, variable, relais, conso ≤ 3 000 / 3 000-6 000 / > 6 000 €, découverts EI, découverts sociétés non financières). Codes de catégorie stables en `enum` dans le schéma d'entrée. Pas les séries APU/ISBLSM (hors sujet démo), ajout trivial.
- **« En vigueur »** = le trimestre dont la période contient la date de référence (Europe/Paris, aujourd'hui par défaut, ou `date_reference`). Le seuil Q4 2026 est déjà publié (2026-09-28) : il est rendu à part dans `suivant`, jamais comme seuil en vigueur avant le 1er octobre.
- **`date_publication`** = `updated_at` de l'observation sur Webstat (mise à jour par la Banque de France, 2-3 jours avant le début du trimestre). Ce n'est pas la date de l'avis au JORF, que Webstat ne donne pas.
- **Instantané calculé au 2026-09-30** (trimestre précédent = Q2). Si la démo a lieu à partir du 1er octobre, Q4 sera en vigueur et le « trimestre précédent » sera Q3 : régénérer avec `go run ./servers/taux-officiel instantane -date <jour de la démo>` (sans réseau, depuis les enregistrements), puis mettre à jour la date dans `TestInstantaneVersionne`. Voir question 1.
- **`eurlex` : texte via CELLAR, pas via eur-lex.europa.eu.** `eur-lex.europa.eu/legal-content/FR/TXT/HTML/?uri=CELEX:…` répond `202` à corps vide (protection anti-robots) ; je ne l'ai pas contournée. `https://publications.europa.eu/resource/celex/<CELEX>` avec `Accept: application/xhtml+xml` et `Accept-Language: fra` redirige vers la manifestation XHTML (domaine autorisé). La redirection pointe en `http://` : le client la passe en `https://`.
- **`lire_article` rend la version publiée au JO, non consolidée**, et le dit (`version`). Pour l'AI Act, les modifications de l'omnibus 2026/1744 n'y figurent donc pas ; `rechercher_acte` les signale (`modifie_par`). Pas de version consolidée : CELLAR la publie sous un autre CELEX (`0…`), non traité cette nuit.
- **Signatures retirées des enregistrements `eurlex`** : le bloc final (`<div class="oj-final">`) nomme les présidents du Parlement et du Conseil. Il est ôté avant enregistrement (`source.filtre` le dit) ; si le bloc est absent ou multiple, le texte n'est pas enregistré, par prudence. Les articles sont intacts.
- **Extraction d'article** : `encoding/xml` (bibliothèque standard, le XHTML est du XML) plutôt que `golang.org/x/net/html`, pour ne pas toucher `go.mod` (commité avant la ligne de base). Tableaux imbriqués (points « 1) », « a) ») rendus une ligne par point, dans l'ordre.
- **Liste blanche d'hôtes** dans chaque client (`Restreindre`) : une requête ou une redirection hors du domaine de l'API est refusée (règle réseau des consignes).
- **Résultat `indisponible`** : résultat d'outil structuré avec `statut: indisponible` et `isError: true`, jamais une erreur de protocole.

## 4. Blocages

Aucun pour l'instant.

## 5. Questions pour Manu (par urgence)

1. **Date de la démo** : l'instantané « trimestre précédent » est calculé au 2026-09-30 (Q2). À partir du 1er octobre, Q4 est en vigueur : faut-il l'instantané Q3 (vrai trimestre précédent) ? Une commande, voir section 3.
2. **`gels-avoirs` en DIRECT restitue le nom d'une personne physique sanctionnée quand on la cherche.** C'est donnée publique et c'est le but du filtrage, mais la règle dit « aucune donnée de personne physique réelle, nulle part ». Si l'engine journalise les résultats d'outils, ce nom finira dans le journal. Faut-il masquer les noms des personnes physiques dans le résultat (ne garder que id, nature, fondements) ? Changement court dans `servers/gels-avoirs/serveur.go`.
3. **Entreprises de démo** : je n'ai pas pu vérifier l'absence d'affaire judiciaire (aucun domaine autorisé ne le permet). Choisies parce que ce sont des sociétés établies, sans affaire notoire à ma connaissance. À valider avant la démo.
4. **Noms d'entités du registre des gels** : un nom de personne morale peut contenir un nom de personne (entreprise éponyme). Non filtré (impossible sans liste de noms). Risque résiduel à connaître.

## 6. Vérifier en 5 minutes

```sh
cd ~/cybercar/mcp_rogue-donnees
go vet ./... && go test -race ./servers/...
go build -o bin/appeler ./servers/appeler && go build -o bin/entreprises ./servers/entreprises
DONNEES_MODE=REJEU  bin/appeler bin/entreprises rechercher_entreprise '{"siren":"405750753"}'   # source.origine = rejeu
DONNEES_MODE=DIRECT bin/appeler bin/entreprises rechercher_entreprise '{"siren":"405750753"}'   # source.origine = direct (réseau)
go build -o bin/gels-avoirs ./servers/gels-avoirs
DONNEES_MODE=REJEU  bin/appeler bin/gels-avoirs verifier_gel '{"nom":"JSC ROSOBORONEXPORT"}'   # 1 correspondance exacte, avertissement « partiel »
go build -o bin/taux-officiel ./servers/taux-officiel
DONNEES_MODE=REJEU  bin/appeler bin/taux-officiel get_taux_usure '{"categorie":"decouvert-societes-non-financieres"}'   # Q3 19 %, suivant Q4 19,08 %
cat servers/taux-officiel/instantane_trimestre_precedent.json | head -20
go build -o bin/eurlex ./servers/eurlex
DONNEES_MODE=REJEU  bin/appeler bin/eurlex rechercher_acte '{"celex":"32024R1689"}'                 # modifie_par 32026R1744
DONNEES_MODE=REJEU  bin/appeler bin/eurlex lire_article '{"celex":"32022R2554","article":"28"}'
```

Schémas : `servers/<nom>/schema.json` = `result.tools` brut de `tools/list`, écrit par `go run ./servers/exporter-schema -o servers/<nom>/schema.json bin/<nom>` ; un test par serveur (`TestSchema`) échoue si le serveur diverge du fichier.
