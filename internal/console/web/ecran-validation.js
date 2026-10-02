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

  const titreAppel = (a) => { const t = humanise(a.outil || 'appel'); return t.charAt(0).toUpperCase() + t.slice(1); };
  const valeur = (v) => typeof v === 'object' && v !== null ? JSON.stringify(v) : String(v);
  let saisie = { valideur: '', motif: '' };   // conservé d'un rafraîchissement à l'autre

  function carteFile(a, choisie, nomLien) {
    const ids = a.applications.map(x => x.id).join(', ');
    const roles = a.roles && a.roles.length ? a.roles.join(', ') : null;
    return h('button', { type: 'button', class: 'attente-carte' + (choisie ? ' choisie' : ''), onclick: () => allerA('validation', { a: a.journal }) },
      h('span', { style: 'display:flex;justify-content:space-between;gap:8px' }, h('b', {}, titreAppel(a)), h('span', { class: 'petit second' }, a.ts ? ilYa(a.ts) : '')),
      h('span', { class: 'moyen second' }, (a.serveur || '') + (ids ? ' · ' + ids : '')),
      h('span', { class: 'petit gras', style: 'color:' + (choisie ? 'var(--accent)' : 'var(--second)') },
        'Rôle requis : ' + (roles || ND) + (a.quatre_yeux ? ' · quatre yeux' : '') + ' · ' + compteur(a)));
  }

  function controle(titre, c) {
    const cls = { ok: 'controle-ok', echec: 'controle-echec', non_etabli: 'controle-non', non_requis: 'controle-non' }[c.etat] || 'controle-non';
    const puce = { ok: '✓ ', echec: '✗ ', non_etabli: '… ', non_requis: '– ' }[c.etat] || '';
    return h('div', { style: 'display:flex;flex-direction:column;gap:4px' }, h('b', { class: 'second' }, titre), h('span', { class: cls }, puce + c.detail));
  }

  function blocControles(a) {
    const c = a.controles;
    return h('section', { class: 'carte' },
      h('div', { class: 'etiq' }, 'Contrôles faits par l\'engine avant transmission'),
      c.etabli ? h('p', { class: 'petit ' + (c.statut === 'acceptee' ? 'ok-txt gras' : 'erreur gras') },
        (c.statut === 'acceptee' ? 'Approbation acceptée' : 'Approbation refusée') + ' · entrée du journal du ' + jourComplet(c.ts) + ' à ' + heureSec(c.ts)) :
        h('p', { class: 'petit second' }, 'D\'après l\'entrée « approbation » du journal : aucune pour l\'instant, les contrôles ne sont pas présumés.'),
      h('div', { class: 'grille4 moyen' },
        controle('Signature', c.signature), controle('Rôle à la date', c.role_a_date),
        controle('Quatre yeux', c.quatre_yeux), controle('Un jeton par rôle', c.un_jeton_par_role)),
      c.statut === 'refusee' && c.motif ? h('p', { class: 'erreur moyen' }, 'Motif de l\'engine : ' + c.motif) : null);
  }

  function detail(a, rafraichir) {
    const lignes = [(a.serveur || ND) + ' / ' + (a.outil || ND)];
    if (a.purgee) lignes.push('Les arguments de cet appel ont été purgés du coffre.');
    else if (a.arguments) for (const k of Object.keys(a.arguments).sort()) lignes.push(k + ' : ' + valeur(a.arguments[k]));
    else lignes.push('Arguments ' + ND + ' (coffre absent ou appel pas encore journalisé).');
    const enAttente = a.statut_session === 'en_attente';
    const err = zoneErreur(), ok = h('p', { class: 'ok-txt gras', hidden: true });
    const roles = a.roles || [];
    const choix = a.valideurs_possibles || [];
    const select = h('select', { class: 'champ', id: 'valideur', 'aria-label': 'Valideur', onchange: e => { saisie.valideur = e.target.value; } },
      choix.length ? choix.map(v => h('option', { value: v.identite + '|' + v.role, selected: saisie.valideur === v.identite + '|' + v.role },
        (v.nom || v.identite) + ' — rôle « ' + v.role + ' »')) : h('option', { value: '' }, 'aucun valideur disponible dans l\'annuaire'));
    const motif = h('textarea', { id: 'motif', class: 'champ', rows: '3', style: 'background:#fff;resize:none', placeholder: 'Obligatoire pour un rejet ; consigné avec le refus.',
      oninput: e => { saisie.motif = e.target.value; } });
    motif.value = saisie.motif;
    const bloque = !enAttente || !!a.rejet || !choix.length;
    async function agir(chemin, corps, msg) {
      err.dire(''); ok.hidden = true;
      try { await post(chemin, corps); ok.hidden = false; ok.textContent = msg; saisie.motif = ''; await rafraichir(true); }
      catch (e) { err.dire(e.message); }
    }
    const quel = () => { const [identite, role] = (select.value || '|').split('|'); return { identite, role }; };
    const dem = a.demandeur ? a.demandeur.identite : ND;
    return h('main', { class: 'principal ample' },
      h('div', { style: 'display:flex;flex-direction:column;gap:4px' },
        h('h1', {}, titreAppel(a) + (a.serveur ? ' — ' + a.serveur : '')),
        h('div', { class: 'second' }, 'Demandée par ' + dem + (a.session ? ' · session ' + nomSession(a.session) : '') + (a.sous_agent ? ' · sous-agent déclaré « ' + a.sous_agent + ' »' : '') + (a.ts ? ' · ' + jourComplet(a.ts) + ' ' + heureSec(a.ts) : '')),
        a.demande ? h('div', { class: 'second' }, 'Demande : « ' + a.demande + ' »') : null),
      h('div', { class: 'grille2' },
        h('section', { class: 'carte', style: 'gap:10px' },
          h('div', { class: 'etiq' }, 'Requête exacte qui sera transmise'),
          h('div', { class: 'requete' }, lignes.join('\n')),
          h('div', { class: 'kv' }, h('span', {}, 'Signature numérique de l\'appel (empreinte)'), h('span', { class: 'mono' }, court(a.empreinte_transmise))),
          h('div', { class: 'petit second' }, 'Votre signature vaudra pour cette requête seulement. Toute modification de la requête exigera une nouvelle validation.')),
        h('section', { class: 'carte', style: 'gap:10px' },
          h('div', { class: 'etiq' }, 'Pourquoi cette validation'),
          a.applications.length ? a.applications.map(x => h('div', { style: 'display:flex;flex-direction:column;gap:4px' },
            h('b', {}, versionTexte(x.id, x.version) + (x.titre ? ' · ' + x.titre : '')),
            h('span', { class: 'moyen second' }, fondementTexte(x.fondement) + ' · ' + motifAffiche(x.motif)))) :
            h('p', { class: 'moyen' }, a.directive ? 'Directive ' + a.directive + ' (détail ' + ND + ')' : motifAffiche(a.motif) || nd()),
          h('div', { class: 'moyen', style: 'border-top:1px solid var(--trait-doux);padding-top:10px;line-height:1.5' }, 'Éléments consultés : ' +
            (a.consultations.length ? a.consultations.map(k => k.serveur + ' / ' + k.outil + ' (appel n° ' + k.seq + ')').join(', ') : 'aucun')),
          h('div', { class: 'moyen gras' }, 'Rôles requis : ' + (roles.join(', ') || ND) + (a.quatre_yeux ? ' · quatre yeux (deux personnes distinctes)' : '') + ' · ' + (a.statut_session === 'en_attente' ? 'jetons déposés ' : 'jetons acceptés ') + compteur(a)))),
      blocControles(a),
      a.motif_jeton ? h('p', { class: 'erreur gras' }, 'Dernier jeton refusé par l\'engine : ' + a.motif_jeton) : null,
      a.rejet ? h('div', { class: 'carte', style: 'border-color:var(--ref)' }, h('b', { class: 'erreur' }, 'Rejet consigné'),
        h('span', { class: 'moyen' }, (a.rejet.nom || a.rejet.valideur) + ' le ' + jourComplet(a.rejet.date) + ' à ' + heureSec(a.rejet.date) + ' : ' + a.rejet.motif)) : null,
      enAttente ? h('div', { style: 'display:flex;flex-direction:column;gap:8px' },
        h('label', { class: 'moyen second', for: 'valideur' }, 'Valideur (identités de l\'annuaire qui détiennent le rôle requis)'), select,
        h('label', { class: 'moyen second', for: 'motif' }, 'Motif du rejet'), motif) : h('p', { class: 'second' }, 'Cette demande n\'attend plus de validation (statut : ' + (a.statut_session || ND) + ').'),
      err, ok,
      enAttente ? h('div', { style: 'margin-top:auto;display:flex;gap:12px;justify-content:flex-end;align-items:center;flex-wrap:wrap' },
        h('span', { class: 'petit second', style: 'margin-right:auto' }, 'La signature est faite par la commande de l\'engine avec la clé du valideur ; l\'approbation est écrite au journal chaîné à la reprise de l\'appel.'),
        h('button', { type: 'button', class: 'btn danger', disabled: !choix.length || !enAttente || undefined, onclick: () => {
          if (!motif.value.trim()) { err.dire('Un rejet exige un motif.'); return; }
          agir('/api/validation/rejeter', { journal: a.journal, valideur: quel().identite, motif: motif.value }, 'Rejet consigné.');
        } }, 'Rejeter'),
        h('button', { type: 'button', class: 'btn', disabled: bloque || undefined, onclick: () => {
          const q = quel(); agir('/api/validation/valider', { journal: a.journal, valideur: q.identite, role: q.role }, 'Jeton signé et déposé : l\'orchestrateur reprend l\'appel.');
        } }, 'Signer et valider')) : null);
  }

  ECRANS.validation = async (ctx) => {
    const racine = h('div', { class: 'corps' });
    ctx.monter(racine);
    let signature = null;
    async function rafraichir(force) {
      const v = await api('/api/file');
      if (!ctx.vivant()) return;
      const sig = JSON.stringify(v);
      if (sig === signature && !force) return;
      signature = sig;
      const att = v.attentes, rep = v.reprises;
      const tout = att.concat(rep);
      const a = tout.find(x => x.journal === ctx.q.get('a')) || att[0] || rep[0];
      racine.textContent = '';
      racine.append(
        h('aside', { class: 'laterale tres-large' },
          h('div', { class: 'etiq', style: 'padding:0 4px 6px' }, 'En attente · ' + att.length),
          att.length ? att.map(x => carteFile(x, a && x.journal === a.journal && att.includes(a), true)) : h('p', { class: 'second' }, 'Aucun appel en attente de validation.'),
          rep.length ? h('div', { class: 'etiq', style: 'padding:14px 4px 6px' }, 'Reprises · ' + rep.length) : null,
          rep.map(x => carteFile(x, a && x.journal === a.journal && !att.includes(a), false)),
          v.erreur_annuaire ? h('p', { class: 'erreur petit' }, 'Annuaire : ' + v.erreur_annuaire) : null),
        a ? detail(a, rafraichir) : h('main', { class: 'principal ample' }, h('div', { class: 'carte' }, h('p', { class: 'second' }, 'Rien à valider pour l\'instant.'))));
    }
    await rafraichir(true);
    // Le sondage ne redessine que si les données changent (la saisie est conservée).
    ctx.suivre(() => rafraichir(false));
  };
})();
