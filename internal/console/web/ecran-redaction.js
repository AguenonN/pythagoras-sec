'use strict';
// Écran 6 · Juriste, rédaction. Lecture seule cette nuit : la relecture
// d'un texte de directive (format JSON) fonctionne ; le compilateur de texte
// libre, l'annuaire et l'activation ne sont pas reliés.

(function () {
  const STATUT = { active: 'active', observation: 'observation', brouillon: 'brouillon', retiree: 'retirée' };

  ECRANS.redaction = async (ctx) => {
    let onglet = 'ecrire';
    let resultat = null;
    const zone = h('textarea', { id: 'texte', class: 'zone-serif', rows: '7', 'aria-label': 'Texte de la directive',
      placeholder: '' });
    const fondement = h('input', { id: 'fondement', class: 'champ', type: 'text', style: 'background:#fff', placeholder: 'Fondement' });
    const questions = h('section', {});
    const sortie = h('section', {});

    async function compiler() {
      try {
        resultat = await api('/api/redaction/relire', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ texte: zone.value }) });
      } catch (e) { resultat = { valide: false, erreurs: [e.message], questions: [] }; }
      dessiner();
    }

    function colQuestions() {
      const out = [h('div', { class: 'etiq' }, '2 · Questions du compilateur'),
        h('div', { class: 'moyen second' }, 'Le compilateur ne devine rien : chaque valeur absente du texte devient une question.')];
      if (!resultat) out.push(h('div', { class: 'carte compacte', style: 'border-style:dashed' }, h('b', {}, 'Aucune question pour l\'instant'),
        h('span', { class: 'moyen second' }, 'Le compilateur n\'est pas relié à cette console : ' + ND + '.')));
      else {
        (resultat.erreurs || []).forEach(e => out.push(h('div', { class: 'carte compacte' }, h('b', { class: 'erreur' }, 'À corriger'), h('span', { class: 'moyen' }, e))));
        (resultat.questions || []).forEach(q => out.push(h('div', { class: 'carte compacte', style: 'border-style:dashed' }, h('b', {}, q))));
      }
      return out;
    }

    function colApplique() {
      const out = [h('div', { class: 'etiq' }, '3 · Ce qui sera appliqué')];
      if (!resultat || !resultat.valide) out.push(h('p', { class: 'second' }, nd(resultat ? 'rien à relire : le texte n\'a pas été accepté' : 'non disponible tant que la directive n\'est pas compilée')));
      else {
        for (const d of resultat.directives) out.push(h('div', { style: 'display:flex;flex-direction:column;gap:8px' },
          h('div', { style: 'display:flex;gap:8px;align-items:center;flex-wrap:wrap' }, h('b', {}, versionTexte(d.id, d.version)),
            h('span', { class: 'badge b-indisponible' }, STATUT[d.statut] || d.statut), h('span', { class: 'petit ok-txt gras' }, 'accepté par le chargeur strict')),
          h('p', { style: 'line-height:1.6' }, d.relecture)));
        out.push(h('div', { class: 'petit second', style: 'border-top:1px solid var(--trait-doux);padding-top:10px' }, 'Relecture produite par gabarit, sans IA. Le texte d\'origine est conservé tel quel dans la directive.'));
      }
      out.push(h('div', { style: 'background:var(--fond);border-radius:8px;padding:12px 14px;font-size:13px;line-height:1.5' }, 'Essai sur le journal d\'hier : ', nd()));
      out.push(h('div', { style: 'margin-top:auto;display:flex;flex-direction:column;gap:10px' },
        h('button', { type: 'button', class: 'btn contour', disabled: true, title: 'Indisponible : contrôle d\'accès non fusionné' }, 'Mettre en observation'),
        h('button', { type: 'button', class: 'btn', disabled: true, title: 'Indisponible : contrôle d\'accès non fusionné' }, 'Activer'),
        h('span', { class: 'petit second', style: 'text-align:center' }, 'Observation : décision journalisée, jamais appliquée. Activation et observation sont indisponibles cette nuit.')));
      return out;
    }

    function dessiner() {
      const gauche = h('section', {},
        h('div', { class: 'onglets' },
          h('button', { type: 'button', class: onglet === 'ecrire' ? 'actif' : '', onclick: () => { onglet = 'ecrire'; dessiner(); } }, 'Écrire une directive'),
          h('button', { type: 'button', class: onglet === 'propositions' ? 'actif' : '', onclick: () => { onglet = 'propositions'; dessiner(); } }, 'Propositions de l\'Analyste · 0')),
        onglet === 'ecrire' ? [
          h('div', { class: 'etiq' }, '1 · Votre directive'),
          h('label', { class: 'moyen second', for: 'texte' }, 'Écrivez la règle en français, avec son fondement.'),
          zone,
          h('label', { class: 'moyen second', for: 'fondement' }, 'Fondement'), fondement,
          h('button', { type: 'button', class: 'btn', onclick: compiler }, 'Compiler'),
          h('span', { class: 'petit second' }, 'Cette nuit, « Compiler » relit un texte de directive au format JSON ; le compilateur de texte libre n\'est pas relié.')] :
          h('div', { class: 'carte compacte' }, h('b', {}, 'Aucune proposition'), h('span', { class: 'moyen second' }, 'Les propositions de l\'Analyste : ' + ND + '.')),
        h('div', { class: 'refus-exemple', style: 'margin-top:auto' }, h('b', { style: 'color:var(--ref)' }, 'Exemple de refus motivé'),
          h('span', { class: 'moyen', style: 'line-height:1.5' }, '« Les agents ne doivent pas discriminer » : critère non observable dans un appel d\'outil. Le compilateur ne produit rien et explique ce qu\'il faudrait nommer (un champ, un groupe, une source).')));
      const milieu = h('section', { style: 'padding:24px 28px' }, colQuestions());
      const droite = h('section', {}, colApplique());
      ctx.monter(h('div', { class: 'col-redaction' }, gauche, milieu, droite));
    }

    ctx.droite('');
    dessiner();
  };
})();
