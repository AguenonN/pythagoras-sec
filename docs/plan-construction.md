# Plan de construction

Deux étages. Le **socle** est commun aux deux profils de déploiement : schéma,
runtime, décision, preuve. L'**approche grand public** est le premier
déploiement : génération et exécution dans le cloud, pour les services publics.

Tout ce qui suit a été décidé. Rien n'y est ajouté.

---

## Propriétés qui ne se négocient pas

Elles valent pour chaque phase et conditionnent chaque choix d'implémentation.

- **Rien n'est généré en code.** Le modèle produit un descripteur, de la
  donnée. Ce qui s'exécute est le runtime, écrit à la main, toujours le même.
- **La décision est prise par effet**, au moment où l'effet se produit, jamais
  une fois pour toutes avant.
- **Le modèle ne peut que durcir.** Ni le générateur ni un classifieur ne peut
  produire une issue plus faible que les règles déterministes.
- **Le déterministe passe avant le modèle.** Tout ce qu'un `if` tranche est
  tranché sans appel de modèle.
- **Fermé sur l'incertitude.** Un verdict en zone grise, un modèle qui ne
  répond pas, une entrée trop longue : non évaluable, donc refus.
- **Le secret ne quitte jamais le proxy.** Le descripteur et son exécution ne
  voient aucun credential.
- **Pas de réconciliation.** La reprise est refusée par défaut, déclarée par
  étape, et jamais automatique sur une classe non idempotente.

Et cinq contraintes qui gardent le profil entreprise atteignable sans le
construire — gratuites maintenant, chères à rattraper :

1. le runtime n'appelle jamais chez nous ;
2. les points d'accès des modèles sont de la configuration ;
3. le dépôt de descripteurs fonctionne hors ligne ;
4. le journal est local et suffisant pour rejouer ;
5. la génération n'est pas sur le chemin de l'exécution.

---

# Socle

Les six phases du socle **ne demandent aucun modèle**. Elles se testent
entièrement avec des descripteurs écrits à la main et des verdicts fixtures.
C'est ce qui permet de prouver le moteur avant d'y brancher quoi que ce soit
de probabiliste.

## B0 — Préparer le dépôt

- Poser le tag `pre-refonte`.
- **Extraire les fixtures de `registredemo` avant toute suppression** :
  `test/integration`, `test/integration/v2_test.go` et
  `deploy/lab/registre-lab` en dépendent. Les supprimer dans le mauvais ordre
  casse les tests d'intégration au moment où le filet sert le plus.
- Retirer `test/extension/kit_test.go` avec `internal/console`.
- Retirer la couche de démonstration : `console`, `registredemo`, les serveurs
  applicatifs, `agents/analyste`, `agents/orchestrateur`, `pyagent`, `demo/`.
- Garder 2–3 serveurs neutres comme fixtures ; régénérer le catalogue.

**Fini quand** : `go test ./...` est vert sur un `main` nettoyé.

## B1 — Le schéma du descripteur

Trois types d'état en v1, et rien d'autre tant qu'un connecteur réel n'a pas
réclamé davantage :

| État | Rôle |
|---|---|
| `appel` | une requête HTTP vers une opération de la spec |
| `liaison` | extraire une valeur d'une réponse pour une étape suivante |
| `choix` | brancher sur une valeur d'un ensemble fermé |

Hors périmètre v1 : exécution parallèle, itération sur collection, attente,
taxonomie d'erreurs complète.

Chaque étape porte en plus, et c'est l'ajout propre au projet :

- sa **classe d'effet** (lecture, écriture, transaction, administration) ;
- sa **réversibilité**, qui décide si une reprise est seulement envisageable.

Le descripteur déclare ses **paramètres** : le connecteur est paramétré, jamais
spécialisé. Toute valeur qui varie d'une requête à l'autre entre par là, sinon
l'empreinte de l'artefact change et le cache ne sert plus.

Chargeur strict : tout champ hors schéma est un refus, avec la position du
fautif.

**Fini quand** : un descripteur se charge ou se refuse, et le refus nomme ce
qui cloche.

## B2 — Le runtime

La boucle, écrite une fois :

```
pour chaque étape :
    construire la requête depuis le gabarit et les liaisons
    classer l'effet (table déterministe)
    Decide → autoriser | caviarder | attente humaine | refuser
    si autorisé : transmettre, puis extraire les liaisons de la réponse
```

Reprise refusée par défaut. Aucune boucle de convergence : une étape qui
échoue ne se rejoue pas pour « arriver à l'état voulu ».

