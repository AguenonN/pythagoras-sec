#!/usr/bin/env python3
"""Réponse au conseiller lisible (Playwright, Chromium).

    python reponse.py http://127.0.0.1:PORT <demande>

Lancé par TestNavigateurReponse. Code de sortie 0 si conforme.
"""
import sys

from playwright.sync_api import sync_playwright

URL, DEMANDE = sys.argv[1].rstrip("/"), sys.argv[2]
ecarts = []


def exiger(cond, msg):
    if not cond:
        ecarts.append(msg)
        print("ÉCART :", msg, file=sys.stderr)


with sync_playwright() as p:
    nav = p.chromium.launch()
    page = nav.new_page(viewport={"width": 1440, "height": 900})
    alertes = []
    page.on("dialog", lambda d: (alertes.append(d.message), d.dismiss()))
    page.on("pageerror", lambda e: exiger(False, "erreur JavaScript : %s" % e))
    page.goto(URL + "/#poste?j=" + DEMANDE)
    page.wait_for_selector(".callout .md")
    rep = page.locator(".callout .md")
    exiger(rep.locator("strong").first.inner_text() == "DOS-FICTIF-003", "gras non rendu")
    exiger("**" not in rep.inner_text(), "astérisques visibles")
    exiger(rep.locator("ul li").count() == 2, "liste non rendue")
    exiger(rep.locator("p").count() >= 3, "paragraphes non rendus")
    exiger(page.locator(".callout img").count() == 0, "balise img interprétée")
    exiger("<img src=x onerror=alert(1)>" in rep.inner_text(), "texte de la balise non affiché tel quel")
    exiger("Décisions de l'engine" not in rep.inner_text(), "relevé laissé dans le texte")
    exiger("banque-synthetique__" not in rep.inner_text(), "nom technique d'outil dans le texte")
    exiger("Seuil de l'usure (base interne) — banque" in rep.inner_text(), "libellé d'outil absent du texte")
    det = page.locator("details.technique")
    exiger(det.count() == 1 and not det.evaluate("d => d.open"), "détail technique absent ou déplié")
    page.click("details.technique summary")
    exiger("Lecture du dossier de crédit — banque" in det.inner_text(), "libellé absent du détail")
    exiger(det.locator(".outil").first.get_attribute("title").startswith("banque-synthetique__evaluer_dossier"),
           "nom technique absent de l'info-bulle")
    exiger(not alertes, "script exécuté depuis le texte")
    nav.close()

sys.exit(1 if ecarts else 0)
