#!/usr/bin/env python3
"""Captures de la session console-3 (lancé par capturer-console3.sh).

    python capturer-console3.py http://127.0.0.1:PORT <dossier de la démo> <dossier des images>
"""
import json
import os
import subprocess
import sys
import time

from playwright.sync_api import sync_playwright

URL, DOSSIER, IMAGES = sys.argv[1].rstrip("/"), sys.argv[2], sys.argv[3]
SIM = os.path.join(DOSSIER, "bin", "simulateur")
CLAIRE = "claire.analyste@exemple.test"
ANNA = "Anna Risques (fictive) — rôle « analyste-risques »"
os.makedirs(IMAGES, exist_ok=True)
erreurs = []
lances = []


def sessions():
    return set(os.listdir(os.path.join(DOSSIER, "demo", "execution", "sessions")))


def etat(sid):
    try:
        with open(os.path.join(DOSSIER, "demo", "execution", "sessions", sid, "etat.json")) as f:
            return json.load(f).get("statut")
    except (OSError, ValueError):
        return None


def lancer(texte, pause="50ms", attendre=None):
    """Lance une demande du simulateur ; rend sa session (et attend un statut)."""
    avant = sessions()
    lances.append(subprocess.Popen([SIM, "demande", "-racine", DOSSIER, "-pause", pause, "--demandeur", CLAIRE, texte],
                                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL))
    for _ in range(200):
        nouvelles = sessions() - avant
        if nouvelles:
            sid = sorted(nouvelles)[0]
            if attendre is None or etat(sid) == attendre:
                return sid
        time.sleep(0.1)
    raise SystemExit("session non apparue : " + texte)


def capture(page, nom):
    time.sleep(0.4)
    page.screenshot(path=os.path.join(IMAGES, nom), full_page=True)
    print("capture", nom)


try:
    a = lancer("Peut-on accorder un prêt de 120 000 euros pour le dossier DOS-FICTIF-002 ?", attendre="en_attente")
    time.sleep(1.1)
    b = lancer("Peut-on accorder un prêt de 150 000 euros pour le dossier DOS-FICTIF-002 ?", attendre="en_attente")
    with sync_playwright() as p:
        nav = p.chromium.launch()
        page = nav.new_page(viewport={"width": 1440, "height": 900})
        page.on("pageerror", lambda e: erreurs.append(str(e)))

        # Poste : saisie lente pendant que la frise d'une demande en cours avance.
        time.sleep(1.1)
        vivante = lancer("Peut-on accorder un prêt de 90 000 euros pour le dossier DOS-FICTIF-002 ?", pause="900ms")
        page.goto(URL + "/#poste?j=" + vivante)
        page.wait_for_selector("text=Déroulé en direct")
        page.click("#demande")
        page.locator("#demande").press_sequentially("Peut-on accorder un prêt de 80 000 euros pour le dossier", delay=150)
        capture(page, "2-poste-saisie-lente.png")

        # Réponse lisible : Markdown, relevé replié puis déplié.
        page.goto(URL + "/#poste?j=20261003-090000-mdwn")
        page.wait_for_selector(".callout .md")
        capture(page, "2-poste-reponse-lisible.png")
        page.click("details.technique summary")
        capture(page, "2-poste-reponse-detail-technique.png")

        # Validation : formulaire, carte de décision avec les contrôles.
        page.goto(URL + "/#validation?a=" + a)
        page.wait_for_selector("#valideur")
        page.select_option("#valideur", label=ANNA)
        capture(page, "5-validation-formulaire.png")
        page.click("text=Signer et valider")
        page.wait_for_selector("#carte-decision >> text=Approbation acceptée", timeout=20000)
        capture(page, "5-validation-carte-decision.png")
        page.click("#demande-suivante")

        # Rejet : carte, puis Poste et Journal.
        page.goto(URL + "/#validation?a=" + b)
        page.wait_for_selector("#motif")
        page.select_option("#valideur", label=ANNA)
        page.fill("#motif", "Montant hors délégation : le dossier doit repasser en comité.")
        page.click("text=Rejeter")
        page.wait_for_selector("#carte-decision")
        capture(page, "5-validation-carte-rejet.png")
        for _ in range(50):
            if etat(b) == "bloque":
                break
            time.sleep(0.2)
        page.goto(URL + "/#poste?j=" + b)
        page.wait_for_selector(".deroule .badge.b-rejete")
        capture(page, "2-poste-rejet-visible.png")
        page.goto(URL + "/#journal?j=" + b)
        page.wait_for_selector(".badge.b-rejete")
        capture(page, "1-journal-rejet-visible.png")

        # Autres écrans.
        page.goto(URL + "/#journal?j=" + a)
        page.wait_for_selector(".grille-appels")
        capture(page, "1-journal-libelles-outils.png")
        for nom, ecran, sel in (("3-regles-libelles.png", "regles", "h1, h2"), ("4-avant-apres-libelles.png", "avant-apres", "h1, h2"),
                                ("7-notifications-libelles.png", "notifications", "h1, h2")):
            page.goto(URL + "/#" + ecran)
            page.wait_for_selector(sel)
            capture(page, nom)

        # Rédaction : compilation « réelle » (simulateur du service), panne en français.
        page.goto(URL + "/#redaction")
        page.wait_for_selector("#texte")
        page.fill("#texte", "Avant d'attribuer un prêt, si la base interne des taux a été consultée, un analyste risques doit valider.")
        page.fill("#fondement", "Politique interne de crédit, article 4")
        page.click("button:text-is(\"Compiler\")")
        page.wait_for_selector("text=accepté par le vérificateur", timeout=20000)
        capture(page, "6-redaction-compilation-reelle.png")
        page.goto(URL + "/?panne#redaction")  # chargement complet : pas l'écran précédent
        page.wait_for_selector("#texte")
        page.fill("#texte", "panne-enregistrement : aucune action le week-end")
        page.click("button:text-is(\"Compiler\")")
        try:
            page.wait_for_selector("text=Le compilateur n'a pas pu répondre", timeout=15000)
        except Exception:
            print("\n".join(l for l in page.inner_text("#app").split("\n") if "ompil" in l or "erreur" in l.lower() or "panne" in l), file=sys.stderr)
            raise
        capture(page, "6-redaction-panne-en-francais.png")

        # Propositions : dossier vide.
        prop = os.path.join(DOSSIER, "demo", "execution", "propositions")
        for f in os.listdir(prop):
            os.rename(os.path.join(prop, f), os.path.join(DOSSIER, f))
        page.goto(URL + "/?vide#redaction?onglet=propositions")
        page.wait_for_selector("#aucune-proposition")
        capture(page, "6-redaction-propositions-vide.png")
        nav.close()
finally:
    for pr in lances:
        pr.terminate()
    for pr in lances:
        pr.wait()

if erreurs:
    print("erreurs JavaScript :", erreurs, file=sys.stderr)
    sys.exit(1)
