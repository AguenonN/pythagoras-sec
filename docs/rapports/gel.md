# Rapport — session « gel » (samedi 03/10, après la fusion n° 4)

Branche `feat/gel`. Engine non modifié (`internal/directive`, `journal`, `rejeu`, `session`, `gate`). Console non modifiée.

## Compteur Mistral

Budget : **400 appels** (autorisé par Manu, prime sur les 40 des règles communes). Compté par les fichiers `agents/.etat/**/appels.jsonl` (une ligne par appel réel).

| Étape | Appels | Cumul |
|---|---|---|
| Phase 1, point 0 : compilations A4, A5-brut, A5a, A5b | 9 | 9 |
| Phase 1, point 9 : 7 règles ATLAS (+ 1 nouvel essai de D-040) | 23 | 32 |
| Scène vierge : compilation « fuite par courriel » sur le jeu vierge | 1 | 33 |

## Phase 1 — derniers changements de code

### Point 0 — tests rouges

- `test_conforme_au_registre` : le registre de démo compte 12 serveurs depuis l'opération (8 + `data-room`, `messagerie`, `bourse`, `communication` de `demo/serveurs-supplementaires.json`), le catalogue versionné n'en avait que 8. Catalogue régénéré par la commande existante (`go run ./cmd/registre-demo -enregistrements demo/scenarios/enregistrements/serveurs -catalogue agents/compilateur/exemples/catalogue-demo.json`), deux fois : mêmes octets (catalogue `7a3b0809…`, registre `25581e19…`). Vocabulaire régénéré : inchangé.
  - **Décision** : le catalogue versionné inclut désormais les serveurs ATLAS (le compilateur en direct doit les connaître pour la scène principale). `internal/registredemo/catalogue_test.go` (test seulement) et les listes de serveurs des tests Python (`test_catalogue_demo.py`, `test_noms_serveurs.py`) comptent les serveurs supplémentaires. Le commentaire de `scripts/nouveau-serveur.sh` (« ceux de agents/compilateur/exemples restent ceux de la démo figée ») ne vaut plus pour les serveurs versionnés dans `demo/serveurs-supplementaires.json`.
