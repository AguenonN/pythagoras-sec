# Règles communes

À lire en entier avant de commencer. Identique pour toutes les sessions.

---

## 1. Autonomie

Personne ne répondra pendant ta session.

- Face à une décision que ce brief ne couvre pas : choisir **l'option la plus
  prudente** — fermée par défaut, la moins irréversible — la noter dans
  `RAPPORT_NUIT.md` avec sa justification, et continuer.
- Ne jamais boucler plus de **3 fois** sur le même problème. Au troisième
  échec, noter le blocage et passer à l'élément suivant.
- Ne jamais inventer un contrat partagé. Ceux de la section 6 sont figés : ils
  se transcrivent, ils ne se redessinent pas. Si l'un te paraît faux, le noter
  dans le rapport et **l'appliquer quand même**.

## 2. Dépôt

- Travailler **uniquement** dans ton worktree et sur ta branche.
- Ne jamais toucher à `main`, ne jamais fusionner, pousser ni forcer.
- Commits petits et fréquents, **tests verts avant chaque commit**.
- Jamais de secret, de binaire, de cache ou de donnée personnelle réelle dans
  un commit.
- Messages de commit en français, à l'impératif, une ligne de sujet sous
  72 caractères puis un corps qui dit *pourquoi*.

**Paquets figés — ne pas modifier :** `internal/chaine`, `internal/session`,
`internal/annuaire`, `internal/identity`, `internal/approval`,
`internal/envelope`, `internal/toolpin`, `internal/scrub`,
`internal/guardrail`, `internal/testca`, `internal/identitytest`.
Les lire, s'en servir, ne rien y changer.

**`internal/directive` est sous test de pureté.** `purete_test.go` parse l'AST
de tous les fichiers non-test du paquet et échoue si l'un importe `os`, `net`,
`net/http`, `io`, `context`, `regexp`, `reflect`, `math/rand`, ou emploie
`time.Now`, `LoadLocation`, `Sleep`. Toute dépendance impure vit **hors** du
paquet. Ce test ne se contourne ni ne se modifie.

## 3. Sécurité de la machine

Pas de `sudo`. Pas de suppression hors du worktree. Pas d'installation globale.
Réseau limité aux registres de paquets (`proxy.golang.org`, `sum.golang.org`)
et aux domaines que ton brief autorise nommément.

## 4. Secrets

Lus depuis l'environnement, jamais affichés, même partiellement, même dans un
message d'erreur ou un log de test. Aucun secret n'est nécessaire pour les
chantiers du socle : si tu crois en avoir besoin, c'est que tu sors du brief.

## 5. Rapport

`RAPPORT_NUIT.md` à la racine de ton worktree, **mis à jour après chaque
commit** :

```markdown
# Rapport — <chantier>

## Fait
- <quoi> (commit <sha court>)

## Non fait, et pourquoi

## Décisions prises seul
- <décision> — <justification> — <ce qu'il faudrait valider>

## Blocages

## Questions
- urgent : …
- à l'occasion : …

## Vérification en 5 minutes
<les commandes à lancer pour contrôler le travail>
```

---

## 6. Contrats figés

Transcrire exactement. Toute divergence entre deux sessions sur ces
définitions rendra la fusion impossible.

### 6.1 Classes d'effet

Ordonnées, du plus faible au plus fort. L'ordre porte la monotonie : une
classe ne peut que monter.

```go
// Classe est ce qu'un effet fait au monde extérieur.
type Classe int

const (
    Lecture        Classe = iota // consultation seule
    Ecriture                     // modification réversible
    Transaction                  // débit, engagement
    Administration               // compte, identité, droits
)
```

Même échelle pour les **paliers** d'un périmètre : un palier est la classe
maximale qu'un connecteur a le droit d'atteindre.

### 6.2 Le descripteur

**Deux types d'état**, et rien d'autre tant qu'un connecteur réel n'a pas
réclamé davantage. La liaison est une **propriété** de `appel`, pas un type :
une propriété sur un état existant ne change pas la surface d'analyse, un
nouveau type la multiplie.

```yaml
version: 1
domaine: api.exemple.com
palier: transaction

# Le connecteur est paramétré, jamais spécialisé : tout ce qui varie d'une
# requête à l'autre entre par ici. Une valeur écrite en dur change
# l'empreinte de l'artefact et détruit le cache.
parametres:
  - { nom: enseigne, type: chaine, requis: true }
  - { nom: plafond,  type: entier, requis: true }

etapes:
  - id: chercher
    type: appel
    operation: searchStores        # operationId, doit exister dans la spec
    requete: { q: "{{ .enseigne }}" }
    classe: lecture
    reversible: true
    reprise: { max: 0 }            # 0 par défaut, voir 6.3
    lie:
      store_id: "$.stores[0].id"   # extraction vers le contexte

  - id: commander
    type: appel
    operation: createOrder
    corps: { cart: "{{ .cart_id }}" }
    classe: transaction
    reversible: false
    lie: {}

  - id: brancher
    type: choix
    sur: "{{ .statut }}"
    cas:                           # ensemble FERMÉ, exhaustif, obligatoire
      ouvert: commander
      ferme:  _fin
```

