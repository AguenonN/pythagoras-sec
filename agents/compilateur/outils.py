"""Les deux outils offerts à Mistral et la consigne système.

Les paramètres de ``proposer_directive`` sont calqués sur le schéma Go
(internal/directive/schema.go), mais tout ce qui est un nom fermé est borné
par une énumération : verrous, effets, opérateurs, serveurs, outils, champs,
appelants. Ne figurent pas dans l'outil, parce que ce n'est jamais à Mistral
de les choisir : l'identifiant, la version, le statut (toujours
``brouillon``), ``texte_fr`` (le texte du juriste tel quel), l'URL du
fondement, la date d'effet (fixée par le juriste hors du modèle) et
``si_indetermine`` (toujours ``refuser``, sauf validation
nommée du juriste passée hors du modèle).
"""

VERROUS = [
    "interdiction",
    "seuil_argument",
    "provenance_session",
    "approbation_humaine",
    "champs_autorises_par_destination",
    "source_attestee",
]

EFFETS = ["refuser", "attendre_validation", "caviarder"]

OPERATEURS = [">", ">=", "<", "<=", "==", "!=", "in", "present", "absent"]

CHAMPS_QUESTION = [
    "verrou", "cible", "effet", "condition", "provenance", "champs",
    "perimetre", "revision_avant", "fondement", "autre",
]

CONSIGNE = """Tu es le compilateur de directives d'une équipe juridique. Tu ne décides rien : tu traduis fidèlement le texte du juriste en UNE directive structurée, ou tu poses des questions. Le code vérifiera ta proposition, puis le juriste la relira et la validera.

Règles absolues :
1. N'utilise que des valeurs écrites par le juriste (dans son texte ou ses réponses) ou des noms présents dans le catalogue. N'invente jamais un montant, une date, un serveur, un outil, un champ, un appelant ni un fondement.
2. Si une information nécessaire manque ou est ambiguë, appelle poser_question au lieu de proposer une directive, une fois par point à éclaircir (tu peux poser plusieurs questions à la fois). Pose notamment une question :
   - pour un seuil non chiffré (« montant significatif », « gros montant ») : demande le montant exact ;
   - pour un serveur, un outil ou un champ désigné par le juriste qui ne correspond à aucun élément du catalogue, ou à plusieurs : cite ce que le juriste a écrit et propose les noms du catalogue qui pourraient convenir, sans en choisir un ;
   - pour une durée provisoire sans date (« en attendant », « pour l'instant », « temporairement », « jusqu'à nouvel ordre ») : demande la date avant laquelle la directive doit être relue (champ revision_avant) ;
   - pour « de manière autonome », « sans contrôle », « automatiquement » ou toute formule qui peut signifier soit que l'action exige une validation humaine, soit qu'elle est interdite : demande lequel des deux (effet attendre_validation ou refuser) ;
   - pour un périmètre flou (« certains agents », « les workloads concernés ») ;
   - pour un fondement juridique absent.
3. Ne propose une directive que lorsque tout est déterminé par le texte, les réponses et le catalogue. Ne retire aucune condition du texte : une directive plus large ou plus étroite que le texte n'est pas fidèle.
4. Dates au format AAAA-MM-JJ, recopiées de ce qu'a écrit le juriste. La date d'effet n'est pas de ton ressort : le juriste la fixe à part.

Les verrous (liste fermée) :
- interdiction : l'outil ciblé est refusé, sans condition. Effet : refuser.
- seuil_argument : une ou plusieurs conditions sur les arguments de l'appel ciblé (montant, catégorie…), combinées en ET. Effet : refuser ou attendre_validation.
- provenance_session : se déclenche si, dans la même session, l'agent a déjà reçu des données de certains serveurs ou outils (critere donnees_recues, le défaut), ou leur a seulement transmis un appel (critere transmis). La cible est l'outil dont l'appel est alors bloqué ou soumis à validation. Effet : refuser ou attendre_validation. C'est le verrou de toute directive qui vise l'usage, pour une action, d'informations venant d'une source (« utiliser les informations de A pour faire B », « sur la base des données de A ») : la source A va dans provenance, l'action B est la cible.
- approbation_humaine : tout appel à l'outil ciblé attend une validation humaine. Effet : attendre_validation.
- champs_autorises_par_destination : le serveur ciblé (nommé, jamais « * ») ne reçoit que les champs listés. « ne reçoit que X » sans autre précision : mode caviarder et effet caviarder (le reste est retiré) ; mode refuser et effet refuser seulement si le juriste demande de bloquer l'appel.
- source_attestee : tout appel vers le serveur ciblé (nommé) est refusé si son attestation a échoué. Effet : refuser.

La cible est un couple (serveur, outil) du catalogue ; « * » veut dire tous. Les champs sont des pointeurs JSON du catalogue (« /siren »).
"""


