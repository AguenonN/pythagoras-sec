'use strict';
// Écran 1 · Journal et graphe de chaîne.

(function () {
  const STATUT_SESSION = { en_cours: 'in progress', en_attente: 'awaiting approval', termine: 'finished', bloque: 'blocked', erreur: 'failed' };
  const RYTHME = { temps_reel: 'in real time', synthese: 'in a digest' };
  const REMISE = { remise: 'remise', echec: 'not delivered', planifiee: 'scheduled' };

  function ligneChaineSession(j) {
    const v = j.verification;
    if (v.ok) return h('span', { class: 'petit ok-txt' }, 'Chain verified · ' + j.entrees + ' entries');
    if (v.cle_absente) return h('span', { class: 'petit gras', style: 'color:var(--ref)' }, 'Chain not verifiable · key missing');
    return h('span', { class: 'petit gras', style: 'color:var(--ref)' }, 'Chain broken at entry #' + (j.premier_ecart + 1));
  }

  function liste(courante) {
    const demandes = toutesDemandes();
    const jours = new Set(demandes.map(j => jourComplet(dateDemande(j))));
    const titre = jours.size === 1 ? 'Requests of ' + [...jours][0] : 'Requests';
    return h('aside', { class: 'laterale' },
      h('div', { class: 'etiq' }, titre),
      demandes.map(j => {
        const ch = choixDe(j);
        const resume = j.sessions.reduce((n, s) => n + s.appels, 0);
        return h('div', { class: 'demande' },
          h('button', { type: 'button', class: 'session-carte' + (j.id === courante.j.id ? ' choisie' : '') + (j.verification.ok ? '' : ' rompue'),
            onclick: () => allerA('journal', { j: j.id, s: ch.s ? ch.s.id : '' }) },
            h('span', { class: 'gras' }, nomChoix(ch) + ' · ' + heureMin(dateDemande(j))),
            h('span', { class: 'moyen second' }, j.demande ? j.demande.slice(0, 70) + (j.demande.length > 70 ? '…' : '') : j.id),
            h('span', { class: 'petit second' }, (j.sessions.length ? resume + ' call(s)' : 'no tool call') + (j.rejetee ? ' · rejected' : j.statut ? ' · ' + STATUT_SESSION[j.statut] : '')),
            ligneChaineSession(j)),
          j.sessions.length > 1 ? j.sessions.map(s => h('button', { type: 'button', class: 'petit second' + (j.id === courante.j.id && courante.s && s.id === courante.s.id ? ' gras' : ''),
            onclick: () => allerA('journal', { j: j.id, s: s.id }) }, '↳ session ' + nomSession(s.id) + ' · ' + s.appels + ' call(s)')) : null);
      }));
  }

  function serveurCellule(e) {
    const a = e.attestation;
    if (!a) return h('span', {}, e.serveur + ' · ', nd('attestation not available'));
    return h('span', {}, e.serveur + ' · ', a.valide ?
      h('span', { class: 'ok-txt' }, a.mode + ' ✓') : h('span', { class: 'non-txt' }, a.mode + ' ✗'));
  }

  function decisionCellule(e, fonde) {
    const ids = [...new Set(e.applications.filter(a => a.appliquee).map(a => a.id))];
    return h('span', { style: 'display:flex;gap:6px;align-items:center;flex-wrap:wrap' },
      badge(e.badge, e.libelle),
      fonde && fonde.length ? h('span', { class: 'lien-fonde' }, 'fonde ' + fonde.map(s => '#' + s).join(' ')) : null,
      !fonde && ids.length ? h('span', { class: 'second' }, ids.join(' · ')) : null,
      e.valide_au_seq ? h('span', { class: 'petit second' }, 'approved afterwards (call #' + e.valide_au_seq + ')') : null,
      e.valide_par && e.valide_par.length ? h('span', { class: 'petit ok-txt' }, 'approved by ' + e.valide_par.map(nomValideur).join(', ')) : null,
      e.suspect ? h('span', { class: 'non-txt', style: 'color:var(--ref)' }, 'unreliable') : null);
  }

  function rang(e, choisi, fonde, onclick) {
    const cls = ['rang', 'grille-appels'];
    if (!e.sous_agent) cls.push('');
    if (e.seq === choisi) cls.push('choisi', e.badge === 'en_attente' ? 'attente' : '');
    if (fonde && fonde.length) cls.push('fonde');
    if (e.suspect) cls.push('suspect');
    return h('div', { class: cls.join(' '), role: 'button', tabindex: '0', onclick,
      onkeydown: ev => { if (ev.key === 'Enter' || ev.key === ' ') { ev.preventDefault(); onclick(); } } },
      h('span', { class: 'num' }, e.seq),
      h('span', e.sous_agent ? { class: 'tiret', style: 'border-radius:12px;padding:2px 8px;display:inline-block' } : {}, e.sous_agent || nd('—')),
      outilLisible(e.serveur, e.outil, { class: 'moyen' }),
      serveurCellule(e),
      decisionCellule(e, fonde));
  }

  function carteDirective(a, liens) {
    const origine = liens.filter(l => l.directive === a.id);
    return h('div', { class: 'carte compacte' },
      h('div', { class: 'gras' }, versionTexte(a.id, a.version) + (a.texte ? ' · ' + Titre(a.texte) : '')),
      h('div', { class: 'moyen second' }, 'Legal basis: ' + fondementTexte(a.fondement)),
      h('div', { class: 'moyen' }, 'Reason: ' + motifAffiche(a.motif)),
      a.appliquee ? null : h('div', { class: 'moyen second' }, 'Rule in observation: decision journaled, never applied.'),
      origine.length ? h('div', { class: 'moyen', style: 'color:var(--accent)' },
        'Originating consultation: ' + origine.map(l => '#' + l.seq).join(', ')) : null);
  }

  // Titre abrégé : même règle que le serveur (internal/console.Titre).
  function Titre(t) {
    t = String(t).trim();
    const i = t.search(/[.;:]/);
    if (i > 0) t = t.slice(0, i).trim();
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
    if (!e) return h('aside', { class: 'volet' }, h('div', { class: 'etiq' }, 'Decision'), nd('No tool call for this request.'));
    const appliquees = e.applications.filter(a => a.appliquee || a.declenchee);
    const liens = e.provenance || [];
    const roles = e.validation && e.validation.roles && e.validation.roles.length ? e.validation.roles.join(', ') + (e.validation.quatre_yeux ? ' · quatre yeux' : '') : null;
    let transmission = e.transmis ? 'forwarded to the server' : (e.badge === 'en_attente' ? 'none (pending)' : 'none (not forwarded)');
    const notifs = e.notifications || [], appros = e.approbations || [];
    return h('aside', { class: 'volet' },
      h('div', { class: 'etiq' }, 'Decision on call #' + e.seq),
      h('div', { style: 'display:flex;align-items:center;gap:10px;flex-wrap:wrap' }, badge(e.badge, e.libelle, 'grand'),
        h('span', { class: 'moyen second' }, heureSec(e.ts) + ' · Europe/Paris')),
      e.origine_libelle ? h('p', { class: 'moyen gras', style: 'color:var(--non)' }, e.origine_libelle) : null,
      e.rejet ? h('div', { class: 'carte compacte', style: 'border-color:var(--ref)' },
        h('b', { class: 'erreur' }, texteRejet(e.rejet)), h('span', { class: 'petit second' }, e.rejet.mention)) : null,
      e.valide_par && e.valide_par.length ? h('p', { class: 'moyen ok-txt gras' }, 'Approved by ' + e.valide_par.map(v => nomValideur(v) + ' (' + v.role + ')').join(', ')) : null,
      appliquees.length ? appliquees.map(a => carteDirective(a, liens)) :
        h('div', { class: 'carte compacte' }, h('div', { class: 'moyen' }, motifAffiche(e.motif)), h('div', { class: 'moyen second' }, 'No rule applied to this call.')),
      h('div', { style: 'display:flex;flex-direction:column;gap:6px' },
        h('div', { class: 'kv' }, h('span', {}, 'Required role'), roles ? h('span', {}, roles) : nd()),
        h('div', { class: 'kv' }, h('span', {}, 'Rules evaluated'), h('span', {},
          e.evaluees + ' · ' + e.appliquees + ' applied · ' + e.indeterminees + ' undetermined')),
        h('div', { class: 'kv' }, h('span', {}, 'Access · target labels'), h('span', {}, (e.acces || ND) + ' · ' + ((e.etiquettes || []).join(', ') || 'aucune'))),
        h('div', { class: 'kv' }, h('span', {}, 'Digital fingerprint of the call'), h('span', { class: 'mono' }, court(e.empreinte_transmise) || ND)),
        h('div', { class: 'kv' }, h('span', {}, 'Journal entry'), h('span', { class: 'mono' }, '#' + e.numero + (e.prev ? ' · previous ' + court(e.prev) : ''))),
        h('div', { class: 'kv' }, h('span', {}, 'Transmission'), h('span', {}, transmission)),
        h('div', { class: 'kv' }, h('span', {}, 'Time added by the engine'), e.duree_us ? h('span', {}, ms(e.duree_us / 1000)) : nd('not measured'))),
      h('div', { class: 'etiq' }, 'Notifications'),
      notifs.length ? notifs.map(n => h('div', { class: 'carte compacte' },
        h('div', { class: 'gras' }, n.destinataire_role + ' · ' + (RYTHME[n.rythme] || n.rythme)),
        h('div', { class: 'moyen second' }, 'Rule ' + n.directive + ', version ' + n.version + (n.si_echec === 'refuser' ? ' · if delivery fails, the call is refused' : '')),
        h('div', { class: 'moyen ' + (n.statut === 'echec' ? 'erreur gras' : 'ok-txt') }, n.statut ? 'Delivery: ' + (REMISE[n.statut] || n.statut) + (n.remis ? ' (' + n.remis + ' destinataire(s))' : '') : 'Delivery: ' + ND))) : h('span', { class: 'moyen second' }, 'No notification requested.'),
      h('div', { class: 'etiq' }, 'Approvals'),
      appros.length ? appros.map(a => h('div', { class: 'carte compacte' },
        h('div', { class: 'gras ' + (a.statut === 'acceptee' ? 'ok-txt' : 'erreur') }, (a.statut === 'acceptee' ? 'Accepted' : 'Refused') + ' · ' + heureSec(a.ts)),
        (a.valideurs || []).map(v => h('div', { class: 'moyen' }, v.identite + ' — role “' + v.role + '” on ' + jourMoisHeure(v.date))),
        a.motif ? h('div', { class: 'moyen erreur' }, 'Reason: ' + a.motif) : null)) : h('span', { class: 'moyen second' }, 'No approval.'),
      (e.alertes || []).map(a => h('p', { class: 'erreur moyen' }, '⚠ ' + a)),
      h('div', { style: 'margin-top:auto;display:flex;gap:10px' },
        h('a', { class: 'btn contour', style: 'flex:1', href: '#avant-apres?' + new URLSearchParams({ s: v.journal + '|' + v.session }) }, 'Replay this call'),
        e.badge === 'en_attente' && !e.valide_au_seq ? h('a', { class: 'btn', style: 'flex:1', href: '#validation?' + new URLSearchParams({ a: v.journal }) }, 'Open the approval') : null));
  }

  // ---- onglet « Demandé / décidé » : le modèle d'un côté, l'engine de l'autre.
  function argument(a) {
    return h('div', { class: 'mono petit', style: 'display:flex;gap:8px;flex-wrap:wrap' },
      h('span', { class: 'second' }, a.chemin),
      a.retire ? h('span', { class: 'non-txt gras', title: 'Value removed by the engine: never displayed' }, 'removed by the engine')
        : h('span', {}, a.valeur));
  }
  function celluleDemande(l) {
    return h('div', { style: 'display:flex;flex-direction:column;gap:6px' },
      h('div', { style: 'display:flex;gap:8px;align-items:center;flex-wrap:wrap' },
        outilLisible('', l.outil, { class: 'gras' }),
        l.delegation ? h('span', { class: 'pastille' }, 'delegation · outside the gateway') : null,
        l.ts ? h('span', { class: 'petit second' }, heureSec(l.ts)) : null),
      l.arguments.length ? l.arguments.map(argument) : h('span', { class: 'petit second' }, 'no argument'),
      l.sans_demande ? h('span', { class: 'petit second' }, 'model request not found (arguments read from the vault)') : null);
  }
  function celluleDecision(l) {
    if (l.delegation) return h('span', { class: 'second moyen' }, 'This delegation does not go through the gateway: no engine decision.');
    if (l.non_recue) return h('span', { class: 'non-txt gras' }, 'Never received by the gateway: no decision.');
    const e = l.decision;
    const appliquees = e.applications.filter(a => a.appliquee);
    const roles = e.validation && e.validation.roles && e.validation.roles.length ? e.validation.roles.join(', ') : null;
    return h('div', { style: 'display:flex;flex-direction:column;gap:6px' },
      h('div', { style: 'display:flex;gap:8px;align-items:center;flex-wrap:wrap' }, badge(e.badge, e.libelle),
        h('span', { class: 'petit second' }, 'call #' + e.seq + ' · ' + heureSec(e.ts))),
      appliquees.length ? appliquees.map(a => h('div', { class: 'moyen' },
        h('span', { class: 'gras' }, versionTexte(a.id, a.version) + (a.texte ? ' · ' + Titre(a.texte) : '')),
        h('div', { class: 'second petit' }, 'Legal basis: ' + (fondementTexte(a.fondement) || ND)))) :
        h('div', { class: 'moyen second' }, 'No rule applied'),
      h('div', { class: 'petit second' }, motifAffiche(e.motif)),
      (e.champs_retires || []).length ? h('div', { class: 'petit' }, 'Fields removed before sending: ' + e.champs_retires.join(', ')) : null,
      e.badge === 'en_attente' ? h('div', { class: 'moyen', style: 'color:var(--accent)' }, 'Held' + (roles ? ' : validation ' + roles + ' requise' : '')) : null,
      e.rejet ? h('div', { class: 'moyen erreur' }, 'Held, then ' + texteRejet(e.rejet).charAt(0).toLowerCase() + texteRejet(e.rejet).slice(1) + ' (' + e.rejet.mention + ')') : null,
      (l.intermediaires || []).map(i => h('div', { class: 'petit second' },
        'Resumed (call #' + i.seq + ', ' + heureSec(i.ts) + ') then held again: ' + motifAffiche(i.motif))),
      l.suite ? h('div', { class: 'carte compacte' },
        h('div', { class: 'gras ok-txt' }, 'Then ' + (l.suite.valide_par && l.suite.valide_par.length ? 'approved by ' + l.suite.valide_par.map(v => nomValideur(v) + ' (' + v.role + ')').join(', ') : 'repris')),
        h('div', { class: 'petit second' }, badge(l.suite.badge, l.suite.libelle).textContent + ' · call #' + l.suite.seq + ' · ' + heureSec(l.suite.ts))) : null);
  }
  function tableauDemande(d) {
    if (!d) return h('p', { class: 'second' }, 'Chargement…');
    return h('div', { style: 'display:flex;flex-direction:column;gap:10px' },
      d.fichiers ? null : h('p', { class: 'second moyen' }, 'The model\'s requests are not available for this session (no mistral/ files): only the engine decisions are shown.'),
      h('div', { class: 'grille-demande entete-colonnes', style: 'display:grid;grid-template-columns:40px minmax(0,1fr) minmax(0,1fr);gap:16px' },
        ['#', 'What the model asked for', 'What the engine decided'].map(t => h('span', {}, t))),
      d.demandes.length ? d.demandes.map(l => h('div', { class: 'rang' + (l.hors_gate ? ' fonde' : ''), style: 'display:grid;grid-template-columns:40px minmax(0,1fr) minmax(0,1fr);gap:16px;padding:12px' },
        h('span', { class: 'num' }, l.ordre), celluleDemande(l), celluleDecision(l))) :
        h('p', { class: 'second' }, 'No call for this request.'));
  }

  ECRANS.journal = async (ctx) => {
    const ch = sessionChoisie(ctx.q);
    if (!ch) { ctx.monter(aucunJournal()); return; }
    let choisi = Number(ctx.q.get('a')) || null;
    let signature = null, derniere = null;
    const racine = h('div', { class: 'corps' });
    ctx.monter(racine);
    const chemin = cheminJournal(ch, 'chaine');
    const onglet = ctx.q.get('onglet') === 'demande' ? 'demande' : 'chaine';
    let donneesDemande = null;
    const lienOnglet = (cle, texte) => h('a', { class: 'onglet' + (onglet === cle ? ' actif' : ''), 'data-onglet': cle, style: onglet === cle ? 'font-weight:700;text-decoration:none;color:var(--accent)' : '',
      href: '#journal?' + new URLSearchParams(Object.assign({ j: ch.j.id, s: ch.s ? ch.s.id : '' }, cle === 'demande' ? { onglet: 'demande' } : {})) }, texte);
    const onglets = () => h('div', { class: 'onglets', style: 'display:flex;gap:18px;border-bottom:1px solid var(--trait);padding-bottom:6px' },
      lienOnglet('chaine', 'Chain'), lienOnglet('demande', 'Requested / decided'));

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
        'Rules in force since ', h('b', {}, jourMoisHeure(v.regles.depuis)), ' · Juriste : ', h('b', {}, v.regles.auteur || ND)]
        : ['Rules in force: ', nd()]);
      racine.textContent = '';
      racine.append(
        liste(ch),
        h('main', { class: 'principal' },
          h('div', { style: 'display:flex;align-items:flex-end;justify-content:space-between;gap:16px;flex-wrap:wrap' },
            h('div', { style: 'display:flex;flex-direction:column;gap:4px' },
              h('h1', {}, 'Request ' + nomSession(v.session || v.journal)),
              h('div', { class: 'second' }, 'Request: ', v.demande ? '« ' + v.demande + ' »' : nd('not available')),
              v.rejet ? h('div', { class: 'erreur gras' }, 'Request rejected by ' + parQui(v.rejet) + ' at ' + heureSec(v.rejet.date) + ' · ' + v.rejet.mention) : null),
            pastilleChaine(v.verification)),
          onglets(),
          onglet === 'demande' ? tableauDemande(donneesDemande) : null,
          onglet === 'demande' ? null : h('div', { class: 'chemin' },
            h('div', { class: 'col' }, h('span', { class: 'petit second' }, 'Requester'),
              dem ? h('span', { class: 'gras' }, (v.demandeur_fiche && v.demandeur_fiche.nom) || dem.identite) : h('span', { class: 'gras' }, nd()),
              dem ? h('span', { class: 'petit second' }, dem.identite) : null,
              h('span', { class: 'petit second' }, dem ? 'groupe(s) : ' + ((dem.groupes || []).join(', ') || 'aucun') + ' · role(s): ' + ((v.demandeur_fiche && v.demandeur_fiche.roles || []).join(', ') || 'aucun') : 'role and group: ' + ND),
              h('span', { class: 'petit second' }, 'identity proven by: ' + (v.source_identite || ND))),
            h('span', { class: 'fl' }, '→'),
            h('div', { class: 'col' }, h('span', { class: 'petit second' }, 'Orchestrator'),
              h('span', { class: 'gras' }, v.appelant || ND),
              h('span', { class: 'petit second' }, 'identity proven by the transport')),
            h('span', { class: 'fl' }, '→'),
            h('div', { style: 'display:flex;gap:8px;flex-wrap:wrap' },
              v.sous_agents.length ? v.sous_agents.map(s => h('span', { class: 'pastille' }, s)) : h('span', { class: 'petit second' }, 'no declared sub-agent')),
            h('span', { class: 'petit second' }, 'sub-agents: declared, not proven')),
          onglet === 'demande' ? null : h('div', { class: 'grille-appels entete-colonnes' },
            ['#', 'Sous-agent', 'Call', 'Serveur · attestation', 'Decision'].map(t => h('span', {}, t))),
          onglet === 'demande' ? null : h('div', { style: 'display:flex;flex-direction:column;gap:8px' },
            v.etapes.length ? v.etapes.map(x => rang(x, choisi, fonde[x.seq], () => { choisi = x.seq; dessiner(derniere); })) :
              h('p', { class: 'second' }, 'No tool call for this request.')),
          onglet === 'demande' ? null : h('div', { class: 'petit second', style: 'display:flex;gap:20px;flex-wrap:wrap;margin-top:4px' },
            h('span', {}, 'Blue frame: a consultation that grounded a provenance decision.'),
            h('span', {}, 'Dashed: declared by the orchestration code, recorded, not proven.'),
            v.fermee ? h('span', {}, 'Session closed: ' + v.fermeture) : h('span', {}, h('span', { class: 'vif' }), 'live tracking'))),
        ...(onglet === 'demande' ? [] : [volet(v, e)]));
    }

    async function rafraichir() {
      const v = await api(chemin);
      if (!ctx.vivant()) return;
      derniere = v;
      if (onglet === 'demande') donneesDemande = await api(cheminJournal(ch, 'demande'));
      if (!ctx.vivant()) return;
      const sig = v.signature + '|' + v.verification.ok + '|' + v.verification.erreur +
        (donneesDemande ? '|' + donneesDemande.demandes.length + '|' + donneesDemande.demandes.map(l => (l.decision ? l.decision.seq : '') + (l.suite ? 's' + l.suite.seq : '')).join(',') : '');
      if (sig === signature) return;
      signature = sig;
      dessiner(v);
    }
    await rafraichir();
    ctx.suivre(rafraichir);
  };
})();