**Fini quand** : un descripteur écrit à la main s'exécute de bout en bout
contre un serveur de test.

## B3 — E_classe, E_instance, canonisation

| | Contenu | Où |
|---|---|---|
| `E_classe` | domaine, palier | dans l'attestation, et c'est la clé de cache |
| `E_instance` | plafond, adresse, habilitation, session | vérifié à l'exécution |

L'échelle des paliers : `lecture < écriture < transaction < administration`.
Une demande est **arrondie au palier minimal qui la couvre**. Le palier
`administration` n'est jamais atteint par arrondi : il exige sa propre
admission et une validation humaine.

`E_classe` se canonise déterministement avant d'être hachée — domaines triés,
palier sur un ensemble fermé. Un périmètre qui ne se canonise pas est non
évaluable, donc refusé.

**Égalité stricte** sur `E_classe` pour un succès de cache. Pas d'inclusion :
l'arrondi fait le travail, avant l'attestation, de façon déclarée.

À l'exécution, `manifeste ⊆ enveloppe` est vérifié à chaque fois.

**Fini quand** : deux formulations équivalentes d'une même demande produisent
la même empreinte de `E_classe`.

## B4 — Le plancher déterministe

Classification minimale, indépendante de ce que le descripteur déclare :

- méthode non-`GET` ⇒ au moins `écriture` ;
- corps portant un montant ⇒ au moins `transaction` ;
- chemin de compte ou d'identité ⇒ `administration` ;
- domaine hors enveloppe ⇒ **refus immédiat, sans consulter aucun modèle**.

La classe retenue est le maximum entre déclarée et plancher. Le générateur
peut sur-déclarer, jamais sous-déclarer.

Même forme que `internal/controle` : une fonction pure après `Decide`, que le
moteur et le rejeu appellent à l'identique.

**Fini quand** : les refus structurels se produisent sans qu'aucun modèle soit
appelé, et le rejeu les recalcule.

## B5 — `Decide` avec verdicts

```
Decide(règles, état de session, effet, verdicts[]) → décision
```

- Le type `Verdict` est pur et vit dans `directive` : distribution brute,
  empreinte de l'état, version du gabarit et du modèle. Le client du
  classifieur vit **dehors** — le test de pureté interdit `net/http` et
  `context` dans le paquet.
- Combinaison par **table de décision** sur trois valeurs (vrai, faux,
  indéterminé), écrite en lignes lisibles, jamais par un modèle.
- Vérificateur **statique** au chargement : toute combinaison non couverte
  produit `refuser`, et remplacer un état par `indéterminé` ne peut jamais
  affaiblir l'issue.
- `Decide(…, nil)` rend exactement ce que l'actuel rend : non-régression sur
  les scénarios existants.

**Fini quand** : le test de propriété de monotonie passe, et les scénarios v2
actuels sont inchangés.

## B6 — Journal et rejeu de l'effet

- L'entrée `appel` devient une entrée `effet`, et porte tout ce que `Decide` a
  lu, verdicts compris.
- Les verdicts journalisent la **distribution brute** ; mode, confiance et
  état se recalculent en Go. Le rejeu ne dépend pas du post-traitement d'un
  fournisseur.
- Chaînage par MAC inchangé.
- Deux modes de rejeu, qui ne promettent pas la même chose : le **rejeu
  logique** réinjecte les verdicts et retrouve l'issue au bit près ; le
  **rejeu modèle** relance l'inférence et mesure une dérive, sans rien
  invalider.

**Fini quand** : **C1** — 100 % des effets journalisés se rejouent à
l'identique.

---

# Approche grand public

Premier déploiement. Deux plans séparés dès le départ : leur rayon d'explosion
n'est pas le même, et les recoller plus tard coûte plus cher que les séparer
maintenant.

| | Génération | Exécution |
|---|---|---|
| Fréquence | rare, par `(domaine, palier)` | à chaque requête |
| Secrets détenus | **aucun** | **tous** |
| Forme | exécution à la demande | chaud, proche de l'utilisateur et de l'API |
| Compte / VPC | séparé | séparé |

## Choix du premier service

Clé statique, spec OpenAPI publique, compte contrôlé. **Pas Outlook** : le mur
de l'enregistrement d'application mangerait le temps, et ce n'est pas le sujet.

Les services grand public — Uber Eats, WhatsApp, Instagram — servent à
expliquer le problème et à l'usage personnel. La valeur est sur les API
internes d'entreprise, qui viendront avec le second profil.

## G0 — Le plan d'exécution

