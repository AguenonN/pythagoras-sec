'use strict';
// Écran 2 · Poste conseiller : saisie de la demande, frise en direct, réponse
// au conseiller, coût des règles mesuré au journal.

(function () {
  const ORIGINES = { direct: 'direct source', rejeu: 'recorded source (replay)' };
  const STATUT = { en_cours: 'in progress', en_attente: 'awaiting human approval', termine: 'finished', bloque: 'blocked', erreur: 'failed' };

  function detail(e) {
    const ids = [...new Set(e.applications.filter(a => a.appliquee).map(a => a.id))];
    if (e.valide_par && e.valide_par.length) return 'Approved by ' + e.valide_par.map(v => nomValideur(v) + ' (' + v.role + ')').join(', ') + ', puis transmis';
    switch (e.badge) {
      case 'caviarde':
        return 'Fields removed before sending: ' + (e.champs_retires || []).join(', ') + (ids.length ? ' (' + ids.join(', ') + ')' : '');
      case 'en_attente': {
        const roles = e.validation && e.validation.roles && e.validation.roles.length ? e.validation.roles.join(', ') : 'humaine';
        return 'Validation ' + roles + ' requise' + (ids.length ? ' (' + ids.join(', ') + ')' : '') + (e.valide_au_seq ? ' — approved afterwards (call #' + e.valide_au_seq + ')' : '');
      }
      case 'refuse':
      case 'source_non_attestee':
        return motifAffiche(e.motif);
      case 'indisponible':
        return 'No data presented as obtained';
      case 'rejete':
        return texteRejet(e.rejet) + ' — ' + e.rejet.mention;
    }
    const morceaux = [];
    if (e.source && ORIGINES[e.source.origine]) morceaux.push(ORIGINES[e.source.origine]);
    if (e.duree_ms) morceaux.push(e.duree_ms + ' ms at the server');
    return morceaux.join(' · ') || 'Server response returned to the agent';
  }

  function ligne(e) {
    const cls = e.badge === 'en_attente' ? 'attente' : (e.suspect ? 'suspect' : '');
    return h('li', { class: cls },
      h('span', { class: 'heure' }, heureSec(e.ts)),
      outilLisible(e.serveur, e.outil, { class: 'libelle' }),
      badge(e.badge, e.libelle),
      h('span', { class: 'moyen ' + (e.badge === 'en_attente' ? '' : 'second'), style: e.badge === 'en_attente' ? 'color:var(--accent)' : '' }, detail(e)),
      e.suspect ? h('span', { class: 'petit gras', style: 'color:var(--ref)' }, 'unreliable entry') : null);
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
      h('div', { class: 'etiq' }, 'Cost of the rules on this request (measured in the journal)'),
      h('div', { class: 'grille4' },
        chiffre(mesure ? ms(c.moyenne_us / 1000) : nd('not measured'), 'time added by the engine, per call (average)'),
        chiffre(c.validations, c.validations > 1 ? 'human approvals requested' : 'human approval requested'),
        chiffre(c.champs_retires, c.champs_retires > 1 ? 'fields removed before sending' : 'field removed before sending'),
        chiffre(c.refuses, c.refuses > 1 ? 'calls blocked' : 'call blocked')),
      h('div', { class: 'petit second' },
        mesure ? 'Added time: measured by the engine for ' + c.mesures + ' call(s) out of ' + c.appels + ' (maximum ' + ms(c.max_us / 1000) + ', total ' + ms(c.total_us / 1000) + ')' +
          (c.non_mesures ? ' ; ' + c.non_mesures + ' call(s) not measured' : '') + '. ' : 'Added time: not measured (journal without durations). ',
        c.attentes_mesurees ? 'Measured human wait: ' + duree(c.attente_secondes) + ' (' + c.attentes_mesurees + ' validation(s)' +
          (c.attentes_rejetees ? (c.attentes_rejetees === c.attentes_mesurees ? ', until rejection' : ', dont ' + c.attentes_rejetees + ' until rejection') : '') + '). ' :
          (c.validations ? 'Human wait: not measured (no approval resumed). ' : ''),
        c.serveurs_ms ? 'Time at the servers: ' + ms(c.serveurs_ms) + ' (excluding rules).' : ''));
  }

  // Partie mise à jour par le sondage : le déroulé (frise, coût) et le volet
  // de la réponse. Le formulaire n'en fait jamais partie.
  function deroule(v) {
    return [
      h('div', { style: 'display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap' },
        h('h2', {}, h('span', { class: 'vif', title: 'live tracking' }), 'Live timeline'),
        h('div', { style: 'display:flex;gap:8px;flex-wrap:wrap' }, ['autorise', 'caviarde', 'en_attente', 'refuse', 'rejete', 'indisponible', 'source_non_attestee'].map(k => badge(k)))),
      v.statut_session ? h('p', { class: 'moyen ' + (v.rejet ? 'erreur gras' : 'second') }, 'Request: ' + (v.demande ? '« ' + v.demande + ' » · ' : '') +
        (v.rejet ? 'rejected by ' + parQui(v.rejet) + ' at ' + heureSec(v.rejet.date) : STATUT[v.statut_session])) : null,
      v.etapes.length ? h('ol', { class: 'deroule' }, v.etapes.map(ligne)) : h('p', { class: 'second' }, v.session ? 'No call yet.' : 'No tool call: the orchestrator answered without consulting any server.'),
      cout(v.cout)];
  }

  function volet(v) {
    const r = v.reponse;
    const dem = v.demandeur;
    const ligneChaine = v.verification.ok ?
      h('span', { class: 'ok-txt gras' }, 'verified') :
      h('span', { class: 'gras', style: 'color:var(--ref)' }, v.verification.cle_absente ? 'not verifiable (key missing)' : 'broken at entry #' + (v.premier_ecart + 1));
    return [
      h('div', { class: 'etiq' }, 'Answer to the advisor'),
      r.terminee ? [
        h('div', { class: 'callout b-' + r.badge }, markdown(r.texte)),
        r.detail_technique ? h('details', { class: 'technique' }, h('summary', {}, 'see technical detail'),
          h('div', { class: 'petit second', style: 'margin-top:8px;display:flex;flex-direction:column;gap:4px' },
            h('span', {}, 'Engine decisions (already shown in the timeline):'),
            r.detail_technique.split('\n').filter(Boolean).map(l => h('span', {}, avecOutils(l))))) : null,
        r.directives_citees && r.directives_citees.length ? h('p', { class: 'moyen second' }, 'Rules cited: ' + r.directives_citees.join(', ')) : null,
      ] : h('div', { class: 'callout b-indisponible' }, v.statut_session === 'en_attente' ? 'The request awaits human approval.' : 'The answer is not written yet.'),
      h('div', { class: 'etiq' }, 'What the engine returned to the agent'),
      h('div', { style: 'display:flex;flex-direction:column;gap:8px;line-height:1.5' },
        h('p', { class: 'gras' }, r.titre),
        r.paragraphes.map(p => h('p', { class: 'second moyen' }, motifAffiche(p)))),
      h('div', { style: 'border-top:1px solid var(--trait);padding-top:14px;display:flex;flex-direction:column;gap:6px' },
        h('div', { class: 'kv' }, h('span', {}, 'Session'), lienSession(v.journal, v.session, nomSession(v.session || v.journal) + ' · see the proof')),
        h('div', { class: 'kv' }, h('span', {}, 'Rules applied'), v.regles ? h('span', {}, 'in force since ' + jourMoisHeure(v.regles.depuis)) : nd()),
        h('div', { class: 'kv' }, h('span', {}, 'Journal chain'), ligneChaine),
        h('div', { class: 'kv' }, h('span', {}, 'Requester'), dem ? h('span', {}, dem.identite) : nd())),
      h('div', { class: 'petit second', style: 'margin-top:auto;line-height:1.5' },
        'The answer text is written by the agent. Badges and the decision come from the engine, not the model: a refusal is shown as the engine returned it, even if the agent claims otherwise.')];
  }

  // Saisie conservée d'un affichage à l'autre (changement de session,
  // retour sur l'écran) : rien de ce que le conseiller a tapé n'est perdu.
  const saisie = { demande: '', demandeur: '' };
  const CLE_DEMANDEUR = 'console.poste.demandeur';
  function lireDemandeur() { try { return localStorage.getItem(CLE_DEMANDEUR) || ''; } catch (e) { return ''; } }
  function ecrireDemandeur(v) { try { localStorage.setItem(CLE_DEMANDEUR, v); } catch (e) { /* stockage indisponible */ } }

  ECRANS.poste = async (ctx) => {
    const ch = sessionChoisie(ctx.q);
    const { identites } = await api('/api/identites');
    if (!ctx.vivant()) return;
    const demande = h('input', { id: 'demande', class: 'champ', type: 'text', maxlength: '2000', placeholder: 'E.g. Can we grant a €150,000 loan for file DOS-FICTIF-002?',
      oninput: e => { saisie.demande = e.target.value; } });
    demande.value = saisie.demande;
    const demandeur = h('select', { id: 'demandeur', class: 'champ', 'aria-label': 'Requester', onchange: e => { saisie.demandeur = e.target.value; } }, (identites || []).map(i =>
      h('option', { value: i.identite }, (i.nom || i.identite) + (i.groupes && i.groupes.length ? ' · ' + i.groupes.join(', ') : ''))));
    // Demandeur : la dernière identité choisie (gardée aussi dans le
    // navigateur), sinon l'identité connectée ; jamais changé par le sondage
    // ni par la session affichée.
    const connue = (id) => id && [...demandeur.options].some(o => o.value === id);
    const retenu = [saisie.demandeur, lireDemandeur(), etat.conseiller].find(connue);
    if (retenu) demandeur.value = retenu;
    saisie.demandeur = demandeur.value;
    const nomDe = (id) => { const i = (identites || []).find(x => x.identite === id); return i ? (i.nom || i.identite) : id; };
    const connecte = () => ctx.droite(demandeur.value ? ['Signed in: ', h('b', { id: 'connecte' }, nomDe(demandeur.value))] : ['Signed in: ', nd()]);
    demandeur.addEventListener('change', () => { ecrireDemandeur(demandeur.value); connecte(); });
    connecte();
    const err = zoneErreur();
    const info = h('p', { class: 'petit second' });
    const bouton = h('button', { type: 'button', class: 'btn' }, 'Run the agents');
    let lancement = null, avant = new Set(toutesSessions().map(x => x.j.id));
    bouton.addEventListener('click', async () => {
      err.dire('');
      bouton.disabled = true;
      try {
        avant = new Set(toutesSessions().map(x => x.j.id));
        lancement = await post('/api/lancer', { demande: demande.value, demandeur: demandeur.value });
        info.textContent = 'Request started (' + lancement.id + '). The session appears as soon as the agents open their journal.';
      } catch (e) { err.dire(e.message); }
      bouton.disabled = false;
    });
    // Demandes suggérées (scénarios) : un clic remplit, ne lance rien.
    const suggestions = (await api('/api/suggestions').catch(() => ({}))).suggestions || [];
    if (!ctx.vivant()) return;
    const proposer = (g) => {
      demande.value = saisie.demande = g.demande;
      if (connue(g.demandeur)) { demandeur.value = saisie.demandeur = g.demandeur; ecrireDemandeur(g.demandeur); connecte(); }
      info.textContent = 'Request filled in: check it, then click “Run the agents”.';
      demande.focus();
    };
    const exemples = suggestions.length ? h('div', { class: 'suggestions', style: 'display:flex;flex-direction:column;gap:6px' },
      h('div', { class: 'etiq' }, 'Suggested requests'),
      h('div', { style: 'display:flex;gap:8px;flex-wrap:wrap' }, suggestions.map(g =>
        h('button', { type: 'button', class: 'btn contour petit', title: g.demande, 'data-suggestion': g.fichier, onclick: () => proposer(g) }, g.titre)))) : null;
    const formulaire = h('div', { class: 'carte', id: 'formulaire-poste' },
      h('div', { style: 'display:flex;justify-content:space-between;align-items:center;gap:12px;flex-wrap:wrap' },
        h('label', { class: 'etiq', for: 'demande' }, 'Request'),
        ch ? h('span', { class: 'moyen second' }, 'Session: ', selecteurSession('poste', ch)) : null),
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
        if (l && l.termine && l.code !== 0) { info.textContent = ''; err.dire('The launcher stopped (code ' + l.code + ') : ' + (l.erreur || 'no message')); lancement = null; }
      }
      if (!courante) {
        if (signature !== 'vide') { signature = 'vide'; dynamique.replaceChildren(h('p', { class: 'second' }, 'No session yet.')); aside.replaceChildren(); }
        return;
      }
      const v = await api(cheminJournal(courante, 'poste'));
      if (!ctx.vivant()) return;
      const sig = v.signature + '|' + v.verification.ok + '|' + v.verification.erreur;
      if (sig === signature) return;
      signature = sig;
      dynamique.replaceChildren(...deroule(v).filter(Boolean));
      aside.replaceChildren(...volet(v).flat(Infinity).filter(Boolean));
    }
    await rafraichir();
    ctx.suivre(rafraichir);
  };
})();
