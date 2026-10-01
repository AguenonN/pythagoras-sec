#!/usr/bin/env python3
"""Capture les six écrans de la console et les maquettes correspondantes.

Usage (depuis la racine du dépôt, avec le venv de agents/) :
    agents/.venv/bin/python docs/rapports/console-captures/capturer.py URL SORTIE [--maquettes DOSSIER_MAQUETTES]

La console doit tourner sur URL avec des journaux issus de
`MCP_CONSERVER_JOURNAUX=<dir> go test ./test/integration/`. Les polices
viennent des fichiers embarqués ; aucune ressource externe n'est chargée.
"""
import json, re, sys, pathlib, urllib.request
from playwright.sync_api import sync_playwright

ECRANS = [
    ("1-journal", "journal", "Main"),
    ("2-poste", "poste", "Conseiller"),
    ("3-regles", "regles", "Regles"),
    ("4-avant-apres", "avant-apres", "AvantApres"),
    ("5-validation", "validation", "Superviseur"),
    ("6-redaction", "redaction", "Redaction"),
]

def main():
    url, sortie = sys.argv[1].rstrip("/"), pathlib.Path(sys.argv[2])
    maq = None
    if "--maquettes" in sys.argv:
        maq = pathlib.Path(sys.argv[sys.argv.index("--maquettes") + 1])
    def option(nom):
        return sys.argv[sys.argv.index(nom) + 1] if nom in sys.argv else None
    etat = json.load(urllib.request.urlopen(url + "/api/etat"))
    def requete(journal):
        for j in etat["journaux"]:
            if j["id"] == journal and j["sessions"]:
                return f"?j={j['id']}&s={j['sessions'][0]['id']}"
        return ""
    journal, rompu = option("--journal"), option("--rompu")
    sortie.mkdir(parents=True, exist_ok=True)
    with sync_playwright() as p:
        nav = p.chromium.launch()
        ctx = nav.new_context(viewport={"width": 1440, "height": 980}, locale="fr-FR", timezone_id="Europe/Paris")
        externes = []
        page = ctx.new_page()
        page2 = ctx.new_page()  # maquettes locales : hors contrôle des ressources
        page.on("request", lambda r: externes.append(r.url) if not r.url.startswith(url) and not r.url.startswith("data:") else None)
        for nom, route, fichier in ECRANS:
            q = requete(journal) if journal and route in ("journal", "poste", "avant-apres") else ""
            page.goto(f"{url}/#{route}{q}")
            page.wait_for_load_state("networkidle")
            page.wait_for_timeout(400)
            page.screenshot(path=str(sortie / f"{nom}.png"), full_page=True)
            if maq:
                src = (maq / f"{fichier}.dc.html").read_text()
                m = re.search(r"<x-dc>(.*)</x-dc>", src, re.S)
                corps = re.sub(r"<helmet>.*?</helmet>", "", m.group(1), flags=re.S)
                fonts = (pathlib.Path(__file__).resolve().parents[3] / "internal/console/web")
                html = f"<!doctype html><meta charset=utf-8><link rel=stylesheet href='file://{fonts}/fonts.css'><style>body{{margin:0}}a{{color:#1D4F7A}}</style>{corps}"
                f = sortie / f"_maquette-{fichier}.html"
                f.write_text(html)
                page2.goto(f"file://{f}")
                page2.wait_for_timeout(300)
                page2.screenshot(path=str(sortie / f"maquette-{nom}.png"), full_page=True)
                f.unlink()
        if rompu:
            page.goto(f"{url}/#journal{requete(rompu)}")
            page.wait_for_load_state("networkidle")
            page.wait_for_timeout(400)
            page.screenshot(path=str(sortie / "1-journal-chaine-rompue.png"), full_page=True)
        nav.close()
    if externes:
        print("RESSOURCES EXTERNES DEMANDÉES :", externes)
        sys.exit(1)
    print("captures dans", sortie, "· aucune ressource externe")

main()
