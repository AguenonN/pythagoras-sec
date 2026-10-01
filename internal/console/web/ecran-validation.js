'use strict';
// Écran 5 · File du superviseur (lecture seule : les actions viendront avec
// le contrôle d'accès des appels).

(function () {
  const cle = (a) => a.journal + '|' + a.session + '|' + a.seq;
  const titreAppel = (a) => { const t = humanise(a.outil); return t.charAt(0).toUpperCase() + t.slice(1); };

  function valeur(v) { return typeof v === 'object' && v !== null ? JSON.stringify(v) : String(v); }

  function carteFile(a, choisie) {
    const ids = a.applications.map(x => x.id).join(', ');
    const roles = a.roles && a.roles.length ? a.roles.join(', ') : null;
    return h('button', { type: 'button', class: 'attente-carte' + (choisie ? ' choisie' : ''), onclick: () => allerA('validation', { a: cle(a) }) },
      h('span', { style: 'display:flex;justify-content:space-between;gap:8px' }, h('b', {}, titreAppel(a)), h('span', { class: 'petit second' }, ilYa(a.ts))),
      h('span', { class: 'moyen second' }, a.serveur + (ids ? ' · ' + ids : '')),
      h('span', { class: 'petit gras', style: 'color:' + (choisie ? 'var(--accent)' : 'var(--second)') },
        'Rôle requis : ' + (roles || ND) + (a.quatre_yeux ? ' · quatre yeux' : '')));
  }

  function controle(titre, texte) {
    return h('div', { style: 'display:flex;flex-direction:column;gap:4px' }, h('b', { class: 'second' }, titre), h('span', { class: 'second' }, texte));
  }

  function detail(a) {
    const lignes = [a.serveur + ' / ' + a.outil];
    if (a.purgee) lignes.push('Les arguments de cet appel ont été purgés du coffre.');
    else if (a.arguments) for (const k of Object.keys(a.arguments).sort()) lignes.push(k + ' : ' + valeur(a.arguments[k]));
    else lignes.push('Arguments ' + ND + ' (coffre absent).');
    const dem = a.demandeur ? a.demandeur.identite : ND;
    return h('main', { class: 'principal ample' },
      h('div', { style: 'display:flex;flex-direction:column;gap:4px' },
        h('h1', {}, titreAppel(a) + ' — ' + a.serveur),
        h('div', { class: 'second' }, 'Demandée par ' + dem + ' · session ' + nomSession(a.session) + (a.sous_agent ? ' · sous-agent déclaré « ' + a.sous_agent + ' »' : '') + ' · ' + jourComplet(a.ts) + ' ' + heureSec(a.ts))),
      h('div', { class: 'grille2' },
        h('section', { class: 'carte', style: 'gap:10px' },
          h('div', { class: 'etiq' }, 'Requête exacte qui sera transmise'),
          h('div', { class: 'requete' }, lignes.join('\n')),
          h('div', { class: 'kv' }, h('span', {}, 'Empreinte transmise'), h('span', { class: 'mono' }, court(a.empreinte_transmise))),
          h('div', { class: 'petit second' }, 'Votre signature vaudra pour cette empreinte seulement. Toute modification de la requête exigera une nouvelle validation.')),
        h('section', { class: 'carte', style: 'gap:10px' },
          h('div', { class: 'etiq' }, 'Pourquoi cette validation'),
          a.applications.length ? a.applications.map(x => h('div', { style: 'display:flex;flex-direction:column;gap:4px' },
            h('b', {}, versionTexte(x.id, x.version) + (x.texte ? ' · ' + TitreDirective(x.texte) : '')),
            h('span', { class: 'moyen second' }, fondementTexte(x.fondement) + ' · ' + motifAffiche(x.motif)))) :
            h('p', { class: 'moyen' }, motifAffiche(a.motif)),
          h('div', { class: 'moyen', style: 'border-top:1px solid var(--trait-doux);padding-top:10px;line-height:1.5' }, 'Éléments consultés : ' +
            (a.consultations.length ? a.consultations.map(k => k.serveur + ' / ' + k.outil + ' (appel n° ' + k.seq + ')').join(', ') : 'aucun')))),
      h('section', { class: 'carte' },
        h('div', { class: 'etiq' }, 'Contrôles faits par l\'engine avant transmission'),
        h('div', { class: 'grille4 moyen' },
          controle('Signature', ND + ' : approbation non encore journalisée'),
          controle('Rôle à la date', ND),
          controle('Quatre yeux', a.quatre_yeux ? 'deux personnes distinctes requises' : ND),
          controle('Un jeton par rôle', ND))),
      h('div', { style: 'display:flex;flex-direction:column;gap:8px' },
        h('label', { class: 'moyen second', for: 'commentaire' }, 'Commentaire consigné au journal (facultatif)'),
        h('textarea', { id: 'commentaire', class: 'champ', rows: '3', disabled: true, style: 'background:#fff;resize:none', placeholder: 'Disponible après la fusion du contrôle d\'accès' })),
      h('div', { style: 'margin-top:auto;display:flex;gap:12px;justify-content:flex-end;align-items:center;flex-wrap:wrap' },
        h('span', { class: 'petit second', style: 'margin-right:auto' }, 'L\'approbation sera écrite au journal chaîné avant la transmission. Les actions sont indisponibles tant que le contrôle d\'accès des appels n\'est pas fusionné.'),
        h('button', { type: 'button', class: 'btn danger', disabled: true, title: 'Indisponible : contrôle d\'accès non fusionné' }, 'Rejeter'),
        h('button', { type: 'button', class: 'btn', disabled: true, title: 'Indisponible : contrôle d\'accès non fusionné' }, 'Signer et valider')));
  }

  ECRANS.validation = async (ctx) => {
    const v = await api('/api/file');
    if (!ctx.vivant()) return;
    const att = v.attentes.slice().sort((x, y) => new Date(y.ts) - new Date(x.ts));
    const a = att.find(x => cle(x) === ctx.q.get('a')) || att[0];
    ctx.monter(h('div', { class: 'corps' },
      h('aside', { class: 'laterale tres-large' },
        h('div', { class: 'etiq', style: 'padding:0 4px 6px' }, 'En attente · ' + att.length),
        att.length ? att.map(x => carteFile(x, a && cle(x) === cle(a))) : h('p', { class: 'second' }, 'Aucun appel en attente de validation.')),
      a ? detail(a) : h('main', { class: 'principal ample' }, h('div', { class: 'carte' }, h('p', { class: 'second' }, 'Rien à valider pour l\'instant.')))));
  };
})();
