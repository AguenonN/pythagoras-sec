'use strict';
// Écran 1 · Journal et graphe de chaîne.

(function () {
  function ligneChaineSession(j) {
    const v = j.verification;
    if (v.ok) return h('span', { class: 'petit ok-txt' }, 'Chaîne vérifiée · ' + j.entrees + ' entrées');
    if (v.cle_absente) return h('span', { class: 'petit gras', style: 'color:var(--ref)' }, 'Chaîne non vérifiable · clé absente');
    return h('span', { class: 'petit gras', style: 'color:var(--ref)' }, 'Chaîne rompue à l\'entrée #' + (j.premier_ecart + 1));
  }

  function liste(courante) {
    const toutes = toutesSessions();
    const jours = new Set(toutes.map(x => jourComplet(x.s.ouverte)));
    const titre = jours.size === 1 ? 'Sessions du ' + [...jours][0] : 'Sessions';
    return h('aside', { class: 'laterale' },
      h('div', { class: 'etiq' }, titre),
      toutes.map(x => h('button', { type: 'button', class: 'session-carte' + (x.j.id === courante.j.id && x.s.id === courante.s.id ? ' choisie' : '') + (x.j.verification.ok ? '' : ' rompue'),
        onclick: () => allerA('journal', { j: x.j.id, s: x.s.id }) },
        h('span', { class: 'gras' }, nomSession(x.s.id) + ' · ' + heureMin(x.s.ouverte)),
        h('span', { class: 'moyen second' }, x.j.id + ' · ' + x.s.appels + ' appel(s)'),
        ligneChaineSession(x.j))));
  }

  function serveurCellule(e) {
    const a = e.attestation;
    if (!a) return h('span', {}, e.serveur + ' · ', nd('attestation non disponible'));
    return h('span', {}, e.serveur + ' · ', a.valide ?
      h('span', { class: 'ok-txt' }, a.mode + ' ✓') : h('span', { class: 'non-txt' }, a.mode + ' ✗'));
  }

  function decisionCellule(e, fonde) {
    const ids = [...new Set(e.applications.filter(a => a.appliquee).map(a => a.id))];
    return h('span', { style: 'display:flex;gap:6px;align-items:center;flex-wrap:wrap' },
      badge(e.badge),
      fonde && fonde.length ? h('span', { class: 'lien-fonde' }, 'fonde ' + fonde.map(s => '#' + s).join(' ')) : null,
      !fonde && ids.length ? h('span', { class: 'second' }, ids.join(' · ')) : null,
      e.suspect ? h('span', { class: 'non-txt', style: 'color:var(--ref)' }, 'non fiable') : null);
  }

  function rang(e, choisi, fonde, onclick) {
    const cls = ['rang', 'grille-appels'];
    if (e.seq === choisi) cls.push('choisi', e.badge === 'en_attente' ? 'attente' : '');
    if (fonde && fonde.length) cls.push('fonde');
    if (e.suspect) cls.push('suspect');
    return h('div', { class: cls.join(' '), role: 'button', tabindex: '0', onclick,
      onkeydown: ev => { if (ev.key === 'Enter' || ev.key === ' ') { ev.preventDefault(); onclick(); } } },
      h('span', { class: 'num' }, e.seq),
      h('span', {}, e.sous_agent || nd('—')),
      h('span', { class: 'mono moyen' }, e.outil),
      serveurCellule(e),
      decisionCellule(e, fonde));
  }

  function carteDirective(a, liens) {
    const origine = liens.filter(l => l.directive === a.id);
    return h('div', { class: 'carte compacte' },
      h('div', { class: 'gras' }, versionTexte(a.id, a.version) + (a.texte ? ' · ' + Titre(a.texte) : '')),
      h('div', { class: 'moyen second' }, 'Fondement : ' + fondementTexte(a.fondement)),
      h('div', { class: 'moyen' }, 'Motif : ' + a.motif),
      a.appliquee ? null : h('div', { class: 'moyen second' }, 'Directive en observation : décision journalisée, jamais appliquée.'),
      origine.length ? h('div', { class: 'moyen', style: 'color:var(--accent)' },
        'Consultation à l\'origine : ' + origine.map(l => '#' + l.seq).join(', ')) : null);
  }

  // Titre abrégé : même règle que le serveur (internal/console.Titre).
  function Titre(t) {
    t = String(t).trim();
    const i = t.search(/[.;:]/);
    if (i > 0) t = t.slice(0, i);
    const r = Array.from(t);
    if (r.length > 60) {
      const cut = r.slice(0, 60).join('');
      const k = cut.lastIndexOf(' ');
      return (k > 20 ? cut.slice(0, k) : cut) + '…';
    }
    return t;
  }
  window.TitreDirective = Titre;

  function volet(v, e) {
    if (!e) return h('aside', { class: 'volet' }, h('div', { class: 'etiq' }, 'Décision'), nd('Aucun appel dans cette session.'));
    const appliquees = e.applications.filter(a => a.appliquee || a.declenchee);
    const liens = e.provenance || [];
    const roles = e.validation && e.validation.roles && e.validation.roles.length ? e.validation.roles.join(', ') : null;
    let transmission = e.transmis ? 'transmis au serveur' : (e.badge === 'en_attente' ? 'aucune (en attente)' : 'aucune (non transmis)');
    return h('aside', { class: 'volet' },
      h('div', { class: 'etiq' }, 'Décision de l\'appel #' + e.seq),
      h('div', { style: 'display:flex;align-items:center;gap:10px;flex-wrap:wrap' }, badge(e.badge, e.badge === 'en_attente' ? 'En attente de validation' : null, 'grand'),
        h('span', { class: 'moyen second' }, heureSec(e.ts) + ' · Europe/Paris')),
      appliquees.length ? appliquees.map(a => carteDirective(a, liens)) :
        h('div', { class: 'carte compacte' }, h('div', { class: 'moyen' }, e.motif), h('div', { class: 'moyen second' }, 'Aucune directive appliquée à cet appel.')),
      h('div', { style: 'display:flex;flex-direction:column;gap:6px' },
        h('div', { class: 'kv' }, h('span', {}, 'Rôle requis'), roles ? h('span', {}, roles) : nd()),
        h('div', { class: 'kv' }, h('span', {}, 'Directives évaluées'), h('span', {},
          e.evaluees + ' · ' + e.appliquees + ' appliquée(s) · ' + e.indeterminees + ' indéterminée(s)')),
        h('div', { class: 'kv' }, h('span', {}, 'Empreinte transmise'), h('span', { class: 'mono' }, court(e.empreinte_transmise) || ND)),
        h('div', { class: 'kv' }, h('span', {}, 'Entrée du journal'), h('span', { class: 'mono' }, '#' + e.numero + (e.prev ? ' · prev ' + court(e.prev) : ''))),
        h('div', { class: 'kv' }, h('span', {}, 'Transmission'), h('span', {}, transmission))),
      (e.alertes || []).map(a => h('p', { class: 'erreur moyen' }, '⚠ ' + a)),
      h('div', { style: 'margin-top:auto;display:flex;gap:10px' },
        h('a', { class: 'btn contour', style: 'flex:1', href: '#avant-apres?' + new URLSearchParams({ j: v.journal, s: v.session }) }, 'Rejouer cet appel'),
        e.badge === 'en_attente' ? h('a', { class: 'btn', style: 'flex:1', href: '#validation?' + new URLSearchParams({ j: v.journal, s: v.session, a: e.seq }) }, 'Ouvrir la validation') : null));
  }

  ECRANS.journal = async (ctx) => {
    const ch = sessionChoisie(ctx.q);
    if (!ch) { ctx.monter(aucunJournal()); return; }
    let choisi = Number(ctx.q.get('a')) || null;
    let signature = null, derniere = null;
    const racine = h('div', { class: 'corps' });
    ctx.monter(racine);
    const chemin = '/api/journal/' + encodeURIComponent(ch.j.id) + '/session/' + encodeURIComponent(ch.s.id) + '/chaine';

    function dessiner(v) {
      const fonde = {};
      for (const e of v.etapes) for (const l of e.provenance || []) (fonde[l.seq] = fonde[l.seq] || []).push(e.seq);
      if (choisi == null || !v.etapes.some(e => e.seq === choisi)) {
        const marquant = [...v.etapes].reverse().find(e => e.badge !== 'autorise' && e.badge !== 'caviarde');
        choisi = (marquant || v.etapes[v.etapes.length - 1] || {}).seq;
      }
      const e = v.etapes.find(x => x.seq === choisi);
      const dem = v.demandeur;
      ctx.droite(v.regles ? [
        'Règles en vigueur depuis le ', h('b', {}, jourMoisHeure(v.regles.depuis)), ' · Juriste : ', h('b', {}, v.regles.auteur || ND)]
        : ['Règles en vigueur : ', nd()]);
      racine.textContent = '';
      racine.append(
        liste(ch),
        h('main', { class: 'principal' },
          h('div', { style: 'display:flex;align-items:flex-end;justify-content:space-between;gap:16px;flex-wrap:wrap' },
            h('div', { style: 'display:flex;flex-direction:column;gap:4px' },
              h('h1', {}, 'Session ' + nomSession(v.session)),
              h('div', { class: 'second' }, 'Demande : ', nd('non disponible dans le journal'))),
            pastilleChaine(v.verification)),
          h('div', { class: 'chemin' },
            h('div', { class: 'col' }, h('span', { class: 'petit second' }, 'Demandeur'),
              dem ? h('span', { class: 'gras' }, dem.identite) : h('span', { class: 'gras' }, nd()),
              h('span', { class: 'petit second' }, dem && dem.groupes && dem.groupes.length ? 'groupe(s) ' + dem.groupes.join(', ') : 'rôle et groupe : ' + ND)),
            h('span', { class: 'fl' }, '→'),
            h('div', { class: 'col' }, h('span', { class: 'petit second' }, 'Orchestrateur'),
              h('span', { class: 'gras' }, v.appelant || ND),
              h('span', { class: 'petit second' }, 'identité prouvée par le transport')),
            h('span', { class: 'fl' }, '→'),
            h('div', { style: 'display:flex;gap:8px;flex-wrap:wrap' },
              v.sous_agents.length ? v.sous_agents.map(s => h('span', { class: 'pastille' }, s)) : h('span', { class: 'petit second' }, 'aucun sous-agent déclaré')),
            h('span', { class: 'petit second' }, 'sous-agents déclarés (non prouvés)')),
          h('div', { class: 'grille-appels entete-colonnes' },
            ['#', 'Sous-agent', 'Appel', 'Serveur · attestation', 'Décision'].map(t => h('span', {}, t))),
          h('div', { style: 'display:flex;flex-direction:column;gap:8px' },
            v.etapes.length ? v.etapes.map(x => rang(x, choisi, fonde[x.seq], () => { choisi = x.seq; dessiner(derniere); })) :
              h('p', { class: 'second' }, 'Aucun appel dans cette session.')),
          h('div', { class: 'petit second', style: 'display:flex;gap:20px;flex-wrap:wrap;margin-top:4px' },
            h('span', {}, 'Cadre bleu : consultation qui a fondé une décision de provenance.'),
            h('span', {}, 'Pointillés : déclaré par le code d\'orchestration, consigné, non prouvé.'),
            v.fermee ? h('span', {}, 'Session fermée : ' + v.fermeture) : h('span', {}, h('span', { class: 'vif' }), 'suivi en continu'))),
        volet(v, e));
    }

    async function rafraichir() {
      const v = await api(chemin);
      if (!ctx.vivant()) return;
      derniere = v;
      const sig = v.signature + '|' + v.verification.ok + '|' + v.verification.erreur;
      if (sig === signature) return;
      signature = sig;
      dessiner(v);
    }
    await rafraichir();
    ctx.suivre(rafraichir);
  };
})();
