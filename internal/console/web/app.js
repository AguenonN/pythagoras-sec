// Console de supervision : JavaScript simple, sans dépendance. Toutes les
// données viennent de l'API locale ; le texte est toujours posé avec
// textContent, jamais interprété comme du HTML.
'use strict';

const EFFETS = {
  autoriser: 'Autorisé', caviarder: 'Caviardé', attendre_validation: 'En attente de validation',
  refuser: 'Refusé', non_rejouable: 'Non rejouable',
};
const EFFET_BADGE = {
  autoriser: 'autorise', caviarder: 'caviarde', attendre_validation: 'en_attente',
  refuser: 'refuse', non_rejouable: 'indisponible',
};
const ECRANS = {
  journal: ecranJournal, poste: ecranPoste, regles: ecranRegles,
  'avant-apres': ecranAvantApres, file: ecranFile, redaction: ecranRedaction,
};
const INTERVALLE_MS = 2000;

let etat = null;       // dernière réponse de /api/etat
let minuteur = null;   // suivi en continu de l'écran courant
let generation = 0;    // invalide les réponses d'un écran quitté

function h(tag, attrs, ...enfants) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v === false || v == null) continue;
    if (k === 'class') e.className = v;
    else if (k.startsWith('on')) e.addEventListener(k.slice(2), v);
    else e.setAttribute(k, v === true ? '' : v);
  }
  for (const c of enfants.flat()) {
    if (c == null || c === false) continue;
    e.append(c.nodeType ? c : document.createTextNode(String(c)));
  }
  return e;
}

async function api(chemin, options) {
  const r = await fetch(chemin, options);
  const corps = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(corps.erreur || ('erreur ' + r.status));
  return corps;
}

function heure(ts) {
  const d = new Date(ts);
  return isNaN(d) ? '' : d.toLocaleTimeString('fr-FR') + ',' + String(d.getMilliseconds()).padStart(3, '0');
}
function dateHeure(ts) {
  const d = new Date(ts);
  return isNaN(d) ? '' : d.toLocaleString('fr-FR');
}
function fondementTexte(f) {
  if (!f) return '';
  return [f.texte, f.article ? 'art. ' + f.article : ''].filter(Boolean).join(', ');
}
function badgeEffet(effet) {
  return h('span', { class: 'badge ' + (EFFET_BADGE[effet] || 'indisponible') }, EFFETS[effet] || effet);
}

// ---------------------------------------------------------------- bannière

function bannière(v) {
  const b = document.getElementById('chaine-banniere');
  b.className = '';
  b.textContent = '';
  if (!v) { b.className = 'inconnue'; b.textContent = 'Aucun journal sélectionné.'; return; }
  if (v.ok) {
    b.className = 'ok';
    b.textContent = '✔ Chaîne du journal vérifiée : ' + v.lignes + ' entrées, aucune altération.';
  } else if (v.cle_absente) {
    b.className = 'rompue';
    b.textContent = '✖ Chaîne non vérifiable : la clé du journal est absente. Aucune entrée ne peut être tenue pour prouvée.';
  } else {
    b.className = 'rompue';
    b.textContent = '✖ CHAÎNE ROMPUE — ' + v.erreur + ' Les entrées à partir de ce point ne sont pas fiables (' +
      v.lignes + ' entrée(s) vérifiée(s) sur ' + v.total + ').';
  }
}

// ---------------------------------------------------------------- navigation

