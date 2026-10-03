#!/usr/bin/env python3
"""Carte de décision de la Validation, rejet visible au Poste et au Journal
(Playwright, Chromium).

    python decision.py http://127.0.0.1:PORT <demande à valider> <demande à rejeter>

Lancé par TestNavigateurDecision. Code de sortie 0 si conforme.
"""
import sys
import time

from playwright.sync_api import sync_playwright

URL, A, B = sys.argv[1].rstrip("/"), sys.argv[2], sys.argv[3]
ecarts = []


def exiger(cond, msg):
    if not cond:
        ecarts.append(msg)
        print("ÉCART :", msg, file=sys.stderr)


with sync_playwright() as p:
    nav = p.chromium.launch()
    page = nav.new_page(viewport={"width": 1440, "height": 900})
    page.on("pageerror", lambda e: exiger(False, "erreur JavaScript : %s" % e))

    # Validation de A : la carte reste, avec les contrôles de l'engine.
    page.goto(URL + "/#validation?a=" + A)
    page.wait_for_selector("#valideur")
    page.select_option("#valideur", label="Anna Risques (fictive) — rôle « analyste-risques »")
    page.click("text=Signer et valider")
    carte = page.locator("#carte-decision")
    carte.wait_for()
    t = carte.inner_text()
    for attendu in ("Validée par", "Anna Risques", "analyste-risques", "Heure", "Voir la session", "Voir la preuve au journal"):
        exiger(attendu in t, "carte de validation : « %s » absent" % attendu)
    try:
        page.wait_for_selector("#carte-decision >> text=Approbation acceptée", timeout=15000)
    except Exception:
        exiger(False, "contrôles de l'engine jamais affichés sur la carte")
    time.sleep(4.5)  # deux sondages : la carte ne disparaît pas d'elle-même
    exiger(page.locator("#carte-decision").count() == 1, "carte de validation disparue sans clic")
    exiger("Signature" in carte.inner_text(), "contrôle de signature absent de la carte")
    page.click("#demande-suivante")
    page.wait_for_selector("#valideur")
    exiger(page.locator("#carte-decision").count() == 0, "carte toujours là après « Demande suivante »")

    # Rejet de B : carte, puis rejet visible au Poste et au Journal.
    page.goto(URL + "/#validation?a=" + B)
    page.wait_for_selector("#motif")
    page.select_option("#valideur", label="Anna Risques (fictive) — rôle « analyste-risques »")
    page.fill("#motif", "Montant hors délégation")
    page.click("text=Rejeter")
    carte.wait_for()
    t = carte.inner_text()
    for attendu in ("Demande rejetée", "Rejetée par", "Anna Risques", "analyste-risques", "Montant hors délégation", "hors journal signé"):
        exiger(attendu in t, "carte de rejet : « %s » absent" % attendu)
    time.sleep(4.5)
    exiger(page.locator("#carte-decision").count() == 1, "carte de rejet disparue sans clic")
    page.screenshot(path=sys.argv[4]) if len(sys.argv) > 4 else None

    page.goto(URL + "/#poste?j=" + B)
    page.wait_for_selector(".deroule .badge.b-rejete")
    texte = page.locator("main.principal").inner_text()
    exiger("rejetée par Anna Risques" in texte, "bandeau du Poste sans « rejetée par »")
    exiger("bloquée" not in texte, "Poste : « bloquée » au lieu de « rejetée »")
    exiger("jusqu'au rejet" in texte, "attente humaine non mesurée jusqu'au rejet")
    exiger("En attente de validation" not in page.locator(".deroule").inner_text(), "frise du Poste encore « en attente »")
    page.goto(URL + "/#journal?j=" + B)
    page.wait_for_selector(".badge.b-rejete")
    texte = page.locator("main.principal").inner_text()
    exiger("Demande rejetée par Anna Risques" in texte, "bandeau du Journal sans le rejet")
    exiger("hors journal signé" in page.locator("aside.volet").inner_text(), "volet du Journal sans la mention")
    nav.close()

sys.exit(1 if ecarts else 0)
