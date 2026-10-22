'use strict';
// Écran 3 · Vitrine des règles.

(function () {
  const STATUT = { active: 'active', observation: 'in observation', brouillon: 'brouillon', retiree: 'withdrawn' };
  const DEPUIS = { active: 'active since ', observation: 'in observation since ', brouillon: 'draft, effective on ', retiree: 'withdrawn, effective since ' };
  // Transitions que la console propose ; l'engine de la console les revérifie.
  const ACTIONS = {
    brouillon: [['observation', 'Put in observation'], ['active', 'Activate']],
    observation: [['active', 'Activate'], ['retiree', 'Withdraw']],
    active: [['retiree', 'Withdraw']],
    retiree: [],
  };

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
        h('div', { class: 'etiq' }, 'Actual decisions produced by this rule'),
        h('div', { class: 'petit second' }, lignes.length ? 'drawn from ' + c.journaux.length + ' session(s) · ' : '',
          lignes.length ? (mal ? h('b', { style: 'color:var(--ref)' }, 'chain not verified') : 'chains verified') : '')),
      lignes.length ? [
        c.nb_decisions > lignes.length ? h('p', { class: 'petit second' }, lignes.length + ' decisions shown out of ' + c.nb_decisions + '.') : null,
        h('div', { class: 'ligne-decision entete-colonnes', style: 'border-top:0' }, ['Paris time', 'UTC timestamp', 'Access', 'Call', 'Decision'].map(t => h('span', {}, t))),
        lignes.map(d => h('div', { class: 'ligne-decision' },
          h('span', {}, jourSemaine(d.ts)),
          h('span', { class: 'mono petit' }, utcCourt(d.ts)),
          h('span', {}, d.acces || nd()),
          h('span', {}, outilLisible(d.serveur, d.outil), ' ', h('a', { class: 'petit', href: '#journal?' + new URLSearchParams({ j: d.journal, s: d.session, a: d.seq }) }, 'voir'), d.declenchee ? '' : h('span', { class: 'petit second' }, ' · evaluated, not triggered')),
          h('span', { style: 'display:flex;gap:6px;align-items:center;flex-wrap:wrap' }, badgeEffet(d.rejoue),
            d.identique ? null : h('span', { class: 'petit gras', style: 'color:var(--ref)' }, 'gap with the journal: ' + (EFFET_BADGE[d.journalise] ? LIBELLE_BADGE[EFFET_BADGE[d.journalise]] : d.journalise)))))
      ] : h('p', { class: 'second' }, 'No journaled decision concerns this rule.'),
      lignes.filter(d => d.ecarts && d.ecarts.length).map(d => h('p', { class: 'erreur moyen' }, '⚠ call #' + d.seq + ' : ' + d.ecarts.map(x => x.detail).join(' ; '))),
      c.fuseaux ? h('div', { class: 'petit second' }, 'Same results whatever the machine\'s time zone: time zones are embedded in the engine.') : null);
  }

  ECRANS.regles = async (ctx) => {
    const v = await api('/api/regles' + (ctx.q.get('directive') ? '?directive=' + encodeURIComponent(ctx.q.get('directive')) : ''));
    if (!ctx.vivant()) return;
    ctx.droite([h('b', {}, v.directives.length + (v.directives.length > 1 ? ' directives' : ' directive')), ' · rules in force since ' + jourMoisHeure(v.depuis)]);
    const c = v.choix;
    const err = zoneErreur(), ok = h('p', { class: 'ok-txt gras', hidden: true });
    function actions() {
      return (ACTIONS[c.statut] || []).map(([vers, nom]) => h('button', { type: 'button', class: 'btn ' + (vers === 'retiree' ? 'danger' : 'contour'), onclick: async () => {
        err.dire(''); ok.hidden = true;
        if (!window.confirm(nom + ' rule ' + c.id + '? The rules will move to the next version.')) return;
        try {
          const r = await post('/api/regles/statut', { directive: c.id, statut: vers });
          ok.hidden = false; ok.textContent = nom + ': done. Rules in force, version ' + r.version + '.';
          setTimeout(() => router(), 800);
        } catch (e) { err.dire(e.message); }
      } }, nom));
    }
    ctx.monter(h('div', { class: 'corps' },
      h('aside', { class: 'laterale large', style: 'gap:6px' },
        h('div', { class: 'etiq', style: 'padding:0 8px 8px' }, 'Rules'),
        v.directives.map(d => item(d, c && d.id === c.id))),
      h('main', { class: 'principal ample' }, c ? [
        h('div', { style: 'display:flex;align-items:flex-end;justify-content:space-between;gap:16px;flex-wrap:wrap' },
          h('div', { style: 'display:flex;flex-direction:column;gap:4px' },
            h('h1', {}, c.id + ' · ' + c.titre),
            h('div', { class: 'second' }, 'Version ' + c.version + ' · ' + (STATUT[c.statut] || c.statut) + ' · ' + (DEPUIS[c.statut] || '') + jourComplet(c.date_effet) + ' ' + heureMin(c.date_effet)),
            h('div', { class: 'second' }, c.active_depuis ? 'Put in force by ' + c.auteur + ' on ' + jourComplet(c.active_depuis) + ' ' + heureMin(c.active_depuis) : 'Effective date and author: ' + ND),
            h('div', { class: 'moyen' }, 'Origin: ' + c.origine)),
          h('div', { class: 'moyen second' }, 'Legal basis: ' + c.fondements.map(fondementTexte).join(' ; '))),
        h('div', { class: 'grille2' },
          h('section', { class: 'carte' },
            h('div', { class: 'etiq' }, 'Lawyer\'s text'),
            h('blockquote', { class: 'citation' }, '« ' + c.texte + ' »'),
            h('div', { class: 'moyen second' }, 'Status: ' + (STATUT[c.statut] || c.statut))),
          h('section', { class: 'carte' },
            h('div', { class: 'etiq' }, 'What will be applied (review without AI)'),
            h('pre', { class: 'relecture' }, c.relecture),
            h('div', { class: 'petit second' }, 'Produced by the deterministic template: same rule, same text, byte for byte.'))),
        (ACTIONS[c.statut] || []).length ? h('div', { style: 'display:flex;gap:12px;flex-wrap:wrap' }, actions()) : null, err, ok,
        tableau(c)] : h('p', { class: 'second' }, 'No rule to display.'))));
  };
})();
