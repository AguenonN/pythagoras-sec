#!/usr/bin/env python3
"""Onglet « Propositions de l'Analyste » sans proposition (Playwright).

    python propositions.py http://127.0.0.1:PORT
"""
import sys

from playwright.sync_api import sync_playwright

URL = sys.argv[1].rstrip("/")
ecarts = []
with sync_playwright() as p:
    nav = p.chromium.launch()
    page = nav.new_page()
    page.on("pageerror", lambda e: ecarts.append("erreur JavaScript : %s" % e))
    page.goto(URL + "/#redaction?onglet=propositions")
    page.wait_for_selector("#aucune-proposition")
    t = page.inner_text("#aucune-proposition")
    for attendu in ("Aucune proposition.", "python3 -m analyste proposer"):
        if attendu not in t:
            ecarts.append("« %s » absent : %s" % (attendu, t))
    if "Propositions de l'Analyste · 0" not in page.inner_text(".onglets"):
        ecarts.append("compteur de l'onglet")
    nav.close()
for e in ecarts:
    print("ÉCART :", e, file=sys.stderr)
sys.exit(1 if ecarts else 0)
