# Fiche juriste : écrire une directive que le compilateur comprend du premier coup

Le compilateur lit votre texte, le traduit en règle, puis **le code** (pas le modèle) vous la relit en français : vous validez ce que vous lisez, jamais ce que le modèle a « compris ». Plus le texte nomme précisément ce qu'il vise, moins il y a de questions.

## Les cinq éléments à nommer

| Élément | Question à laquelle le texte répond | Exemple |
|---|---|---|
| **Source** | Quel outil, quel serveur, quelle famille de données ? | « les appels à `banque-synthetique/attribuer_pret` », « tout outil étiqueté `donnees_sante` » |
| **Seuil chiffré** | À partir de quel montant, quelle date, quelle heure ? Un nombre, pas « élevé » | « supérieur à 100 000 euros » |
| **Rôle** | Qui valide, qui est prévenu, qui est visé ? Un rôle ou un groupe **de l'annuaire** | « validation d'un `analyste-risques` », « demandeur du groupe `prestataires-externes` » |
| **Fuseau** | Pour tout jour ou toute heure : quel fuseau ? | « le week-end, heure de Paris » |
| **Fondement** | Quelle règle de droit ou politique interne ? | « RGPD, art. 9 », « politique d'indemnisation, art. 4.2 » |

Et dites **ce qui doit arriver** : refuser, attendre la validation d'un rôle (avec ou sans « quatre yeux » : validateur distinct du demandeur), caviarder des champs, prévenir quelqu'un (en temps réel, ou en synthèse ; et si la notification échoue : refuser ou continuer), marquer.

Modèle de phrase : *« [Sujet visé], si [condition chiffrée / de rôle / de temps], alors [effet]. »* Exemple complet : « Indemnisation d'un sinistre : une indemnisation supérieure à 100 000 euros n'est jamais validée par un agent. » Fondement : « Politique d'indemnisation, art. 4.2 ».

## Ce que la règle peut observer (les « feuilles », en français)

- **un argument de l'appel** : un champ comparé à un nombre, un texte, une liste (`>`, `<`, `=`, `parmi`, « commence par », « présent », « absent ») ;
- **ce que la demande a déjà consulté** : un serveur, un outil, une étiquette, avec « appel parti » ou « réponse reçue » (c'est ainsi qu'on écrit « sans avoir consulté le registre des gels ») ;
- **qui demande** : une identité, ou l'appartenance à un groupe de l'annuaire ;
- **le moment** : jour de la semaine, plage horaire, date, **toujours avec un fuseau** ;
- **l'état du serveur** : attesté ou non ;
- **la provenance des données reçues** : origine, date de publication ;
- **la personne concernée** par l'action (RH) : son groupe, résolu par le référentiel, jamais lu en clair ;
- **l'outil visé** : son nom, son serveur, son accès (lecture ou action), ses étiquettes ;
- **le sous-agent** qui appelle (déclaratif : pour resserrer, jamais pour accorder).

Ces éléments se combinent avec « et », « ou », « non », « il existe… dans la liste », « pour tous… dans la liste » (6 niveaux et 64 conditions au plus).

## Ce qui n'est pas exprimable, et comment le reformuler

| Vous voulez dire… | Pourquoi ce n'est pas exprimable | Reformulation |
|---|---|---|
| « sauf si c'est raisonnable », « en cas de doute », « discrimination » | **critère non observable** : aucun champ d'appel ne le porte | Remplacer par un fait observable : un seuil, un rôle, un groupe, une validation humaine (« attend la validation d'un responsable ») |
| « si le prix est supérieur à celui de l'autre serveur » | **comparaison entre deux serveurs** : une règle compare une valeur à un nombre écrit dans la règle, pas à une valeur lue ailleurs | Écrire le seuil chiffré, ou exiger d'avoir consulté l'autre serveur (« a consulté le serveur taux-officiel ») puis une validation |
| « si l'agent a bien compris la demande » | intention du modèle | Viser ce qu'il fait : l'outil appelé, ses arguments |
| « jamais » sans cible | rien à observer | Nommer l'outil, le serveur ou l'étiquette |
| « tout le monde sauf X » sur un nom absent de l'annuaire | l'identité doit exister dans l'annuaire | Utiliser un rôle ou un groupe existant |
| une étiquette, un outil ou un serveur qui n'existe pas | le compilateur ne connaît que le catalogue du registre | Vérifier les noms (console, onglet règles) ; ajouter le serveur avec `docs/demo/etendre.md` |

Si le compilateur répond **« non observable »**, il cite l'extrait fautif et la raison : reformulez cet extrait seulement. S'il **pose une question** (fuseau, périmètre, rôle), répondez dans la console : une réponse ne refait pas les appels déjà faits.

## Avant de mettre en vigueur

1. Relire la **relecture en français** : c'est la règle qui sera appliquée.
2. Choisir : brouillon, **observation** (la règle est évaluée et journalisée sans bloquer), ou active.
3. Le fondement est une affirmation du juriste : le compilateur le recopie, ne le vérifie pas.