- `test_secteurs` (A4, A5) : compilations **réenregistrées en réel** sur le catalogue et le vocabulaire actuels (`mistral-medium-latest`, modèle du compilateur). Mistral pose d'autres questions qu'à l'enregistrement d'origine : les réponses des parcours (`demo/scenarios/compilation/A4.json`, `A5a.json`, `A5b.json`) ont été réécrites avec le **même contenu**, sur le texte exact des nouvelles questions. Résultats identiques aux directives installées (D-024 à D-029, D-030, D-031), à l'ordre près des `champs_autorises` du caviardage de A5b : le test compare désormais cette liste sans tenir compte de l'ordre (sans portée pour l'engine).
  - **Constat pour la scène** : avec les serveurs ATLAS au catalogue, sur le texte brut A5 (« mails … rémunération »), Mistral demande si « mails » vise `messagerie/envoyer_message` (ATLAS) ou un autre outil. Question observable, utile à montrer.

### Points 1 et 2 — modèle des agents ; demandes en direct hors des fichiers versionnés

- Orchestrateur et sous-agents : `mistral-medium-latest` par défaut (`orchestrateur.modele.MODELE_AGENTS`) ; `--modele` dans `python -m orchestrateur executer` (ou `ORCHESTRATEUR_MODELE`), dans `scripts/lancer-demo.sh demande|scenario|enregistrer`. L'enregistrement note le modèle réellement utilisé. Le compilateur garde le sien (`MODELE_COMPILATEUR`, `mistral-medium-latest`). Tests : `TestCLI.test_option_modele` et défaut vérifié dans `test_reel_puis_rejeu`.
- Les serveurs de données du registre enregistrent leurs réponses dans une copie de travail, `demo/execution/serveurs/<serveur>/` (ignorée par Git), remplie à la préparation et avant chaque demande depuis `demo/scenarios/enregistrements/serveurs/` (sans rien écraser). `scenario <nom>` en réel écrit l'enregistrement de Mistral sous `demo/execution/enregistrements/<nom>/`. **Nouvelle sous-commande `scripts/lancer-demo.sh enregistrer <nom>`** : seule à écrire les secours versionnés (`demo/scenarios/enregistrements/<nom>/<date>.json`, et les réponses de serveurs obtenues pendant le passage, recopiées vers les enregistrements versionnés). Le rejeu (`--mode rejeu`) lit toujours le dernier enregistrement versionné.
- Test : `test/extension/demande_directe_test.go` — chaque chemin `-enregistrements` du registre est ignoré par Git, et un scénario lancé en réel (sans clé Mistral : arrêt au premier appel au modèle, après le lancement du gate et des serveurs en MIXTE) laisse `git status` inchangé.

### Point 3 — `base-interne-avec-d009` en erreur

Cause : l'enregistrement du 02/10 09:19 a été fait **avant** le garde-fou d'idempotence de l'orchestrateur. Dans ce passage, après l'attribution transmise par `credit-historique` (réponse 10), l'orchestrateur a délégué au `credit` une vérification du seuil officiel (réponse 12), et le `credit` a renvoyé la même attribution (réponse 14). Avec le code actuel, cet appel n'est plus transmis au gate (« Non envoyé par l'orchestrateur : cette action … a déjà été effectuée ») : le sous-agent reçoit ce résultat et demande une 15e réponse au modèle, que l'enregistrement n'a pas → `ErreurRejeu`. Ni l'orchestrateur ni l'engine ne sont en défaut (le garde-fou fait ce qu'il doit) : l'enregistrement est périmé. Correction : scénario réenregistré sous le code actuel (phase 4).

### Point 4 — clé du journal changée

- `scripts/lancer-demo.sh demande|scenario` vérifie, avant de lancer le gate, chaque `demo/execution/sessions/*/journal.jsonl` non vide avec `bin/journal verifier` et la clé courante ; au premier échec : « journaux signés avec une autre clé : scripts/lancer-demo.sh archiver-sessions » (avec le nombre et le premier journal en cause), code 1, gate non lancé.
- `scripts/lancer-demo.sh archiver-sessions` : déplace les **seules** sessions dont le journal ne se vérifie pas dans `demo/execution/sessions-archivees/<AAAAMMJJ-HHMMSS>/` ; rien n'est supprimé ; « rien à archiver » sinon.
- Test : `test/extension/cle_journal_test.go` (journal signé avec une autre clé → message clair, pas de gate ; archivage ; second archivage vide).
- **Console (non modifiée, point 5)** : elle affiche « chaîne rompue à l'entrée n° 1 » pour un journal signé avec une autre clé (`internal/console/web/app.js`, `alerteChaines`). Modification à faire par la session console : quand la première entrée ne se vérifie pas (`premier_ecart == 0`), afficher « journal signé avec une autre clé (ou clé différente de celle de la console) : `scripts/lancer-demo.sh archiver-sessions` ».

### Point 5 — propositions de démo de l'Analyste

`scripts/lancer-demo.sh` (préparation) recopie `demo/scenarios/propositions/*.json` dans `demo/execution/propositions/` quand ce dossier est vide ou absent (jamais par-dessus des propositions présentes). Contenu versionné régénéré en phase 5.

### Point 6 — `ops@cabinet.fr`

Remplacée par `ops@exemple.test` : auteur du registre de démo (`internal/registredemo.Meta`) et du lab (`deploy/lab/registre-lab`), tests du registre et du gate (`internal/registre`, `test/gate`). Restent (non modifiés) : les journaux signés des données de test de l'Analyste (`agents/analyste/tests/donnees/sessions*/`), qu'on ne peut pas réécrire sans casser leur chaîne (voir phase 5) ; et d'autres adresses du même domaine `cabinet.fr` (`juriste@`, `alice@`, `bob@`, `dpo@`, `me.dupont@`), hors de la demande du brief, dont deux fichiers antérieurs au tag `baseline-pre-hackathon` (intouchables) et des parcours du compilateur enregistrés (l'auteur entre dans la requête à Mistral). **Question pour Manu** : faut-il les remplacer aussi (au prix de réenregistrer ces parcours) ?

