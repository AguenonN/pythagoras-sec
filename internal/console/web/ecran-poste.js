'use strict';
// Écran 2 · Poste conseiller.

(function () {
  const ORIGINES = { direct: 'source directe', rejeu: 'source enregistrée (rejeu)' };

  function detail(e) {
    const ids = [...new Set(e.applications.filter(a => a.appliquee).map(a => a.id))];
    switch (e.badge) {
      case 'caviarde':
        return 'Champs retirés avant envoi : ' + (e.champs_retires || []).join(', ') + (ids.length ? ' (' + ids.join(', ') + ')' : '');
      case 'en_attente': {
        const roles = e.validation && e.validation.roles && e.validation.roles.length ? e.validation.roles.join(', ') : 'humaine';
        return 'Validation ' + roles + ' requise' + (ids.length ? ' (' + ids.join(', ') + ')' : '');
      }
      case 'refuse':
      case 'source_non_attestee':
        return motifAffiche(e.motif);
      case 'indisponible':
        return 'Aucune donnée présentée comme obtenue';
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
      h('span', { class: 'libelle' }, humanise(e.outil) + ' ', h('span', { class: 'second petit' }, '(' + e.serveur + ')')),
      badge(e.badge),
      h('span', { class: 'moyen ' + (e.badge === 'en_attente' ? '' : 'second'), style: e.badge === 'en_attente' ? 'color:var(--accent)' : '' }, detail(e)),
      e.suspect ? h('span', { class: 'petit gras', style: 'color:var(--ref)' }, 'entrée non fiable') : null);
  }

  function chiffre(valeur, texte) {
    return h('div', { style: 'display:flex;flex-direction:column;gap:2px' },
      h('span', { class: 'chiffre' }, valeur), h('span', { class: 'moyen second' }, texte));
  }

  function dessiner(v, ch) {
    const c = v.cout;
    const dem = v.demandeur;
    const ligneChaine = v.verification.ok ?
      h('span', { class: 'ok-txt gras' }, 'vérifiée') :
      h('span', { class: 'gras', style: 'color:var(--ref)' }, v.verification.cle_absente ? 'non vérifiable (clé absente)' : 'rompue à l\'entrée #' + (v.premier_ecart + 1));
    const r = v.reponse;
    return h('div', { class: 'corps', style: 'display:grid;grid-template-columns:minmax(0,1fr) 460px' },
      h('main', { class: 'principal ample' },
        h('div', { class: 'carte' },
          h('div', { style: 'display:flex;justify-content:space-between;align-items:center;gap:12px;flex-wrap:wrap' },
            h('label', { class: 'etiq', for: 'demande' }, 'Demande'),
            h('span', { class: 'moyen second' }, 'Session : ', selecteurSession('poste', ch))),
          h('div', { style: 'display:flex;gap:12px' },
            h('input', { id: 'demande', class: 'champ', type: 'text', disabled: true, value: '', placeholder: 'Demande non disponible dans le journal' }),
            h('button', { type: 'button', class: 'btn', disabled: true, title: 'Disponible après la fusion du contrôle d\'accès' }, 'Lancer les agents'))),
        h('div', { style: 'display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap' },
          h('h2', {}, h('span', { class: 'vif', title: 'suivi en continu' }), 'Déroulé en direct'),
          h('div', { style: 'display:flex;gap:8px;flex-wrap:wrap' }, ['autorise', 'caviarde', 'en_attente', 'refuse', 'indisponible', 'source_non_attestee'].map(k => badge(k)))),
        v.etapes.length ? h('ol', { class: 'deroule' }, v.etapes.map(ligne)) : h('p', { class: 'second' }, 'Aucun appel pour l\'instant.'),
        h('div', { class: 'carte', style: 'margin-top:auto' },
          h('div', { class: 'etiq' }, 'Coût des règles sur cette demande'),
          h('div', { class: 'grille4' },
            chiffre(c.appels ? ms(c.evaluation_moyenne_ms) : '—', 'temps ajouté par l\'engine'),
            chiffre(c.validations, c.validations > 1 ? 'validations humaines demandées' : 'validation humaine demandée'),
            chiffre(c.champs_retires, c.champs_retires > 1 ? 'champs retirés avant envoi' : 'champ retiré avant envoi'),
            chiffre(c.refuses, c.refuses > 1 ? 'appels refusés' : 'appel refusé')),
          h('div', { class: 'petit second' }, 'Temps ajouté : mesuré en rejouant chaque décision (maximum ' + ms(c.evaluation_max_ms) +
            (c.non_rejoues ? ' ; ' + c.non_rejoues + ' appel(s) non rejouable(s) exclu(s)' : '') + '). Attente des validations humaines : ' + ND + '.'))),
      h('aside', { class: 'volet', style: 'width:auto;padding:28px;gap:18px' },
        h('div', { class: 'etiq' }, 'Réponse au conseiller'),
        h('div', { class: 'callout b-' + r.badge }, r.titre),
        h('div', { style: 'display:flex;flex-direction:column;gap:12px;line-height:1.55' },
          r.paragraphes.map(p => h('p', {}, motifAffiche(p))),
          h('p', { class: 'second' }, 'Texte rédigé par l\'agent : ', nd('non disponible (le journal ne conserve que son empreinte)'))),
        h('div', { style: 'border-top:1px solid var(--trait);padding-top:14px;display:flex;flex-direction:column;gap:6px' },
          h('div', { class: 'kv' }, h('span', {}, 'Session'), h('a', { href: '#journal?' + new URLSearchParams({ j: v.journal, s: v.session }) }, nomSession(v.session) + ' · voir la preuve')),
          h('div', { class: 'kv' }, h('span', {}, 'Règles appliquées'), v.regles ? h('span', {}, 'en vigueur depuis le ' + jourMoisHeure(v.regles.depuis)) : nd()),
          h('div', { class: 'kv' }, h('span', {}, 'Chaîne du journal'), ligneChaine),
          h('div', { class: 'kv' }, h('span', {}, 'Demandeur'), dem ? h('span', {}, dem.identite) : nd())),
        h('div', { class: 'petit second', style: 'margin-top:auto;line-height:1.5' },
          'La réponse est écrite par l\'agent. Les badges et la décision viennent de l\'engine, pas du modèle : un refus s\'affiche tel que l\'engine l\'a rendu, même si l\'agent affirme le contraire.')));
  }

  ECRANS.poste = async (ctx) => {
    const ch = sessionChoisie(ctx.q);
    if (!ch) { ctx.monter(aucunJournal()); return; }
    const chemin = '/api/journal/' + encodeURIComponent(ch.j.id) + '/session/' + encodeURIComponent(ch.s.id) + '/poste';
    let signature = null;
    async function rafraichir() {
      const v = await api(chemin);
      if (!ctx.vivant()) return;
      ctx.droite(v.demandeur ? ['Connecté : ', h('b', {}, v.demandeur.identite)] : ['Connecté : ', nd()]);
      const sig = v.signature + '|' + v.verification.ok + '|' + v.verification.erreur;
      if (sig === signature) return;
      signature = sig;
      ctx.monter(dessiner(v, ch));
    }
    await rafraichir();
    ctx.suivre(rafraichir);
  };
})();
