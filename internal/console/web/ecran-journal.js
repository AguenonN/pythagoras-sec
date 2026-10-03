'use strict';
// Écran 1 · Journal et graphe de chaîne.

(function () {
  const STATUT_SESSION = { en_cours: 'en cours', en_attente: 'en attente de validation', termine: 'terminée', bloque: 'bloquée', erreur: 'en erreur' };
  const RYTHME = { temps_reel: 'en temps réel', synthese: 'en synthèse' };
  const REMISE = { remise: 'remise', echec: 'non remise', planifiee: 'planifiée' };

  function ligneChaineSession(j) {
    const v = j.verification;
    if (v.ok) return h('span', { class: 'petit ok-txt' }, 'Chaîne vérifiée · ' + j.entrees + ' entrées');
    if (v.cle_absente) return h('span', { class: 'petit gras', style: 'color:var(--ref)' }, 'Chaîne non vérifiable · clé absente');
    return h('span', { class: 'petit gras', style: 'color:var(--ref)' }, 'Chaîne rompue à l\'entrée #' + (j.premier_ecart + 1));
  }

  function liste(courante) {
    const demandes = toutesDemandes();
    const jours = new Set(demandes.map(j => jourComplet(dateDemande(j))));
    const titre = jours.size === 1 ? 'Demandes du ' + [...jours][0] : 'Demandes';
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
            h('span', { class: 'petit second' }, (j.sessions.length ? resume + ' appel(s)' : 'aucun appel d\'outil') + (j.statut ? ' · ' + STATUT_SESSION[j.statut] : '')),
            ligneChaineSession(j)),
          j.sessions.length > 1 ? j.sessions.map(s => h('button', { type: 'button', class: 'petit second' + (j.id === courante.j.id && courante.s && s.id === courante.s.id ? ' gras' : ''),
            onclick: () => allerA('journal', { j: j.id, s: s.id }) }, '↳ session ' + nomSession(s.id) + ' · ' + s.appels + ' appel(s)')) : null);
      }));
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
      badge(e.badge, e.libelle),
      fonde && fonde.length ? h('span', { class: 'lien-fonde' }, 'fonde ' + fonde.map(s => '#' + s).join(' ')) : null,
      !fonde && ids.length ? h('span', { class: 'second' }, ids.join(' · ')) : null,
      e.valide_au_seq ? h('span', { class: 'petit second' }, 'validé ensuite (appel n° ' + e.valide_au_seq + ')') : null,
      e.valide_par && e.valide_par.length ? h('span', { class: 'petit ok-txt' }, 'validé par ' + e.valide_par.map(nomValideur).join(', ')) : null,
      e.suspect ? h('span', { class: 'non-txt', style: 'color:var(--ref)' }, 'non fiable') : null);
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
      h('span', { class: 'mono moyen' }, e.outil),
      serveurCellule(e),
      decisionCellule(e, fonde));
  }

  function carteDirective(a, liens) {
    const origine = liens.filter(l => l.directive === a.id);
    return h('div', { class: 'carte compacte' },
      h('div', { class: 'gras' }, versionTexte(a.id, a.version) + (a.texte ? ' · ' + Titre(a.texte) : '')),
      h('div', { class: 'moyen second' }, 'Fondement : ' + fondementTexte(a.fondement)),
      h('div', { class: 'moyen' }, 'Motif : ' + motifAffiche(a.motif)),
      a.appliquee ? null : h('div', { class: 'moyen second' }, 'Directive en observation : décision journalisée, jamais appliquée.'),
      origine.length ? h('div', { class: 'moyen', style: 'color:var(--accent)' },
        'Consultation à l\'origine : ' + origine.map(l => '#' + l.seq).join(', ')) : null);
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
    if (!e) return h('aside', { class: 'volet' }, h('div', { class: 'etiq' }, 'Décision'), nd('Aucun appel d\'outil pour cette demande.'));
    const appliquees = e.applications.filter(a => a.appliquee || a.declenchee);
    const liens = e.provenance || [];
    const roles = e.validation && e.validation.roles && e.validation.roles.length ? e.validation.roles.join(', ') + (e.validation.quatre_yeux ? ' · quatre yeux' : '') : null;
    let transmission = e.transmis ? 'transmis au serveur' : (e.badge === 'en_attente' ? 'aucune (en attente)' : 'aucune (non transmis)');
    const notifs = e.notifications || [], appros = e.approbations || [];
    return h('aside', { class: 'volet' },
      h('div', { class: 'etiq' }, 'Décision de l\'appel #' + e.seq),
      h('div', { style: 'display:flex;align-items:center;gap:10px;flex-wrap:wrap' }, badge(e.badge, e.libelle, 'grand'),
        h('span', { class: 'moyen second' }, heureSec(e.ts) + ' · Europe/Paris')),
      e.origine_libelle ? h('p', { class: 'moyen gras', style: 'color:var(--non)' }, e.origine_libelle) : null,
      e.valide_par && e.valide_par.length ? h('p', { class: 'moyen ok-txt gras' }, 'Validé par ' + e.valide_par.map(v => nomValideur(v) + ' (' + v.role + ')').join(', ')) : null,
      appliquees.length ? appliquees.map(a => carteDirective(a, liens)) :
        h('div', { class: 'carte compacte' }, h('div', { class: 'moyen' }, motifAffiche(e.motif)), h('div', { class: 'moyen second' }, 'Aucune directive appliquée à cet appel.')),
      h('div', { style: 'display:flex;flex-direction:column;gap:6px' },
        h('div', { class: 'kv' }, h('span', {}, 'Rôle requis'), roles ? h('span', {}, roles) : nd()),
        h('div', { class: 'kv' }, h('span', {}, 'Directives évaluées'), h('span', {},
          e.evaluees + ' · ' + e.appliquees + ' appliquée(s) · ' + e.indeterminees + ' indéterminée(s)')),
        h('div', { class: 'kv' }, h('span', {}, 'Accès · étiquettes de la cible'), h('span', {}, (e.acces || ND) + ' · ' + ((e.etiquettes || []).join(', ') || 'aucune'))),
        h('div', { class: 'kv' }, h('span', {}, 'Signature numérique de l\'appel (empreinte)'), h('span', { class: 'mono' }, court(e.empreinte_transmise) || ND)),
        h('div', { class: 'kv' }, h('span', {}, 'Entrée du journal'), h('span', { class: 'mono' }, '#' + e.numero + (e.prev ? ' · précédente ' + court(e.prev) : ''))),
        h('div', { class: 'kv' }, h('span', {}, 'Transmission'), h('span', {}, transmission)),
        h('div', { class: 'kv' }, h('span', {}, 'Temps ajouté par l\'engine'), e.duree_us ? h('span', {}, ms(e.duree_us / 1000)) : nd('non mesuré'))),
      h('div', { class: 'etiq' }, 'Notifications'),
      notifs.length ? notifs.map(n => h('div', { class: 'carte compacte' },
        h('div', { class: 'gras' }, n.destinataire_role + ' · ' + (RYTHME[n.rythme] || n.rythme)),
        h('div', { class: 'moyen second' }, 'Directive ' + n.directive + ', version ' + n.version + (n.si_echec === 'refuser' ? ' · si la remise échoue, l\'appel est refusé' : '')),
        h('div', { class: 'moyen ' + (n.statut === 'echec' ? 'erreur gras' : 'ok-txt') }, n.statut ? 'Remise : ' + (REMISE[n.statut] || n.statut) + (n.remis ? ' (' + n.remis + ' destinataire(s))' : '') : 'Remise : ' + ND))) : h('span', { class: 'moyen second' }, 'Aucune notification demandée.'),
      h('div', { class: 'etiq' }, 'Approbations'),
      appros.length ? appros.map(a => h('div', { class: 'carte compacte' },
        h('div', { class: 'gras ' + (a.statut === 'acceptee' ? 'ok-txt' : 'erreur') }, (a.statut === 'acceptee' ? 'Acceptée' : 'Refusée') + ' · ' + heureSec(a.ts)),
        (a.valideurs || []).map(v => h('div', { class: 'moyen' }, v.identite + ' — rôle « ' + v.role + ' » au ' + jourMoisHeure(v.date))),
        a.motif ? h('div', { class: 'moyen erreur' }, 'Motif : ' + a.motif) : null)) : h('span', { class: 'moyen second' }, 'Aucune approbation.'),
      (e.alertes || []).map(a => h('p', { class: 'erreur moyen' }, '⚠ ' + a)),
      h('div', { style: 'margin-top:auto;display:flex;gap:10px' },
        h('a', { class: 'btn contour', style: 'flex:1', href: '#avant-apres?' + new URLSearchParams({ s: v.journal + '|' + v.session }) }, 'Rejouer cet appel'),
        e.badge === 'en_attente' && !e.valide_au_seq ? h('a', { class: 'btn', style: 'flex:1', href: '#validation?' + new URLSearchParams({ a: v.journal }) }, 'Ouvrir la validation') : null));
  }

  // ---- onglet « Demandé / décidé » : le modèle d'un côté, l'engine de l'autre.
  function argument(a) {
    return h('div', { class: 'mono petit', style: 'display:flex;gap:8px;flex-wrap:wrap' },
      h('span', { class: 'second' }, a.chemin),
      a.retire ? h('span', { class: 'non-txt gras', title: 'Valeur retirée par l\'engine : jamais affichée' }, 'retiré par l\'engine')
        : h('span', {}, a.valeur));
  }
  function celluleDemande(l) {
    return h('div', { style: 'display:flex;flex-direction:column;gap:6px' },
      h('div', { style: 'display:flex;gap:8px;align-items:center;flex-wrap:wrap' },
        h('span', { class: 'mono gras' }, l.outil),
        l.delegation ? h('span', { class: 'pastille' }, 'délégation · hors du gate') : null,
        l.ts ? h('span', { class: 'petit second' }, heureSec(l.ts)) : null),
      l.arguments.length ? l.arguments.map(argument) : h('span', { class: 'petit second' }, 'aucun argument'),
      l.sans_demande ? h('span', { class: 'petit second' }, 'demande du modèle non retrouvée (arguments lus au coffre)') : null);
  }
  function celluleDecision(l) {
    if (l.delegation) return h('span', { class: 'second moyen' }, 'Cette délégation ne passe pas par le gate : aucune décision de l\'engine.');
    if (l.non_recue) return h('span', { class: 'non-txt gras' }, 'Jamais reçu par le gate : aucune décision.');
    const e = l.decision;
    const appliquees = e.applications.filter(a => a.appliquee);
    const roles = e.validation && e.validation.roles && e.validation.roles.length ? e.validation.roles.join(', ') : null;
    return h('div', { style: 'display:flex;flex-direction:column;gap:6px' },
      h('div', { style: 'display:flex;gap:8px;align-items:center;flex-wrap:wrap' }, badge(e.badge, e.libelle),
        h('span', { class: 'petit second' }, 'appel n° ' + e.seq + ' · ' + heureSec(e.ts))),
      appliquees.length ? appliquees.map(a => h('div', { class: 'moyen' },
        h('span', { class: 'gras' }, versionTexte(a.id, a.version) + (a.texte ? ' · ' + Titre(a.texte) : '')),
        h('div', { class: 'second petit' }, 'Fondement : ' + (fondementTexte(a.fondement) || ND)))) :
        h('div', { class: 'moyen second' }, 'Aucune directive appliquée'),
      h('div', { class: 'petit second' }, motifAffiche(e.motif)),
      (e.champs_retires || []).length ? h('div', { class: 'petit' }, 'Champs retirés avant envoi : ' + e.champs_retires.join(', ')) : null,
      e.badge === 'en_attente' ? h('div', { class: 'moyen', style: 'color:var(--accent)' }, 'Mise en attente' + (roles ? ' : validation ' + roles + ' requise' : '')) : null,
      (l.intermediaires || []).map(i => h('div', { class: 'petit second' },
        'Repris (appel n° ' + i.seq + ', ' + heureSec(i.ts) + ') puis remis en attente : ' + motifAffiche(i.motif))),
      l.suite ? h('div', { class: 'carte compacte' },
        h('div', { class: 'gras ok-txt' }, 'Puis ' + (l.suite.valide_par && l.suite.valide_par.length ? 'validé par ' + l.suite.valide_par.map(v => nomValideur(v) + ' (' + v.role + ')').join(', ') : 'repris')),
        h('div', { class: 'petit second' }, badge(l.suite.badge, l.suite.libelle).textContent + ' · appel n° ' + l.suite.seq + ' · ' + heureSec(l.suite.ts))) : null);
  }
  function tableauDemande(d) {
    if (!d) return h('p', { class: 'second' }, 'Chargement…');
    return h('div', { style: 'display:flex;flex-direction:column;gap:10px' },
      d.fichiers ? null : h('p', { class: 'second moyen' }, 'Les demandes du modèle ne sont pas disponibles pour cette session (aucun fichier mistral/) : seules les décisions de l\'engine sont montrées.'),
      h('div', { class: 'grille-demande entete-colonnes', style: 'display:grid;grid-template-columns:40px minmax(0,1fr) minmax(0,1fr);gap:16px' },
        ['#', 'Ce que le modèle a demandé', 'Ce que l\'engine a décidé'].map(t => h('span', {}, t))),
      d.demandes.length ? d.demandes.map(l => h('div', { class: 'rang' + (l.hors_gate ? ' fonde' : ''), style: 'display:grid;grid-template-columns:40px minmax(0,1fr) minmax(0,1fr);gap:16px;padding:12px' },
        h('span', { class: 'num' }, l.ordre), celluleDemande(l), celluleDecision(l))) :
        h('p', { class: 'second' }, 'Aucun appel pour cette demande.'));
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
      lienOnglet('chaine', 'Chaîne'), lienOnglet('demande', 'Demandé / décidé'));

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
              h('h1', {}, 'Demande ' + nomSession(v.session || v.journal)),
              h('div', { class: 'second' }, 'Demande : ', v.demande ? '« ' + v.demande + ' »' : nd('non disponible'))),
            pastilleChaine(v.verification)),
          onglets(),
          onglet === 'demande' ? tableauDemande(donneesDemande) : null,
          onglet === 'demande' ? null : h('div', { class: 'chemin' },
            h('div', { class: 'col' }, h('span', { class: 'petit second' }, 'Demandeur'),
              dem ? h('span', { class: 'gras' }, (v.demandeur_fiche && v.demandeur_fiche.nom) || dem.identite) : h('span', { class: 'gras' }, nd()),
              dem ? h('span', { class: 'petit second' }, dem.identite) : null,
              h('span', { class: 'petit second' }, dem ? 'groupe(s) : ' + ((dem.groupes || []).join(', ') || 'aucun') + ' · rôle(s) : ' + ((v.demandeur_fiche && v.demandeur_fiche.roles || []).join(', ') || 'aucun') : 'rôle et groupe : ' + ND),
              h('span', { class: 'petit second' }, 'identité prouvée par : ' + (v.source_identite || ND))),
            h('span', { class: 'fl' }, '→'),
            h('div', { class: 'col' }, h('span', { class: 'petit second' }, 'Orchestrateur'),
              h('span', { class: 'gras' }, v.appelant || ND),
              h('span', { class: 'petit second' }, 'identité prouvée par le transport')),
            h('span', { class: 'fl' }, '→'),
            h('div', { style: 'display:flex;gap:8px;flex-wrap:wrap' },
              v.sous_agents.length ? v.sous_agents.map(s => h('span', { class: 'pastille' }, s)) : h('span', { class: 'petit second' }, 'aucun sous-agent déclaré')),
            h('span', { class: 'petit second' }, 'sous-agents : déclaré, non prouvé')),
          onglet === 'demande' ? null : h('div', { class: 'grille-appels entete-colonnes' },
            ['#', 'Sous-agent', 'Appel', 'Serveur · attestation', 'Décision'].map(t => h('span', {}, t))),
          onglet === 'demande' ? null : h('div', { style: 'display:flex;flex-direction:column;gap:8px' },
            v.etapes.length ? v.etapes.map(x => rang(x, choisi, fonde[x.seq], () => { choisi = x.seq; dessiner(derniere); })) :
              h('p', { class: 'second' }, 'Aucun appel d\'outil pour cette demande.')),
          onglet === 'demande' ? null : h('div', { class: 'petit second', style: 'display:flex;gap:20px;flex-wrap:wrap;margin-top:4px' },
            h('span', {}, 'Cadre bleu : consultation qui a fondé une décision de provenance.'),
            h('span', {}, 'Pointillés : déclaré par le code d\'orchestration, consigné, non prouvé.'),
            v.fermee ? h('span', {}, 'Session fermée : ' + v.fermeture) : h('span', {}, h('span', { class: 'vif' }), 'suivi en continu'))),
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
