# Rapport de nuit — session « Données »

Branche `feat/donnees`, worktree `~/cybercar/mcp_rogue-donnees`. Rien n'a été poussé ni fusionné.

## 1. Fait

| Commit | Contenu |
|---|---|
| `7f7e928` | `servers/internal/rejeu` : enregistrement et rejeu (DIRECT / REJEU / MIXTE), tests sans réseau |
| `9e3bdc9` | Outillage : `servers/internal/lanceur` (main commun), `servers/internal/essai` (aides de test), `servers/exporter-schema` (écrit `result.tools` brut de `tools/list`, lu sur le stdio du binaire), `servers/appeler` (essai manuel d'un outil) |
| `c500c7a` | Serveur `entreprises` : `rechercher_entreprise(siren)`, 3 entreprises de démo enregistrées |

### Serveurs

| Serveur | DIRECT | REJEU | SHA-256 du binaire (`go build -trimpath`, go1.25.0 linux/amd64) |
|---|---|---|---|
| `entreprises` | OK (3 SIREN, 2026-09-29 ~22:00 UTC) | OK | `15c53e7d1c68e3066ec2e8835876bc7f98df95c6cf8b5d58addbbf29b824d2c6` |

**Entreprises de démo** (actives, personnes morales, diffusion publique) :

| SIREN | Dénomination | Forme | Catégorie |
|---|---|---|---|
| 405750753 | TRICOTS SAINT-JAMES | SAS (5710) | ETI |
| 382891752 | BIOCOOP | SA coopérative de commerçants-détaillants (5560) | ETI |
| 508858230 | OPINEL DISTRIBUTION | SAS (5710) | PME |

## 2. Pas fait (et pourquoi)

En cours : serveurs `gels-avoirs`, `taux-officiel`, `eurlex`, `legifrance`.

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
- **Résultat `indisponible`** : résultat d'outil structuré avec `statut: indisponible` et `isError: true`, jamais une erreur de protocole.

## 4. Blocages

Aucun pour l'instant.

## 5. Questions pour Manu (par urgence)

1. **Entreprises de démo** : je n'ai pas pu vérifier l'absence d'affaire judiciaire (aucun domaine autorisé ne le permet). Choisies parce que ce sont des sociétés établies, sans affaire notoire à ma connaissance. À valider avant la démo.

## 6. Vérifier en 5 minutes

```sh
cd ~/cybercar/mcp_rogue-donnees
go vet ./... && go test -race ./servers/...
go build -o bin/appeler ./servers/appeler && go build -o bin/entreprises ./servers/entreprises
DONNEES_MODE=REJEU  bin/appeler bin/entreprises rechercher_entreprise '{"siren":"405750753"}'   # source.origine = rejeu
DONNEES_MODE=DIRECT bin/appeler bin/entreprises rechercher_entreprise '{"siren":"405750753"}'   # source.origine = direct (réseau)
```