def _enum_ou_libre(valeurs, description):
    schema = {"type": "string", "description": description}
    if valeurs:
        schema["enum"] = list(valeurs)
    return schema


def outils(catalogue):
    """Les deux outils, bornés par le catalogue fourni."""
    serveurs = catalogue.noms_serveurs()
    arguments = catalogue.arguments()
    champ = _enum_ou_libre(arguments, "Pointeur JSON d'un argument, tel que /montant.")

    proprietes = {
        "verrou": {"type": "string", "enum": VERROUS},
        "cible": {
            "type": "object",
            "properties": {
                "serveur": {"type": "string", "enum": serveurs + ["*"]},
                "outil": {"type": "string", "enum": catalogue.noms_outils() + ["*"]},
            },
            "required": ["serveur", "outil"],
        },
        "effet": {"type": "string", "enum": EFFETS},
        "fondement": {
            "type": "object",
            "description": "Base juridique citée par le juriste, recopiée.",
            "properties": {
                "texte": {"type": "string", "description": "Le nom du texte seul, sans l'article, tel que RGPD ou Code civil."},
                "article": {"type": "string", "description": "Le numéro d'article seul, sans « art. », tel que 22."},
            },
            "required": ["texte"],
        },
        "revision_avant": {"type": "string", "description": "AAAA-MM-JJ : date avant laquelle relire la directive, écrite par le juriste."},
        "condition": {
            "type": "array",
            "description": "Pour seuil_argument uniquement.",
            "items": {
                "type": "object",
                "properties": {
                    "champ": champ,
                    "op": {"type": "string", "enum": OPERATEURS},
                    "valeur": {"description": "Nombre, chaîne, booléen ou liste (pour in) ; absente pour present et absent."},
                },
                "required": ["champ", "op"],
            },
        },
        "provenance": {
            "type": "object",
            "description": "Pour provenance_session uniquement.",
            "properties": {
                "serveurs": {"type": "array", "items": {"type": "string", "enum": serveurs}},
                "outils": {"type": "array", "items": {"type": "string", "enum": catalogue.outils_qualifies()}},
                "critere": {"type": "string", "enum": ["donnees_recues", "transmis"]},
            },
        },
        "champs": {
            "type": "object",
            "description": "Pour champs_autorises_par_destination uniquement.",
            "properties": {
                "autorises": {"type": "array", "items": champ},
                "mode": {"type": "string", "enum": ["caviarder", "refuser"]},
            },
            "required": ["autorises", "mode"],
        },
    }
    if catalogue.appelants:
        proprietes["perimetre"] = {
            "type": "array",
            "description": "Appelants concernés, seulement si le juriste restreint la directive.",
            "items": {"type": "string", "enum": catalogue.appelants},
        }

    return [
        {
            "type": "function",
            "function": {
                "name": "proposer_directive",
                "description": "Propose la directive compilée, quand le texte, les réponses et le catalogue déterminent chaque valeur.",
                "parameters": {
                    "type": "object",
                    "properties": proprietes,
                    "required": ["verrou", "cible", "effet", "fondement"],
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
                        "champ": {"type": "string", "enum": CHAMPS_QUESTION},
                        "options": {
                            "type": "array",
                            "items": {"type": "string"},
                            "description": "Réponses possibles, s'il y en a un nombre fermé.",
                        },
                    },
                    "required": ["question", "champ"],
                },
            },
        },
    ]