function parseHash() {
  const [ecran, q] = location.hash.replace(/^#/, '').split('?');
  return { ecran: ecran || 'journal', q: new URLSearchParams(q || '') };
}
function allerA(ecran, params) {
  const q = new URLSearchParams(params || {}).toString();
  location.hash = ecran + (q ? '?' + q : '');
}

async function router() {
  clearInterval(minuteur);
  const gen = ++generation;
  const { ecran, q } = parseHash();
  for (const a of document.querySelectorAll('#nav a')) a.classList.toggle('actif', a.dataset.ecran === ecran);
  const main = document.getElementById('contenu');
  try {
    etat = await api('/api/etat');
    if (gen !== generation) return;
    await (ECRANS[ecran] || ecranJournal)(main, q, gen);
  } catch (e) {
    main.textContent = '';
    main.append(h('p', { class: 'erreurs' }, 'Erreur : ' + e.message));
  }
}
window.addEventListener('hashchange', router);
window.addEventListener('load', router);

// Sélecteurs de journal / session partagés par les écrans 1 et 2.
function choixSession(q) {
  const journaux = etat.journaux.filter(j => j.sessions.length > 0);
  if (journaux.length === 0) return null;
  let j = journaux.find(x => x.id === q.get('j')) || journaux[0];
  let s = j.sessions.find(x => x.id === q.get('s')) || j.sessions[0];
  return { journaux, j, s };
}

function selecteursSession(ecran, ch) {
  const selJ = h('select', { 'aria-label': 'Journal', onchange: e => {
    const j = ch.journaux.find(x => x.id === e.target.value);
    allerA(ecran, { j: j.id, s: j.sessions[0].id });
  } }, ch.journaux.map(j => h('option', { value: j.id, selected: j.id === ch.j.id },
    j.id + (j.verification.ok ? '' : ' — chaîne non vérifiée'))));
  const selS = h('select', { 'aria-label': 'Session', onchange: e => allerA(ecran, { j: ch.j.id, s: e.target.value }) },
    ch.j.sessions.map((s, i) => h('option', { value: s.id, selected: s.id === ch.s.id },
      'Session ' + (i + 1) + ' — ' + s.appels + ' appel(s), ' + dateHeure(s.ouverte) + (s.fermee ? '' : ' (en cours)'))));
  return h('div', { class: 'barre' },
    h('label', {}, 'Journal', selJ), h('label', {}, 'Session', selS));
}

function videJournaux(main) {
  main.textContent = '';
  main.append(h('div', { class: 'carte' },
    h('h2', {}, 'Aucun journal'),
    h('p', {}, 'Aucun journal de session n\'a été trouvé dans le répertoire de données de la console.')));
  bannière(null);
}

// ---------------------------------------------------------------- écran 1

async function ecranJournal(main, q, gen) {
  const ch = choixSession(q);
  if (!ch) return videJournaux(main);
  const ecran = h('div', {});
  main.textContent = '';
  main.append(h('h2', {}, 'Journal et chaîne d\'une session'), selecteursSession('journal', ch), ecran);
  let signature = null;
  const chemin = '/api/journal/' + encodeURIComponent(ch.j.id) + '/session/' + encodeURIComponent(ch.s.id) + '/chaine';
  async function rafraichir() {
    const v = await api(chemin);
    if (gen !== generation) return;
    bannière(v.verification);
    const sig = v.signature + '|' + v.verification.ok + '|' + v.verification.erreur;
    if (sig === signature) return;
    signature = sig;
    ecran.textContent = '';
    ecran.append(...dessinerChaine(v));
  }
  await rafraichir();
  minuteur = setInterval(() => rafraichir().catch(() => {}), INTERVALLE_MS);
}

function dessinerChaine(v) {
  const out = [];
  out.push(h('div', { class: 'carte' },
    h('h3', {}, 'Qui demande, qui agit'),
    h('div', { class: 'chemin' },
      noeud('Demandeur', v.demandeur ? v.demandeur.identite : 'non identifié',
        v.demandeur && v.demandeur.groupes ? v.demandeur.groupes.join(', ') : 'le demandeur n\'est pas résolu'),
      h('span', { class: 'fleche' }, '→'),
      noeud('Agent (identité prouvée)', v.appelant || 'inconnu', 'Session ouverte le ' + dateHeure(v.ouverte)),
      h('span', { class: 'fleche' }, '→'),
      noeud('Sous-agents déclarés', v.sous_agents.length ? v.sous_agents.join(', ') : 'aucun',
        'déclaration de l\'agent, non prouvée')),
    v.jeu ? h('p', { class: 'fond' }, 'Règles appliquées : jeu « ' + v.jeu.id + ' », version ' + v.jeu.version +
      (v.jeu_connu ? '.' : ' — ce jeu n\'est pas disponible à la console : le texte des règles n\'est pas affiché.')) : null,
    h('p', { class: 'fond' }, 'Serveurs rencontrés : ', v.serveurs.map((s, i) => [i ? ' · ' : '', s.nom + (attestationCourte(s.attestation))]))));
  // Consultations qui ont fondé une décision : table inverse.
  const fonde = {};
  for (const e of v.etapes) for (const l of e.provenance || []) (fonde[l.seq] = fonde[l.seq] || []).push(e.seq);
  for (const e of v.etapes) out.push(carteEtape(e, fonde[e.seq]));
  if (v.fermee) out.push(h('p', { class: 'discret' }, 'Session fermée : ' + v.fermeture));
  return out;
}

function attestationCourte(a) {
  if (!a) return '';
  return a.valide ? ' (attesté)' : ' (NON attesté)';
}

function noeud(titre, valeur, detail, ko) {
  return h('div', { class: 'noeud' + (ko ? ' ko' : '') },
    h('div', { class: 'titre' }, titre), h('div', { class: 'valeur' }, valeur),
    detail ? h('div', { class: 'detail' }, detail) : null);
}

const MODES = { binaire: 'programme vérifié', spiffe: 'identité cryptographique', 'tls-pin': 'certificat épinglé', aucune: 'aucune vérification' };

function noeudServeur(e) {
  const a = e.attestation;
  let detail = 'pas d\'attestation consignée';
  if (a) detail = (a.valide ? 'Attesté' : 'NON attesté') + ' — ' + (MODES[a.mode] || a.mode) +
    (a.valide ? '' : (a.detail ? ' : ' + a.detail : ''));
  const src = e.source && e.source.origine ? ' · source ' + ({ direct: 'en direct', rejeu: 'enregistrée', aucune: 'sans origine' }[e.source.origine] || 'non déclarée') : '';
  return noeud('Serveur', e.serveur, detail + src, a && !a.valide);
}

function carteEtape(e, fondeDes) {
  const id = 'appel-' + e.seq;
  const carte = h('div', { class: 'carte' + (e.suspect ? ' suspect' : ''), id },
    h('h3', {}, 'Appel n° ' + e.seq + '  ', h('span', { class: 'badge ' + e.badge }, e.libelle),
      e.suspect ? h('span', { class: 'badge rouge' }, ' entrée non fiable (chaîne rompue)') : null,
      h('span', { class: 'fond' }, '   ' + heure(e.ts))),
    h('div', { class: 'chemin' },
      noeud('Sous-agent déclaré', e.sous_agent || '—', 'non prouvé'),
      h('span', { class: 'fleche' }, '→'),
      noeud('Appel', e.outil, e.champs_retires && e.champs_retires.length ? 'champs retirés : ' + e.champs_retires.join(', ') : null),
      h('span', { class: 'fleche' }, '→'),
      noeudServeur(e),
      h('span', { class: 'fleche' }, '→'),
      h('div', { class: 'noeud decision' }, h('div', { class: 'titre' }, 'Décision'),
        h('div', { class: 'valeur' }, badgeEffet(e.effet)),
        h('div', { class: 'detail' }, e.transmis ? 'appel transmis au serveur' : 'appel non transmis'))),
    h('p', {}, e.motif),
    e.applications.length ? h('ul', { class: 'directives' }, e.applications.map(a => directiveLi(a))) : h('p', { class: 'fond' }, 'Aucune directive applicable à cet appel.'),
    e.provenance && e.provenance.length ? h('div', { class: 'provenance' }, 'Décision fondée sur : ',
      e.provenance.map((l, i) => [i ? ' ; ' : '', h('button', { class: 'lien', type: 'button', onclick: () => saut(l.seq) },
        'consultation n° ' + l.seq + ' (' + l.serveur + ' / ' + l.outil + ')'), ' — directive ' + l.directive])) : null,
    fondeDes && fondeDes.length ? h('div', { class: 'retour' }, 'Cette consultation a fondé la décision de : ',
      fondeDes.map((s, i) => [i ? ', ' : '', h('button', { class: 'lien', type: 'button', onclick: () => saut(s) }, 'appel n° ' + s)])) : null,
    e.validation ? h('p', { class: 'fond' }, 'Validation requise' + (e.validation.roles ? ' — rôles : ' + e.validation.roles.join(', ') : '') +
      (e.validation.quatre_yeux ? ' — deux personnes distinctes' : '')) : null,
    (e.alertes || []).map(a => h('p', { class: 'erreurs' }, '⚠ ' + a)));
  return carte;
}

function directiveLi(a) {
  return h('li', { class: a.appliquee ? '' : 'inactive' },
    h('strong', {}, 'Directive ' + a.id + ', version ' + a.version), ' — ',
    a.appliquee ? 'appliquée (' + (EFFETS[a.effet] || a.effet).toLowerCase() + ')' :
      (a.declenchee ? 'déclenchée en observation, non appliquée' : 'examinée, non déclenchée'),
    h('div', { class: 'fond' }, 'Fondement : ' + fondementTexte(a.fondement)),
    h('div', {}, 'Motif : ' + a.motif),
    a.texte ? h('div', { class: 'texte-juriste' }, a.texte) : null);
}

function saut(seq) {
  const el = document.getElementById('appel-' + seq);
  if (!el) return;
  el.scrollIntoView({ behavior: 'smooth', block: 'center' });
  el.classList.add('cible');
  setTimeout(() => el.classList.remove('cible'), 2500);
}

// ---------------------------------------------------------------- écran 2

async function ecranPoste(main, q, gen) {
  const ch = choixSession(q);
  if (!ch) return videJournaux(main);
  const ecran = h('div', {});
  main.textContent = '';
  main.append(h('h2', {}, 'Poste conseiller'), selecteursSession('poste', ch), ecran);
  let signature = null, choisi = null;
  const chemin = '/api/journal/' + encodeURIComponent(ch.j.id) + '/session/' + encodeURIComponent(ch.s.id) + '/poste';
  async function rafraichir() {
    const v = await api(chemin);
    if (gen !== generation) return;
    bannière(v.verification);
    const sig = v.signature + '|' + v.verification.ok + '|' + v.verification.erreur;
    if (sig === signature) return;
    if (signature === null || choisi === null) choisi = v.etapes.length ? v.etapes[v.etapes.length - 1].seq : null;
    signature = sig;
    dessiner(v);
  }
  function dessiner(v) {
    ecran.textContent = '';
    const detail = h('div', { class: 'carte' });
    const majDetail = () => {
      detail.textContent = '';
      const e = v.etapes.find(x => x.seq === choisi);
      if (!e) { detail.append(h('p', { class: 'discret' }, 'Choisissez un appel dans la frise.')); return; }
      detail.append(h('h3', {}, 'Appel n° ' + e.seq + ' — ' + e.serveur + ' / ' + e.outil),
        h('p', {}, h('span', { class: 'badge ' + e.badge }, e.libelle), '  ', e.motif),
        e.applications.length ? h('ul', { class: 'directives' }, e.applications.map(a => directiveLi(a))) : null);
    };
    const frise = h('div', { class: 'frise', role: 'list' }, v.etapes.map(e => {
      const j = h('div', { class: 'jalon' + (e.seq === choisi ? ' choisi' : ''), role: 'listitem', tabindex: '0',
        onclick: () => { choisi = e.seq; dessiner(v); },
        onkeydown: ev => { if (ev.key === 'Enter' || ev.key === ' ') { choisi = e.seq; dessiner(v); } } },
        h('div', { class: 'n' }, 'Appel ' + e.seq + ' · ' + heure(e.ts)),
        h('div', { class: 'outil' }, e.outil),
        h('div', { class: 'serveur' }, e.serveur),
        h('div', {}, h('span', { class: 'badge ' + e.badge }, e.libelle)));
      return j;
    }));
    majDetail();
    const c = v.cout;
    ecran.append(
      h('div', { class: 'carte' }, h('h3', {}, h('span', { class: 'vif', title: 'suivi en continu' }), 'Frise des appels de la session'),
        v.etapes.length ? frise : h('p', { class: 'discret' }, 'Aucun appel pour l\'instant.')),
      detail,
      h('div', { class: 'carte' }, h('h3', {}, 'Réponse finale'), h('p', { class: 'reponse' }, v.reponse)),
      h('div', { class: 'carte' }, h('h3', {}, 'Coût de la règle'),
        h('div', { class: 'cout' },
          h('div', {}, h('b', {}, c.validations), 'validation(s) humaine(s) demandée(s)'),
          h('div', {}, h('b', {}, c.refuses), 'appel(s) refusé(s)'),
          h('div', {}, h('b', {}, c.caviardes), 'appel(s) avec champs retirés'),
          h('div', {}, h('b', {}, c.appels ? ms(c.evaluation_moyenne_ms) : '—'), 'temps moyen ajouté par la règle, par appel'),
          h('div', {}, h('b', {}, ms(c.serveurs_ms)), 'temps passé chez les serveurs (hors règle)')),
        h('p', { class: 'fond' }, 'Le temps ajouté par la règle est mesuré en rejouant chaque décision (maximum ' + ms(c.evaluation_max_ms) +
          (c.non_rejoues ? ' ; ' + c.non_rejoues + ' appel(s) non rejouable(s), exclus' : '') +
          '). Le temps d\'attente des validations humaines n\'est pas encore mesuré.')));
  }
  await rafraichir();
  minuteur = setInterval(() => rafraichir().catch(() => {}), INTERVALLE_MS);
}

function ms(x) {
  if (x == null) return '—';
  if (x < 1) return (x * 1000).toFixed(0) + ' µs';
  if (x < 1000) return x.toFixed(x < 10 ? 1 : 0) + ' ms';
  return (x / 1000).toFixed(1) + ' s';
}

// ---------------------------------------------------------------- écran 3

async function ecranRegles(main, q) {
  bannière(verifGlobale());
  if (!etat.jeux.length) { main.textContent = 'Aucun jeu de directives.'; return; }
  const i = Math.min(Number(q.get('jeu') || 0), etat.jeux.length - 1);
  const v = await api('/api/vitrine?jeu=' + i);
  main.textContent = '';
  const sel = h('select', { 'aria-label': 'Jeu de directives', onchange: e => allerA('regles', { jeu: e.target.value }) },
    etat.jeux.map(j => h('option', { value: j.index, selected: j.index === i }, j.libelle + ' — ' + j.directives + ' directive(s)')));
  main.append(h('h2', {}, 'Vitrine des règles'), h('div', { class: 'barre' }, h('label', {}, 'Jeu de directives', sel)),
    h('p', { class: 'fond' }, 'Source : ' + v.source));
  for (const d of v.directives) {
    const dec = d.decision;
    main.append(h('div', { class: 'carte' },
      h('h3', {}, 'Directive ' + d.id + ', version ' + d.version + '  ', h('span', { class: 'fond' }, '(' + d.statut + ')')),
      h('div', { class: 'texte-juriste' }, d.texte),
      h('p', { class: 'fond' }, 'Fondement : ' + d.fondements.map(fondementTexte).join(' ; ')),
      h('h3', {}, 'Relecture'),
      h('pre', {}, d.relecture),
      h('h3', {}, 'Une décision réelle, rejouée'),
      dec ? h('div', { class: 'rejoue' },
        h('p', {}, 'Appel n° ' + dec.seq + ' — ' + dec.serveur + ' / ' + dec.outil + ', le ' + dateHeure(dec.ts) + ' (journal « ' + dec.journal + ' »).'),
        h('p', {}, dec.declenchee ? 'La directive s\'est déclenchée. ' : 'La directive a été examinée sans se déclencher. ',
          'Décision du journal : ', badgeEffet(dec.journalise), ' — décision rejouée : ', badgeEffet(dec.rejoue), '  ',
          dec.identique ? h('span', { class: 'badge autorise' }, 'identique') : h('span', { class: 'badge rouge' }, 'ÉCART')),
        h('p', { class: 'fond' }, dec.motif),
        (dec.ecarts || []).map(x => h('p', { class: 'erreurs' }, '⚠ ' + x.detail)))
        : h('p', { class: 'discret' }, 'Aucun journal disponible ne contient de décision prise sous cette directive.')));
  }
}

function verifGlobale() {
  const js = etat.journaux;
  if (!js.length) return null;
  const pire = js.find(j => !j.verification.ok);
  if (pire) return pire.verification;
  return { ok: true, lignes: js.reduce((n, j) => n + j.verification.lignes, 0) };
}

// ---------------------------------------------------------------- écran 4

async function ecranAvantApres(main, q) {
  const js = etat.journaux;
  if (!js.length) return videJournaux(main);
  if (etat.jeux.length < 2) { main.textContent = 'Il faut deux jeux de directives pour comparer.'; return; }
  const j = js.find(x => x.id === q.get('j')) || js[0];
  const idx = (cle, defaut) => Math.min(Number(q.get(cle) ?? defaut), etat.jeux.length - 1);
  const pos = etat.jeux.findIndex(x => x.libelle === 'credit-demo, version 1');
  const a = idx('a', pos >= 0 ? pos : 0);
  const b = idx('b', etat.jeux.findIndex((x, n) => n !== a));
  const v = await api('/api/avant-apres?journal=' + encodeURIComponent(j.id) + '&avant=' + a + '&apres=' + b);
  bannière(v.verification);
  const aller = (cle, val) => { const p = { j: j.id, a, b }; p[cle] = val; allerA('avant-apres', p); };
  const selJeu = (cle, val) => h('select', { onchange: e => aller(cle, e.target.value) },
    etat.jeux.map(x => h('option', { value: x.index, selected: x.index === val }, x.libelle)));
  main.textContent = '';
  main.append(h('h2', {}, 'Avant / après : la même trace, deux jeux de règles'),
    h('div', { class: 'barre' },
      h('label', {}, 'Journal', h('select', { onchange: e => aller('j', e.target.value) }, js.map(x => h('option', { value: x.id, selected: x.id === j.id }, x.id)))),
      h('label', {}, 'Avant', selJeu('a', a)), h('label', {}, 'Après', selJeu('b', b))),
    h('p', { class: 'note' }, 'Chaque appel du journal est rejoué, à sa date, sous chacun des deux jeux. Les lignes surlignées sont celles dont l\'effet change. ' +
      v.changes + ' appel(s) sur ' + v.lignes.length + ' changent.'),
    h('div', {}, v.transitions.map(t => h('span', { class: 'transition' + (t.avant !== t.apres ? ' change' : '') },
      badgeEffet(t.avant), '→', badgeEffet(t.apres), h('strong', {}, '× ' + t.nombre)))),
    h('table', {}, h('thead', {}, h('tr', {}, ['Appel', 'Serveur / outil', v.avant, v.apres, 'Motif après'].map(t => h('th', {}, t)))),
      h('tbody', {}, v.lignes.map(l => h('tr', { class: l.change ? 'change' : '' },
        h('td', {}, l.seq), h('td', {}, l.serveur + ' / ' + l.outil),
        h('td', {}, badgeEffet(l.avant)), h('td', {}, badgeEffet(l.apres)),
        h('td', {}, l.change ? l.motif_apres : '', (l.ecarts || []).map(x => h('div', { class: 'erreurs' }, '⚠ ' + x.detail))))))));
}

// ---------------------------------------------------------------- écran 5

async function ecranFile(main) {
  const v = await api('/api/file');
  bannière(verifGlobale());
  main.textContent = '';
  main.append(h('h2', {}, 'File du superviseur'),
    h('p', { class: 'note' }, 'Cet écran est en lecture seule cette nuit : les boutons de validation seront activés quand la reprise d\'un appel retenu sera disponible.'));
  if (!v.attentes.length) { main.append(h('p', { class: 'discret' }, 'Aucun appel en attente de validation.')); return; }
  for (const a of v.attentes) {
    main.append(h('div', { class: 'carte' },
      h('h3', {}, 'Appel n° ' + a.seq + ' — ' + a.serveur + ' / ' + a.outil + '  ', h('span', { class: 'badge en_attente' }, 'En attente de validation')),
      h('p', { class: 'fond' }, 'Journal « ' + a.journal + ' », ' + dateHeure(a.ts)),
      h('p', {}, a.motif),
      h('p', {}, 'Directive ' + a.directive + ', version ' + a.version + ' — ' + fondementTexte(a.fondement)),
      h('p', {}, 'Rôle(s) requis : ', (a.roles && a.roles.length) ? a.roles.join(', ') : 'non précisé(s) par la directive',
        a.quatre_yeux ? ' — deux personnes distinctes' : ''),
      h('h3', {}, 'Requête exacte à valider'),
      h('pre', {}, a.purgee ? 'Les arguments de cet appel ont été purgés du coffre.' :
        (a.arguments ? JSON.stringify(a.arguments, null, 2) : 'Arguments non disponibles (coffre absent).')),
      h('p', { class: 'fond' }, 'Empreinte de la requête : ' + a.empreinte_transmise),
      h('div', { class: 'barre' },
        h('button', { disabled: true, title: 'Indisponible cette nuit' }, 'Valider cet appel'),
        h('button', { disabled: true, title: 'Indisponible cette nuit' }, 'Refuser cet appel'))));
  }
}

// ---------------------------------------------------------------- écran 6

async function ecranRedaction(main) {
  bannière(verifGlobale());
  main.textContent = '';
  const zone = h('textarea', { id: 'texte', placeholder: 'Collez ici un jeu de directives, ou décrivez la règle en français…', 'aria-label': 'Texte de la règle' });
  const sortie = h('div', {});
  const relire = async () => {
    sortie.textContent = '';
    const r = await api('/api/redaction/relire', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ texte: zone.value }) });
    if (r.erreurs && r.erreurs.length) sortie.append(h('div', { class: 'carte' }, h('h3', {}, 'À corriger'), h('ul', { class: 'erreurs' }, r.erreurs.map(e => h('li', {}, e)))));
    if (r.questions.length) sortie.append(h('div', { class: 'carte' }, h('h3', {}, 'Questions du compilateur'), h('ul', {}, r.questions.map(e => h('li', {}, e)))));
    if (r.valide) sortie.append(h('div', { class: 'carte' }, h('h3', {}, 'Relecture en français'), h('pre', {}, r.relecture),
      h('p', { class: 'fond' }, 'Empreinte du jeu : ' + r.empreinte)));
  };
  main.append(h('h2', {}, 'Rédaction d\'une règle'),
    h('p', { class: 'note' }, 'Cet écran est en lecture seule cette nuit : la relecture fonctionne, mais l\'activation d\'une directive sera possible quand le contrôle d\'accès des appels (gate v2) sera fusionné. Le compilateur de texte libre n\'est pas encore relié.'),
    zone, h('div', { class: 'barre' }, h('button', { type: 'button', onclick: () => relire().catch(e => sortie.append(h('p', { class: 'erreurs' }, e.message))) }, 'Relire'),
      h('button', { disabled: true, title: 'Indisponible cette nuit' }, 'Activer cette directive')), sortie);
}
