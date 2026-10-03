"""Écrit <session>/mistral/<n>.json (un fichier par réponse du modèle :
{"requete": {"messages": [...]}, "reponse": {...}}) à partir d'un enregistrement
de demo/scenarios/enregistrements/<scénario>/*.json, pour qu'une session rejouée
hors ligne porte les demandes du modèle que l'écran « Demandé / décidé » lit.

    python convertir-enregistrement.py <enregistrement.json> <dossier de session>

L'historique de la requête est reconstitué : les messages « assistant » déjà
produits par le même agent (l'enregistrement ne garde pas la requête).
"""
import json
import os
import sys

enr, session = sys.argv[1:3]
doc = json.load(open(enr, encoding='utf-8'))
sortie = os.path.join(session, 'mistral')
os.makedirs(sortie, exist_ok=True)
historique = {}
for i, r in enumerate(doc['reponses']):
    agent = r.get('agent', '')
    hist = historique.setdefault(agent, [])
    requete = {'messages': [{'role': 'system', 'content': 'agent : ' + agent}] + list(hist)}
    json.dump({'requete': requete, 'reponse': r['reponse']},
              open(os.path.join(sortie, '%02d-%s.json' % (len(doc['reponses']) - i, agent or 'agent')), 'w', encoding='utf-8'),
              ensure_ascii=False)
    for c in r['reponse'].get('choices', []):
        hist.append({'role': 'assistant', **{k: v for k, v in c['message'].items() if k in ('content', 'tool_calls')}})
        hist.append({'role': 'tool', 'content': '{}'})
print(len(doc['reponses']), 'fichiers dans', sortie)
