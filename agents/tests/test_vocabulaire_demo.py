"""Le vocabulaire du compilateur (exemples/vocabulaire-demo.json) est généré
depuis le registre et l'annuaire de démo (go run ./cmd/registre-demo
-vocabulaire ..., depuis la racine du dépôt). Ce test le redérive des mêmes
sources, en Python, et échoue s'il en diverge ; côté Go, cmd/registre-demo
fait la même vérification, octet pour octet."""

import json
import os
import unittest

from compilateur import scenarios_v2, vocabulaire

from .aides import CATALOGUE_DEMO

RACINE = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

ROLE = "rôle de l'annuaire (valideur, destinataire d'une notification)"
GROUPE = "groupe de l'annuaire : personnes qui lancent les agents (feuille demandeur)"
SUJET = "%s Référentiel RH : personne concernée par l'action (feuille sujet)."


def lire(chemin):
    with open(os.path.join(RACINE, chemin), encoding="utf-8") as f:
        return json.load(f)


def deriver(etiquettes, annuaire, groupes_rh):
    portees = {}
    for s in sorted(etiquettes["serveurs"]):
        for e in etiquettes["serveurs"][s]:
            portees.setdefault(e, []).append("serveur " + s)
    for o in sorted(etiquettes["outils"]):
        lieu = "outil %s (tout serveur)" % o[2:] if o.startswith("*/") else "outil " + o
        for e in etiquettes["outils"][o]:
            portees.setdefault(e, []).append(lieu)
    roles, groupes = set(), {}
    for i in annuaire["identites"]:
        roles |= {r["role"] for r in i.get("roles", [])}
        for g in i.get("groupes", []):
            groupes[g] = GROUPE
    for g, d in sorted(groupes_rh["groupes"].items()):
        d = SUJET % d.strip()
        groupes[g] = groupes[g] + " ; " + d if g in groupes else d
    return {
        "etiquettes": [{"nom": e, "description": "étiquette du registre, portée par : " + ", ".join(portees[e])}
                       for e in sorted(portees)],
        "roles": [{"nom": r, "description": ROLE} for r in sorted(roles)],
        "groupes": [{"nom": g, "description": groupes[g]} for g in sorted(groupes)],
    }


def depuis_sources():
    return deriver(lire("demo/etiquettes.json"), lire("demo/annuaire-source.json"),
                   lire("servers/referentiel-rh/groupes_fictifs.json"))


class TestVocabulaireDemo(unittest.TestCase):
    def setUp(self):
        with open(scenarios_v2.COMPLEMENT, encoding="utf-8") as f:
            self.versionne = json.load(f)

    def test_conforme_aux_sources(self):
        self.assertEqual({k: self.versionne[k] for k in ("etiquettes", "roles", "groupes")}, depuis_sources())
        self.assertIn("ne pas modifier à la main", self.versionne["origine"])

    def test_divergence_detectee(self):
        annuaire = lire("demo/annuaire-source.json")
        annuaire["identites"].append({"identite": "x@exemple.test", "groupes": ["groupe-ajoute"]})
        altere = deriver(lire("demo/etiquettes.json"), annuaire, lire("servers/referentiel-rh/groupes_fictifs.json"))
        self.assertNotEqual(altere["groupes"], self.versionne["groupes"])

    def test_noms_du_jeu_de_demo(self):
        """Chaque rôle, groupe et étiquette que cite le jeu de démo est connu
        du compilateur : une directive ajoutée peut les reprendre."""
        v = vocabulaire.Vocabulaire.lire(CATALOGUE_DEMO, scenarios_v2.COMPLEMENT, fuseaux=["Europe/Paris"])
        cites = {"roles": set(), "groupes": set(), "etiquettes": set()}

        def visiter(x):
            if isinstance(x, dict):
                for k, val in x.items():
                    if k in ("par_role", "destinataire_role"):
                        cites["roles"].add(val)
                    elif k == "groupe":
                        cites["groupes"].add(val)
                    elif k == "etiquette":
                        cites["etiquettes"].add(val)
                    visiter(val)
            elif isinstance(x, list):
                for y in x:
                    visiter(y)
        visiter(lire("demo/directives/demo.json")["directives"])
        self.assertLessEqual(cites["roles"], set(v.roles))
        self.assertLessEqual(cites["groupes"], set(v.groupes))
        self.assertLessEqual(cites["etiquettes"], set(v.etiquettes))


if __name__ == "__main__":
    unittest.main()
