'use strict';
// Écran 2 · Poste conseiller : saisie de la demande, frise en direct, réponse
// au conseiller, coût des règles mesuré au journal.

(function () {
  const ORIGINES = { direct: 'source directe', rejeu: 'source enregistrée (rejeu)' };
  const STATUT = { en_cours: 'en cours', en_attente: 'en attente de validation humaine', termine: 'terminée', bloque: 'bloquée', erreur: 'en erreur' };

  function detail(e) {
    const ids = [...new Set(e.applications.filter(a => a.appliquee).map(a => a.id))];
    if (e.valide_par && e.valide_par.length) return 'Validé par ' + e.valide_par.map(v => nomValideur(v) + ' (' + v.role + ')').join(', ') + ', puis transmis';
    switch (e.badge) {
      case 'caviarde':
        return 'Champs retirés avant envoi : ' + (e.champs_retires || []).join(', ') + (ids.length ? ' (' + ids.join(', ') + ')' : '');
      case 'en_attente': {
        const roles = e.validation && e.validation.roles && e.validation.roles.length ? e.validation.roles.join(', ') : 'humaine';
        return 'Validation ' + roles + ' requise' + (ids.length ? ' (' + ids.join(', ') + ')' : '') + (e.valide_au_seq ? ' — validée ensuite (appel n° ' + e.valide_au_seq + ')' : '');
      }
      case 'refuse':
      case 'source_non_attestee':
        return motifAffiche(e.motif);
      case 'indisponible':
        return 'Aucune donnée présentée comme obtenue';
      case 'rejete':
        return texteRejet(e.rejet) + ' — ' + e.rejet.mention;
    }
    const morceaux = [];
    if (e.source && ORIGINES[e.source.origine]) morceaux.push(ORIGINES[e.source.origine]);
    if (e.duree_ms) morceaux.push(e.duree_ms + ' ms chez le serveur');
    return morceaux.join(' · ') || 'Réponse du serveur remise à l\'agent';
  }

  function ligne(e) {
    const cls = e.badge === 'en_attente' ? 'attente' : (e.suspect ? 'suspect' : '');
    return h('li', { class: cls },
      h('span', { class: 'heure' }, heureSec(e.ts)),
      outilLisible(e.serveur, e.outil, { class: 'libelle' }),
      badge(e.badge, e.libelle),
      h('span', { class: 'moyen ' + (e.badge === 'en_attente' ? '' : 'second'), style: e.badge === 'en_attente' ? 'color:var(--accent)' : '' }, detail(e)),
      e.suspect ? h('span', { class: 'petit gras', style: 'color:var(--ref)' }, 'entrée non fiable') : null);
  }

  function chiffre(valeur, texte) {
    return h('div', { style: 'display:flex;flex-direction:column;gap:2px' },
      h('span', { class: 'chiffre' }, valeur), h('span', { class: 'moyen second' }, texte));
  }

  function duree(s) {
    if (s < 90) return s + ' s';
    return Math.floor(s / 60) + ' min ' + String(s % 60).padStart(2, '0') + ' s';
  }

  function cout(c) {
    const mesure = c.mesures > 0;
    return h('div', { class: 'carte', style: 'margin-top:auto' },
      h('div', { class: 'etiq' }, 'Coût des règles sur cette demande (mesuré au journal)'),
      h('div', { class: 'grille4' },
        chiffre(mesure ? ms(c.moyenne_us / 1000) : nd('non mesuré'), 'temps ajouté par l\'engine, par appel (moyenne)'),
        chiffre(c.validations, c.validations > 1 ? 'validations humaines demandées' : 'validation humaine demandée'),
        chiffre(c.champs_retires, c.champs_retires > 1 ? 'champs retirés avant envoi' : 'champ retiré avant envoi'),
        chiffre(c.refuses, c.refuses > 1 ? 'appels refusés' : 'appel refusé')),
      h('div', { class: 'petit second' },
        mesure ? 'Temps ajouté : mesuré par l\'engine pour ' + c.mesures + ' appel(s) sur ' + c.appels + ' (maximum ' + ms(c.max_us / 1000) + ', total ' + ms(c.total_us / 1000) + ')' +
          (c.non_mesures ? ' ; ' + c.non_mesures + ' appel(s) non mesuré(s)' : '') + '. ' : 'Temps ajouté : non mesuré (journal sans durée). ',
        c.attentes_mesurees ? 'Attente humaine mesurée : ' + duree(c.attente_secondes) + ' (' + c.attentes_mesurees + ' validation(s)' +
          (c.attentes_rejetees ? (c.attentes_rejetees === c.attentes_mesurees ? ', jusqu\'au rejet' : ', dont ' + c.attentes_rejetees + ' jusqu\'au rejet') : '') + '). ' :
          (c.validations ? 'Attente humaine : non mesurée (aucune validation reprise). ' : ''),
        c.serveurs_ms ? 'Temps chez les serveurs : ' + ms(c.serveurs_ms) + ' (hors règles).' : ''));
  }

  // Partie mise à jour par le sondage : le déroulé (frise, coût) et le volet
  // de la réponse. Le formulaire n'en fait jamais partie.
  function deroule(v) {
    return [
      h('div', { style: 'display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap' },
        h('h2', {}, h('span', { class: 'vif', title: 'suivi en continu' }), 'Déroulé en direct'),
        h('div', { style: 'display:flex;gap:8px;flex-wrap:wrap' }, ['autorise', 'caviarde', 'en_attente', 'refuse', 'rejete', 'indisponible', 'source_non_attestee'].map(k => badge(k)))),
      v.statut_session ? h('p', { class: 'moyen ' + (v.rejet ? 'erreur gras' : 'second') }, 'Demande : ' + (v.demande ? '« ' + v.demande + ' » · ' : '') +
        (v.rejet ? 'rejetée par ' + parQui(v.rejet) + ' à ' + heureSec(v.rejet.date) : STATUT[v.statut_session])) : null,
      v.etapes.length ? h('ol', { class: 'deroule' }, v.etapes.map(ligne)) : h('p', { class: 'second' }, v.session ? 'Aucun appel pour l\'instant.' : 'Aucun appel d\'outil : l\'orchestrateur a répondu sans consulter de serveur.'),
      cout(v.cout)];
  }

  function volet(v) {
    const r = v.reponse;
    const dem = v.demandeur;
    const ligneChaine = v.verification.ok ?
      h('span', { class: 'ok-txt gras' }, 'vérifiée') :
      h('span', { class: 'gras', style: 'color:var(--ref)' }, v.verification.cle_absente ? 'non vérifiable (clé absente)' : 'rompue à l\'entrée #' + (v.premier_ecart + 1));
    return [
      h('div', { class: 'etiq' }, 'Réponse au conseiller'),
      r.terminee ? [
        h('div', { class: 'callout b-' + r.badge }, markdown(r.texte)),
        r.detail_technique ? h('details', { class: 'technique' }, h('summary', {}, 'voir le détail technique'),
          h('div', { class: 'petit second', style: 'margin-top:8px;display:flex;flex-direction:column;gap:4px' },
            h('span', {}, 'Décisions de l\'engine (déjà montrées par la frise) :'),
            r.detail_technique.split('\n').filter(Boolean).map(l => h('span', {}, avecOutils(l))))) : null,
        r.directives_citees && r.directives_citees.length ? h('p', { class: 'moyen second' }, 'Directives citées : ' + r.directives_citees.join(', ')) : null,
      ] : h('div', { class: 'callout b-indisponible' }, v.statut_session === 'en_attente' ? 'La demande attend une validation humaine.' : 'La réponse n\'est pas encore rédigée.'),
      h('div', { class: 'etiq' }, 'Ce que l\'engine a rendu à l\'agent'),
      h('div', { style: 'display:flex;flex-direction:column;gap:8px;line-height:1.5' },
        h('p', { class: 'gras' }, r.titre),
        r.paragraphes.map(p => h('p', { class: 'second moyen' }, motifAffiche(p)))),
      h('div', { style: 'border-top:1px solid var(--trait);padding-top:14px;display:flex;flex-direction:column;gap:6px' },
        h('div', { class: 'kv' }, h('span', {}, 'Session'), lienSession(v.journal, v.session, nomSession(v.session || v.journal) + ' · voir la preuve')),
        h('div', { class: 'kv' }, h('span', {}, 'Règles appliquées'), v.regles ? h('span', {}, 'en vigueur depuis le ' + jourMoisHeure(v.regles.depuis)) : nd()),
        h('div', { class: 'kv' }, h('span', {}, 'Chaîne du journal'), ligneChaine),
        h('div', { class: 'kv' }, h('span', {}, 'Demandeur'), dem ? h('span', {}, dem.identite) : nd())),
      h('div', { class: 'petit second', style: 'margin-top:auto;line-height:1.5' },
        'Le texte de la réponse est écrit par l\'agent. Les badges et la décision viennent de l\'engine, pas du modèle : un refus s\'affiche tel que l\'engine l\'a rendu, même si l\'agent affirme le contraire.')];
  }

  // Saisie conservée d'un affichage à l'autre (changement de session,
  // retour sur l'écran) : rien de ce que le conseiller a tapé n'est perdu.
  const saisie = { demande: '', demandeur: '' };

  ECRANS.poste = async (ctx) => {
    const ch = sessionChoisie(ctx.q);
    const { identites } = await api('/api/identites');
    if (!ctx.vivant()) return;
    const demande = h('input', { id: 'demande', class: 'champ', type: 'text', maxlength: '2000', placeholder: 'Ex. Peut-on accorder un prêt de 150 000 euros pour le dossier DOS-FICTIF-002 ?',
      oninput: e => { saisie.demande = e.target.value; } });
    demande.value = saisie.demande;
    const demandeur = h('select', { id: 'demandeur', class: 'champ', 'aria-label': 'Demandeur', onchange: e => { saisie.demandeur = e.target.value; } }, (identites || []).map(i =>
      h('option', { value: i.identite }, (i.nom || i.identite) + (i.groupes && i.groupes.length ? ' · ' + i.groupes.join(', ') : ''))));
    if (saisie.demandeur && [...demandeur.options].some(o => o.value === saisie.demandeur)) demandeur.value = saisie.demandeur;
    saisie.demandeur = demandeur.value;
    const err = zoneErreur();
    const info = h('p', { class: 'petit second' });
    const bouton = h('button', { type: 'button', class: 'btn' }, 'Lancer les agents');
    let lancement = null, avant = new Set(toutesSessions().map(x => x.j.id));
    bouton.addEventListener('click', async () => {
      err.dire('');
      bouton.disabled = true;
      try {
        avant = new Set(toutesSessions().map(x => x.j.id));
        lancement = await post('/api/lancer', { demande: demande.value, demandeur: demandeur.value });
        info.textContent = 'Demande lancée (' + lancement.id + '). La session apparaît dès que les agents ouvrent leur journal.';
      } catch (e) { err.dire(e.message); }
      bouton.disabled = false;
    });
    // Demandes suggérées (scénarios) : un clic remplit, ne lance rien.
    const suggestions = (await api('/api/suggestions').catch(() => ({}))).suggestions || [];
    if (!ctx.vivant()) return;
    const proposer = (g) => {
      demande.value = saisie.demande = g.demande;
      if (g.demandeur && [...demandeur.options].some(o => o.value === g.demandeur)) demandeur.value = saisie.demandeur = g.demandeur;
      info.textContent = 'Demande remplie : vérifiez-la puis cliquez sur « Lancer les agents ».';
      demande.focus();
    };
    const exemples = suggestions.length ? h('div', { class: 'suggestions', style: 'display:flex;flex-direction:column;gap:6px' },
      h('div', { class: 'etiq' }, 'Demandes suggérées'),
      h('div', { style: 'display:flex;gap:8px;flex-wrap:wrap' }, suggestions.map(g =>
        h('button', { type: 'button', class: 'btn contour petit', title: g.demande, 'data-suggestion': g.fichier, onclick: () => proposer(g) }, g.titre)))) : null;
    const formulaire = h('div', { class: 'carte', id: 'formulaire-poste' },
      h('div', { style: 'display:flex;justify-content:space-between;align-items:center;gap:12px;flex-wrap:wrap' },
        h('label', { class: 'etiq', for: 'demande' }, 'Demande'),
        ch ? h('span', { class: 'moyen second' }, 'Session : ', selecteurSession('poste', ch)) : null),
      h('div', { style: 'display:flex;gap:12px;flex-wrap:wrap' }, demande, demandeur, bouton),
      exemples, err, info);
    // Gabarit monté une fois : le sondage ne remplace que le contenu de
    // « dynamique » et du volet, jamais le formulaire (focus, sélection et
    // liste déroulante ouverte sont préservés).
    const dynamique = h('div', { style: 'display:flex;flex-direction:column;gap:20px;flex:1' });
    const aside = h('aside', { class: 'volet', style: 'width:auto;padding:28px;gap:18px' });
    ctx.monter(h('div', { class: 'corps', style: 'display:grid;grid-template-columns:minmax(0,1fr) 460px' },
      h('main', { class: 'principal ample' }, formulaire, dynamique), aside));
    let courante = ch, signature = null;

    async function rafraichir() {
      // Après un lancement, bascule sur la session la plus récente apparue.
      if (lancement) {
        etat = await api('/api/etat');
        const nouvelle = toutesSessions().find(x => !avant.has(x.j.id));
        if (nouvelle) { lancement = null; allerA('poste', { j: nouvelle.j.id, s: nouvelle.s ? nouvelle.s.id : '' }); return; }
        const l = (await api('/api/lancements')).lancements.find(x => x.id === lancement.id);
        if (l && l.termine && l.code !== 0) { info.textContent = ''; err.dire('Le lanceur s\'est arrêté (code ' + l.code + ') : ' + (l.erreur || 'sans message')); lancement = null; }
      }
      if (!courante) {
        if (signature !== 'vide') { signature = 'vide'; dynamique.replaceChildren(h('p', { class: 'second' }, 'Aucune session pour l\'instant.')); aside.replaceChildren(); }
        return;
      }
      const v = await api(cheminJournal(courante, 'poste'));
      if (!ctx.vivant()) return;
      const sig = v.signature + '|' + v.verification.ok + '|' + v.verification.erreur;
      if (sig === signature) return;
      signature = sig;
      ctx.droite(v.demandeur ? ['Connecté : ', h('b', {}, (v.demandeur_fiche && v.demandeur_fiche.nom) || v.demandeur.identite)] : ['Connecté : ', nd()]);
      dynamique.replaceChildren(...deroule(v).filter(Boolean));
      aside.replaceChildren(...volet(v).flat(Infinity).filter(Boolean));
    }
    await rafraichir();
    ctx.suivre(rafraichir);
  };
})();
