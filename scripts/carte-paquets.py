#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Extrait le graphe des paquets d'un module Go, pour la carte des briques.

La carte ne vaut que si ses arêtes sont les imports réels : un schéma dessiné
à la main ment dès le premier refactor. Ce script lit les `import` des fichiers
non-test et en déduit la structure.

    python3 scripts/carte-paquets.py                    # écrit carte/graphe.json
    python3 scripts/carte-paquets.py --module autre     # autre nom de module
    python3 scripts/carte-paquets.py --racine ../projet --sortie /tmp/g.json

Sortie : un objet {chemin: {...}} où chaque paquet porte
    id, name, kind (pkg|cmd), loc, test, files,
    doc       le commentaire de doc du paquet, tel qu'il est dans le code
    deps      les paquets internes qu'il importe        (vers la gauche)
    usedBy    les paquets internes qui l'importent      (vers la droite)
    depth     profondeur topologique : 0 = aucune dépendance interne

Les annotations éditoriales (couche, sort, explication) vivent à part, dans
carte/annotations.json, et sont fusionnées si le fichier existe : le graphe se
régénère sans les écraser.
"""

import argparse
import json
import pathlib
import re
import sys

# Racines de paquets scannées, dans l'ordre d'affichage.
RACINES = ("internal", "cmd", "pkg")


def doc_paquet(fichiers):
    """Le commentaire de doc : les lignes // qui précèdent `package`."""
    for f in fichiers:
        buf = []
        for ligne in f.read_text(encoding="utf-8", errors="replace").splitlines():
            if ligne.startswith("//"):
                buf.append(ligne[2:].strip())
            elif ligne.startswith("package "):
                if buf:
                    return " ".join(x for x in buf if x)
                buf = []
            else:
                buf = []
    return ""


def lignes(fichiers):
    return sum(sum(1 for _ in f.open(encoding="utf-8", errors="replace")) for f in fichiers)


def imports(src: str):
    """Les chemins importés, et eux seuls.

    Une simple recherche du nom du module dans tout le fichier attraperait un
    chemin cité dans un commentaire ou dans une chaîne — par exemple l'argument
    d'un `exec.Command("go", "build", "mod/internal/x")` — et inventerait une
    arête. On ne lit donc que les déclarations d'import.
    """
    out, i, lignes_src = [], 0, src.splitlines()
    while i < len(lignes_src):
        s = lignes_src[i].strip()
        if s.startswith("import ("):                      # bloc
            i += 1
            while i < len(lignes_src) and lignes_src[i].strip() != ")":
                l = lignes_src[i].strip()
                if l and not l.startswith("//"):
                    m = re.search(r'"([^"]+)"', l)
                    if m:
                        out.append(m.group(1))
                i += 1
        elif s.startswith("import "):                     # déclaration simple
            m = re.search(r'"([^"]+)"', s)
            if m:
                out.append(m.group(1))
        elif s.startswith(("func ", "type ", "var ", "const ")):
            break                                         # plus d'import au-delà
        i += 1
    return out


def construire(racine: pathlib.Path, module: str):
    prefixe = module + "/"
    paquets = {}

    for base in RACINES:
        d0 = racine / base
        if not d0.is_dir():
            continue
        # rglob, pas iterdir : un paquet imbriqué (internal/x/y) est un paquet
        # comme un autre, et l'omettre fait mentir la carte sur son exhaustivité.
        rep = sorted({f.parent for f in d0.rglob("*.go")})
        for d in rep:
            go = sorted(d.glob("*.go"))
            if not go:
                continue
            src = [f for f in go if not f.name.endswith("_test.go")]
            tst = [f for f in go if f.name.endswith("_test.go")]
            cle = d.relative_to(racine).as_posix()

            deps = set()
            for f in src:
                for p in imports(f.read_text(encoding="utf-8", errors="replace")):
                    if p.startswith(prefixe):
                        deps.add(p[len(prefixe):])
            deps.discard(cle)

            paquets[cle] = {
                "id": cle,
                "name": d.name if d.parent.name in RACINES else f"{d.parent.name}/{d.name}",
                "kind": "cmd" if base == "cmd" else "pkg",
                "loc": lignes(src),
                "test": lignes(tst),
                "files": len(src),
                "doc": doc_paquet(src),
                "deps": sorted(deps),
            }

    # Une arête vers un paquet non scanné (sous-paquet imbriqué) ne se dessine
    # pas : on la retire plutôt que de laisser un lien mort dans la carte.
    pendantes = 0
    for v in paquets.values():
        avant = len(v["deps"])
        v["deps"] = [d for d in v["deps"] if d in paquets]
        pendantes += avant - len(v["deps"])

    for cle, v in paquets.items():
        v["usedBy"] = sorted(o for o, ov in paquets.items() if cle in ov["deps"])

    # Profondeur topologique. La garde `vus` protège d'un cycle d'imports, que
    # Go interdit mais qu'un graphe mal extrait pourrait produire.
    memo = {}

    def profondeur(cle, vus=()):
        if cle in memo:
            return memo[cle]
        if cle in vus:
            return 0
        d = paquets[cle]["deps"]
        r = 0 if not d else 1 + max(profondeur(x, vus + (cle,)) for x in d)
        memo[cle] = r
        return r

    for cle in paquets:
        paquets[cle]["depth"] = profondeur(cle)

    return paquets, pendantes


def main():
    p = argparse.ArgumentParser(description=__doc__,
                                formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--racine", default=".", help="racine du module (défaut : .)")
    p.add_argument("--module", default="mcprogue", help="nom du module Go")
    p.add_argument("--sortie", default="carte/graphe.json")
    p.add_argument("--annotations", default="carte/annotations.json")
    a = p.parse_args()

    racine = pathlib.Path(a.racine).resolve()
    paquets, pendantes = construire(racine, a.module)
    if not paquets:
        sys.exit(f"aucun paquet sous {racine} dans {'/, '.join(RACINES)}/")

    ann = racine / a.annotations
    manquants = []
    if ann.is_file():
        notes = json.loads(ann.read_text(encoding="utf-8"))
        for cle, v in paquets.items():
            if cle in notes:
                v.update(notes[cle])
            else:
                manquants.append(cle)

    sortie = racine / a.sortie
    sortie.parent.mkdir(parents=True, exist_ok=True)
    sortie.write_text(json.dumps(paquets, ensure_ascii=False, separators=(",", ":")),
                      encoding="utf-8")

    aretes = sum(len(v["deps"]) for v in paquets.values())
    print(f"{len(paquets)} paquets, {aretes} arêtes, profondeur max "
          f"{max(v['depth'] for v in paquets.values())}")
    print(f"{sum(v['loc'] for v in paquets.values())} lignes de source, "
          f"{sum(v['test'] for v in paquets.values())} de tests")
    if pendantes:
        print(f"{pendantes} arête(s) vers un sous-paquet non scanné, retirée(s)")
    if manquants:
        print(f"sans annotation : {', '.join(manquants)}")
    print(f"→ {sortie}")


if __name__ == "__main__":
    main()