### Point 7 — consignes des agents crédit

Gardées (décision de Manu). Documenté dans `docs/demo/secteurs.md` (« Une exception, voulue ») ; le test des consignes neutres porte désormais sur **tous** les sous-agents sauf `credit`, `credit-historique`, `conformite`, exception écrite et justifiée dans le test. Pour `assistant-salarie` et `equipe-deal`, « « interne » ou « externe » » (valeurs de l'argument `/destination`) n'est pas une règle et n'est pas compté.

### Point 8 — documentation

- `docs/equipe/modifier-la-console.md` : options `-conseiller`, `-compilateur reel|rejeu`, `-mode-compilateur`, route `GET /api/outils` (libellés), règle « le sondage ne détruit pas les formulaires », tests du navigateur (Playwright) et leur commande.
- `docs/demo/operation.md` : section « Commandes exactes pour dimanche » (clé, préparation, console avec `-conseiller`, les trois scènes sans / avec les règles, en direct et en secours `--mode rejeu`, depuis la console) ; section sur le passage des règles par le compilateur.

### Point 9 — jeu ATLAS passé par le compilateur (23 appels)

Les 7 textes de `operation.json` passés par le service du compilateur en réel (`--jeu-existant demo/directives/demo.json`), réponses de la juriste fidèles au jeu vérifié. Détail : `docs/rapports/gel/compilation-atlas.json`. **Le jeu vérifié est gardé.**

| Règle | Résultat |
|---|---|
| D-041, D-042, D-045 | identiques (D-041/D-042 : termes du « et » dans un autre ordre, équivalent) |
| D-044 | condition de date perdue après la question du fuseau |
| D-040 | inversée deux fois (« refusé si membre des initiés ») — le compilateur ne sait pas écrire « n'appartient à aucun des groupes » ; la relecture le montre |
| D-043 | refusée par les vérifications du code (`session.a_consulte` mal formé) |
| D-046 | impossible : une marque nouvelle n'est pas une étiquette du registre ; le compilateur la refuse |

**Décision (réversible)** : D-040 à D-046 ajoutées à `demo/directives/demo.json` **version 5, en brouillon** (version 4 archivée dans `historique/demo.v4.json`, origines dans `origines.json`, `directive verifier` : ok). Sans cela, la console — qui lance toute demande sous `demo.json` — ne peut pas jouer la scène ATLAS « avec les règles ». Rien n'est activé : Manu décide en phase 2. Les scénarios « sans » (`op-scene-*-sans`) pointent désormais vers `demo/scenarios/jeux/operation-sans-regles.json` (copie figée de la version 4), pour rester « sans règle » quelle que soit l'activation. Tests ajustés : registre de démo avec les serveurs supplémentaires (`directives_test.go`), origines D-040 à D-046, étiquette d'une marque non comptée parmi les étiquettes d'outils (`test_vocabulaire_demo.py`), relectures attendues de la scène (version 6 au lieu de 5).

### Autres écarts (point 10)

- Fait : rôle du valideur (champ `role` de `rejet.json`, ajouté par console-3) repris dans le texte de l'orchestrateur « Validation rejetée par Claire Analyste (analyste-risques) : … » ; les tests qui vérifient le refus d'options retirées (`--v1`, `--v2`, `--id-directive`) n'écrivent plus l'erreur d'argparse dans la sortie.
- Non fait, hors code ou hors périmètre : onglet « Demandé / décidé » à vérifier visuellement (console) ; `-conseiller` par défaut (le lanceur ne lance pas la console : la commande documentée le passe) ; périmètre de l'orchestrateur écrit en dur (question de fond, pas de changement avant dimanche) ; grain « document » des étiquettes (engine).
- Compilations `demo/scenarios/compilation/B1` à `B6` et `R1` à `R6` (directives des secteurs) : ne se rejouent pas sur le catalogue actuel ; elles ne servent à aucun test ni à la scène (les directives D-011 à D-023 sont installées). Réenregistrées en fin de session si le budget le permet (voir phase 4).

## Phase 2 — directives en brouillon de `demo/directives/demo.json` (version 5)

Actives : D-001 à D-008. Libres : D-009, D-010. En brouillon (28) :

| Id | Texte | Effet | Fondement |
|---|---|---|---|
| D-011 | Aucun virement n'est exécuté si le registre national des gels des avoirs n'a pas été consulté, avec une réponse reçue, dans la même demande. | refus | Code monétaire et financier — à valider par le juriste (art. L562-4 et suivants); règlements de l'Union européenne relatifs aux mesures restrictives |
| D-012 | Lorsqu'une déclaration de soupçon a été transmise dans la demande, aucun courrier ni message n'est envoyé au client concerné. | refus | Code monétaire et financier — à valider par le juriste (art. L561-18) |
| D-013 | Toute modification qui porte le plafond de découvert d'un compte au-delà de 10 000 euros attend la validation d'un analyste des risques. | attente : analyste-risques | Politique de risque interne — à valider par le juriste |
| D-014 | Aucun export de données client n'est fait vers une destination externe à la banque. | refus | RGPD — à valider par le juriste (art. 5); Code monétaire et financier (art. L511-33) |
| D-015 | Vers une destination interne, l'export ne comporte ni les IBAN ni les revenus du client. | caviardage | RGPD — à valider par le juriste (art. 5); Code monétaire et financier (art. L511-33) |
| D-016 | Toute clôture de compte est validée par une seconde personne, distincte de celle qui la demande. | attente : None, quatre yeux | Contrôle interne — à valider par le juriste |
| D-017 | Un refus de crédit n'est jamais notifié au demandeur sans la validation d'une personne. | attente : None | RGPD — à valider par le juriste (art. 22) |
| D-018 | Aucune recherche de salariés ne porte sur un critère protégé : âge, état de santé, grossesse, nationalité ou origine, appartenance syndicale. | refus | Code du travail — à valider par le juriste (art. L1132-1) |
| D-019 | Lors de la consultation d'une fiche salarié, les données de santé et l'appartenance syndicale sont retirées. | caviardage | RGPD — à valider par le juriste (art. 9) |
| D-020 | Aucune sanction disciplinaire n'est notifiée à un salarié par l'outil sanctionner du serveur rh-synthetique si la convocation à un entretien préalable (outil convoquer_entretien_prealable du même serveur) n'a pas été transmise dans la même demande. | refus | Code du travail — à valider par le juriste (art. L1332-2) |
| D-021 | Tout licenciement attend la validation d'un membre de la direction des ressources humaines (rôle drh), distinct de la personne qui le demande. | attente : drh | Code du travail (art. L1232-2); contrôle interne — à valider par le juriste |
| D-022 | Aucun export de la paie n'est fait vers un pays situé hors de l'Union européenne. | refus | RGPD — à valider par le juriste (art. chapitre V (transferts hors de l'Union)) |
| D-023 | Dans une liste de salariés, les salaires sont retirés. | caviardage | RGPD — à valider par le juriste (art. 5.1.c) |
| D-024 | Aucun agent ne lit ni ne modifie les comptes, prêts, découverts ou virements d'un client personne politiquement exposée : le dossier relève d'un conseiller humain. | refus + notifier conformite | Politique interne de la banque — à valider par le responsable juridique |
| D-025 | Aucun agent ne lit ni ne modifie les comptes, prêts, découverts ou virements d'un client personne politiquement exposée : le dossier relève d'un conseiller humain. | refus + notifier conformite | Politique interne de la banque — à valider par le responsable juridique |
| D-026 | Aucun agent ne lit ni ne modifie les comptes, prêts, découverts ou virements d'un client personne politiquement exposée : le dossier relève d'un conseiller humain. | refus + notifier conformite | Politique interne de la banque — à valider par le responsable juridique |
| D-027 | Aucun agent ne lit ni ne modifie les comptes, prêts, découverts ou virements d'un client personne politiquement exposée : le dossier relève d'un conseiller humain. | refus + notifier conformite | Politique interne de la banque — à valider par le responsable juridique |
| D-028 | Aucun agent ne lit ni ne modifie les comptes, prêts, découverts ou virements d'un client personne politiquement exposée : le dossier relève d'un conseiller humain. | refus + notifier conformite | Politique interne de la banque — à valider par le responsable juridique |
| D-029 | Aucun agent ne lit ni ne modifie les comptes, prêts, découverts ou virements d'un client personne politiquement exposée : le dossier relève d'un conseiller humain. | refus + notifier conformite | Politique interne de la banque — à valider par le responsable juridique |
| D-030 | Aucun agent n'envoie directement à un salarié un message rédigé dans une session où des données de rémunération ont été consultées ; le message est relu et validé par un gestionnaire RH. | attente : gestionnaire-rh | Règlement intérieur, confidentialité des rémunérations — à valider par le juriste; RGPD (art. 5) |
| D-031 | Les salaires et primes sont retirés des fiches salariés consultées par un agent, sauf quand le demandeur appartient au service paie. | caviardage | Règlement intérieur, confidentialité des rémunérations — à valider par le juriste; RGPD (art. 5) |
| D-040 | Accès à l'information privilégiée : seul un agent agissant pour une personne inscrite sur la liste d'initiés du projet ATLAS, ou membre de l'équipe ATLAS du cabinet conseil, peut lire un document du projet ATLAS dans la data room. Toute autre lecture est refusée et la conformité est notifiée. | refus + notifier conformite | Règlement (UE) n° 596/2014 relatif aux abus de marché — information privilégiée et listes d'initiés — à valider par le juriste (art. 7, 18) |
| D-041 | Divulgation illicite par message : aucun agent n'envoie un message vers une destination externe au cours d'une demande où un document du projet ATLAS a été consulté. L'envoi est refusé et la conformité est notifiée. | refus + notifier conformite | Règlement (UE) n° 596/2014 relatif aux abus de marché — divulgation illicite d'information privilégiée — à valider par le juriste ; secret professionnel de l'avocat (art. 10, 14) |
| D-042 | Divulgation illicite par partage : aucun agent ne partage un document vers une destination externe au cours d'une demande où un document du projet ATLAS a été consulté. Le partage est refusé et la conformité est notifiée. | refus + notifier conformite | Règlement (UE) n° 596/2014 relatif aux abus de marché — divulgation illicite d'information privilégiée — à valider par le juriste ; secret professionnel de l'avocat (art. 10, 14) |
| D-043 | Opérations d'initiés sur titres : aucun agent ne passe d'ordre sur les titres VENTOUX ou BIOLYS avant le 15 octobre 2026, au cours d'une demande où un document du projet ATLAS a été consulté, ou pour le compte d'une personne inscrite sur la liste d'initiés ATLAS. L'ordre est refusé et la conformité est notifiée. | refus + notifier conformite | Règlement (UE) n° 596/2014 relatif aux abus de marché — opérations d'initiés et période d'abstention — à valider par le juriste (art. 8, 14, 19) |
| D-044 | Communication avant l'annonce : la publication d'un communiqué avant le 15 octobre 2026 attend la validation de la directrice juridique, par une personne distincte de l'auteur de la demande. | attente : direction-juridique, quatre yeux | Règlement (UE) n° 596/2014 relatif aux abus de marché — communication de l'information privilégiée au public — à valider par le juriste ; contrôle interne (art. 17) |
| D-045 | Communication avant l'annonce : la publication d'un communiqué avant le 15 octobre 2026 attend aussi la validation de la conformité, distincte de la directrice juridique. | attente : conformite, quatre yeux | Règlement (UE) n° 596/2014 relatif aux abus de marché — communication de l'information privilégiée au public — à valider par le juriste ; contrôle interne (art. 17) |
| D-046 | Traçabilité de l'information privilégiée : toute lecture d'un document du projet ATLAS est marquée au journal comme accès à une information privilégiée (registre d'accès à l'information privilégiée). | marque « acces-information-privilegiee » | Règlement (UE) n° 596/2014 relatif aux abus de marché — listes d'initiés et traçabilité des accès — à valider par le juriste (art. 18) |

**Fait** : Manu a activé D-011 à D-031 et D-040 à D-046 (voir plus bas, « Réponses de Manu », point 4).

## Réponses de Manu (03/10, soir) — traitées avant la phase 3

1. **Adresses `@cabinet.fr`** → `@exemple.test` partout **hors engine** (agents, tests, données du lab et d'intégration, relecture : commit `43d63ca`). **Non remplacées, signalées** : les fichiers de test et données des paquets de l'engine (`internal/directive`, `internal/gate`, `internal/journal`, `internal/rejeu` : brief « l'engine n'est pas modifié ») et deux fichiers antérieurs au tag `baseline-pre-hackathon` (`internal/approval/approval_test.go`, `pyagent/agent_juriste.py` : règles communes) ; les attendus de `internal/relecture` dérivés des jeux de `internal/directive/testdata` gardent donc l'ancienne adresse. Journaux signés des données de test de l'Analyste : phase 5. Aucun parcours du compilateur cassé par le remplacement (l'auteur n'entre pas dans les requêtes à Mistral).
2. Encadré « Compilation en direct » dans `docs/demo/operation.md` (fuite par courriel seule ; inversion de D-040 et sa relecture, réponse type au jury).
3. **`scripts/lancer-demo.sh scene-vierge <dossier> [--jeu F]`** : dossier autonome hors du dépôt (copie de HEAD, jeu remplacé, historique et origines vides, `demo/execution/` neuf, registre local des gels recopié, préparation complète). **Signalé** : le gate et `directive verifier` refusent un jeu sans directive ; le jeu « vierge » porte une directive de traçabilité sans blocage, **T-001** (préfixe distinct : la première règle compilée est bien D-001, vérifié). Jeu « ventoux » : `demo/scenarios/jeux/ventoux.json` (D-043, D-044, D-045). Deux consoles sur deux ports : oui (`-addr`), vérifié. Option `DEMO_GELS_HORS_LIGNE=1` de la préparation : pas de téléchargement du registre des gels s'il en existe un local (tests hors ligne). Test de bout en bout en rejeu : `test/extension/scene_vierge_test.go` (vierge → courriel autorisé → règle compilée par le **vrai** service sur réponses enregistrées → D-001 → activée → refusée), vert. 1 appel Mistral (second tour de la compilation sur le jeu vierge).
4. Phase 2 : Manu a activé D-011 à D-031 et D-040 à D-046 ; `demo.json` **version 33**, 36 directives actives, D-009 et D-010 libres, `directive verifier` ok, historique v5 à v32 ; commit `cf4713c`. Tests qui figeaient l'état « brouillon » ajustés (statut et date d'effet seuls changés) ; tests de l'Analyste figés sur `historique/demo.v5.json` (leurs données datent de cet état).

## Phase 3 — données du jour (03/10, 19:58)

- `bin/gels-avoirs preparer` : registre national des gels des avoirs **publié le 02/10/2026 à 10:36:17 (heure de Paris)**, 6 609 entrées (4 646 personnes physiques, 1 924 personnes morales, 39 navires), sha256 `7371004f…`. Pas de publication plus récente au 03/10. Registre local : `servers/gels-avoirs/enregistrements_locaux/` (hors Git), recopié par `scene-vierge`.
- Taux d'usure au **04/10/2026**, découverts aux sociétés non financières : **19,08 %**, trimestre 2026-T4 (01/10 au 31/12), publication Banque de France du 28/09/2026 10:01 UTC. Interrogé **en direct** (Webstat) le 03/10 : même seuil, même empreinte de la source (`3efa7ee4…`) que l'enregistrement versionné du 29/09. **Conforme, rien à signaler.**
- Remarque : l'outil d'essai manuel `go run ./servers/appeler` en `DONNEES_MODE=DIRECT` réécrit l'enregistrement versionné du serveur (date d'obtention) ; restauré (`git checkout`). Le lanceur, lui, n'écrit plus dans les enregistrements versionnés (point 2).

## Phase 4 — réenregistrement des secours

Méthode : `scripts/lancer-demo.sh enregistrer <nom>` (réel, `mistral-medium-latest`, données en MIXTE, horloge réelle, jeu du scénario), validations déposées par un pilote au moment de l'attente, selon le fichier du scénario (jetons signés par `lancer-demo.sh valider`, ou `rejet.json` au format de la console pour un rejet) ; puis `verifier` (chaîne HMAC + rejeu des décisions), rejeu hors ligne (`--mode rejeu`) et `comparer` (journal réel / journal rejoué). Un secours n'est retenu que si les trois sont bons ; l'ancien enregistrement est alors supprimé (l'historique Git le garde).

**Journal historique de l'incident du 1er octobre** : il n'est versionné nulle part dans ce worktree (aucun `journal.jsonl` suivi, hors données de test de l'Analyste) ; il est sans doute dans `demo/execution/sessions/` d'un autre checkout, hors de mon périmètre : **non vérifié** (`cmd/journal verifier`), à faire par Manu sur la machine qui l'a. L'enregistrement de l'orchestrateur de ce passage (`archives/nominal-20261001-213129-gels-indisponible.json`), rejoué sous le code actuel : **il ne se rejoue pas à l'identique** — en rejeu, le registre des gels répond (il était indisponible le 01/10), D-001 n'a donc rien à refuser, et l'attribution s'arrête en attente de D-002 (« bloque »). L'incident reste une pièce historique, pas un secours rejouable.

### Routage des trois scènes ATLAS jouées en direct (demande de Manu, 03/10 soir)

5 passages réels par scène (`scripts/lancer-demo.sh scenario <scène sans règles>`, en direct, rien de versionné), sous-agent obtenu :

| Scène | Demandeur | Attendu | Obtenu |
|---|---|---|---|
| Fuite par courriel (`op-scene-fuite-sans`) | chloe.arceau | equipe-deal | equipe-deal **5/5** |
| Mauvais collaborateur (`op-scene-collab-sans`), demande d'origine | damien.arceau | equipe-deal | assistant-salarie **5/5** (0/5 bon) |
| Mauvais collaborateur, demande reformulée « En tant que collaborateur du cabinet Arceau Avocats, … » | damien.arceau | equipe-deal | equipe-deal **5/5** |
| Délit d'initié (`op-scene-initie-sans`) | victor.ventoux | assistant-salarie | assistant-salarie **5/5** |

La décision de l'engine ne dépend pas du routage (D-040 porte sur le demandeur) ; la demande a tout de même été reformulée (jamais la consigne de l'orchestrateur), dans `op-scene-collab-sans` **et** `op-acces-piege` (même demande sans / avec règles), donc aussi dans la demande suggérée par la console ; les deux secours ont été réenregistrés (anciens dans `archives/…-ancienne-demande-mal-routee.json`). `docs/demo/operation.md` donne les trois demandes exactes et ce taux. Coût : 20 passages, 116 appels.

### Bilan provisoire de la phase 4 et budget

**387 appels sur 400** à ce stade : le modèle des agents (`mistral-medium-latest`) fait environ deux fois plus d'appels par passage que l'estimation, et la mesure du routage en a pris 116. Faits : les 15 scénarios ATLAS, les 11 scénarios de base. **Reste 13 appels** : je les réserve aux textes du compilateur de la scène (week-end, base interne, texte brut de la rémunération). **Non réenregistrés faute de budget** : les scénarios des secteurs (`secteur-*`, une quarantaine, dont les personnes politiquement exposées et les trois versions de l'e-mail de rémunération) ; leurs enregistrements du 02-03/10 sont **gardés** (pas supprimés) et leur rejeu sous le code actuel est vérifié en phase 6.
