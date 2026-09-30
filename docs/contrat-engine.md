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

## 4. Rien ne part sans une entrée transmission chaînée

L'entrée `appel` porte le résultat, donc elle ne peut être écrite
qu'après la transmission. Si le journal tombe entre les deux, un appel
serait parti sans aucune trace. L'engine écrit donc, avant
`Tour.Transmettre`, une entrée `transmission` qui porte la session, le
seq, le serveur, l'outil, l'empreinte transmise et la décision.

- Écrire l'entrée `transmission` verrou tenu, après `Decide` et le dépôt
  au coffre, avec le `TS` de l'appel.
- Si elle ne peut pas être écrite : refuser l'appel, **ne rien
  transmettre**, et refuser tous les appels suivants (le journal n'est
  plus fiable).
- Un refus de l'engine (`non_transmis`) n'a pas d'entrée `transmission`.
- L'entrée `appel` suit, comme avant. Si c'est elle qui échoue, la
  réponse du serveur est retenue : l'agent ne reçoit pas de donnée sans
  preuve.

Au rejeu (`rejeu.RejouerJournal`, contrôle `transmission`), chacun de ces
cas est une altération :

- un appel transmis sans exactement une entrée `transmission` qui le
  précède ;
- une entrée `transmission` qui diffère de l'appel sur le serveur,
  l'outil, l'empreinte transmise ou l'effet ;
- une entrée `transmission` pour un appel `non_transmis`.

Une entrée `transmission` qu'aucune entrée `appel` ne suit est un appel
parti dont le résultat n'a pas été journalisé.
`rejeu.TransmissionsSansAppel` les rend. Elles ne se rejouent pas, faute
de décision complète : elles se signalent.

## 5. Le refus par le registre est une décision journalisée

Un serveur dont l'attestation n'est pas valide est refusé même si
aucune directive `source_attestee` ne le cible. Ce contrôle
s'applique après `Decide`, par `controle.Registre`, la fonction même
qu'appelle le rejeu.

- L'entrée `appel` porte `decision.effet = refuser`,
  `decision.origine = registre` et le motif du registre ; les
  applications restent celles de `Decide`.
- Une directive qui refuse déjà garde la main : l'origine reste celle
  des directives, qui citent leur fondement.

Au rejeu, le contrôle est recalculé sur l'attestation que l'entrée
porte. Une origine effacée ou inventée est une altération, tout comme un
statut incohérent avec l'effet (contrôle `statut`) :

- un refus n'est jamais transmis ;
- une autorisation l'est toujours ;
- une attente ne l'est qu'avec une approbation.

## Rappels

- L'appelant est fixé à la création de la session. Le passer tel que le
  transport l'a prouvé, jamais tel que le client le déclare ; un
  changement est refusé et journalisé (`identite_changee`).
- L'état passé à `Decide` est `tour.Etat()`, et l'instantané journalisé
  dans l'entrée `appel` est ce même état.
- Déposer les arguments au coffre (`Coffre.Deposer`) avant d'écrire
  l'entrée `appel` qui cite leur empreinte.
