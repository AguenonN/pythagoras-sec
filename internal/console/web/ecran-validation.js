'use strict';
// Écran 5 · File du superviseur : requête exacte, directives, rôles requis,
// signature du jeton et rejet. Les quatre contrôles viennent de l'entrée
// « approbation » du journal, après la reprise de l'appel ; ils ne sont
// jamais présumés.

(function () {
  // « 1 / 2 » : jetons déposés sur rôles requis ; après la reprise, ceux que
  // l'engine a acceptés, tels que le journal les porte.
  function compteur(a) {
    const n = a.statut_session === 'en_attente' ? a.jetons_deposes : ((a.controles && a.controles.valideurs) || []).length;
    return n + ' / ' + a.jetons_requis;
  }

  const titreAppel = (a) => a.outil ? libelleOutil(a.serveur, a.outil) : 'Call';
  const valeur = (v) => typeof v === 'object' && v !== null ? JSON.stringify(v) : String(v);
  let saisie = { valideur: '', motif: '' };   // conservé d'un rafraîchissement à l'autre
  // Dernière décision prise sur cet écran : sa carte reste affichée jusqu'au
  // clic sur « Demande suivante » (ou sur une autre demande de la file).
  let decision = null;

  // Carte de décision : ce qui a été décidé, par qui, sous quel rôle, quand ;
  // pour une validation, les contrôles de l'engine dès que l'appel est repris
  // (lus au journal, jamais présumés).
  function carteDecision(d, tout, suivante, completer) {
    const x = tout.find(a => a.journal === d.journal && a.empreinte_transmise === d.empreinte);
    const rejetee = d.type === 'rejetee';
    const kv = (k, v) => h('div', { class: 'kv' }, h('span', {}, k), typeof v === 'string' ? h('span', {}, v) : v);
    const seq = x ? x.seq : d.seq;
    const manque = !rejetee && x && x.statut_session === 'en_attente' ? x.jetons_requis - x.jetons_deposes : 0;
    return h('section', { class: 'carte decision' + (rejetee ? ' rejetee' : ' validee'), id: 'carte-decision', style: 'gap:12px' },
      h('div', { class: 'etiq' }, 'Decision'),
      h('div', { style: 'display:flex;align-items:center;gap:12px;flex-wrap:wrap' },
        h('h1', {}, rejetee ? 'Request rejected' : (manque > 0 ? 'Approval submitted (' + x.jetons_deposes + ' / ' + x.jetons_requis + ')' : 'Request approved')),
        rejetee ? badge('rejete') : badge('autorise', 'Signed token')),
      kv('Call', d.outil ? outilLisible(d.serveur, d.outil) : ND),
      d.demande ? kv('Request', '« ' + d.demande + ' »') : null,
      kv(rejetee ? 'Rejected by' : 'Approved by', d.par.nom ? d.par.nom + ' (' + d.par.valideur + ')' : d.par.valideur),
      kv('Role', d.par.role || ND),
      kv('Time', jourComplet(d.par.date) + ' at ' + heureSec(d.par.date)),
      rejetee ? [kv('Reason', d.motif), h('p', { class: 'petit second' }, 'Decision recorded by the console, outside the signed journal.')] :
        (x && x.controles && x.controles.etabli ? blocControles(x) :
          h('p', { class: 'moyen second' }, manque > 0 ?
            'Still missing ' + manque + ' approval(s): when the call resumes, the engine checks the signature, the role on that date, four eyes and one token per role.' :
            'Token submitted: the orchestrator resumes the call; the engine checks will show here once they are in the journal.')),
      h('div', { style: 'display:flex;gap:16px;flex-wrap:wrap;align-items:center' },
        lienSession(d.journal, d.session, 'See session ' + nomSession(d.session || d.journal)),
        h('a', { href: '#journal?' + new URLSearchParams({ j: d.journal, s: d.session || '', a: seq || '' }) }, 'See the proof in the journal (call #' + (seq || ND) + ')')),
      h('div', { style: 'display:flex;gap:12px;justify-content:flex-end;flex-wrap:wrap' },
        manque > 0 ? h('button', { type: 'button', class: 'btn contour', onclick: completer }, 'Complete the approval') : null,
        h('button', { type: 'button', class: 'btn', id: 'demande-suivante', onclick: suivante }, 'Next request')));
  }

  function carteFile(a, choisie, nomLien) {
    const ids = a.applications.map(x => x.id).join(', ');
    const roles = a.roles && a.roles.length ? a.roles.join(', ') : null;
    return h('button', { type: 'button', class: 'attente-carte' + (choisie ? ' choisie' : ''), onclick: () => allerA('validation', { a: a.journal }) },
      h('span', { style: 'display:flex;justify-content:space-between;gap:8px' }, h('b', {}, titreAppel(a)), h('span', { class: 'petit second' }, a.ts ? ilYa(a.ts) : '')),
      h('span', { class: 'moyen second' }, (a.serveur || '') + (ids ? ' · ' + ids : '')),
      h('span', { class: 'petit gras', style: 'color:' + (choisie ? 'var(--accent)' : 'var(--second)') },
        'Required role: ' + (roles || ND) + (a.quatre_yeux ? ' · quatre yeux' : '') + ' · ' + compteur(a)));
  }

  function controle(titre, c) {
    const cls = { ok: 'controle-ok', echec: 'controle-echec', non_etabli: 'controle-non', non_requis: 'controle-non' }[c.etat] || 'controle-non';
    const puce = { ok: '✓ ', echec: '✗ ', non_etabli: '… ', non_requis: '– ' }[c.etat] || '';
    return h('div', { style: 'display:flex;flex-direction:column;gap:4px' }, h('b', { class: 'second' }, titre), h('span', { class: cls }, puce + c.detail));
  }

  function blocControles(a) {
    const c = a.controles;
    return h('section', { class: 'carte' },
      h('div', { class: 'etiq' }, 'Checks made by the engine before forwarding'),
      c.etabli ? h('p', { class: 'petit ' + (c.statut === 'acceptee' ? 'ok-txt gras' : 'erreur gras') },
        (c.statut === 'acceptee' ? 'Approval accepted' : 'Approval refused') + ' · journal entry of ' + jourComplet(c.ts) + ' at ' + heureSec(c.ts)) :
        h('p', { class: 'petit second' }, 'From the journal “approval” entry: none yet, checks are not presumed.'),
      h('div', { class: 'grille4 moyen' },
        controle('Signature', c.signature), controle('Role on that date', c.role_a_date),
        controle('Four eyes', c.quatre_yeux), controle('One token per role', c.un_jeton_par_role)),
      c.statut === 'refusee' && c.motif ? h('p', { class: 'erreur moyen' }, 'Motif de l\'engine : ' + c.motif) : null);
  }

  // Ce que le sondage redessine : la requête, les directives, les contrôles,
  // le rejet éventuel.
  function infos(a) {
    const lignes = [];
    if (a.purgee) lignes.push('The arguments of this call were purged from the vault.');
    else if (a.arguments) for (const k of Object.keys(a.arguments).sort()) lignes.push(k + ' : ' + valeur(a.arguments[k]));
    else lignes.push('Arguments ' + ND + ' (vault missing or call not journaled yet).');
    const enAttente = a.statut_session === 'en_attente';
    const roles = a.roles || [];
    const dem = a.demandeur ? a.demandeur.identite : ND;
    return [
      h('div', { style: 'display:flex;flex-direction:column;gap:4px' },
        h('h1', {}, a.outil ? outilLisible(a.serveur, a.outil) : 'Call'),
        h('div', { class: 'second' }, 'Requested by ' + dem + (a.session ? ' · session ' + nomSession(a.session) : '') + (a.sous_agent ? ' · declared sub-agent “' + a.sous_agent + ' »' : '') + (a.ts ? ' · ' + jourComplet(a.ts) + ' ' + heureSec(a.ts) : '')),
        a.demande ? h('div', { class: 'second' }, 'Demande : « ' + a.demande + ' »') : null),
      h('div', { class: 'grille2' },
        h('section', { class: 'carte', style: 'gap:10px' },
          h('div', { class: 'etiq' }, 'Exact request that will be forwarded'),
          h('div', { class: 'requete' }, a.outil ? outilLisible(a.serveur, a.outil, { class: 'gras' }) : ND, lignes.length ? '\n' + lignes.join('\n') : ''),
          h('div', { class: 'kv' }, h('span', {}, 'Digital fingerprint of the call'), h('span', { class: 'mono' }, court(a.empreinte_transmise))),
          h('div', { class: 'petit second' }, 'Your signature covers this exact request only. Any change to the request will require a new approval.')),
        h('section', { class: 'carte', style: 'gap:10px' },
          h('div', { class: 'etiq' }, 'Why this approval'),
          a.applications.length ? a.applications.map(x => h('div', { style: 'display:flex;flex-direction:column;gap:4px' },
            h('b', {}, versionTexte(x.id, x.version) + (x.titre ? ' · ' + x.titre : '')),
            h('span', { class: 'moyen second' }, fondementTexte(x.fondement) + ' · ' + motifAffiche(x.motif)))) :
            h('p', { class: 'moyen' }, a.directive ? 'Rule ' + a.directive + ' (detail ' + ND + ')' : motifAffiche(a.motif) || nd()),
          h('div', { class: 'moyen', style: 'border-top:1px solid var(--trait-doux);padding-top:10px;line-height:1.5' }, 'Items consulted: ' +
            (a.consultations.length ? a.consultations.map(k => libelleOutil(k.serveur, k.outil) + ' (call #' + k.seq + ')').join(', ') : 'aucun')),
          h('div', { class: 'moyen gras' }, 'Required roles: ' + (roles.join(', ') || ND) + (a.quatre_yeux ? ' · quatre yeux (deux personnes distinctes)' : '') + ' · ' + (enAttente ? 'tokens submitted ' : 'tokens accepted ') + compteur(a)))),
      blocControles(a),
      a.motif_jeton ? h('p', { class: 'erreur gras' }, 'Last token refused by the engine: ' + a.motif_jeton) : null,
      a.rejet ? h('div', { class: 'carte', style: 'border-color:var(--ref)' }, h('b', { class: 'erreur' }, 'Rejection recorded'),
        h('span', { class: 'moyen' }, texteRejet(a.rejet)), h('span', { class: 'petit second' }, 'Decision recorded by the console, outside the signed journal.')) : null,
      enAttente ? null : h('p', { class: 'second' }, 'This request no longer awaits approval (status: ' + (a.statut_session || ND) + ').')];
  }

  // Clé du formulaire : tant qu'elle ne change pas, le formulaire (valideur,
  // motif) reste le même nœud, jamais reconstruit par le sondage.
  const cleFormulaire = (a) => [a.journal, a.empreinte_transmise, a.statut_session === 'en_attente', !!a.rejet,
    (a.valideurs_possibles || []).map(v => v.identite + '|' + v.role).join(',')].join('#');

  function formulaire(a, rafraichir, decider) {
    if (a.statut_session !== 'en_attente') return null;
    const err = zoneErreur(), ok = h('p', { class: 'ok-txt gras', hidden: true });
    const choix = a.valideurs_possibles || [];
    const select = h('select', { class: 'champ', id: 'valideur', 'aria-label': 'Approver', onchange: e => { saisie.valideur = e.target.value; } },
      choix.length ? choix.map(v => h('option', { value: v.identite + '|' + v.role, selected: saisie.valideur === v.identite + '|' + v.role },
        (v.nom || v.identite) + ' — role “' + v.role + ' »')) : h('option', { value: '' }, 'no approver available in the directory'));
    const motif = h('textarea', { id: 'motif', class: 'champ', rows: '3', style: 'background:#fff;resize:none', placeholder: 'Required for a rejection; recorded with the refusal.',
      oninput: e => { saisie.motif = e.target.value; } });
    motif.value = saisie.motif;
    const bloque = !!a.rejet || !choix.length;
    async function agir(chemin, corps, type) {
      err.dire(''); ok.hidden = true;
      try {
        const r = await post(chemin, corps);
        saisie.motif = '';
        const par = type === 'rejetee' ? { valideur: r.valideur, nom: r.nom, role: r.role, date: r.date } : r.depot;
        decider({ type, journal: a.journal, session: a.session, empreinte: a.empreinte_transmise, seq: a.seq, outil: a.outil, serveur: a.serveur,
          demande: a.demande, par, motif: type === 'rejetee' ? r.motif : '' });
        await rafraichir(true);
      } catch (e) { err.dire(e.message); }
    }
    const quel = () => { const [identite, role] = (select.value || '|').split('|'); return { identite, role }; };
    return h('div', { style: 'display:flex;flex-direction:column;gap:8px;flex:1' },
      h('label', { class: 'moyen second', for: 'valideur' }, 'Approver (directory identities holding the required role)'), select,
      h('label', { class: 'moyen second', for: 'motif' }, 'Reason for rejection'), motif,
      err, ok,
      h('div', { style: 'margin-top:auto;display:flex;gap:12px;justify-content:flex-end;align-items:center;flex-wrap:wrap' },
        h('span', { class: 'petit second', style: 'margin-right:auto' }, 'The engine command signs with the approver\'s key; the approval is written to the chained journal when the call resumes.'),
        h('button', { type: 'button', class: 'btn danger', disabled: !choix.length || undefined, onclick: () => {
          if (!motif.value.trim()) { err.dire('A rejection requires a reason.'); return; }
          const q = quel(); agir('/api/validation/rejeter', { journal: a.journal, valideur: q.identite, role: q.role, motif: motif.value }, 'rejetee');
        } }, 'Reject'),
        h('button', { type: 'button', class: 'btn', disabled: bloque || undefined, onclick: () => {
          const q = quel(); agir('/api/validation/valider', { journal: a.journal, valideur: q.identite, role: q.role }, 'validee');
        } }, 'Sign and approve')));
  }

  ECRANS.validation = async (ctx) => {
    // Gabarit monté une fois : la liste et les informations sont redessinées
    // au sondage, le formulaire seulement si l'appel affiché change.
    const laterale = h('aside', { class: 'laterale tres-large' });
    const zoneInfos = h('div', { style: 'display:flex;flex-direction:column;gap:20px' });
    const zoneFormulaire = h('div', { style: 'display:flex;flex-direction:column;flex:1' });
    ctx.monter(h('div', { class: 'corps' }, laterale, h('main', { class: 'principal ample' }, zoneInfos, zoneFormulaire)));
    let signature = null, cle = null;
    // L'appel affiché reste le même tant qu'il est dans la file : une
    // nouvelle attente ne le remplace pas sous les yeux du valideur.
    let choisi = ctx.q.get('a');
    // Un clic sur une autre demande de la file referme la carte de décision.
    if (decision && choisi && choisi !== decision.journal) decision = null;
    if (decision) choisi = decision.journal;
    const decider = (d) => { decision = d; };
    const suivante = () => {
      const fini = decision ? decision.journal : null;
      decision = null;
      choisi = (derniere.attentes.find(x => x.journal !== fini) || {}).journal || null;
      rafraichir(true);
    };
    const completer = () => { choisi = decision.journal; decision = null; rafraichir(true); };
    let derniere = { attentes: [], reprises: [] };
    async function rafraichir(force) {
      const v = await api('/api/file');
      if (!ctx.vivant()) return;
      derniere = v;
      const sig = JSON.stringify(v);
      if (sig === signature && !force) return;
      signature = sig;
      const att = v.attentes, rep = v.reprises;
      const tout = att.concat(rep);
      const a = decision ? tout.find(x => x.journal === decision.journal) : (tout.find(x => x.journal === choisi) || att[0] || rep[0]);
      choisi = decision ? decision.journal : (a ? a.journal : null);
      laterale.replaceChildren(...[
        h('div', { class: 'etiq', style: 'padding:0 4px 6px' }, 'En attente · ' + att.length),
        att.length ? att.map(x => carteFile(x, a && x.journal === a.journal && att.includes(a), true)) : h('p', { class: 'second' }, 'No call awaiting approval.'),
        rep.length ? h('div', { class: 'etiq', style: 'padding:14px 4px 6px' }, 'Reprises · ' + rep.length) : null,
        rep.map(x => carteFile(x, a && x.journal === a.journal && !att.includes(a), false)),
        v.erreur_annuaire ? h('p', { class: 'erreur petit' }, 'Directory: ' + v.erreur_annuaire) : null].flat(Infinity).filter(Boolean));
      if (decision) {
        cle = null;
        zoneInfos.replaceChildren(carteDecision(decision, tout, suivante, completer));
        zoneFormulaire.replaceChildren();
        return;
      }
      if (!a) {
        cle = null;
        zoneInfos.replaceChildren(h('div', { class: 'carte' }, h('p', { class: 'second' }, 'Nothing to approve yet.')));
        zoneFormulaire.replaceChildren();
        return;
      }
      zoneInfos.replaceChildren(...infos(a).filter(Boolean));
      const k = cleFormulaire(a);
      if (k !== cle) {
        cle = k;
        const f = formulaire(a, rafraichir, decider);
        zoneFormulaire.replaceChildren(...(f ? [f] : []));
      }
    }
    await rafraichir(true);
    // Le sondage ne redessine que si les données changent ; le formulaire
    // n'est jamais reconstruit tant que l'appel affiché reste le même.
    ctx.suivre(() => rafraichir(false));
  };
})();
