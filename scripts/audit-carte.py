# -*- coding: utf-8 -*-
"""Audit de la carte : re-extrait le graphe par une seconde voie, et compare.

La méthode du script de la carte est une regex sur tout le fichier : elle
attraperait un chemin cité dans un commentaire ou une chaîne. Ici on ne lit
QUE les déclarations d'import réelles, en suivant la grammaire Go.
"""
import json, pathlib, re, sys

R = pathlib.Path(__file__).resolve().parent.parent
MOD = "mcprogue"

def imports_reels(src: str):
    """Chemins importés, en ne lisant que les déclarations d'import."""
    out, i, lignes = [], 0, src.splitlines()
    while i < len(lignes):
        s = lignes[i].strip()
        if s.startswith("import ("):            # bloc
            i += 1
            while i < len(lignes) and lignes[i].strip() != ")":
                l = lignes[i].strip()
                if l and not l.startswith("//"):
                    m = re.search(r'"([^"]+)"', l)
                    if m: out.append(m.group(1))
                i += 1
        elif s.startswith("import "):            # ligne unique
            m = re.search(r'"([^"]+)"', s)
            if m: out.append(m.group(1))
        elif s.startswith(("func ", "type ", "var ", "const ")):
            break                                # plus d'import au-delà
        i += 1
    return out

# --- A. graphe de référence, par parseur ---------------------------------
ref, fichiers_scannes = {}, 0
for base in ("internal", "cmd", "pkg"):
    d0 = R / base
    if not d0.is_dir(): continue
    for d in sorted({f.parent for f in d0.rglob("*.go")}):
        src = [f for f in sorted(d.glob("*.go")) if not f.name.endswith("_test.go")]
        if not sorted(d.glob("*.go")): continue
        deps = set()
        for f in src:
            fichiers_scannes += 1
            for p in imports_reels(f.read_text(encoding="utf-8", errors="replace")):
                if p.startswith(MOD + "/"):
                    deps.add(p[len(MOD)+1:])
        cle = d.relative_to(R).as_posix()
        deps.discard(cle)
        ref[cle] = deps

print(f"A. parseur d'imports : {len(ref)} paquets, {fichiers_scannes} fichiers lus")

# --- B. comparaison avec le graphe publié --------------------------------
pub = json.load(open(R / "carte" / "graphe.json", encoding="utf-8"))
ecarts = []
if set(ref) != set(pub):
    ecarts.append(f"paquets : réf-seul={sorted(set(ref)-set(pub))} pub-seul={sorted(set(pub)-set(ref))}")
for cle in sorted(set(ref) & set(pub)):
    a = {d for d in ref[cle] if d in ref}      # arêtes vers un paquet scanné
    b = set(pub[cle]["deps"])
    if a != b:
        ecarts.append(f"{cle}: fantômes={sorted(b-a)} manquantes={sorted(a-b)}")
print(f"B. écarts regex vs parseur : {len(ecarts)}")
for e in ecarts: print("   !", e)

# --- C. chemins du module cités HORS import (pièges potentiels) ----------
pieges = 0
for f in R.rglob("*.go"):
    if ".git" in f.parts or f.name.endswith("_test.go"): continue
    t = f.read_text(encoding="utf-8", errors="replace")
    reels = set(imports_reels(t))
    for m in re.finditer(r'"' + MOD + r'/([a-z0-9_/-]+)"', t):
        if m.group(0).strip('"') not in reels:
            pieges += 1
            print(f"   ! {f.relative_to(R)} cite {m.group(0)} hors import")
print(f"C. citations hors import : {pieges}")

# --- D. paquets non scannés qui existent quand même ----------------------
rates = []
for f in R.rglob("*.go"):
    if ".git" in f.parts: continue
    rel = f.relative_to(R).parent.as_posix()
    if rel.split("/")[0] in ("internal", "cmd", "pkg") and rel not in pub:
        rates.append(rel)
print(f"D. répertoires Go hors carte : {len(set(rates))}")
for r in sorted(set(rates)): print("   ·", r)
