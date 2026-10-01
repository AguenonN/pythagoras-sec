"""Les trois outils offerts à Mistral pour compiler un jeu v2, et la consigne.

Aucun schéma n'est récursif : la condition est une liste de nœuds à plat,
chaque nœud citant ses enfants par identifiant (voir arbre.py). Tout nom
fermé est borné par une énumération tirée du vocabulaire : types de nœuds,
opérateurs, effets, serveurs, outils, arguments, étiquettes, rôles, groupes,
fuseaux, jours.

Ne figurent pas dans les outils, parce que ce n'est jamais à Mistral de les
choisir : l'identifiant, la version, le statut (toujours ``brouillon``),
``texte_fr`` (le texte du juriste, ou un extrait recopié mot pour mot),
la date d'effet, ``si_indetermine``, et le rythme et la conduite en cas
d'échec d'une notification (questions fermées posées par le code).
"""

from .arbre import TYPES
from .vocabulaire import JOURS

OPERATEURS = [">", ">=", "<", "<=", "==", "!=", "dans", "prefixe", "present", "absent"]
OPERATEURS_DATE = [">", ">=", "<", "<=", "==", "!="]
EFFETS = ["refuser", "attendre_validation", "caviarder", "notifier", "marquer"]
MODES_ATTESTATION = ["spiffe", "tls-pin", "binaire", "aucune"]
SUJETS_QUESTION = [
    "seuil", "fuseau", "autonome", "demandeur_ou_personne_concernee", "etiquette",
    "consultation_ou_production", "role", "groupe", "cible", "argument", "perimetre",
    "fondement", "revision_avant", "effet", "autre",
]

