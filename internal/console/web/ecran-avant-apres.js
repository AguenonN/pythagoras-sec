'use strict';
// Écran 4 · Avant / après une directive : les sessions choisies rejouées avec
// les règles telles qu'elles sont, et avec ces mêmes règles privées de la
// directive choisie.

(function () {
  const CHAMPS = { effet: 'décision', directives: 'directives appliquées', roles: 'rôles requis', notifications: 'notifications', champs: 'champs retirés' };

  function cote(c, diff, nom) {
    const lignes = [
      h('div', {}, c.effet === 'non_rejouable' ? badge('indisponible', 'Non rejouable') : badgeEffet(c.effet))];
    const ligne = (cle, titre, valeur) => h('div', { class: 'petit' + (diff.includes(cle) ? ' diff-cell' : ' second') }, titre + ' : ' + (valeur || 'aucun'));
    if (c.effet !== 'non_rejouable') {
      lignes.push(ligne('directives', 'Directives', (c.directives || []).join(', ')));
      lignes.push(ligne('roles', 'Rôles requis', (c.roles || []).join(', ') + (c.quatre_yeux ? ' (quatre yeux)' : '')));
      lignes.push(ligne('notifications', 'Notifications', (c.notifications || []).join(' ; ')));
      lignes.push(ligne('champs', 'Champs retirés', (c.champs_retires || []).join(', ')));
    }
    return h('div', { style: 'display:flex;flex-direction:column;gap:4px' }, lignes);
  }

  function rang(l) {
    return h('div', { class: 'rang-aa grille-aa2' + (l.change ? ' diff' : '') },
      h('span', { class: 'num mono second' }, l.seq),
      h('div', { style: 'display:flex;flex-direction:column;gap:2px' },
        h('span', { class: 'mono moyen' + (l.change ? ' gras' : '') }, l.outil),
        h('span', { class: 'second petit' }, l.serveur + ' · ' + nomSession(l.session) + ' · ' + heureMin(l.ts)),
        l.change ? h('span', { class: 'petit gras', style: 'color:var(--cav)' },
          'Ce qui change : ' + l.differences.map(d => CHAMPS[d] || d).join(', ') + ' · ' + (l.qualification === 'violation' ? 'violation' : 'exposition')) : null),
      cote(l.avant, l.differences), cote(l.apres, l.differences),
      (l.ecarts || []).length ? h('span', { class: 'petit erreur', style: 'grid-column:1/-1' }, '⚠ ' + l.ecarts.map(x => x.detail).join(' ; ')) : null);
  }

  const carte = (titre, contenu) => h('section', { class: 'carte', style: 'gap:8px' }, h('div', { class: 'etiq' }, titre), contenu);

  ECRANS['avant-apres'] = async (ctx) => {
    const params = new URLSearchParams();
    if (ctx.q.get('d')) params.set('directive', ctx.q.get('d'));
    // Sélection : paramètre « s » (une session, « journal|session ») ou toutes.
    const choisies = ctx.q.get('sel') ? ctx.q.get('sel').split(',') : (ctx.q.get('s') ? [ctx.q.get('s')] : []);
    const tout = ctx.q.get('tout') === '1';
    if (choisies.length) params.set('sessions', choisies.join(','));
    const v = await api('/api/avant-apres?' + params);
    if (!ctx.vivant()) return;
    const c = v.choix, k = v.compteurs;
    const cle = (s) => s.journal + '|' + s.session;
    const aller = (d, sel) => allerA('avant-apres', Object.assign({ d }, sel.length ? { sel: sel.join(',') } : {}));
    const select = h('select', { class: 'champ', id: 'directive', onchange: e => aller(e.target.value, choisies) },
      v.directives.map(o => h('option', { value: o.id, selected: c && o.id === c.id }, o.id + ' · ' + o.titre)));
    const cases = h('div', { class: 'cases' }, v.sessions.map(s => h('label', {},
      h('input', { type: 'checkbox', checked: s.choisie, onchange: () => {
        const sel = new Set(v.sessions.filter(x => x.choisie).map(cle));
        if (sel.has(cle(s))) sel.delete(cle(s)); else sel.add(cle(s));
        aller(c ? c.id : '', [...sel]);
      } }), nomSession(s.session) + ' · ' + heureMin(s.ouverte) + (s.demande ? ' · ' + s.demande.slice(0, 40) : ''))));
    ctx.monter(h('div', { class: 'corps' }, h('main', { class: 'principal ample' },
      h('div', { style: 'display:flex;align-items:flex-end;justify-content:space-between;gap:24px;flex-wrap:wrap' },
        h('div', { style: 'display:flex;flex-direction:column;gap:6px' },
          h('h1', {}, 'La même exécution, avant et après la directive du juriste'),
          h('div', { class: 'second' }, 'Sessions rejouées à leur date d\'origine, avec les règles en vigueur et avec ces mêmes règles privées de la directive choisie.')),
        h('div', { style: 'display:flex;gap:12px;align-items:center;flex-wrap:wrap' },
          h('label', { class: 'moyen second', for: 'directive' }, 'Directive comparée'), select)),
      v.sessions.length ? h('div', { class: 'carte compacte' }, h('div', { class: 'etiq' }, 'Sessions à rejouer'), cases) : null,
      h('div', { class: v.verifiees ? 'chaine' : 'chaine rompue', style: 'align-self:flex-start' }, v.verifiees ? 'Chaînes des sessions rejouées vérifiées' : 'Une chaîne n\'est pas vérifiée : ces rejeux ne prouvent rien'),
      !c ? h('div', { class: 'carte' }, h('p', { class: 'second' }, 'Aucune directive à comparer : les règles en vigueur sont illisibles ou vides.')) : [
        h('div', { class: 'grille-aa2 entete-colonnes', style: 'padding:0 16px' },
          ['#', 'Appel', v.libelle_avant, v.libelle_apres].map(t => h('span', { class: 'gras' }, t))),
        h('div', { class: 'petit second' }, tout ? 'Tous les appels rejoués. ' : 'Seuls les appels dont la décision change sont montrés. ',
          h('a', { href: '#' + location.hash.replace(/^#/, '').split('&tout=1')[0] + (tout ? '' : '&tout=1') }, tout ? 'Ne montrer que ceux qui changent' : 'Montrer tous les appels')),
        v.lignes.length ? h('div', { style: 'display:flex;flex-direction:column;gap:8px' }, v.lignes.filter(l => tout || l.change).map(rang)) : h('p', { class: 'second' }, 'Aucun appel dans les sessions choisies.'),
        h('div', { class: 'grille3', style: 'margin-top:8px' },
          carte('Ce qui s\'est passé ' + v.libelle_avant.charAt(0).toLowerCase() + v.libelle_avant.slice(1), h('p', { style: 'line-height:1.55' }, v.explication)),
          carte('Pourquoi c\'est un risque', h('p', { class: 'second', style: 'line-height:1.55' }, nd('non disponible : l\'analyse de l\'Analyste n\'est pas reliée à cet écran'))),
          carte('Qualification de l\'écart', h('p', { style: 'line-height:1.55' },
            h('b', {}, k.exposition + (k.exposition > 1 ? ' expositions' : ' exposition')), ' : la directive est postérieure à l\'appel. ',
            h('b', {}, k.violation + (k.violation > 1 ? ' violations' : ' violation')), ' : la directive était en vigueur à la date de l\'appel.'))),
        h('div', { class: 'carte', style: 'margin-top:auto;flex-direction:row;align-items:center;justify-content:space-between;flex-wrap:wrap' },
          h('div', { style: 'display:flex;gap:32px;flex-wrap:wrap' },
            [[k.sessions, k.sessions > 1 ? 'sessions rejouées' : 'session rejouée'],
             [k.appels, k.appels > 1 ? 'appels rejoués' : 'appel rejoué'],
             [k.changes, k.changes > 1 ? 'appels dont la décision change' : 'appel dont la décision change'],
             [k.non_rejouables, k.non_rejouables > 1 ? 'appels non rejouables' : 'appel non rejouable']].map(([n, t]) =>
              h('div', { style: 'display:flex;flex-direction:column' }, h('span', { class: 'chiffre-grand' }, n), h('span', { class: 'moyen second' }, t)))),
          h('a', { class: 'btn', href: '#redaction' }, 'Écrire ou reprendre une directive'))])));
  };
})();
