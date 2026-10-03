#!/usr/bin/env python3
"""Formulaires de la console face au sondage (Playwright, Chromium).

    python formulaires.py http://127.0.0.1:PORT <session en attente>

Lancé par TestNavigateurFormulaires (navigateur_test.go), pendant que le test
Go fait vivre des sessions (appels, attentes, blocages) : le sondage de 2 s a
donc toujours quelque chose à redessiner. Sur chaque écran qui a un champ, on
tape lentement (caractère par caractère, plus de 6 s) et on change le choix
d'une liste déroulante pendant un sondage ; on exige ensuite :
  - le même nœud DOM pour chaque champ (jamais reconstruit) ;
  - la saisie complète, le choix conservé, le focus toujours sur le champ ;
  - au moins une mise à jour de l'écran pendant la saisie (sinon le test ne
    prouverait rien), sauf sur Rédaction, qui ne sonde pas.
Code de sortie 0 si tout est conforme ; sinon, les écarts sur la sortie
d'erreur.
"""
import sys
import time

from playwright.sync_api import sync_playwright

URL = sys.argv[1].rstrip("/")
ATTENTE = sys.argv[2]  # session dont l'attente dure tout le test
ecarts = []
DELAI = 170  # ms entre deux caractères : 40 caractères ≈ 6,8 s


def ecart(msg):
    ecarts.append(msg)
    print("ÉCART :", msg, file=sys.stderr)


def marquer(page, ids, zone):
    """Retient les nœuds des champs et compte les mutations de la zone
    redessinée par le sondage."""
    page.evaluate("""([ids, zone]) => {
        window.__noeuds = Object.fromEntries(ids.map(i => [i, document.getElementById(i)]));
        window.__mutations = 0;
        const z = document.querySelector(zone);
        if (z) new MutationObserver(() => { window.__mutations++; }).observe(z, { childList: true, subtree: true });
    }""", [ids, zone])


def verifier(page, ecran, ids, valeurs, focus, sonde=True):
    for i in ids:
        if not page.evaluate("(i) => document.getElementById(i) === window.__noeuds[i]", i):
            ecart("%s : le champ #%s a été reconstruit pendant la saisie" % (ecran, i))
    for i, v in valeurs.items():
        lu = page.eval_on_selector("#" + i, "e => e.value")
        if lu != v:
            ecart("%s : #%s vaut %r au lieu de %r" % (ecran, i, lu, v))
    actif = page.evaluate("() => document.activeElement && document.activeElement.id")
    if actif != focus:
        ecart("%s : le focus est sur %r au lieu de #%s" % (ecran, actif, focus))
    n = page.evaluate("() => window.__mutations")
    if sonde and n == 0:
        ecart("%s : aucune mise à jour pendant la saisie (le test ne prouve rien)" % ecran)
    print("%s : %d mise(s) à jour de l'écran pendant la saisie" % (ecran, n))


def choisir_au_clavier(page, sel, pas=2):
    """Change le choix d'une liste au clavier, en plusieurs fois, en
    laissant passer un sondage entre deux touches."""
    page.focus(sel)
    for _ in range(pas):
        page.keyboard.press("ArrowDown")
        time.sleep(2.3)
    return page.eval_on_selector(sel, "e => e.value")


def session_vivante(page):
    """Une demande en cours (de l'agitateur) : sa frise va changer."""
    page.goto(URL + "/#journal")
    for _ in range(50):
        js = page.evaluate("() => fetch('/api/etat').then(r => r.json())")["journaux"]
        vivantes = sorted((j for j in js if j.get("statut") == "en_cours"), key=lambda j: j.get("debut") or "", reverse=True)
        if vivantes:
            return vivantes[0]["id"]
        time.sleep(0.2)
    raise SystemExit("aucune demande en cours")


def poste(page):
    page.goto(URL + "/#poste?j=" + session_vivante(page))
    page.wait_for_selector("#demande")
    page.wait_for_selector("text=Déroulé en direct")
    marquer(page, ["demande", "demandeur"], "main.principal")
    texte = "Peut-on accorder un prêt de 90 000 euros ?"
    page.click("#demande")
    page.locator("#demande").press_sequentially(texte, delay=DELAI)
    verifier(page, "Poste", ["demande", "demandeur"], {"demande": texte}, "demande")
    choisi = choisir_au_clavier(page, "#demandeur")
    verifier(page, "Poste (liste)", ["demande", "demandeur"], {"demande": texte, "demandeur": choisi}, "demandeur")


def validation(page):
    page.goto(URL + "/#validation?a=" + ATTENTE)
    page.wait_for_selector("#valideur")
    marquer(page, ["valideur", "motif"], "#app")
    motif = "Montant hors délégation du conseiller."
    page.click("#motif")
    page.locator("#motif").press_sequentially(motif, delay=DELAI)
    verifier(page, "Validation", ["valideur", "motif"], {"motif": motif}, "motif")
    choisi = choisir_au_clavier(page, "#valideur", pas=1)
    verifier(page, "Validation (liste)", ["valideur", "motif"], {"motif": motif, "valideur": choisi}, "valideur")


def redaction(page):
    page.goto(URL + "/#redaction")
    page.wait_for_selector("#texte")
    marquer(page, ["texte", "fondement"], "body")
    texte = "Aucune action d'agent le week-end, heure de Paris."
    page.click("#texte")
    page.locator("#texte").press_sequentially(texte, delay=DELAI)
    page.click("#fondement")
    page.locator("#fondement").press_sequentially("Note interne", delay=DELAI)
    verifier(page, "Rédaction", ["texte", "fondement"], {"texte": texte, "fondement": "Note interne"}, "fondement", sonde=False)


with sync_playwright() as p:
    nav = p.chromium.launch()
    page = nav.new_page(viewport={"width": 1440, "height": 900})
    page.on("pageerror", lambda e: ecart("erreur JavaScript : %s" % e))
    for f in (poste, validation, redaction):
        f(page)
    nav.close()

sys.exit(1 if ecarts else 0)