- Le runtime, et le **proxy d'egress comme unique sortie réseau**.
- Le coffre : le jeton du fournisseur y reste, le proxy l'attache au dernier
  moment.
- Le descripteur ne reçoit qu'un **JWT interne**, de courte durée, lié à la
  session et à l'empreinte de l'artefact.
- Décision par effet câblée sur chaque requête sortante.

**Fini quand** : un descripteur s'exécute contre une vraie API, et aucun
credential n'est jamais à portée de ce qui l'exécute.

## G1 — Le plan de génération

- Entrée : la spec OpenAPI et la demande. Sortie : un descripteur.
- **Vérification déterministe contre la spec** : chaque étape référence une
  opération qui existe, avec des paramètres qui existent, aux types attendus.
  Un endpoint inventé ne passe pas.
- Boucle de réparation **bornée à deux tentatives**, journalisées. Une
  réparation qui réclame plus de droits est un refus, pas une renégociation :
  l'enveloppe ne s'élargit jamais.
- La trappe de sortie — du code, pour ce qui ne rentre pas dans le descripteur
  — n'est pas construite ici. Mesurer d'abord quelle proportion la réclame.

**Fini quand** : un descripteur est produit et vérifié pour le premier
service, sans intervention manuelle.

## G2 — Classification et verdicts

- La **normalisation** de la demande vers `(domaine, palier)` est une
  classification, pas une génération.
- Les classifieurs ne lisent **jamais** le code ni la spec entière : seulement
  des faits extraits, courts et fermés.
- Un lot = un état + N questions, répondues en parallèle sans dépendance. Une
  dépendance réelle entre questions est un second tour, et on se tient à deux.
- **Moindre privilège sur les entrées** : un lot par niveau de privilège, au
  minimum deux — ce que l'utilisateur ou une réponse peut influencer, et ce
  que le moteur produit.
- Le générateur et le juge ne sont pas du même modèle : des poids partagés
  font des angles morts partagés.
- Un contrôle sans FPR/FNR mesurés est non évaluable, donc refus.

**Fini quand** : les verdicts entrent dans `Decide` et le rejeu logique reste
exact.

## G3 — Attestation et cache

- Forme in-toto, signature DSSE : le **sujet** est l'artefact, le **prédicat**
  porte `E_classe`. La signature couvre l'énoncé entier, donc lier le
  périmètre ne coûte rien de plus.
- Le cache est **clé sur `E_classe`**, égalité stricte. La clé sort ainsi de
  la main du modèle : deux formulations divergentes du même périmètre donnent
  la même clé.
- Ce qui entre en cache est admis. Rien ne s'exécute qui ne soit attesté.

**Fini quand** : un succès de cache réutilise un artefact et son attestation
vérifie.

## G4 — Mesure

C'est la phase qui tranche, et sans elle il n'y a qu'une démonstration.

| Mesure | Témoin |
|---|---|
| FPR / FNR par contrôle | — |
| Calibration : confiance annoncée contre justesse observée | — |
| **Erreur jointe** du vecteur de verdicts | le produit des erreurs marginales |
| Apport réel des classifieurs | **le plancher déterministe seul** |
| Résistance | le seul sous-ensemble avec injections |

Jeu étiqueté d'environ 200 cas, dont **30 % d'injections**.

Et une mesure qui ne se simule pas : **combien de connecteurs fonctionnent au
premier appel réel**. Un taux de génération mesuré sur des fixtures ne vaut
rien — les fixtures ne mentent pas, elles ignorent.

**Fini quand** : **C2** est tranchée. Si les classifieurs n'apportent rien par
rapport au plancher, il vaut mieux le savoir ici.

---

## Ordre et dépendances

```
B0 → B1 → B2 ─┐
     B3 ──────┼→ B4 → B5 → B6 ──→ G0 → G1 → G2 → G3 → G4
```

B0 à B6 sans aucun modèle. La moitié risquée — les classifieurs — n'entre
qu'en G2, après que le rejeu exact a été démontré en B6. Si G4 échoue, tout ce
qui précède reste vrai et utile.

## Ce qui n'est pas dans ce plan

Volontairement, et chaque élément a sa raison :

- **Le profil entreprise.** Les cinq contraintes le gardent atteignable ; on
  ne le construit pas avant d'avoir un client.
- **Le dépôt de descripteurs et sa poussée vers les clients.** Il ne sert que
  la couche SaaS commune, qui n'existe pas encore.
- **Les classifieurs hébergés.** Ils viennent après le jeu étiqueté de G4, qui
  est aussi leur jeu d'entraînement.
- **La trappe de sortie en code.** Elle attend la mesure de G1.
