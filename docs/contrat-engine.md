# Contrat de l'engine

Ce que l'agrégateur (l'engine qui reçoit les `tools/call` et les route vers
les serveurs) doit respecter pour que le journal reste rejouable. Les
paquets `directive`, `journal`, `session` et `rejeu` supposent ces règles ;
aucun ne peut les vérifier seul au moment de l'appel. Le rejeu, lui, les
vérifie après coup, et toute règle enfreinte y apparaît comme une
altération.

## 1. L'entrée `appel` est horodatée à la date passée à Decide

`directive.Decide` ne lit pas l'horloge : la date vient de
`directive.Appel.Horodatage` (non-rétroactivité des directives, échéances
de relecture). Le rejeu repart de `journal.Entree.TS`.

- Lire l'horloge **une seule fois** par appel, passer cette valeur à
  `Decide`, et écrire l'entrée `appel` avec `TS` égal à cette même valeur.
- Ne pas laisser `journal.Ecrivain.Ecrire` poser la date (il le fait quand
  `TS` est vide, avec l'heure d'écriture, qui est plus tardive).

Si la règle n'est pas tenue : une directive dont la date d'effet tombe
entre la décision et l'écriture fait diverger le rejeu, qui signale une
altération (`controle: decision` ou `transmis`) sur un appel honnête.

## 2. Le verrou de session est toujours rendu

`session.Gestionnaire.Entrer` prend le verrou de la session ; seul
`Tour.Terminer` le rend. Tant qu'il est tenu, aucun autre appel de la
session n'avance, et `Fermer` (départ du client) attend.

```go
tour, err := g.Entrer(ctx, ss, appelant)
defer tour.Terminer() // sans effet sur un tour nul : sûr avant le test d'erreur
if err != nil {
	return refus(err)
}
```

- `Terminer` en `defer`, sur **tous** les chemins, y compris le refus par
  l'engine, l'erreur de journal et le refus d'approbation.
- Le délai du backend (`Config.DelaiBackend`) ne borne que `Transmettre`.
  Tout ce que l'engine fait d'autre avec le verrou tenu (attente d'une
  approbation humaine, webhook) doit avoir son propre délai.
- Un refus de l'engine se termine sans `Transmettre` : ce n'est jamais une
  consultation.
- Écrire l'entrée `appel` **avant** `Terminer`, pour que l'ordre du journal
  soit celui des appels de la session et que `session_fermee` (écrite par
  `Terminer` quand une borne est atteinte) vienne après.

## 3. Chaque purge du coffre est chaînée

Le coffre n'est pas chaîné. Ce qui distingue une purge d'une suppression
est l'entrée `purge` écrite dans la chaîne du journal.

- Attacher le journal au coffre dès l'ouverture :
  `coffre.Journaliser(ecrivain)`. Sans journal, `PurgerEmpreintes` et
  `PurgerAvant` refusent (`ErrPurgeSansJournal`).
- Purger **uniquement** par `PurgerEmpreintes` ou `PurgerAvant`, avec une
  `DemandePurge` qui porte un motif et un auteur. L'entrée `purge` porte
  les empreintes, la date (celle des pierres tombales), le motif et
  l'auteur, et elle est écrite avant la réécriture du coffre.
- Ne jamais éditer, remplacer ou restaurer le fichier du coffre à la main.

Au rejeu, une pierre tombale sans entrée `purge` chaînée à la même date,
ou des arguments absents sans pierre tombale, sont des altérations. Une
purge conforme porte la cause `purge` et aucune nature : elle n'est
jamais une violation.

## Rappels

- L'appelant est fixé à la création de la session. Le passer tel que le
  transport l'a prouvé, jamais tel que le client le déclare ; un
  changement est refusé et journalisé (`identite_changee`).
- L'état passé à `Decide` est `tour.Etat()`, et l'instantané journalisé
  dans l'entrée `appel` est ce même état.
- Déposer les arguments au coffre (`Coffre.Deposer`) avant d'écrire
  l'entrée `appel` qui cite leur empreinte.
