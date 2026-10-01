'use strict';
// Écran 3 · Vitrine des règles.

(function () {
  const STATUT = { active: 'active', observation: 'observation', brouillon: 'brouillon', retiree: 'retirée' };
  const DEPUIS = { active: 'active depuis le ', observation: 'en observation depuis le ', brouillon: 'brouillon, effet prévu le ', retiree: 'retirée, effet depuis le ' };

  function item(d, choisie) {
    return h('a', { class: 'item-directive' + (choisie ? ' choisie' : ''), href: '#regles?directive=' + encodeURIComponent(d.id) },
      h('span', {}, h('b', {}, d.id), ' ', d.titre),
      h('span', { class: 'petit b-statut-' + d.statut }, STATUT[d.statut] || d.statut));
  }

  function tableau(c) {
    const lignes = c.decisions;
    const mal = !c.verifies;
    return h('section', { class: 'carte' },
      h('div', { style: 'display:flex;justify-content:space-between;align-items:baseline;gap:12px;flex-wrap:wrap' },
        h('div', { class: 'etiq' }, 'Décisions réelles rejouées'),
        h('div', { class: 'petit second' }, lignes.length ? 'tirées de ' + c.journaux.join(', ') + ' · ' : '',
          lignes.length ? (mal ? h('b', { style: 'color:var(--ref)' }, 'chaîne non vérifiée') : 'chaîne vérifiée') : '')),
      lignes.length ? [
        h('div', { class: 'ligne-decision entete-colonnes', style: 'border-top:0' }, ['Heure de Paris', 'Horodatage UTC', 'Accès', 'Appel', 'Décision'].map(t => h('span', {}, t))),
        lignes.map(d => h('div', { class: 'ligne-decision' },
          h('span', {}, jourSemaine(d.ts)),
          h('span', { class: 'mono petit' }, utcCourt(d.ts)),
          h('span', {}, d.acces || nd()),
          h('span', {}, d.outil, d.declenchee ? '' : h('span', { class: 'petit second' }, ' · directive examinée, non déclenchée')),
          h('span', { style: 'display:flex;gap:6px;align-items:center;flex-wrap:wrap' }, badgeEffet(d.rejoue),
            d.identique ? null : h('span', { class: 'petit gras', style: 'color:var(--ref)' }, 'écart avec le journal : ' + (EFFET_BADGE[d.journalise] ? LIBELLE_BADGE[EFFET_BADGE[d.journalise]] : d.journalise)))))
      ] : h('p', { class: 'second' }, 'Aucune décision journalisée ne concerne cette directive.'),
      lignes.some(d => d.ecarts && d.ecarts.length) ? lignes.filter(d => d.ecarts && d.ecarts.length).map(d => h('p', { class: 'erreur moyen' }, '⚠ appel n° ' + d.seq + ' : ' + d.ecarts.map(x => x.detail).join(' ; '))) : null,
      c.fuseaux ? h('div', { class: 'petit second' }, 'Mêmes résultats quel que soit le fuseau de la machine : les fuseaux sont embarqués dans l\'engine.') : null);
  }

  ECRANS.regles = async (ctx) => {
    const v = await api('/api/regles' + (ctx.q.get('directive') ? '?directive=' + encodeURIComponent(ctx.q.get('directive')) : ''));
    if (!ctx.vivant()) return;
    ctx.droite([h('b', {}, v.directives.length + (v.directives.length > 1 ? ' directives' : ' directive')), ' · en vigueur depuis le ' + jourMoisHeure(v.depuis)]);
    const c = v.choix;
    ctx.monter(h('div', { class: 'corps' },
      h('aside', { class: 'laterale large', style: 'gap:6px' },
        h('div', { class: 'etiq', style: 'padding:0 8px 8px' }, 'Directives'),
        v.directives.map(d => item(d, c && d.id === c.id))),
      h('main', { class: 'principal ample' }, c ? [
        h('div', { style: 'display:flex;align-items:flex-end;justify-content:space-between;gap:16px;flex-wrap:wrap' },
          h('div', { style: 'display:flex;flex-direction:column;gap:4px' },
            h('h1', {}, c.id + ' · ' + c.titre),
            h('div', { class: 'second' }, 'Version ' + c.version + ' · ' + (DEPUIS[c.statut] || '') + jourComplet(c.date_effet) + ' ' + heureMin(c.date_effet) + ' · auteur ' + (v.auteur || ND))),
          h('div', { class: 'moyen second' }, 'Fondement : ' + c.fondements.map(fondementTexte).join(' ; '))),
        h('div', { class: 'grille2' },
          h('section', { class: 'carte' },
            h('div', { class: 'etiq' }, 'Texte du juriste'),
            h('blockquote', { class: 'citation' }, '« ' + c.texte + ' »'),
            h('div', { class: 'moyen' }, h('span', { class: 'second' }, 'Questions du compilateur : '), nd())),
          h('section', { class: 'carte' },
            h('div', { class: 'etiq' }, 'Ce qui sera appliqué (relecture sans IA)'),
            h('p', { style: 'line-height:1.6' }, c.relecture),
            h('div', { class: 'petit second' }, 'Produit par le gabarit déterministe : même directive, même texte, octet pour octet.'))),
        tableau(c)] : h('p', { class: 'second' }, 'Aucune directive à afficher.'))));
  };
})();
