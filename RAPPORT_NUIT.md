# Rapport de nuit — session « Données »

Branche `feat/donnees`, worktree `~/cybercar/mcp_rogue-donnees`. Rien n'a été poussé ni fusionné.

## 1. Fait

| Commit | Contenu |
|---|---|
| `7f7e928` | `servers/internal/rejeu` : enregistrement et rejeu (DIRECT / REJEU / MIXTE), tests sans réseau |
| `9e3bdc9` | Outillage : `servers/internal/lanceur` (main commun), `servers/internal/essai` (aides de test), `servers/exporter-schema` (écrit `result.tools` brut de `tools/list`, lu sur le stdio du binaire), `servers/appeler` (essai manuel d'un outil) |
| `c500c7a` | Serveur `entreprises` (renommé depuis `recherche-entreprises`, voir plus bas) : `rechercher_entreprise(siren)`, 3 entreprises de démo enregistrées |
| `d8070b2` | Serveur `gels-avoirs` : `verifier_gel(nom)`, registre du 2026-09-29 enregistré (filtré) |
| `31ec0bc` | Serveur `taux-officiel` : `get_taux_usure(categorie, date_reference?)`, 10 séries enregistrées, `instantane_trimestre_precedent.json` |
| `6124862` | `rejeu` : chaque client limité à l'hôte de son API, redirections comprises (http→https) |
| `9726e1f` | Serveur `eurlex` (bonus) : `rechercher_acte(celex)`, `lire_article(celex, article)`, 4 actes enregistrés |
| `d65a5aa` | Serveur `legifrance` (bonus) : `lire_article(identifiant, date_reference?)`, 3 versions d'articles enregistrées ; `rejeu.Relire` pour le repli MIXTE quand le jeton OAuth échoue |
| (dernier) | Ce rapport, version finale |

**Critère de réussite atteint, bonus compris** : les **cinq** serveurs répondent en DIRECT et en REJEU, enregistrements commités, `schema.json` exporté pour chacun. `go vet ./...` et `go test -race ./...` verts sur tout le dépôt à chaque commit.

### Serveurs

Essais manuels DIRECT du 2026-09-29 entre 22:00 et 22:15 UTC. Empreintes des binaires à `d65a5aa` (`go build -trimpath`, go1.25.0 linux/amd64 ; elles changent avec tout commit touchant `servers/internal`) :

| Serveur | Outils | DIRECT (essai manuel) | REJEU | SHA-256 du binaire |
|---|---|---|---|---|
| `recherche-entreprises` | `rechercher_entreprise` | OK, 3 SIREN | OK | `a5308db05f4d113773bd65349898b8c39c182bfd10bacc445f1b835d4dc27374` |
| `gels-avoirs` | `verifier_gel` | OK, « Rosoboronexport » → 1 correspondance ; registre publié le 2026-09-29T15:47:57+02:00 (12 Mo, ~2 s) | OK | `65068dd2712b39aff7b0c2b30ffb31096de577af666da6ca199714359e2139b8` |
| `taux-officiel` | `get_taux_usure` | OK, 10 catégories ; en vigueur 2026-Q3, suivant 2026-Q4 déjà publié le 2026-09-28 | OK | `bac995e8e1371ac11903faf2407cb7dcff5151379cceb85e06ea2cb32ec3bdb3` |
| `eurlex` | `rechercher_acte`, `lire_article` | OK, 4 actes + 1 article chacun | OK | `a759b6f1224810e00f78d25ee36dde453b58a7eb69bacb61cf4a28b31c81ae75` |
| `legifrance` | `lire_article` | OK, Code conso L314-6 (v2 et v3), CMF L313-5-1 | OK | `d4442cce94c8aba0080689dae8bf4e508dea24ab0f1300aeb9873c70291d3fe5` |

**Panne simulée** (proxy injoignable, `HTTPS_PROXY=http://127.0.0.1:9`, aucun réseau réel) : en MIXTE, `taux-officiel` sert l'enregistrement en ~3 s (3 tentatives), `source.origine = rejeu`, date de l'enregistrement et `motif` (l'erreur réseau) visibles ; en DIRECT, `recherche-entreprises` rend `statut: indisponible`, `isError: true`, sans plantage.

Enregistrements : `recherche-entreprises` 16 Ko, `gels-avoirs` 1,2 Mo, `taux-officiel` 336 Ko, `eurlex` 3,9 Mo, `legifrance` 60 Ko.

**Entreprises de démo** (actives, personnes morales, diffusion publique) :

| SIREN | Dénomination | Forme | Catégorie |
|---|---|---|---|
| 405750753 | TRICOTS SAINT-JAMES | SAS (5710) | ETI |
| 382891752 | BIOCOOP | SA coopérative de commerçants-détaillants (5560) | ETI |
| 508858230 | OPINEL DISTRIBUTION | SAS (5710) | PME |

**Cas positif de filtrage** : `JSC ROSOBORONEXPORT`, personne morale, `id_registre` 4493, fondements (UE) 2022/427 et 2023/571 (régime UE Ukraine, R (UE) 269/2014). `verifier_gel {"nom":"JSC ROSOBORONEXPORT"}` → correspondance `exacte`.

**Instantané périmé** (`servers/taux-officiel/instantane_trimestre_precedent.json`) : **date de référence 2026-10-04 (jour de la démo)**, trimestre en vigueur 2026-Q4 (depuis le 1er octobre), instantané = **2026-Q3** (publié le 2026-06-29), 10 catégories. Ex. découverts aux sociétés non financières : 19,00 % (Q3, périmé) contre 19,08 % en vigueur (Q4) ; immobilier taux variable : 5,28 % contre 5,52 %. `TestJourDeDemoSertLeQuatriemeTrimestre` vérifie qu'en REJEU, `get_taux_usure` avec `date_reference = 2026-10-04` sert le 4e trimestre pour les 10 catégories.

**Actes UE vérifiés sur CELLAR** (tous `en_vigueur`) : DORA `32022R2554` (14/12/2022) ; RGPD `32016R0679` (27/04/2016) ; AI Act `32024R1689` (13/06/2024), **`modifie_par: ["32026R1744"]`** ; `32026R1744` = règlement (UE) 2026/1744 du 8 juillet 2026 modifiant 2024/1689, 2018/1139 et 2023/1230 (« train de mesures omnibus numérique sur l'IA »). En rejeu, **tout article** de ces 4 actes est lisible (le texte entier est enregistré).

**Articles Légifrance enregistrés** : Code de la consommation **L314-6** (définition du prêt usuraire), version 2.0 `LEGIARTI000032303335` (`ABROGE_DIFF`, en vigueur jusqu'au 2026-11-20) et version 3.0 `LEGIARTI000052190104` (`VIGUEUR_DIFF`, à partir du 2026-11-20) ; Code monétaire et financier **L313-5-1** (usure des découverts aux personnes morales et professionnels), `LEGIARTI000027643291`, `VIGUEUR`. Le changement de version de L314-6 au 20 novembre est un bon cas de démo « texte en vigueur à la date ».

## 2. Pas fait (et pourquoi)

Tout le brief est fait. Limites connues, non traitées cette nuit :

- **`eurlex` : pas de texte consolidé** (CELEX `0…`), seulement la version publiée au JO ; les modifications sont signalées par `modifie_par`.
- **`taux-officiel` : pas de séries APU/ISBLSM**, hors sujet pour la démo ; ajout d'une ligne dans `Categories`.
- **`legifrance` : pas d'outil de recherche** (le brief ne demandait que `lire_article(identifiant)`). J'ai trouvé les identifiants avec `POST /search` à la main (2 appels).
- **Pas de transport HTTP** pour les serveurs : stdio seulement, comme les autres binaires `cmd/`.
- **Mistral : aucun appel** (le brief de cette session n'en demandait pas ; la variable n'était d'ailleurs pas définie).

## 3. Décisions prises seul

- **Mode par défaut = `REJEU`** quand `DONNEES_MODE` est absente (fermé par défaut : aucun réseau sans demande explicite). Une valeur inconnue est une erreur, pas un repli.
- **Nom de la variable** : `DONNEES_MODE` (le brief ne la nommait pas).
- **Statuts HTTP** : 404 → `introuvable` (pas de repli sur l'enregistrement en MIXTE : l'API a répondu). 400/422 → `requete_refusee`. 401/403 → `indisponible` sans nouvelle tentative. 429/5xx/réseau → 3 tentatives (attente 1 s puis 2 s), puis `indisponible`, et repli sur l'enregistrement en MIXTE.
- **Seules les réponses 2xx sont enregistrées.** Un enregistrement dont la clé ou l'empreinte ne correspond pas est refusé (`indisponible`), jamais servi.
- **Un enregistrement par requête** (clé = SHA-256 de méthode + URL nettoyée + corps). Un nouvel appel DIRECT l'écrase ; l'historique reste dans git.
- **Mémoïsation en mémoire 10 min** en DIRECT/MIXTE (règle « mise en cache de toute réponse déjà obtenue ») ; la réponse resservie garde `origine: direct` et **sa date d'obtention d'origine**.
- **Filtre avant enregistrement** : un serveur peut réduire un corps avant de le versionner (retrait des personnes physiques). L'enregistrement porte alors `filtre` et `sha256_origine` (empreinte du corps reçu), et la `source` d'une réponse rejouée affiche le filtre : un rejeu filtré est partiel et le dit.
- **`recherche-entreprises` ne restitue ni nom ni adresse d'un entrepreneur individuel** (nature juridique 1xxx) ni d'une unité en diffusion partielle, et ne les enregistre jamais. Requête `minimal=true&include=siege,complements` : pas de dirigeants dans la réponse, donc aucun nom de personne physique dans les enregistrements.
- **SIREN contrôlé localement** (9 chiffres, clé de Luhn) avant tout appel : `requete_refusee` sans réseau.
- **Répertoire d'enregistrements** : option `-enregistrements`, par défaut `servers/<nom>/enregistrements` relatif au répertoire courant (lancer les binaires depuis la racine du dépôt).
- **`gels-avoirs` : l'enregistrement est filtré.** Le registre complet contient 4 648 personnes physiques : il ne peut pas être versionné tel quel. L'enregistrement ne garde que les personnes morales (1 924) et navires (39), avec nom, alias (sans commentaire), fondements juridiques, références UE/ONU et numéro OMI. Motifs, adresses, téléphones, courriels, sites et identifiants sont retirés (ils peuvent nommer des personnes). `sha256_origine` garde l'empreinte du registre complet reçu. En rejeu, la réponse porte `source.filtre` et un `avertissement` : **une absence de correspondance en rejeu n'est pas probante**.
- **`gels-avoirs` en DIRECT renvoie aussi les correspondances de personnes physiques** (nom tel qu'au registre, nature, fondements), car c'est la fonction du filtrage. Rien de cela n'est enregistré ni commité. Voir question 2.
- **Correspondance** : noms normalisés (minuscules, sans accents, ponctuation → espace). `exacte` = mêmes mots dans le même ordre ; `partielle` = tous les mots cherchés présents dans le nom ou un alias. Pas de correspondance approchée (fautes de frappe) : plus simple à expliquer au jury, et un faux négatif possible, à dire. Au plus 20 correspondances rendues, total toujours donné.
- **Webstat : l'API exige une clé.** Le catalogue Opendatasoft répond sans clé mais le jeu `observations` renvoie 404 sans authentification. Le site embarque une clé dans son JavaScript public : **je ne l'ai pas utilisée** (ce n'est pas une clé qui nous a été donnée). J'utilise l'**export CSV public** du site : `https://webstat.banque-france.fr/export/csv/fr/catalog/mir1/<série>`, sans clé, une série par requête (l'export multi-séries renvoie 404).
- **Séries retenues** : les 10 séries trimestrielles (`MIR1.Q…`) « seuil de l'usure » non arrêtées (immobilier fixe < 10 / 10-20 / ≥ 20 ans, variable, relais, conso ≤ 3 000 / 3 000-6 000 / > 6 000 €, découverts EI, découverts sociétés non financières). Codes de catégorie stables en `enum` dans le schéma d'entrée. Pas les séries APU/ISBLSM (hors sujet démo), ajout trivial.
- **« En vigueur »** = le trimestre dont la période contient la date de référence (Europe/Paris, aujourd'hui par défaut, ou `date_reference`). Le seuil Q4 2026 est déjà publié (2026-09-28) : il est rendu à part dans `suivant`, jamais comme seuil en vigueur avant le 1er octobre.
- **`date_publication`** = `updated_at` de l'observation sur Webstat (mise à jour par la Banque de France, 2-3 jours avant le début du trimestre). Ce n'est pas la date de l'avis au JORF, que Webstat ne donne pas.
- **Instantané calculé au jour de la démo, 2026-10-04** (sur instruction de Manu) : Q4 en vigueur, instantané Q3. Régénération : `go run ./servers/taux-officiel instantane -date 2026-10-04` (sans réseau, depuis les enregistrements) ; la date est la constante `JourDeDemo` des tests.
- **`eurlex` : texte via CELLAR, pas via eur-lex.europa.eu.** `eur-lex.europa.eu/legal-content/FR/TXT/HTML/?uri=CELEX:…` répond `202` à corps vide (protection anti-robots) ; je ne l'ai pas contournée. `https://publications.europa.eu/resource/celex/<CELEX>` avec `Accept: application/xhtml+xml` et `Accept-Language: fra` redirige vers la manifestation XHTML (domaine autorisé). La redirection pointe en `http://` : le client la passe en `https://`.
- **`lire_article` rend la version publiée au JO, non consolidée**, et le dit (`version`). Pour l'AI Act, les modifications de l'omnibus 2026/1744 n'y figurent donc pas ; `rechercher_acte` les signale (`modifie_par`). Pas de version consolidée : CELLAR la publie sous un autre CELEX (`0…`), non traité cette nuit.
- **Signatures retirées des enregistrements `eurlex`** : le bloc final (`<div class="oj-final">`) nomme les présidents du Parlement et du Conseil. Il est ôté avant enregistrement (`source.filtre` le dit) ; si le bloc est absent ou multiple, le texte n'est pas enregistré, par prudence. Les articles sont intacts.
- **Extraction d'article** : `encoding/xml` (bibliothèque standard, le XHTML est du XML) plutôt que `golang.org/x/net/html`, pour ne pas toucher `go.mod` (commité avant la ligne de base). Tableaux imbriqués (points « 1) », « a) ») rendus une ligne par point, dans l'ordre.
- **Liste blanche d'hôtes** dans chaque client (`Restreindre`) : une requête ou une redirection hors du domaine de l'API est refusée (règle réseau des consignes).
- **`legifrance` : point d'accès `POST /consult/getArticle`** (documenté dans l'API Légifrance de la DILA sur PISTE, corps `{"id":"LEGIARTI…"}`). Il a répondu `200` dès la première tentative : pas de `500` cette nuit. L'authentification OAuth `client_credentials` (scope `openid`) passe.
- **`legifrance` rend la version en vigueur à la date**, pas forcément celle demandée : si l'identifiant fourni est une version passée ou future, l'outil lit la version applicable (second appel) et le dit (`identifiant_demande` et `en_vigueur.identifiant`). Une version déjà publiée pour plus tard est dans `suivante`. Versions `MODIFIE_MORT_NE`, `ANNULE`, etc. jamais choisies. Dates Légifrance (ms, minuit UTC) rendues en `AAAA-MM-JJ`.
- **Jeton OAuth** : obtenu hors de `rejeu` (jamais enregistré), gardé en mémoire jusqu'à une minute avant expiration, aucune redirection suivie (le secret ne part que vers `oauth.piste.gouv.fr`), corps d'erreur OAuth jamais repris dans un message. En REJEU, aucun jeton n'est demandé et les identifiants PISTE sont inutiles. En MIXTE, un échec du jeton sert l'enregistrement (`rejeu.Relire`). L'en-tête `Authorization` n'est jamais enregistré ; vérifié : aucun fichier commité ne contient la valeur de `PISTE_CLIENT_ID` ni de `PISTE_CLIENT_SECRET`.
- **Clé d'un enregistrement POST** = méthode + URL + corps de requête (`{"id":…}`) : une version d'article = un fichier.
- **Résultat `indisponible`** : résultat d'outil structuré avec `statut: indisponible` et `isError: true`, jamais une erreur de protocole.

## 4. Blocages

Aucun blocage dur. Contournements, tous documentés en section 3 :

- **Webstat** : API à clé (pas de clé fournie). Contourné par l'export CSV public du site. L'export multi-séries renvoie `404 No series available for this export` : une requête par série.
- **eur-lex.europa.eu** : `202` à corps vide (anti-robots), non contourné. Texte obtenu par CELLAR (`publications.europa.eu`).
- **Gels des avoirs** : **pas d'erreur de certificat** ici (curl et Go valident la chaîne TLS de `gels-avoirs.dgtresor.gouv.fr`).
- **Légifrance** : le `500` de la veille ne s'est pas reproduit.
- Webstat : quelques observations anciennes ont une valeur vide, ce qui a fait échouer le premier essai DIRECT (`indisponible`, comme prévu) ; ces lignes sont désormais ignorées.

## 5. Questions pour Manu (par urgence)

1. ~~Date de la démo~~ : réglé, démo le 2026-10-04, instantané Q3.
2. **`gels-avoirs` en DIRECT restitue le nom d'une personne physique sanctionnée quand on la cherche.** C'est donnée publique et c'est le but du filtrage, mais la règle dit « aucune donnée de personne physique réelle, nulle part ». Si l'engine journalise les résultats d'outils, ce nom finira dans le journal. Faut-il masquer les noms des personnes physiques dans le résultat (ne garder que id, nature, fondements) ? Changement court dans `servers/gels-avoirs/serveur.go`.
3. **Entreprises de démo** : je n'ai pas pu vérifier l'absence d'affaire judiciaire (aucun domaine autorisé ne le permet). Choisies parce que ce sont des sociétés établies, sans affaire notoire à ma connaissance. À valider avant la démo.
4. **Noms d'entités du registre des gels** : un nom de personne morale peut contenir un nom de personne (entreprise éponyme). Non filtré (impossible sans liste de noms). Risque résiduel à connaître.

## 6. Vérifier en 5 minutes

```sh
cd ~/cybercar/mcp_rogue-donnees
go vet ./... && go test -race ./servers/...
go build -o bin/appeler ./servers/appeler && go build -o bin/recherche-entreprises ./servers/recherche-entreprises
DONNEES_MODE=REJEU  bin/appeler bin/recherche-entreprises rechercher_entreprise '{"siren":"405750753"}'   # source.origine = rejeu
DONNEES_MODE=DIRECT bin/appeler bin/recherche-entreprises rechercher_entreprise '{"siren":"405750753"}'   # source.origine = direct (réseau)
go build -o bin/gels-avoirs ./servers/gels-avoirs
DONNEES_MODE=REJEU  bin/appeler bin/gels-avoirs verifier_gel '{"nom":"JSC ROSOBORONEXPORT"}'   # 1 correspondance exacte, avertissement « partiel »
go build -o bin/taux-officiel ./servers/taux-officiel
DONNEES_MODE=REJEU  bin/appeler bin/taux-officiel get_taux_usure '{"categorie":"decouvert-societes-non-financieres","date_reference":"2026-10-04"}'   # jour de la démo : Q4 19,08 %
cat servers/taux-officiel/instantane_trimestre_precedent.json | head -20
go build -o bin/eurlex ./servers/eurlex
DONNEES_MODE=REJEU  bin/appeler bin/eurlex rechercher_acte '{"celex":"32024R1689"}'                 # modifie_par 32026R1744
DONNEES_MODE=REJEU  bin/appeler bin/eurlex lire_article '{"celex":"32022R2554","article":"28"}'
go build -o bin/legifrance ./servers/legifrance
DONNEES_MODE=REJEU  bin/appeler bin/legifrance lire_article '{"identifiant":"LEGIARTI000032303335"}'   # v2.0, suivante v3.0 au 2026-11-20
DONNEES_MODE=REJEU  bin/appeler bin/legifrance lire_article '{"identifiant":"LEGIARTI000032303335","date_reference":"2026-12-01"}'   # sert la v3.0
# Panne simulée, sans réseau réel : MIXTE bascule sur l'enregistrement et le dit
HTTPS_PROXY=http://127.0.0.1:9 DONNEES_MODE=MIXTE bin/appeler bin/taux-officiel get_taux_usure '{"categorie":"pret-relais"}'
# Schéma figé = tools/list brut
go build -o bin/exporter-schema ./servers/exporter-schema && bin/exporter-schema bin/recherche-entreprises | diff - servers/recherche-entreprises/schema.json && echo schéma identique
```

Sans `DONNEES_MODE`, les serveurs sont en REJEU (aucun réseau). DIRECT et MIXTE font des appels réels (1 req/s au plus par API) ; `legifrance` y exige `PISTE_CLIENT_ID` et `PISTE_CLIENT_SECRET`. Lancer les binaires depuis la racine du dépôt (ou passer `-enregistrements`).

Schémas : `servers/<nom>/schema.json` = `result.tools` brut de `tools/list`, écrit par `go run ./servers/exporter-schema -o servers/<nom>/schema.json bin/<nom>` ; un test par serveur (`TestSchema`) échoue si le serveur diverge du fichier.
