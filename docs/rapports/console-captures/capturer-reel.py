"""Captures de chaque écran de la console sur les journaux réels de
demo/scenarios/ (rejoués hors ligne par « lancer-demo.sh scenario … --mode rejeu »).

    python capturer-reel.py http://127.0.0.1:8090 docs/rapports/console-captures/reel

Échoue si une erreur JavaScript ou une réponse d'API en erreur apparaît.
"""
import json
import sys
import urllib.request
from playwright.sync_api import sync_playwright

base, sortie = sys.argv[1].rstrip('/'), sys.argv[2]
etat = json.load(urllib.request.urlopen(base + '/api/etat'))


def dossier(nom):
    return next(j['id'] for j in etat['journaux'] if j['id'].split('-', 2)[2].startswith(nom))


ecrans = [
    ('1-journal-nominal', '#journal?j=' + dossier('nominal')),
    ('1-journal-top-management', '#journal?j=' + dossier('top-management')),
    ('1-journal-consigne-sans-d001', '#journal?j=' + dossier('consigne-sans-d001')),
    ('1-journal-demande-sans-outil', '#journal?j=' + dossier('sans-outil')),
    ('2-poste-nominal', '#poste?j=' + dossier('nominal')),
    ('2-poste-demande-sans-outil', '#poste?j=' + dossier('sans-outil')),
    ('3-regles', '#regles'),
    ('4-avant-apres-d001', '#avant-apres?d=D-001'),
    ('4-avant-apres-d009', '#avant-apres?d=D-009'),
    ('5-validation', '#validation'),
    ('6-redaction', '#redaction'),
    ('7-notifications', '#notifications'),
]
erreurs = []
with sync_playwright() as p:
    nav = p.chromium.launch()
    page = nav.new_page(viewport={'width': 1500, 'height': 950})
    page.on('pageerror', lambda e: erreurs.append(str(e)))
    page.on('console', lambda m: m.type == 'error' and erreurs.append(m.text))
    page.on('response', lambda r: r.status >= 400 and erreurs.append('%d %s' % (r.status, r.url)))
    for nom, ancre in ecrans:
        page.goto(base + '/' + ancre)
        page.reload()
        page.wait_for_timeout(1500)
        page.screenshot(path='%s/%s.png' % (sortie, nom), full_page=True)
        print(nom)
        if nom == '2-poste-nominal':
            page.click('[data-suggestion]')
            page.screenshot(path='%s/2-poste-suggestion-cliquee.png' % sortie)
            print('2-poste-suggestion-cliquee')
    nav.close()
if erreurs:
    print('ERREURS', erreurs)
    sys.exit(1)