CONSIGNE = """Tu es le compilateur de directives d'une équipe juridique. Tu ne décides rien : tu traduis fidèlement le texte du juriste en un JEU de directives structurées, ou tu poses des questions, ou tu signales un critère non observable. Le code vérifiera ta proposition, puis le juriste la relira et la validera.

Règles absolues :
1. N'utilise que des valeurs écrites par le juriste (dans son texte ou ses réponses) ou des noms du catalogue. N'invente jamais un montant, une date, une heure, un fuseau, un serveur, un outil, un argument, une étiquette, un rôle, un groupe ni un fondement. Une étiquette, un rôle ou un groupe n'est utilisable que si le juriste l'a écrit ou choisi dans une réponse : sinon, pose une question en proposant les noms connus qui pourraient convenir, sans en choisir un.
2. Un paragraphe peut contenir plusieurs règles : propose alors plusieurs directives dans le même jeu, une par règle, dans l'ordre du texte. Pour chacune, recopie dans « extrait » le passage exact du texte qu'elle traduit, mot pour mot.
3. Pose une question (poser_question, une par point, plusieurs à la fois si besoin) au lieu de proposer, notamment :
   - seuil non chiffré (« montant significatif », « gros montant ») : demande le montant exact (sujet seuil) ;
   - règle horaire ou calendaire sans fuseau (« le week-end », « la nuit », « après 20 h ») : demande le fuseau (sujet fuseau) en proposant des fuseaux admis ;
   - « de manière autonome », « sans contrôle », « automatiquement » : demande si l'action exige une validation humaine (attendre_validation) ou est interdite (refuser) (sujet autonome) ;
   - « l'utilisateur », « user », « la personne » : demande s'il s'agit du demandeur (la personne pour qui l'agent travaille, feuille demandeur) ou de la personne concernée par l'action (feuille sujet) (sujet demandeur_ou_personne_concernee) ;
   - « rapport », « données client » ou toute catégorie de serveurs ou d'outils : demande à quelle étiquette du registre la rattacher (sujet etiquette), en proposant les étiquettes connues ;
   - « aucune exécution », « bloquer », « interdire » sans préciser : demande s'il faut bloquer aussi la consultation (outils de lecture) ou seulement la production et les actions (outils classés action) (sujet consultation_ou_production) ;
   - un rôle ou un groupe désigné par une lettre ou une formule (« groupe X », « une telle personne ») : demande lequel parmi les noms connus (sujet role ou groupe) ;
   - fondement juridique absent (sujet fondement) ; durée provisoire sans date (« en attendant », « pour l'instant ») : demande la date de relecture (sujet revision_avant).
4. Signale un critère non observable (signaler_non_observable) quand la règle repose sur une notion qu'aucune feuille ne peut constater dans un appel : une intention, une qualité morale, un effet (« ne pas discriminer », « de bonne foi »), ou une implication de personnes sans critère (« impliquant le top management », « décision importante »). Explique pourquoi ce n'est pas observable, et demande par quel critère observable (montant, outil, groupe, étiquette…) le remplacer. Ne propose rien pour cette règle tant que le juriste n'a pas donné ce critère.
5. Ne propose un jeu que lorsque chaque valeur est déterminée par le texte, les réponses et le catalogue. Ne retire aucune condition du texte et n'en ajoute aucune : chaque « si », « sans », « à partir de », « au-delà de », « le week-end »… devient un nœud de la condition.
6. Ne pose pas de question sur ce que le texte fixe déjà. « attend une validation humaine » sans autre précision : attendre_validation sans par_role ni quatre_yeux. par_role et destinataire_role seulement pour un rôle que le juriste a nommé ou choisi.

Une directive (proposer_jeu) :
- declencheur : la cible de l'appel d'outil. serveur et outil du catalogue ou « * » ; etiquette (étiquette du registre portée par le serveur cible) ; acces « action » pour ne viser que les outils classés action, « read » pour la lecture, seulement si le juriste distingue lecture et actions et si cela écarte des outils de la cible (jamais sur un outil nommé). Les clés présentes se combinent en ET.
- condition : une liste de nœuds à plat (noeuds), chacun avec un id unique ; racine est l'id du nœud racine, qui doit figurer dans noeuds ; sans condition, racine vaut "" et noeuds vaut []. Un nœud et, ou cite ses enfants dans enfants ; non, existe, tous citent exactement un enfant. Chaque nœud n'a qu'un parent. Au plus 6 niveaux et 64 nœuds.
  Types de nœuds :
  - et, ou, non ;
  - existe / tous : champ = pointeur vers une liste en argument (par exemple /salarie_ids) ; l'enfant est évalué sur chaque élément, et dans l'enfant champ vaut "" pour désigner l'élément lui-même ;
  - argument : champ (pointeur JSON), op, valeur ;
  - a_consulte : la session a consulté un serveur (serveur), un outil (outil « serveur/outil ») ou un serveur portant une étiquette (etiquette) ; critere donnees_recues (données reçues, le choix par défaut pour « consulter », « consultation ») ou transmis (appel parti, quel que soit le résultat, seulement si le juriste le dit). « sans consultation de X » s'écrit non(a_consulte X). Toute règle qui vise l'usage des informations d'une source (« à partir de », « sur la base de », « en utilisant les informations de » A) porte la condition a_consulte A ;
  - demandeur : la personne pour qui l'agent travaille, selon l'annuaire : groupe, ou identites ;
  - sujet : la personne concernée par l'action, désignée par l'identifiant au pointeur champ ; groupe ;
  - temps : jours, heure_de et heure_a (HH:MM, intervalle [de, a[), date_op et date_valeur (AAAA-MM-JJ) ; fuseau obligatoire ;
  - etiquette_cible : le serveur cible porte l'étiquette ;
  - attestation : verifiee (booléen), mode ;
  - metadonnee_reponse : sur une réponse reçue d'un serveur (serveur) ou d'un serveur étiqueté (etiquette), meta source.origine ou source.date, op, valeur ;
  - appelant (egal ou prefixe), sous_agent_declare (egal ou dans).
- effets : une liste. refuser ; attendre_validation (par_role, quatre_yeux vrai seulement si le juriste demande deux personnes ou « quatre yeux ») ; caviarder (champs_autorises, mode caviarder ou refuser ; cible avec un serveur nommé) ; notifier (destinataire_role) ; marquer (etiquette). refuser ne se combine ni avec attendre_validation ni avec caviarder.
- fondements : le premier est celui que citent les motifs ; texte seul (RGPD, Code civil) et article seul (22), recopié chaque fois que le juriste l'écrit.
Une condition sur un argument doit porter sur un argument de chaque outil visé : sinon, restreins la cible ou fais une directive par outil.
"""


def _enum(valeurs, description=None, vide=False):
    s = {"type": "string"}
    if valeurs:
        s["enum"] = list(valeurs) + ([""] if vide else [])
    if description:
        s["description"] = description
    return s


def _liste(items, description=None):
    s = {"type": "array", "items": items}
    if description:
        s["description"] = description
    return s


