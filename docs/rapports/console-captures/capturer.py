#!/usr/bin/env python3
"""Capture les écrans de la console sur des journaux réels, en faisant les
actions comme un utilisateur (validation, rédaction, activation), et les
maquettes correspondantes pour comparaison.

Usage (depuis la racine du dépôt, avec le venv de agents/) :

    docs/rapports/console-captures/preparer-captures.sh /tmp/essai 8088
    agents/.venv/bin/python docs/rapports/console-captures/capturer.py http://127.0.0.1:8088 docs/rapports/console-captures --maquettes docs/demo/maquettes

La console tourne sur une démonstration de mise au point (simulateur
d'orchestrateur, simulateur de compilateur, annuaire fictif). Ce script
modifie les règles de cette démonstration (il active, puis retire une
directive) : il ne s'exécute pas deux fois sur le même dossier. Aucune
ressource externe n'est chargée : le script échoue s'il en voit une.
"""
import json, re, sys, pathlib, urllib.request
from playwright.sync_api import sync_playwright

MAQUETTES = [("1-journal", "Main"), ("2-poste", "Conseiller"), ("3-regles", "Regles"),
             ("4-avant-apres", "AvantApres"), ("5-validation", "Superviseur"), ("6-redaction", "Redaction")]


def main():
    url, sortie = sys.argv[1].rstrip("/"), pathlib.Path(sys.argv[2])
    maq = pathlib.Path(sys.argv[sys.argv.index("--maquettes") + 1]) if "--maquettes" in sys.argv else None
    sortie.mkdir(parents=True, exist_ok=True)

    def api(chemin):
        return json.load(urllib.request.urlopen(url + chemin))

    etat = api("/api/etat")
    def session(motif):
        for j in sorted(etat["journaux"], key=lambda j: j["id"], reverse=True):
            if motif in (j.get("demande") or "") and j["id"] != "20261002-000000-falsif":
                return f"j={j['id']}&s={j['sessions'][0]['id']}"
        return ""
    gros, rompu = session("3 000 000"), "j=20261002-000000-falsif&s=" + next(
        j for j in etat["journaux"] if j["id"] == "20261002-000000-falsif")["sessions"][0]["id"]

    externes = []
    with sync_playwright() as p:
        nav = p.chromium.launch()
        ctx = nav.new_context(viewport={"width": 1440, "height": 980}, locale="fr-FR", timezone_id="Europe/Paris")
        page = ctx.new_page()
        page.on("dialog", lambda d: d.accept())
        page.on("request", lambda r: externes.append(r.url) if not r.url.startswith(url) and not r.url.startswith("data:") else None)
        erreurs = []
        page.on("pageerror", lambda e: erreurs.append(str(e)))

        def va(route):
            page.goto(f"{url}/#{route}")
            page.wait_for_load_state("networkidle")
            page.wait_for_timeout(700)

        def photo(nom):
            page.screenshot(path=str(sortie / f"{nom}.png"), full_page=True)

        def clic(nom, exact=False):
            page.get_by_role("button", name=nom, exact=exact).first.click()
            page.wait_for_load_state("networkidle")
            page.wait_for_timeout(900)

        # 1 · Journal : une session validée par deux rôles, puis la chaîne rompue.
        va("journal?" + gros)
        photo("1-journal")
        va("journal?" + rompu)
        photo("1-journal-chaine-rompue")
        # 2 · Poste conseiller : la demande qui attend sa validation.
        va("poste")
        photo("2-poste")
        # 3 · Vitrine des règles.
        va("regles?directive=D-005")
        photo("3-regles")
        # 4 · Avant / après la directive D-004 (caviardage) et D-001.
        va("avant-apres?directive=D-001")
        photo("4-avant-apres")
        # 5 · File du superviseur : l'appel en attente, puis après la reprise.
        va("validation")
        photo("5-validation")
        clic("Signer et valider")
        page.wait_for_timeout(2500)  # le simulateur relit les jetons chaque seconde
        va("validation")
        page.get_by_text("Reprises").first.scroll_into_view_if_needed()
        photo("5-validation-apres-reprise")
        # 7 · Notifications.
        va("notifications")
        photo("7-notifications")
        # 6 · Rédaction : texte, questions du compilateur, réponses, essai, observation.
        va("redaction")
        photo("6-redaction")
        page.locator("#texte").fill("Aucune action d'agent le week-end, heure de Paris.")
        page.locator("#fondement").fill("Règlement intérieur, art. 12 (à valider)")
        clic("Compiler")
        photo("6-redaction-questions")
        for sel in page.locator("select").all():
            if sel.is_visible():
                sel.select_option(index=1)
        clic("Envoyer les réponses")
        photo("6-redaction-compilee")
        clic("Essayer sur les journaux existants")
        photo("6-redaction-essai")
        clic("Mettre en observation")
        photo("6-redaction-observation")
        va("regles?directive=D-009")
        photo("3-regles-observation")
        # Propositions de l'Analyste.
        va("redaction")
        page.get_by_role("button", name=re.compile("Propositions")).first.click()
        page.wait_for_timeout(500)
        photo("6-redaction-propositions")
        # 2 · Lancer les agents depuis le poste.
        va("poste")
        page.get_by_placeholder(re.compile("Ex\\.")).fill("Peut-on accorder un prêt de 90 000 euros ?")
        clic("Lancer les agents")
        page.wait_for_timeout(3000)
        photo("2-poste-lancement")

        if maq:
            page2 = ctx.new_page()  # maquettes locales : hors contrôle des ressources
            fonts = pathlib.Path(__file__).resolve().parents[3] / "internal/console/web"
            for nom, fichier in MAQUETTES:
                src = (maq / f"{fichier}.dc.html").read_text()
                m = re.search(r"<x-dc>(.*)</x-dc>", src, re.S)
                corps = re.sub(r"<helmet>.*?</helmet>", "", m.group(1), flags=re.S)
                html = f"<!doctype html><meta charset=utf-8><link rel=stylesheet href='file://{fonts}/fonts.css'><style>body{{margin:0}}a{{color:#1D4F7A}}</style>{corps}"
                f = sortie / f"_maquette-{fichier}.html"
                f.write_text(html)
                page2.goto(f"file://{f}")
                page2.wait_for_timeout(300)
                page2.screenshot(path=str(sortie / f"maquette-{nom}.png"), full_page=True)
                f.unlink()
        nav.close()
    if erreurs:
        print("ERREURS JAVASCRIPT :", erreurs)
    if externes:
        print("RESSOURCES EXTERNES DEMANDÉES :", externes)
        sys.exit(1)
    print("captures dans", sortie, "· aucune ressource externe")


main()
