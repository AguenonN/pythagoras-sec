'use strict';
// Écran 4 · Avant / après la directive du juriste.

(function () {
  function rang(l, choix) {
    const diff = l.change;
    return h('div', { class: 'rang-aa grille-aa' + (diff ? ' diff' : '') },
      h('span', { class: 'num mono second' }, l.seq),
      h('span', { class: 'mono moyen' + (diff ? ' gras' : '') }, l.outil, h('span', { class: 'second petit' }, ' · ' + l.serveur)),
      h('span', {}, badgeEffet(l.avant)),
      h('span', { style: diff ? 'color:var(--cav);font-weight:600;font-size:18px' : 'color:var(--discret)' }, diff ? '→' : '='),
      h('span', { style: 'display:flex;gap:8px;align-items:center;flex-wrap:wrap' }, badgeEffet(l.apres),
        diff ? h('span', { class: 'moyen' }, 'par ' + choix.id) : null,
        diff && l.motif_apres ? h('span', { class: 'petit second' }, motifAffiche(l.motif_apres)) : null),
      (l.ecarts || []).length ? h('span', { class: 'petit erreur', style: 'grid-column:1/-1' }, '⚠ ' + l.ecarts.map(x => x.detail).join(' ; ')) : null);
  }

  function carte(titre, contenu) {
    return h('section', { class: 'carte', style: 'gap:8px' }, h('div', { class: 'etiq' }, titre), contenu);
  }

  ECRANS['avant-apres'] = async (ctx) => {
    const ch = sessionChoisie(ctx.q);
    if (!ch) { ctx.monter(aucunJournal()); return; }
    const params = new URLSearchParams({ journal: ch.j.id, session: ch.s.id });
    if (ctx.q.get('d')) params.set('directive', ctx.q.get('d'));
    const v = await api('/api/avant-apres?' + params);
    if (!ctx.vivant()) return;
    const c = v.choix;
    const cle = (o) => o.id + '#' + o.version;
    const select = v.options.length ? h('select', { class: 'champ', id: 'directive', onchange: e => allerA('avant-apres', { j: ch.j.id, s: ch.s.id, d: e.target.value }) },
      v.options.map(o => h('option', { value: cle(o), selected: c && cle(o) === cle(c) }, o.id + ' · ' + o.titre + (o.avant === 'Sans la directive' ? '' : ' (' + o.apres.replace('Avec ', '') + ')')))) : null;
    const nonCascade = 'aucun effet en cascade';
    ctx.monter(h('div', { class: 'corps' }, h('main', { class: 'principal ample' },
      h('div', { style: 'display:flex;align-items:flex-end;justify-content:space-between;gap:24px;flex-wrap:wrap' },
        h('div', { style: 'display:flex;flex-direction:column;gap:6px' },
          h('h1', {}, 'La même exécution, avant et après la directive du juriste'),
          h('div', { class: 'second' }, 'Session ' + nomSession(v.session) + ' du ' + jourComplet(v.ouverte) + ' ' + heureMin(v.ouverte) + ' · rejouée à sa date d\'origine · ' + nonCascade)),
        h('div', { style: 'display:flex;gap:12px;align-items:center;flex-wrap:wrap' },
          h('label', { class: 'moyen second', for: 'directive' }, 'Directive du juriste comparée'), select,
          selecteurSession('avant-apres', ch))),
      pastilleChaine(v.verification),
      !c ? h('div', { class: 'carte' }, h('p', { class: 'second' }, v.options.length ? 'Aucune directive à comparer pour cette session.' :
        'Aucune autre directive à comparer : les règles disponibles ne diffèrent pas de celles que cette session a appliquées.')) : [
        h('div', { class: 'grille-aa entete-colonnes', style: 'padding:0 16px' },
          ['#', 'Appel', c.avant, '', c.apres].map(t => h('span', {}, t))),
        h('div', { style: 'display:flex;flex-direction:column;gap:8px' }, v.lignes.map(l => rang(l, c))),
        h('div', { class: 'grille3', style: 'margin-top:8px' },
          carte('Ce qui s\'est passé ' + c.avant.charAt(0).toLowerCase() + c.avant.slice(1), h('p', { style: 'line-height:1.55' }, v.explication)),
          carte('Pourquoi c\'est un risque', h('p', { class: 'second', style: 'line-height:1.55' }, nd('non disponible : l\'analyse de l\'Analyste n\'est pas encore reliée'))),
          carte('Qualification de l\'écart', h('p', { style: 'line-height:1.55' }, h('b', {}, 'Exposition'), ', pas violation : cette directive n\'était pas dans les règles en vigueur à la date de l\'appel (ou y figurait sous une autre version). Une violation n\'est constatée qu\'au rejeu sous les règles d\'origine, à la date d\'origine.'))),
        h('div', { class: 'carte', style: 'margin-top:auto;flex-direction:row;align-items:center;justify-content:space-between;flex-wrap:wrap' },
          h('div', { style: 'display:flex;gap:32px;flex-wrap:wrap' },
            [[v.sessions, v.sessions > 1 ? 'sessions rejouées' : 'session rejouée'],
             [v.changes, v.changes > 1 ? 'appels dont l\'effet change' : 'appel dont l\'effet change'],
             [v.non_rejouables, v.non_rejouables > 1 ? 'appels non rejouables' : 'appel non rejouable']].map(([n, t]) =>
              h('div', { style: 'display:flex;flex-direction:column' }, h('span', { class: 'chiffre-grand' }, n), h('span', { class: 'moyen second' }, t)))),
          h('a', { class: 'btn', href: '#redaction' }, 'Voir la proposition de l\'Analyste (le juriste décide)'))])));
  };
})();
