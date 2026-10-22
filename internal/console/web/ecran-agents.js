// Live agents : the Mistral orchestrator at the centre, the sub-agents it
// delegates to, and the data sources (MCP servers) they call, drawn from the
// signed journal of the selected request and refreshed every 2 s. Every edge
// is a call the engine actually decided; nothing is animated that the
// journal does not contain. A session whose folder ends in "-rejeu" is a
// replay of recorded model answers and is labelled as such.
'use strict';

(function () {
  const SVG = 'http://www.w3.org/2000/svg';
  const W = 1000, HT = 640, CX = 500, CY = 320;

  // Data sources of the demo registry. real: public data fetched from the
  // official source (or recorded from it); otherwise fictional.
  const SOURCES = {
    'gels-avoirs': ['Asset-freeze register', 'DG Trésor · public API', true],
    'recherche-entreprises': ['Company registry', 'api.gouv.fr · public API', true],
    'taux-officiel': ['Usury thresholds', 'Banque de France · real figures, previous quarter', true],
    'legifrance': ['Légifrance', 'French law · PISTE API', true],
    'eurlex': ['EUR-Lex', 'EU law · Publications Office', true],
    'banque-synthetique': ['Bank core system', 'synthetic · fictional clients', false],
    'data-room': ['M&A data room', 'synthetic · project ATLAS', false],
    'messagerie': ['Messaging', 'synthetic', false],
    'bourse': ['Securities account', 'synthetic', false],
    'communication': ['Press releases', 'synthetic', false],
    'rh-synthetique': ['HR system', 'synthetic · fictional employees', false],
    'referentiel-rh': ['HR directory', 'synthetic', false],
  };
  const AGENTS = {
    'analyse-dossier': 'reads the credit file',
    'conformite': 'company + asset-freeze checks',
    'credit': 'decides the loan (official rate)',
    'credit-historique': 'legacy loan tool (internal rates)',
    'lcb-ft': 'anti-money-laundering',
    'operations': 'client accounts and transfers',
    'redaction': 'writes the report',
    'rh': 'HR requests', 'rh-gestion': 'HR management', 'rh-disciplinaire': 'disciplinary procedure',
    'equipe-deal': 'M&A deal team', 'assistant-salarie': 'employee assistant',
  };
  const DECISION = {
    autorise: ['Allowed', 'var(--ok)'], caviarde: ['Redacted', 'var(--cav)'], en_attente: ['Awaiting human approval', 'var(--att)'],
    refuse: ['Blocked', 'var(--ref)'], indisponible: ['Source unavailable', 'var(--ind)'], source_non_attestee: ['Unattested source', 'var(--non)'],
    rejete: ['Rejected', 'var(--ref)'],
  };
  const decision = (b) => DECISION[b] || [b || '—', 'var(--ind)'];

  function s(tag, attrs, ...enfants) {
    const n = document.createElementNS(SVG, tag);
    for (const [k, v] of Object.entries(attrs || {})) if (v != null) n.setAttribute(k, v);
    for (const e of enfants.flat()) if (e != null) n.append(typeof e === 'string' ? document.createTextNode(e) : e);
    return n;
  }
  const autour = (i, n, r, depart) => {
    const a = (depart || -Math.PI / 2) + (2 * Math.PI * i) / Math.max(n, 1);
    return [CX + r * Math.cos(a), CY + r * Math.sin(a) * 0.86];
  };
  const heure = (ts) => { try { return new Date(ts).toLocaleTimeString('en-GB'); } catch (e) { return '—'; } };

  function noeud(x, y, w, titre, sous, opts) {
    const o = opts || {};
    const g = s('g', { class: 'ag-noeud' + (o.classe ? ' ' + o.classe : ''), transform: 'translate(' + x + ',' + y + ')' });
    g.append(s('rect', { x: -w / 2, y: -24, width: w, height: 48, rx: 10,
      fill: o.fond || 'var(--surface)', stroke: o.bord || 'var(--trait)', 'stroke-width': o.epais || 1.2,
      'stroke-dasharray': o.pointilles ? '4 3' : null }));
    g.append(s('text', { y: sous ? -4 : 5, 'text-anchor': 'middle', 'font-size': 13, 'font-weight': 600, fill: o.couleur || 'var(--texte)' }, titre));
    if (sous) g.append(s('text', { y: 13, 'text-anchor': 'middle', 'font-size': 10.5, fill: 'var(--second)' }, sous));
    return g;
  }

  function dessiner(v, replay, nouvelle) {
    const etapes = v.etapes || [];
    const agents = [...new Set([...(v.sous_agents || []), ...etapes.map(e => e.sous_agent).filter(Boolean)])];
    const appeles = new Set(etapes.map(e => e.serveur));
    const serveurs = [...new Set([...Object.keys(SOURCES), ...appeles])];
    const posA = {}, posS = {};
    agents.forEach((a, i) => { posA[a] = autour(i, agents.length, 175); });
    serveurs.forEach((n, i) => { posS[n] = autour(i, serveurs.length, 405, -Math.PI / 2 + Math.PI / serveurs.length); });

    // Last decision per (agent, server) pair, and call count.
    const paires = new Map();
    for (const e of etapes) {
      const cle = (e.sous_agent || '?') + '|' + e.serveur;
      const p = paires.get(cle) || { agent: e.sous_agent || '?', serveur: e.serveur, n: 0 };
      p.n++; p.badge = e.badge; p.seq = e.seq;
      paires.set(cle, p);
    }
    const derniere = etapes[etapes.length - 1];

    const svg = s('svg', { viewBox: '0 0 ' + W + ' ' + HT, width: '100%', role: 'img', class: 'ag-carte',
      'aria-label': 'Orchestrator, sub-agents and data sources of this request' });
    // Edges orchestrator -> sub-agents (delegations).
    for (const a of agents) {
      const [x, y] = posA[a];
      svg.append(s('line', { x1: CX, y1: CY, x2: x, y2: y, stroke: 'var(--accent)', 'stroke-width': 1.6, 'stroke-opacity': 0.55 }));
    }
    // Edges sub-agent -> data source (calls decided by the engine).
    for (const p of paires.values()) {
      const de = posA[p.agent] || [CX, CY], a = posS[p.serveur];
      if (!a) continue;
      const [lib, coul] = decision(p.badge);
      const actif = derniere && p.seq === derniere.seq;
      svg.append(s('line', { x1: de[0], y1: de[1], x2: a[0], y2: a[1], stroke: coul, 'stroke-width': actif ? 3.2 : 2,
        'stroke-opacity': actif ? 1 : 0.7, class: actif && nouvelle ? 'ag-pulse' : null },
        s('title', {}, p.agent + ' → ' + p.serveur + ' · ' + p.n + ' call(s) · last: ' + lib)));
      const mx = (de[0] + a[0]) / 2, my = (de[1] + a[1]) / 2;
      if (p.n > 1) svg.append(s('text', { x: mx, y: my - 4, 'text-anchor': 'middle', 'font-size': 10.5, fill: coul, 'font-weight': 600 }, '×' + p.n));
    }
    // Data sources.
    for (const n of serveurs) {
      const [x, y] = posS[n];
      const [titre, detail, reel] = SOURCES[n] || [n, '', false];
      const utilise = appeles.has(n);
      const g = noeud(x, y, 168, titre, detail, {
        bord: utilise ? (reel ? 'var(--ok)' : 'var(--accent)') : 'var(--trait-doux)',
        epais: utilise ? 2 : 1, pointilles: !reel, couleur: utilise ? 'var(--texte)' : 'var(--discret)',
        fond: utilise ? 'var(--surface)' : 'var(--lateral)' });
      g.append(s('title', {}, n + (reel ? ' · real public data' : ' · fictional data') + (utilise ? ' · called in this request' : ' · not called')));
      svg.append(g);
    }
    // Sub-agents.
    for (const a of agents) {
      const [x, y] = posA[a];
      const actif = derniere && derniere.sous_agent === a;
      svg.append(noeud(x, y, 150, a, AGENTS[a] || 'sub-agent', { bord: actif ? 'var(--accent)' : 'var(--trait-champ)', epais: actif ? 2.4 : 1.2,
        classe: actif && nouvelle ? 'ag-pulse' : null }));
    }
    // Orchestrator.
    svg.append(noeud(CX, CY, 210, 'Mistral orchestrator', replay ? 'recorded model answers (replay)' : 'mistral-medium · live',
      { fond: 'var(--accent)', bord: 'var(--accent-fonce)', couleur: '#fff', epais: 2 }));
    return svg;
  }

  function fil(v) {
    const etapes = [...(v.etapes || [])].reverse();
    return h('div', { class: 'ag-fil' }, etapes.length ? etapes.map(e => {
      const [lib, coul] = decision(e.badge);
      return h('div', { class: 'ag-ligne' },
        h('span', { class: 'mono petit second' }, heure(e.ts)),
        h('div', {},
          h('div', {}, h('b', {}, e.sous_agent || '—'), ' → ', h('span', { class: 'mono' }, e.serveur + ' / ' + e.outil)),
          h('div', { class: 'petit', style: 'color:' + coul + ';font-weight:600' }, lib),
          e.motif && e.badge !== 'autorise' ? h('div', { class: 'petit second' }, e.motif) : null));
    }) : h('p', { class: 'second' }, 'No tool call yet.'));
  }

  ECRANS['agents'] = async function (ctx) {
    if (!etat.journaux || !etat.journaux.length) return ctx.monter(aucunJournal());
    let choix = sessionChoisie(ctx.q);
    let signature = null;
    const charger = async () => {
      if (!ctx.q.get('j')) { // follow the newest request, including one started after this page was opened
        etat = await api('/api/etat');
        if (!ctx.vivant() || !etat.journaux || !etat.journaux.length) return;
        choix = sessionChoisie(ctx.q);
      }
      const v = await api(cheminJournal(choix, 'chaine'));
      if (!ctx.vivant() || v.signature === signature) return;
      const nouvelle = signature !== null && signature.split(':')[0] !== String(v.signature).split(':')[0];
      signature = v.signature;
      const replay = /-rejeu$/.test(choix.j.id);
      const ok = v.verification && v.verification.ok;
      ctx.monter(h('div', { class: 'principal ample' },
        h('div', { style: 'display:flex;align-items:flex-end;justify-content:space-between;gap:16px;flex-wrap:wrap' },
          h('div', { style: 'display:flex;flex-direction:column;gap:6px' },
            h('h1', {}, 'Live agents'),
            h('div', { class: 'second' }, (v.demande ? '“' + v.demande + '”' : 'Request ' + choix.j.id))),
          h('div', { style: 'display:flex;gap:10px;align-items:center;flex-wrap:wrap' },
            h('span', { class: replay ? 'ag-etat replay' : 'ag-etat live' }, replay ? 'Replay of a recorded run' : (v.fermee ? 'Live run · finished' : '● Live run')),
            h('span', { class: ok ? 'chaine' : 'chaine rompue' }, ok ? 'HMAC chain verified' : 'Chain not verified'),
            selecteurSession('agents', choix))),
        h('div', { class: 'ag-grille' },
          h('div', { class: 'carte', style: 'padding:8px' }, dessiner(v, replay, nouvelle),
            h('div', { class: 'petit second', style: 'padding:0 12px 6px' },
              'Each line is a tool call decided by the policy engine and written to the signed journal. ',
              'Solid green border: real public data source · dashed: fictional data · grey: available, not called.')),
          h('div', { class: 'carte' }, h('div', { class: 'etiq' }, 'Calls, newest first · ' + (v.etapes || []).length), fil(v)))));
    };
    await charger();
    ctx.suivre(charger);
  };
})();