def outils(vocab):
    """Les trois outils, bornés par le vocabulaire fourni."""
    cat = vocab.catalogue
    arguments = cat.arguments()
    serveurs = cat.noms_serveurs()
    noeud = {
        "type": "object",
        "properties": {
            "id": {"type": "string", "description": "Identifiant unique du nœud, tel que n1."},
            "type": _enum(TYPES),
            "enfants": _liste({"type": "string"}, "Identifiants des enfants (et, ou : au moins un ; non, existe, tous : exactement un)."),
            "champ": _enum(arguments, "Pointeur JSON d'un argument ; \"\" pour l'élément courant sous existe ou tous.", vide=True),
            "op": _enum(OPERATEURS),
            "valeur": {"description": "Nombre, chaîne, booléen, ou liste pour dans ; absente pour present et absent."},
            "serveur": _enum(serveurs),
            "outil": _enum(cat.outils_qualifies(), "serveur/outil"),
            "etiquette": _enum(vocab.etiquettes),
            "critere": _enum(["donnees_recues", "transmis"]),
            "groupe": _enum(vocab.groupes),
            "identites": _liste({"type": "string"}),
            "egal": {"type": "string"},
            "prefixe": {"type": "string"},
            "dans": _liste({"type": "string"}),
            "verifiee": {"type": "boolean"},
            "mode": _enum(MODES_ATTESTATION),
            "jours": _liste(_enum(JOURS)),
            "heure_de": {"type": "string", "description": "HH:MM"},
            "heure_a": {"type": "string", "description": "HH:MM"},
            "date_op": _enum(OPERATEURS_DATE),
            "date_valeur": {"type": "string", "description": "AAAA-MM-JJ"},
            "fuseau": _enum(vocab.fuseaux),
            "meta": _enum(["source.origine", "source.date"]),
        },
        "required": ["id", "type"],
    }
    effet = {
        "type": "object",
        "properties": {
            "effet": _enum(EFFETS),
            "par_role": _enum(vocab.roles),
            "quatre_yeux": {"type": "boolean"},
            "champs_autorises": _liste(_enum(arguments)),
            "mode": _enum(["caviarder", "refuser"]),
            "destinataire_role": _enum(vocab.roles),
            "etiquette": _enum(vocab.etiquettes),
        },
        "required": ["effet"],
    }
    directive = {
        "type": "object",
        "properties": {
            "extrait": {"type": "string", "description": "Le passage du texte du juriste que traduit cette directive, recopié mot pour mot."},
            "fondements": _liste({
                "type": "object",
                "properties": {
                    "texte": {"type": "string", "description": "Le nom du texte seul, tel que RGPD ou Code civil."},
                    "article": {"type": "string", "description": "Le numéro d'article seul, tel que 22."},
                },
                "required": ["texte"],
            }),
            "revision_avant": {"type": "string", "description": "AAAA-MM-JJ, écrite par le juriste."},
            "declencheur": {
                "type": "object",
                "properties": {
                    "serveur": _enum(serveurs + ["*"]),
                    "outil": _enum(cat.noms_outils() + ["*"]),
                    "etiquette": _enum(vocab.etiquettes),
                    "acces": _enum(["action", "read"]),
                },
            },
            "racine": {"type": "string", "description": "Identifiant du nœud racine ; vide s'il n'y a pas de condition."},
            "noeuds": _liste(noeud, "La condition, à plat."),
            "effets": _liste(effet),
        },
        "required": ["extrait", "fondements", "declencheur", "effets"],
    }
    return [
        {
            "type": "function",
            "function": {
                "name": "proposer_jeu",
                "description": "Propose le jeu compilé, une directive par règle du texte, quand le texte, les réponses et le catalogue déterminent chaque valeur.",
                "parameters": {
                    "type": "object",
                    "properties": {"directives": _liste(directive)},
                    "required": ["directives"],
                },
            },
        },
        {
            "type": "function",
            "function": {
                "name": "poser_question",
                "description": "Pose une question au juriste quand une information manque ou est ambiguë. Une question par point.",
                "parameters": {
                    "type": "object",
                    "properties": {
                        "question": {"type": "string", "description": "La question, en français, adressée au juriste."},
                        "sujet": _enum(SUJETS_QUESTION),
                        "extrait": {"type": "string", "description": "Les mots du juriste qui posent question, recopiés."},
                        "options": _liste({"type": "string"}, "Réponses possibles, s'il y en a un nombre fermé."),
                    },
                    "required": ["question", "sujet"],
                },
            },
        },
        {
            "type": "function",
            "function": {
                "name": "signaler_non_observable",
                "description": "Refuse de traduire une règle qui repose sur un critère non observable dans un appel d'outil, en expliquant pourquoi, et demande un critère observable.",
                "parameters": {
                    "type": "object",
                    "properties": {
                        "extrait": {"type": "string", "description": "Les mots du juriste qui portent le critère, recopiés."},
                        "raison": {"type": "string", "description": "Pourquoi aucune feuille ne peut constater ce critère dans un appel."},
                        "question": {"type": "string", "description": "La question au juriste : par quel critère observable le remplacer ?"},
                    },
                    "required": ["extrait", "raison", "question"],
                },
            },
        },
    ]
