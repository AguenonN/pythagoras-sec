#!/usr/bin/env python3
"""Demandeur du Poste stable (Playwright, Chromium).

    python demandeur.py http://127.0.0.1:PORT <session de Bruno> <session de Claire>

Lancé par TestNavigateurDemandeur ; la console a pour identité connectée
anna.risques@exemple.test. Code de sortie 0 si conforme.
"""
import sys
import time

from playwright.sync_api import sync_playwright

URL, BRUNO, CLAIRE = sys.argv[1].rstrip("/"), sys.argv[2], sys.argv[3]
ANNA, JULIE = "anna.risques@exemple.test", "julie.juridique@exemple.test"
ecarts = []


def exiger(cond, msg):
    if not cond:
        ecarts.append(msg)
        print("ÉCART :", msg, file=sys.stderr)


def ouvrir(page, session):
    page.goto(URL + "/#poste?j=" + session)
    page.wait_for_selector("text=Déroulé en direct")
    return page.eval_on_selector("#demandeur", "e => e.value")


with sync_playwright() as p:
    nav = p.chromium.launch()
    page = nav.new_context(viewport={"width": 1440, "height": 900}).new_page()
    page.on("pageerror", lambda e: exiger(False, "erreur JavaScript : %s" % e))
    exiger(ouvrir(page, BRUNO) == ANNA, "par défaut, le demandeur n'est pas l'identité connectée")
    exiger("Anna Risques" in page.inner_text("#connecte"), "en-tête : l'identité connectée n'est pas affichée")
    time.sleep(4.5)
    exiger(page.eval_on_selector("#demandeur", "e => e.value") == ANNA, "le demandeur a changé seul pendant le sondage")
    exiger(ouvrir(page, CLAIRE) == ANNA, "le demandeur suit la session affichée")
    page.select_option("#demandeur", JULIE)
    exiger("Julie" in page.inner_text("#connecte"), "en-tête non mis à jour après le choix")
    exiger(ouvrir(page, BRUNO) == JULIE, "le dernier choix n'est pas gardé d'une session à l'autre")
    page.reload()
    page.wait_for_selector("text=Déroulé en direct")
    exiger(page.eval_on_selector("#demandeur", "e => e.value") == JULIE, "le dernier choix n'est pas gardé au rechargement")
    nav.close()

sys.exit(1 if ecarts else 0)
