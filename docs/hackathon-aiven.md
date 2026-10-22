# Hackathon Accel AI Innovate — journal dans Aiven PostgreSQL

Ajout du 22/10/2026 : `cmd/journal-sql`. Le moteur n'est pas modifié.

## Ce que fait la commande

| Sous-commande | Rôle |
|---|---|
| `schema` | Tables `journaux`, `entrees`, `consultations`, `applications`, `impacts` et vue `detective` |
| `exporter` | Vérifie la chaîne HMAC, puis écrit le journal en SQL. Un journal rompu n'entre pas dans la base (code 1, aucun SQL) |
| `impacts -gels` | Rejoue le journal sous le jeu d'origine, puis sous ce jeu augmenté de D-090 « aucun prêt à une entité nouvellement gelée » (`rejeu.Comparer`). Écrit dans `impacts` chaque décision, avant/après |
| `impacts -apres` | « Et si cette règle avait existé ? » : les directives candidates (un brouillon du compilateur, une directive seule, une liste) remplacent ou complètent le jeu, évaluées comme actives depuis sa date d'effet |

La commande n'ouvre aucune connexion et n'ajoute aucune dépendance : elle
écrit du SQL sur la sortie standard, à envoyer à `psql`.

## Pas à pas

```bash
export AIVEN_PG_URI='postgres://avnadmin:…@….aivencloud.com:…/defaultdb?sslmode=require'
export MCP_GATE_CLE_JOURNAL=…          # la clé du journal, comme pour mcp-gate
go build -o bin/journal-sql ./cmd/journal-sql

# 1. Schéma (une fois)
bin/journal-sql schema | psql "$AIVEN_PG_URI"

# 2. Verser un passage (rejouable : le même journal remplace ses lignes)
bin/journal-sql exporter demo/execution/sessions/<dossier>/journal.jsonl | psql "$AIVEN_PG_URI"

# 3. Le registre des gels ajoute un SIREN : quelles décisions tombent ?
bin/journal-sql impacts demo/execution/sessions/<dossier>/journal.jsonl \
    -directives demo/scenarios/jeux/avec-d009.json \
    -gels <SIREN> -evenement "gels 2026-10-22" | psql "$AIVEN_PG_URI"

# 4. Et si cette règle avait existé ? (le brouillon du compilateur, ou la règle prête)
bin/journal-sql impacts demo/execution/sessions/<dossier>/journal.jsonl \
    -directives demo/scenarios/jeux/avec-d009.json \
    -apres demo/scenarios/jeux/hypothese-seuil-officiel.json | psql "$AIVEN_PG_URI"
```

Le résumé (décisions rejouées, changées) sort sur l'erreur standard.
Le coffre est par défaut `coffre.jsonl` à côté du journal (`-coffre` sinon).

## Requêtes pour la démo

```sql
-- Detective : sur quelles données l'agent a-t-il décidé ?
SELECT session, seq, action, sous_agent, effet, donnee_lue, source_origine, source_date
FROM detective WHERE action = 'banque-synthetique/attribuer_pret';

-- Quelles directives ont joué sur chaque décision ?
SELECT e.session, e.seq, a.directive, a.declenchee, a.appliquee, a.effet
FROM applications a JOIN entrees e USING (journal, rang)
WHERE a.declenchee ORDER BY e.session, e.seq;

-- Impact rétroactif : la mesure
SELECT evenement, count(*) AS rejouees, count(*) FILTER (WHERE change) AS invalidees
FROM impacts GROUP BY evenement;

SELECT session, seq, serveur || '/' || outil AS action, avant, apres
FROM impacts WHERE change;
```

## Limites, à dire telles quelles

- D-090 porte sur l'argument `siren` d'`attribuer_pret` : le moteur ne lit
  pas le contenu des réponses, seulement leurs métadonnées. L'impact se
  calcule donc à partir des arguments déposés au coffre ; un appel dont les
  arguments ont été purgés est `non_rejouable`, jamais compté comme sûr.
- Le rejeu évalue « si nous avions su » (cadre d'exposition) : la date
  d'effet de D-090 est celle du jeu d'origine.
- Données fictives (banque synthétique) ; préciser dans la démo ce qui vient
  d'un passage réel et ce qui vient d'un rejeu enregistré.