Règles du chargeur, strictes : tout champ hors schéma est un refus, désigné par
sa position. `operation` doit exister dans la spec. Un `choix` dont les `cas`
ne couvrent pas l'ensemble fermé est un refus. Un `id` dupliqué est un refus.

### 6.3 Reprise

```
reprise.max = 0 par défaut.
reprise.max > 0 est REFUSÉ au chargement si reversible = false.
```

Aucune boucle de convergence. Une étape qui échoue ne se rejoue pas pour
« arriver à l'état voulu » : sur une transaction, c'est une double facturation.

### 6.4 Périmètre

```go
// EClasse est la part du périmètre qui entre dans l'attestation. Elle est une
// fonction pure de la demande normalisée : rien de ce qui dépend du demandeur
// n'y figure, sinon deux utilisateurs produisent deux périmètres et le cache
// ne sert plus.
type EClasse struct {
    Domaines []string // triés, minuscules
    Palier   Classe
}

// Canoniser rend l'empreinte qui sert de clé de cache et entre dans
// l'attestation. Un périmètre qui ne se canonise pas est non évaluable.
func (e EClasse) Canoniser() (string, error)

// EInstance ne figure JAMAIS dans l'attestation : elle porte des données du
// demandeur, et elle se vérifie mécaniquement à l'exécution.
type EInstance struct {
    Plafond     int64
    Adresse     string
    Demandeur   string
    Session     string
}
```

- Échelle des paliers : `Lecture < Ecriture < Transaction < Administration`.
- Une demande est **arrondie au palier minimal qui la couvre**.
- `Administration` n'est **jamais** atteint par arrondi : il exige sa propre
  admission et une validation humaine.
- **Égalité stricte** sur `EClasse` pour un succès de cache. Jamais
  d'inclusion : raisonner sur les périmètres au moment de la réutilisation est
  exactement là où les failles se logent.

### 6.5 Verdict et décision

```go
// Verdict est ce qu'un contrôle a observé. Donnée pure : Decide la lit, ne la
// produit jamais. La distribution est la SEULE sortie du modèle ; mode,
// confiance et état se recalculent ici, pour que le rejeu ne dépende pas du
// post-traitement d'un fournisseur.
type Verdict struct {
    ControleID        string
    Distribution      []float64
    EmpreinteEtat     string // sha256 des champs effectivement passés
    EmpreinteQuestion string // sha256 du gabarit figé
    Modele            string // identifiant de version
    Erreur            string // non vide ⇒ Indetermine
}

func (v Verdict) Mode() int
func (v Verdict) Confiance() float64
func (v Verdict) Etat(c Controle) Trivalent // Vrai | Faux | Indetermine

// Decide gagne un paramètre. Le reste de la signature ne bouge pas, et
// Decide(…, nil) doit rendre exactement ce que l'actuel rend.
func Decide(jeu Jeu, etat EtatSession, effet Effet, verdicts []Verdict) Decision
```

### 6.6 Invariants

Ils valent pour tout le code, et chacun doit avoir un test qui le prouve.

1. **Monotonie.** Pour tout jeu, tout état, tout effet et tout vecteur de
   verdicts : `Decide(…, verdicts).Effet >= Decide(…, nil).Effet`. Un modèle ne
   peut que durcir.
2. **Monotonie des classes.** La classe retenue est
   `max(déclarée, plancher, verdict)`.
3. **Fermé sur l'incertitude.** Confiance en zone grise, modèle muet, entrée
   trop longue, questions contradictoires ⇒ `Indetermine` ⇒ refus par défaut.
4. **Couverture totale.** Toute combinaison de verdicts non couverte par la
   table de décision produit `Refuser`, vérifié **statiquement** au chargement.
5. **Le déterministe avant le modèle.** Tout ce qu'un `if` tranche est tranché
   sans appel de modèle.
6. **Pureté.** `Decide` ne lit ni horloge, ni réseau, ni fichier.

### 6.7 Cinq contraintes de déploiement

Elles ne coûtent rien maintenant et sont très chères à rattraper. Aucune
session n'a le droit de les enfreindre.

1. L'exécuteur n'appelle jamais un service distant qui nous appartient.
2. Les points d'accès des modèles sont de la **configuration**, jamais des
   constantes.
3. Le dépôt de descripteurs fonctionne hors ligne.
4. Le journal est local et suffisant pour rejouer.
5. La génération n'est jamais sur le chemin de l'exécution : un descripteur en
   cache s'exécute sans qu'aucun modèle soit joignable.

---

## 7. Le projet, en six phrases

Un agent LLM appelle des outils externes. Quand l'outil n'existe pas encore, on
le fabrique à la demande — mais on ne génère **pas de code** : un modèle produit
un **descripteur**, de la donnée, et un runtime écrit à la main l'interprète.

La surface vient de la spec OpenAPI du service, qui sert aussi d'étalon pour
vérifier le descripteur sans modèle. La procédure — quelles opérations, dans
quel ordre, avec quelles liaisons — est la seule chose que le modèle produit.

Chaque effet est décidé **au moment où il se produit**, par une fonction pure,
et le journal chaîné permet de rejouer la décision à l'identique. Le code
généré ne détient jamais de credential : le proxy est l'unique sortie réseau et
attache le secret au dernier moment.

Le plan complet est dans `docs/plan-construction.md`.
