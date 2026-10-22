// Without / With Pythagoras : the same request, the same Mistral agents, the
// same data sources. "Without" shows a run in which no policy intervened (the
// "sans-d009" session) ; "With" shows the run under the bank's policies (the
// "avec-d009" session). Both are read from their signed journals; the path is
// played back step by step, the outcome and the engine's measured overhead
// come from the journal, never from a script.
'use strict';

(function () {
  const OUTILS = {
    'banque-synthetique/evaluer_dossier': 'Read the credit file',
    'banque-synthetique/get_taux_interne': 'Usury threshold — bank internal table',
    'banque-synthetique/attribuer_pret': 'Grant the loan',
    'banque-synthetique/consulter_client': 'Client record',
    'banque-synthetique/historique_operations': 'Transaction history',
    'recherche-entreprises/rechercher_entreprise': 'Company registry lookup',
    'gels-avoirs/verifier_gel': 'Asset-freeze register check',
    'taux-officiel/get_taux_usure': 'Usury threshold — Banque de France',
  };
  const ETAT = {
    autorise: ['Executed', 'ok'], caviarde: ['Redacted', 'cav'], en_attente: ['Held for human approval', 'att'],
    refuse: ['Blocked', 'ref'], indisponible: ['Source unavailable', 'ind'], source_non_attestee: ['Unattested source', 'non'],
    rejete: ['Rejected', 'ref'],
  };
  const PAS_MS = 380;

  const etiquette = (e) => OUTILS[e.serveur + '/' + e.outil] || (e.serveur + ' / ' + e.outil);
  const heure = (ts) => { try { return new Date(ts).toLocaleTimeString('en-GB'); } catch (x) { return ''; } };
  const sansMouvement = () => window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  // Newest request whose folder name matches, live runs before replays.
  function demandeDe(motif) {
    const js = toutesDemandes().filter(j => motif.test(j.id));
    return js.find(j => !/-rejeu$/.test(j.id)) || js[0] || null;
  }

  function compteur(valeur, format) {
    const n = h('b', { class: 'cp-chiffre' }, format(0));
    if (sansMouvement()) { n.textContent = format(valeur); return n; }
    const debut = performance.now(), duree = 900;
    const pas = (t) => {
      const k = Math.min(1, (t - debut) / duree);
      n.textContent = format(valeur * (1 - Math.pow(1 - k, 3)));
      if (k < 1 && n.isConnected) requestAnimationFrame(pas);
    };
    requestAnimationFrame(pas);
    return n;
  }
  const us = (x) => x >= 1000 ? (x / 1000).toFixed(1) + ' ms' : Math.round(x) + ' µs';

  function etapeVue(e, i, avec) {
    const [lib, cls] = ETAT[e.badge] || [e.badge, 'ind'];
    const regle = (e.applications || []).filter(a => a.appliquee).map(a => a.id).join(', ');
    return h('li', { class: 'cp-etape', style: 'animation-delay:' + (i * PAS_MS) + 'ms' },
      h('span', { class: 'cp-point ' + (avec ? cls : (cls === 'ind' ? 'ind' : 'ok')) }),
      h('div', { class: 'cp-etape-corps' },
        h('div', {}, h('span', { class: 'cp-agent' }, e.sous_agent || 'agent'), ' ', h('b', {}, etiquette(e))),
        h('div', { class: 'petit second' }, heure(e.ts) + ' · ' + e.serveur)),
      h('div', { class: 'cp-etape-droite' },
        h('span', { class: 'cp-badge ' + (avec ? cls : (cls === 'ind' ? 'ind' : 'ok')) }, avec ? lib + (regle ? ' · ' + regle : '') : (cls === 'ind' ? lib : 'Executed')),
        avec && e.duree_us ? h('span', { class: 'petit second' }, '+' + us(e.duree_us) + ' engine') : null));
  }

  function issue(v, avec) {
    const etapes = v.etapes || [];
    const prets = etapes.filter(e => e.outil === 'attribuer_pret');
    const officiel = etapes.some(e => e.serveur === 'taux-officiel' && e.badge === 'autorise');
    const interne = etapes.some(e => e.outil === 'get_taux_interne');
    const attente = prets.find(e => e.badge === 'en_attente');
    const accorde = prets.find(e => e.transmis);
    const valideur = accorde && accorde.valide_par && accorde.valide_par[0];
    if (!avec) {
      return h('div', { class: 'cp-issue mauvais' },
        h('div', { class: 'etiq' }, 'Outcome'),
        h('h2', {}, accorde ? 'Loan granted automatically' : (prets.length ? 'Loan attempted' : 'No loan decision')),
        h('ul', { class: 'cp-faits' },
          h('li', {}, interne ? 'Decided on the bank\'s internal rate table' : 'Internal rate table not read'),
          h('li', {}, officiel ? 'Official usury threshold checked' : 'Official usury threshold (Banque de France) never checked'),
          h('li', {}, 'No human in the loop · no rule intervened')));
    }
    const c = v.cout || {};
    return h('div', { class: 'cp-issue bon' },
      h('div', { class: 'etiq' }, 'Outcome'),
      h('h2', {}, attente ? 'Held, then approved by a risk analyst' : (accorde ? 'Loan granted under policy' : 'Loan blocked by policy')),
      h('ul', { class: 'cp-faits' },
        attente ? h('li', {}, 'Rule ' + ((attente.applications || []).filter(a => a.appliquee).map(a => a.id).join(', ') || '—') + ': internal rates used without the official threshold → human approval required') : null,
        valideur ? h('li', {}, 'Approved by ' + (valideur.nom || valideur.identite) + (c.attentes_mesurees ? ' after ' + c.attente_secondes + ' s' : '')) : null,
        h('li', {}, 'Every call and every data read signed in the journal · replayable months later')),
      h('div', { class: 'cp-chiffres' },
        h('div', {}, compteur(c.moyenne_us || 0, us), h('span', {}, 'added by the engine per call')),
        h('div', {}, compteur(c.appels || 0, x => String(Math.round(x))), h('span', {}, 'tool calls checked')),
        h('div', {}, compteur(c.validations || 0, x => String(Math.round(x))), h('span', {}, 'held for approval')),
        h('div', {}, compteur(c.refuses || 0, x => String(Math.round(x))), h('span', {}, 'blocked'))));
  }

  function ecran(avec) {
    return async function (ctx) {
      if (!etat.journaux || !etat.journaux.length) return ctx.monter(aucunJournal());
      const j = (ctx.q.get('j') && etat.journaux.find(x => x.id === ctx.q.get('j'))) ||
        demandeDe(avec ? /avec-d009/ : /sans-d009/);
      if (!j) return ctx.monter(h('div', { class: 'principal ample' }, h('div', { class: 'carte' },
        h('p', {}, 'No ' + (avec ? '“with policy”' : '“without policy”') + ' run found. Run: scripts/lancer-demo.sh scenario base-interne-' + (avec ? 'avec' : 'sans') + '-d009'))));
      const x = choixDe(j, ctx.q.get('s'));
      const v = await api(cheminJournal(x, 'poste'));
      if (!ctx.vivant()) return;
      const replay = /-rejeu$/.test(j.id);
      const etapes = v.etapes || [];
      const fin = etapes.length * PAS_MS;
      const page = h('div', { class: 'principal ample cp' },
        h('div', { class: 'cp-tete' },
          h('div', {},
            h('div', { class: 'cp-bascule' },
              h('a', { href: '#sans', class: avec ? '' : 'actif' }, 'Without Pythagoras'),
              h('a', { href: '#avec', class: avec ? 'actif' : '' }, 'With Pythagoras')),
            h('h1', { class: 'cp-titre' }, avec ? 'With Pythagoras' : 'Without Pythagoras'),
            h('div', { class: 'second' }, avec
              ? 'The same agents call their tools through the Pythagoras gateway: every call is checked against the bank\'s rules before it leaves.'
              : 'Mistral agents call the bank\'s tools directly. Nothing checks which data the decision rests on.')),
          h('div', { style: 'display:flex;flex-direction:column;align-items:flex-end;gap:8px' },
            h('span', { class: replay ? 'ag-etat replay' : 'ag-etat live' }, replay ? 'Replay of a recorded run' : 'Live run · Mistral'),
            h('button', { class: 'btn contour', onclick: () => ECRANS[ctx.ecran](ctx) }, '↻ Replay the path'))),
        h('div', { class: 'carte cp-demande' },
          h('div', { class: 'etiq' }, 'Request from a bank advisor'),
          h('p', { class: 'cp-citation' }, v.demande || '—')),
        h('div', { class: 'cp-grille' },
          h('div', { class: 'carte' },
            h('div', { class: 'etiq' }, 'Agent path · ' + etapes.length + ' tool calls'),
            h('div', { class: 'cp-barre' }, h('span', { style: 'animation-duration:' + fin + 'ms' })),
            h('ol', { class: 'cp-chemin' }, etapes.map((e, i) => etapeVue(e, i, avec)))),
          h('div', { class: 'cp-droite', style: 'animation-delay:' + fin + 'ms' }, issue(v, avec),
            v.reponse && v.reponse.texte ? h('details', { class: 'carte' },
              h('summary', { class: 'etiq' }, 'Answer returned to the advisor (by the Mistral orchestrator)'),
              h('p', { class: 'cp-reponse' }, v.reponse.texte)) : null,
            avec ? null : h('p', { class: 'petit second' }, 'How this page is produced: the same request run through the gateway with the bank\'s rule removed, ',
              'so no policy intervened. A plain agent stack would not even keep this journal.'))));
      ctx.monter(page);
    };
  }
  ECRANS['sans'] = ecran(false);
  ECRANS['avec'] = ecran(true);
})();
