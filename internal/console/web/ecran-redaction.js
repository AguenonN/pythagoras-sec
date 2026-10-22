'use strict';
// Écran 6 · Juriste, rédaction. Deux entrées de même rang : écrire une
// directive, ou reprendre une proposition de l'Analyste. Dans les deux cas le
// texte passe par le compilateur ; rien n'est mis en vigueur sans l'action du
// juriste.

(function () {
  const STATUT = { active: 'active', observation: 'in observation', brouillon: 'brouillon', retiree: 'withdrawn' };
  const ORIGINE_Q = { mistral: 'asked by the model', code: 'asked by the code' };
  const ETAT_COMPLET = 'j' + 'eu'; // valeur de l'état renvoyé par le service du compilateur
  const DECISION = { rejetee: 'rejected', reprise_modifiee: 'resumed with edits', reprise_telle_quelle: 'resumed as is' };

  ECRANS.redaction = async (ctx) => {
    let donnees = await api('/api/redaction');
    if (!ctx.vivant()) return;
    const S = { onglet: ctx.q.get('onglet') === 'propositions' ? 'propositions' : 'ecrire', texte: '', fondement: '', conversation: '', proposition: '',
      compil: null, reponses: {}, essai: null, err: '', info: '', occupe: false, motifs: {} };
    const racine = h('div', { class: 'col-redaction' });
    ctx.monter(racine);
    ctx.droite(['Lawyer: ', h('b', {}, donnees.juriste_nom || donnees.juriste)]);

    async function recharger() { donnees = await api('/api/redaction'); }
    async function action(f) {
      S.err = ''; S.info = ''; S.occupe = true; dessiner();
      try { await f(); } catch (e) { S.err = e.message; }
      S.occupe = false; dessiner();
    }

    // ---- compilation
    const compiler = (reponses) => action(async () => {
      S.essai = null;
      S.compil = await post('/api/redaction/compiler', { texte: S.texte, fondement: S.fondement, conversation: reponses ? S.conversation : '',
        reponses: reponses || [], proposition: S.proposition });
      S.conversation = S.compil.conversation || '';
      if (S.compil.etat === 'questions') S.reponses = {};
    });
    const repondre = () => compiler(S.compil.questions.map(q => ({ question: q.id, reponse: S.reponses[q.id] || '' })));
    const essayer = () => action(async () => { S.essai = await post('/api/redaction/essai', { conversation: S.conversation }); });
    const mettre = (statut, nom) => action(async () => {
      if (!window.confirm(nom + ': new rules will take effect now; the previous ones are archived. Confirm?')) return;
      const r = await post('/api/redaction/mettre-en-vigueur', { conversation: S.conversation, statut });
      S.info = nom + ' : fait (' + r.directives.join(', ') + '). New rules in force; the previous ones are archived.';
      S.compil = null; S.essai = null; S.conversation = '';
      await recharger();
    });
    const retirer = (id) => action(async () => {
      if (!window.confirm('Withdraw rule ' + id + '? New rules will take effect; the previous ones are archived.')) return;
      const r = await post('/api/regles/statut', { directive: id, statut: 'retiree' });
      S.info = 'Rule ' + id + ' withdrawn. New rules in force; the previous ones are archived.';
      await recharger();
    });

    // ---- propositions
    const decider = (p, decision) => action(async () => {
      const motif = (S.motifs[p.id] || '').trim();
      if (decision === 'rejetee' && !motif) throw new Error('A rejection requires a reason.');
      const r = await post('/api/propositions/' + encodeURIComponent(p.id) + '/decision', { decision, motif, texte: '' });
      await recharger();
      if (decision === 'rejetee') { S.info = 'Proposal rejected.'; return; }
      S.texte = r.decision.texte_retenu; S.fondement = p.fondement_propose; S.proposition = p.id; S.onglet = 'ecrire'; S.essai = null;
      if (decision === 'reprise_telle_quelle') {
        S.compil = r.compilation || null; S.conversation = S.compil ? S.compil.conversation : '';
        if (r.erreur_compilation) S.err = r.erreur_compilation;
      } else { S.compil = null; S.conversation = ''; S.info = 'Text copied: edit it, then compile.'; }
    });

    function carteProposition(p) {
      const d = p.decision;
      return h('div', { class: 'carte compacte', style: 'gap:8px' },
        h('div', { class: 'gras' }, p.constat.type + ' · ' + (p.date ? jourComplet(p.date) : '')),
        h('div', { class: 'moyen' }, p.constat.detail),
        p.explication ? h('div', { class: 'moyen second' }, p.explication) : null,
        h('div', { class: 'petit' }, 'Sessions cited: ', (p.sessions || []).length ? (p.sessions || []).map((s, i) => [i ? ', ' : '', lienProposition(s)]) : 'aucune'),
        h('blockquote', { class: 'citation', style: 'font-size:16px' }, '« ' + p.texte_propose + ' »'),
        h('div', { class: 'petit second' }, 'Proposed legal basis: ' + p.fondement_propose + ' ', h('span', { class: 'badge b-caviarde' }, 'to approve')),
        d ? h('div', { class: 'moyen gras' }, 'Lawyer\'s decision: ' + (DECISION[d.decision] || d.decision) + (d.motif ? ' — ' + d.motif : '')) : null,
        h('input', { class: 'champ', placeholder: 'Reason for rejection (required to reject)', value: S.motifs[p.id] || '', oninput: e => { S.motifs[p.id] = e.target.value; }, 'aria-label': 'Reason for rejection' }),
        h('div', { style: 'display:flex;gap:8px;flex-wrap:wrap' },
          h('button', { type: 'button', class: 'btn danger', onclick: () => decider(p, 'rejetee') }, 'Reject'),
          h('button', { type: 'button', class: 'btn contour', onclick: () => decider(p, 'reprise_modifiee') }, 'Resume with edits'),
          h('button', { type: 'button', class: 'btn', onclick: () => decider(p, 'reprise_telle_quelle') }, 'Resume as is')));
    }
    // Une session citée est un dossier de session ou un identifiant de session.
    function lienProposition(s) {
      const trouve = etat.journaux.find(j => j.id === s) || etat.journaux.find(j => j.sessions.some(x => x.id === s));
      if (!trouve) return h('span', { class: 'mono petit' }, s);
      const sess = trouve.id === s ? trouve.sessions[0] : trouve.sessions.find(x => x.id === s);
      return lienSession(trouve.id, sess ? sess.id : '', nomSession(sess ? sess.id : s));
    }

    // ---- colonnes
    function colGauche() {
      const nonDecidees = donnees.propositions.filter(p => !p.decision).length;
      const corps = S.onglet === 'ecrire' ? [
        h('div', { class: 'etiq' }, '1 · Votre directive'),
        S.proposition ? h('p', { class: 'petit second' }, 'Resuming proposal ' + S.proposition + '. ', h('a', { href: '#', onclick: e => { e.preventDefault(); S.proposition = ''; dessiner(); } }, 'detach')) : null,
        h('label', { class: 'moyen second', for: 'texte' }, 'Write the rule in plain language, with its legal basis.'),
        h('textarea', { id: 'texte', class: 'zone-serif', rows: '7', oninput: e => { S.texte = e.target.value; } }, S.texte),
        h('label', { class: 'moyen second', for: 'fondement' }, 'Legal basis'),
        h('input', { id: 'fondement', class: 'champ', type: 'text', style: 'background:#fff', value: S.fondement, oninput: e => { S.fondement = e.target.value; } }),
        h('div', { style: 'display:flex;align-items:center;gap:12px;flex-wrap:wrap' },
          h('button', { type: 'button', class: 'btn', disabled: S.occupe || undefined, onclick: () => compiler(null) }, S.occupe ? 'Compilation…' : 'Compile'),
          h('span', { class: 'petit second', id: 'mode-compilateur', title: donnees.mode_compilateur === 'reel' ?
            'A new text is sent to the model; a question already asked is read back without a new call.' :
            'Only the model\'s recorded answers are read back; no call is made.' },
            donnees.mode_compilateur === 'reel' ? 'live compilation' : 'replayed compilation')),
      ] : (donnees.propositions.length ? donnees.propositions.map(carteProposition) :
        h('div', { class: 'carte compacte', id: 'aucune-proposition' }, h('b', {}, 'No proposal.'),
          h('span', { class: 'moyen second' }, 'The Analyst has proposed nothing yet (demo/execution/propositions/ is empty). To produce some, from the repository root:'),
          h('code', { class: 'mono petit', style: 'user-select:all' }, 'cd agents && python3 -m analyste proposer'),
          h('span', { class: 'petit second' }, 'Replay mode by default (recorded answers); --mode reel to call the model.')));
      return h('section', {},
        h('div', { class: 'onglets' },
          h('button', { type: 'button', class: S.onglet === 'ecrire' ? 'actif' : '', onclick: () => { S.onglet = 'ecrire'; dessiner(); } }, 'Write a rule'),
          h('button', { type: 'button', class: S.onglet === 'propositions' ? 'actif' : '', onclick: () => { S.onglet = 'propositions'; dessiner(); } }, 'Propositions de l\'Analyste · ' + nonDecidees)),
        corps,
        h('div', { class: 'refus-exemple', style: 'margin-top:auto' }, h('b', { style: 'color:var(--ref)' }, 'A non-observable text is refused, with its reason'),
          h('span', { class: 'moyen', style: 'line-height:1.5' }, '“Agents must not discriminate”: a criterion not observable in a tool call. The compiler produces nothing and explains what would need to be named (a field, a group, a source).')));
    }

    function colQuestions() {
      const c = S.compil;
      const out = [h('div', { class: 'etiq' }, '2 · Compiler questions'),
        h('div', { class: 'moyen second' }, 'The compiler guesses nothing: every value missing from the text becomes a question.')];
      if (!c) { out.push(h('div', { class: 'carte compacte', style: 'border-style:dashed' }, h('b', {}, 'No question yet'), h('span', { class: 'moyen second' }, 'Compile a text to start.'))); return out; }
      if (c.etat === 'non_observable' && c.non_observable) out.push(h('div', { class: 'refus-exemple' }, h('b', { style: 'color:var(--ref)' }, 'Reasoned refusal: criterion not observable'),
        h('span', { class: 'moyen' }, '« ' + c.non_observable.extrait + ' »'), h('span', { class: 'moyen' }, c.non_observable.raison)));
      if (c.etat === 'refuse') out.push(h('div', { class: 'refus-exemple' }, h('b', { style: 'color:var(--ref)' }, 'Reasoned refusal'), c.erreurs.map(e => h('span', { class: 'moyen' }, e))));
      if (c.etat === 'erreurs') out.push(h('div', { class: 'carte compacte' }, h('b', { class: 'erreur' }, 'To fix'), c.erreurs.map(e => h('span', { class: 'moyen' }, e))));
      if (c.etat === ETAT_COMPLET) out.push(h('div', { class: 'carte compacte' }, h('b', { class: 'ok-txt' }, 'No question: the text is complete.')));
      if (c.etat === 'questions') {
        c.questions.forEach(q => out.push(h('div', { class: 'carte compacte' },
          h('b', {}, q.texte), h('span', { class: 'petit second' }, 'Question ' + (ORIGINE_Q[q.origine] || q.origine)),
          q.options && q.options.length ?
            h('select', { class: 'champ', 'aria-label': q.texte, onchange: e => { S.reponses[q.id] = e.target.value; } },
              [h('option', { value: '' }, 'Choisir…')].concat(q.options.map(o => h('option', { value: o, selected: S.reponses[q.id] === o }, o)))) :
            h('input', { class: 'champ', style: 'background:#fff', value: S.reponses[q.id] || '', 'aria-label': q.texte, oninput: e => { S.reponses[q.id] = e.target.value; } }))));
        out.push(h('button', { type: 'button', class: 'btn', disabled: S.occupe || undefined, onclick: repondre }, 'Send the answers'));
      }
      return out;
    }

    function blocEssai() {
      const e = S.essai;
      if (!e) return h('div', { style: 'background:var(--fond);border-radius:8px;padding:12px 14px;font-size:13px;line-height:1.5' }, 'Trial on existing journals: not done yet.');
      const k = e.compteurs;
      return h('div', { style: 'background:var(--fond);border-radius:8px;padding:12px 14px;font-size:13px;line-height:1.5;display:flex;flex-direction:column;gap:6px' },
        h('b', {}, 'Trial on existing journals'),
        h('span', {}, k.changes + (k.changes > 1 ? ' past calls would have changed decision' : ' past call would have changed decision') + ', out of ' + k.appels + ' replayed in ' + k.sessions + ' session(s).'),
        h('span', { class: 'second' }, k.exposition + ' exposition(s), ' + k.violation + ' violation(s), ' + k.non_rejouables + ' non rejouable(s).'),
        h('span', {}, e.explication),
        e.verifiees ? null : h('span', { class: 'erreur gras' }, 'A chain is not verified: this trial proves nothing.'),
        e.lignes.filter(l => l.change).slice(0, 8).map(l => h('span', { class: 'petit' }, 'Appel n° ' + l.seq + ' (' + libelleOutil(l.serveur, l.outil) + ', ' + nomSession(l.session) + ') : ' + LIBELLE_BADGE[EFFET_BADGE[l.avant.effet]] + ' → ' + LIBELLE_BADGE[EFFET_BADGE[l.apres.effet]])),
        h('a', { class: 'petit', href: '#avant-apres' }, 'See the detail on the Before / after screen'));
    }

    function colDroite() {
      const c = S.compil;
      const out = [h('div', { class: 'etiq' }, '3 · What will be applied')];
      const aj = c && c.ajoutees ? c.ajoutees : [];
      if (!aj.length) out.push(h('p', { class: 'second' }, nd(c ? 'nothing to review yet' : 'not available until the rule is compiled')));
      aj.forEach(d => out.push(h('div', { style: 'display:flex;flex-direction:column;gap:8px' },
        h('div', { style: 'display:flex;gap:8px;align-items:center;flex-wrap:wrap' }, h('b', {}, versionTexte(d.id, d.version) + ' · ' + d.titre),
          h('span', { class: 'badge b-indisponible' }, STATUT[d.statut] || d.statut), h('span', { class: 'petit ok-txt gras' }, 'accepted by the checker')),
        h('pre', { class: 'relecture' }, d.relecture),
        h('div', { class: 'petit second' }, 'Legal basis: ' + d.fondements.map(fondementTexte).join(' ; ')))));
      if (aj.length) out.push(h('div', { class: 'petit second', style: 'border-top:1px solid var(--trait-doux);padding-top:10px' }, 'Review produced by template, without AI. The original text is kept as is in the rule.'));
      if (aj.length) {
        out.push(blocEssai());
        out.push(h('div', { style: 'display:flex;flex-direction:column;gap:10px' },
          h('button', { type: 'button', class: 'btn contour', disabled: S.occupe || undefined, onclick: essayer }, 'Try on existing journals'),
          h('button', { type: 'button', class: 'btn contour', disabled: S.occupe || undefined, onclick: () => mettre('observation', 'Put in observation') }, 'Put in observation'),
          h('button', { type: 'button', class: 'btn', disabled: S.occupe || undefined, onclick: () => mettre('active', 'Activation (signed ' + (donnees.juriste_nom || 'J. Juriste') + ')') }, 'Activate (signed ' + (donnees.juriste_nom || 'J. Juriste') + ')'),
          h('span', { class: 'petit second', style: 'text-align:center' }, 'Keep as draft: do nothing. Observation: decision journaled, never applied. Nothing is activated without your action.')));
      }
      // Directives des règles en vigueur : origine et retrait.
      const regles = donnees.regles ? donnees.regles.directives : [];
      out.push(h('div', { class: 'etiq', style: 'margin-top:12px' }, 'Rules in force' + (donnees.regles ? ' since ' + jourMoisHeure(donnees.regles.depuis) : '')));
      regles.forEach(d => out.push(h('div', { class: 'kv', style: 'align-items:center' },
        h('span', {}, h('b', {}, d.id), ' ', d.titre, h('div', { class: 'petit second' }, (STATUT[d.statut] || d.statut) + ' · ' + (donnees.origines[d.id] || ''))),
        d.statut === 'active' || d.statut === 'observation' ? h('button', { type: 'button', class: 'btn danger', style: 'min-height:32px;padding:0 12px', disabled: S.occupe || undefined, onclick: () => retirer(d.id) }, 'Withdraw') : null)));
      return out;
    }

    function dessiner() {
      racine.textContent = '';
      const messages = [S.err ? h('p', { class: 'erreur gras', role: 'alert' }, S.err) : null, S.info ? h('p', { class: 'ok-txt gras' }, S.info) : null];
      const milieu = h('section', { style: 'padding:24px 28px' }, colQuestions(), messages);
      racine.append(colGauche(), milieu, h('section', {}, colDroite()));
    }
    dessiner();
  };
})();
