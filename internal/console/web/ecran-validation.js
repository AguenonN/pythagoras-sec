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

  const titreAppel = (a) => a.outil ? libelleOutil(a.serveur, a.outil) : 'Appel';
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
      h('div', { class: 'etiq' }, 'Décision'),
      h('div', { style: 'display:flex;align-items:center;gap:12px;flex-wrap:wrap' },
        h('h1', {}, rejetee ? 'Demande rejetée' : (manque > 0 ? 'Validation déposée (' + x.jetons_deposes + ' / ' + x.jetons_requis + ')' : 'Demande validée')),
        rejetee ? badge('rejete') : badge('autorise', 'Jeton signé')),
      kv('Appel', d.outil ? outilLisible(d.serveur, d.outil) : ND),
      d.demande ? kv('Demande', '« ' + d.demande + ' »') : null,
      kv(rejetee ? 'Rejetée par' : 'Validée par', d.par.nom ? d.par.nom + ' (' + d.par.valideur + ')' : d.par.valideur),
      kv('Rôle', d.par.role || ND),
      kv('Heure', jourComplet(d.par.date) + ' à ' + heureSec(d.par.date)),
      rejetee ? [kv('Motif', d.motif), h('p', { class: 'petit second' }, 'Décision consignée par la console, hors journal signé.')] :
        (x && x.controles && x.controles.etabli ? blocControles(x) :
          h('p', { class: 'moyen second' }, manque > 0 ?
            'Il manque encore ' + manque + ' validation(s) : l\'engine contrôlera signature, rôle à la date, quatre yeux et un jeton par rôle à la reprise de l\'appel.' :
            'Jeton déposé : l\'orchestrateur reprend l\'appel ; les contrôles de l\'engine s\'afficheront ici dès qu\'ils seront au journal.')),
      h('div', { style: 'display:flex;gap:16px;flex-wrap:wrap;align-items:center' },
        lienSession(d.journal, d.session, 'Voir la session ' + nomSession(d.session || d.journal)),
        h('a', { href: '#journal?' + new URLSearchParams({ j: d.journal, s: d.session || '', a: seq || '' }) }, 'Voir la preuve au journal (appel n° ' + (seq || ND) + ')')),
      h('div', { style: 'display:flex;gap:12px;justify-content:flex-end;flex-wrap:wrap' },
        manque > 0 ? h('button', { type: 'button', class: 'btn contour', onclick: completer }, 'Compléter la validation') : null,
        h('button', { type: 'button', class: 'btn', id: 'demande-suivante', onclick: suivante }, 'Demande suivante')));
  }

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

  // Ce que le sondage redessine : la requête, les directives, les contrôles,
  // le rejet éventuel.
  function infos(a) {
    const lignes = [];
    if (a.purgee) lignes.push('Les arguments de cet appel ont été purgés du coffre.');
    else if (a.arguments) for (const k of Object.keys(a.arguments).sort()) lignes.push(k + ' : ' + valeur(a.arguments[k]));
    else lignes.push('Arguments ' + ND + ' (coffre absent ou appel pas encore journalisé).');
    const enAttente = a.statut_session === 'en_attente';
    const roles = a.roles || [];
    const dem = a.demandeur ? a.demandeur.identite : ND;
    return [
      h('div', { style: 'display:flex;flex-direction:column;gap:4px' },
        h('h1', {}, a.outil ? outilLisible(a.serveur, a.outil) : 'Appel'),
        h('div', { class: 'second' }, 'Demandée par ' + dem + (a.session ? ' · session ' + nomSession(a.session) : '') + (a.sous_agent ? ' · sous-agent déclaré « ' + a.sous_agent + ' »' : '') + (a.ts ? ' · ' + jourComplet(a.ts) + ' ' + heureSec(a.ts) : '')),
        a.demande ? h('div', { class: 'second' }, 'Demande : « ' + a.demande + ' »') : null),
      h('div', { class: 'grille2' },
        h('section', { class: 'carte', style: 'gap:10px' },
          h('div', { class: 'etiq' }, 'Requête exacte qui sera transmise'),
          h('div', { class: 'requete' }, a.outil ? outilLisible(a.serveur, a.outil, { class: 'gras' }) : ND, lignes.length ? '\n' + lignes.join('\n') : ''),
          h('div', { class: 'kv' }, h('span', {}, 'Signature numérique de l\'appel (empreinte)'), h('span', { class: 'mono' }, court(a.empreinte_transmise))),
          h('div', { class: 'petit second' }, 'Votre signature vaudra pour cette requête seulement. Toute modification de la requête exigera une nouvelle validation.')),
        h('section', { class: 'carte', style: 'gap:10px' },
          h('div', { class: 'etiq' }, 'Pourquoi cette validation'),
          a.applications.length ? a.applications.map(x => h('div', { style: 'display:flex;flex-direction:column;gap:4px' },
            h('b', {}, versionTexte(x.id, x.version) + (x.titre ? ' · ' + x.titre : '')),
            h('span', { class: 'moyen second' }, fondementTexte(x.fondement) + ' · ' + motifAffiche(x.motif)))) :
            h('p', { class: 'moyen' }, a.directive ? 'Directive ' + a.directive + ' (détail ' + ND + ')' : motifAffiche(a.motif) || nd()),
          h('div', { class: 'moyen', style: 'border-top:1px solid var(--trait-doux);padding-top:10px;line-height:1.5' }, 'Éléments consultés : ' +
            (a.consultations.length ? a.consultations.map(k => libelleOutil(k.serveur, k.outil) + ' (appel n° ' + k.seq + ')').join(', ') : 'aucun')),
          h('div', { class: 'moyen gras' }, 'Rôles requis : ' + (roles.join(', ') || ND) + (a.quatre_yeux ? ' · quatre yeux (deux personnes distinctes)' : '') + ' · ' + (enAttente ? 'jetons déposés ' : 'jetons acceptés ') + compteur(a)))),
      blocControles(a),
      a.motif_jeton ? h('p', { class: 'erreur gras' }, 'Dernier jeton refusé par l\'engine : ' + a.motif_jeton) : null,
      a.rejet ? h('div', { class: 'carte', style: 'border-color:var(--ref)' }, h('b', { class: 'erreur' }, 'Rejet consigné'),
        h('span', { class: 'moyen' }, texteRejet(a.rejet)), h('span', { class: 'petit second' }, 'Décision consignée par la console, hors journal signé.')) : null,
      enAttente ? null : h('p', { class: 'second' }, 'Cette demande n\'attend plus de validation (statut : ' + (a.statut_session || ND) + ').')];
  }

  // Clé du formulaire : tant qu'elle ne change pas, le formulaire (valideur,
  // motif) reste le même nœud, jamais reconstruit par le sondage.
  const cleFormulaire = (a) => [a.journal, a.empreinte_transmise, a.statut_session === 'en_attente', !!a.rejet,
    (a.valideurs_possibles || []).map(v => v.identite + '|' + v.role).join(',')].join('#');

  function formulaire(a, rafraichir, decider) {
    if (a.statut_session !== 'en_attente') return null;
    const err = zoneErreur(), ok = h('p', { class: 'ok-txt gras', hidden: true });
    const choix = a.valideurs_possibles || [];
    const select = h('select', { class: 'champ', id: 'valideur', 'aria-label': 'Valideur', onchange: e => { saisie.valideur = e.target.value; } },
      choix.length ? choix.map(v => h('option', { value: v.identite + '|' + v.role, selected: saisie.valideur === v.identite + '|' + v.role },
        (v.nom || v.identite) + ' — rôle « ' + v.role + ' »')) : h('option', { value: '' }, 'aucun valideur disponible dans l\'annuaire'));
    const motif = h('textarea', { id: 'motif', class: 'champ', rows: '3', style: 'background:#fff;resize:none', placeholder: 'Obligatoire pour un rejet ; consigné avec le refus.',
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
      h('label', { class: 'moyen second', for: 'valideur' }, 'Valideur (identités de l\'annuaire qui détiennent le rôle requis)'), select,
      h('label', { class: 'moyen second', for: 'motif' }, 'Motif du rejet'), motif,
      err, ok,
      h('div', { style: 'margin-top:auto;display:flex;gap:12px;justify-content:flex-end;align-items:center;flex-wrap:wrap' },
        h('span', { class: 'petit second', style: 'margin-right:auto' }, 'La signature est faite par la commande de l\'engine avec la clé du valideur ; l\'approbation est écrite au journal chaîné à la reprise de l\'appel.'),
        h('button', { type: 'button', class: 'btn danger', disabled: !choix.length || undefined, onclick: () => {
          if (!motif.value.trim()) { err.dire('Un rejet exige un motif.'); return; }
          const q = quel(); agir('/api/validation/rejeter', { journal: a.journal, valideur: q.identite, role: q.role, motif: motif.value }, 'rejetee');
        } }, 'Rejeter'),
        h('button', { type: 'button', class: 'btn', disabled: bloque || undefined, onclick: () => {
          const q = quel(); agir('/api/validation/valider', { journal: a.journal, valideur: q.identite, role: q.role }, 'validee');
        } }, 'Signer et valider')));
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
        att.length ? att.map(x => carteFile(x, a && x.journal === a.journal && att.includes(a), true)) : h('p', { class: 'second' }, 'Aucun appel en attente de validation.'),
        rep.length ? h('div', { class: 'etiq', style: 'padding:14px 4px 6px' }, 'Reprises · ' + rep.length) : null,
        rep.map(x => carteFile(x, a && x.journal === a.journal && !att.includes(a), false)),
        v.erreur_annuaire ? h('p', { class: 'erreur petit' }, 'Annuaire : ' + v.erreur_annuaire) : null].flat(Infinity).filter(Boolean));
      if (decision) {
        cle = null;
        zoneInfos.replaceChildren(carteDecision(decision, tout, suivante, completer));
        zoneFormulaire.replaceChildren();
        return;
      }
      if (!a) {
        cle = null;
        zoneInfos.replaceChildren(h('div', { class: 'carte' }, h('p', { class: 'second' }, 'Rien à valider pour l\'instant.')));
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
