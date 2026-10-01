// Console de gouvernance : JavaScript simple, sans dépendance. Les données
// viennent de l'API locale ; le texte est toujours posé avec textContent,
// jamais interprété comme du HTML. Ce fichier porte le socle (gabarit,
// navigation, formats) ; chaque écran est dans son propre fichier et
// s'enregistre dans ECRANS.
'use strict';

const ECRANS = {};
const NAV = [
  ['journal', 'Journal'], ['poste', 'Poste conseiller'], ['regles', 'Règles'],
  ['avant-apres', 'Avant / après'], ['validation', 'Validation'], ['redaction', 'Rédaction'],
];
const LIBELLE_BADGE = {
  autorise: 'Autorisé', caviarde: 'Caviardé', en_attente: 'En attente', refuse: 'Refusé',
  indisponible: 'Indisponible', source_non_attestee: 'Source non attestée',
};
const EFFET_BADGE = {
  autoriser: 'autorise', caviarder: 'caviarde', attendre_validation: 'en_attente', refuser: 'refuse',
  non_rejouable: 'indisponible',
};
const INTERVALLE_MS = 2000;
const ND = 'non disponible';

let etat = null;          // dernière réponse de /api/etat
let minuteur = null;      // suivi en continu de l'écran courant
let generation = 0;       // invalide les réponses d'un écran quitté

// ------------------------------------------------------------------ DOM

function h(tag, attrs, ...enfants) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v === false || v == null) continue;
    if (k === 'class') e.className = v;
    else if (k === 'style') e.style.cssText = v; // CSSOM : permis par la CSP, contrairement à l'attribut
    else if (k.startsWith('on')) e.addEventListener(k.slice(2), v);
    else e.setAttribute(k, v === true ? '' : v);
  }
  for (const c of enfants.flat(Infinity)) {
    if (c == null || c === false) continue;
    e.append(c.nodeType ? c : document.createTextNode(String(c)));
  }
  return e;
}
const nd = (texte) => h('span', { class: 'nd' }, texte || ND);

async function api(chemin, options) {
  const r = await fetch(chemin, options);
  const corps = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(corps.erreur || ('erreur ' + r.status));
  return corps;
}

// ------------------------------------------------------------------ formats

const PARIS = 'Europe/Paris';
function fmt(ts, opts) {
  const d = new Date(ts);
  return isNaN(d) ? '' : new Intl.DateTimeFormat('fr-FR', Object.assign({ timeZone: PARIS }, opts)).format(d);
}
const heureSec = (ts) => fmt(ts, { hour: '2-digit', minute: '2-digit', second: '2-digit' });
const heureMin = (ts) => fmt(ts, { hour: '2-digit', minute: '2-digit' });
const jourMois = (ts) => fmt(ts, { day: '2-digit', month: '2-digit' });
const jourMoisHeure = (ts) => jourMois(ts) + ' ' + heureMin(ts);
const jourComplet = (ts) => fmt(ts, { day: '2-digit', month: '2-digit', year: 'numeric' });
function jourSemaine(ts) {
  return fmt(ts, { weekday: 'short' }) + ' ' + jourMois(ts) + ' · ' + heureMin(ts);
}
function utcCourt(ts) {
  const d = new Date(ts);
  return isNaN(d) ? '' : d.toISOString().slice(5, 16) + 'Z';
}
function court(hex) {
  if (!hex) return '';
  const s = String(hex).replace(/^sha256:/, '');
  return s.length > 10 ? s.slice(0, 4) + '…' + s.slice(-4) : s;
}
const nomSession = (id) => 'S-' + String(id).slice(0, 4);
function ilYa(ts) {
  const s = Math.max(0, (Date.now() - new Date(ts).getTime()) / 1000);
  if (s < 90) return 'il y a un instant';
  if (s < 5400) return 'il y a ' + Math.round(s / 60) + ' min';
  if (s < 129600) return 'il y a ' + Math.round(s / 3600) + ' h';
  return 'il y a ' + Math.round(s / 86400) + ' j';
}
function ms(x) {
  if (x == null) return '—';
  if (x < 1) return (x * 1000).toFixed(0) + ' µs';
  if (x < 1000) return x.toFixed(x < 10 ? 1 : 0).replace('.', ',') + ' ms';
  return (x / 1000).toFixed(1).replace('.', ',') + ' s';
}
function fondementTexte(f) {
  if (!f) return '';
  return [f.texte, f.article ? 'art. ' + f.article : ''].filter(Boolean).join(', ');
}
const versionTexte = (id, version) => id + ', version ' + version;
// Le motif est celui que l'engine a journalisé ; à l'écran, la version d'une
// directive s'écrit en toutes lettres et le nom technique du verrou est omis.
function motifAffiche(m) {
  return String(m || '')
    .replace(/\b([A-Z]+-\d+) v(\d+)\b/g, '$1, version $2')
    .replace(/\((interdiction|seuil_argument|provenance_session|approbation_humaine|champs_autorises_par_destination|source_attestee), /g, '(');
}
function humanise(nom) { return String(nom || '').replace(/_/g, ' '); }

function badge(cle, texte, extra) {
  return h('span', { class: 'badge b-' + cle + (extra ? ' ' + extra : '') }, texte || LIBELLE_BADGE[cle] || cle);
}
const badgeEffet = (effet) => badge(EFFET_BADGE[effet] || 'indisponible',
  effet === 'non_rejouable' ? 'Non rejouable' : LIBELLE_BADGE[EFFET_BADGE[effet]]);

// ------------------------------------------------------------------ chaîne

function etatChaine(v) {
  // v : { ok, lignes, total, erreur, cle_absente }
  if (!v) return null;
  if (v.ok) return { ok: true, court: 'Chaîne vérifiée · ' + v.lignes + ' / ' + v.total + ' entrées' };
  if (v.cle_absente) return { ok: false, court: 'Chaîne non vérifiable · clé du journal absente' };
  return { ok: false, court: 'Chaîne rompue · ' + v.lignes + ' / ' + v.total + ' entrées vérifiées', detail: v.erreur };
}
function pastilleChaine(v) {
  const c = etatChaine(v);
  if (!c) return h('div', { class: 'chaine rompue' }, 'Chaîne : ' + ND);
  return h('div', { class: 'chaine' + (c.ok ? '' : ' rompue'), title: c.detail || '' }, c.court);
}

// ------------------------------------------------------------------ gabarit

let coque = null;
function monterCoque() {
  if (coque) return coque;
  const nav = h('nav', { class: 'nav', 'aria-label': 'Écrans' },
    NAV.map(([id, nom]) => h('a', { href: '#' + id, 'data-ecran': id }, nom)));
  coque = {
    produit: h('div', { class: 'produit' }, 'Console de gouvernance'),
    nav,
    droite: h('div', { class: 'droite' }),
    alerte: h('div', { class: 'alerte-chaine', role: 'alert', hidden: true }),
    app: h('div', { id: 'app' }),
  };
  document.getElementById('coque').append(
    h('header', { class: 'entete' }, coque.produit, nav, coque.droite), coque.alerte, coque.app);
  return coque;
}

function alerteChaines() {
  const c = monterCoque();
  const mal = (etat ? etat.journaux : []).filter(j => !j.verification.ok);
  c.alerte.hidden = mal.length === 0;
  c.alerte.textContent = '';
  if (!mal.length) return;
  c.alerte.append(mal.map((j, i) => (i ? ' · ' : '') + 'Journal « ' + j.id + ' » : ' +
    (j.verification.cle_absente ? 'chaîne non vérifiable, clé absente.' :
      'chaîne rompue à l\'entrée n° ' + (j.premier_ecart + 1) + '. Les entrées suivantes ne sont pas fiables.')).join(''));
}

function parseHash() {
  const [ecran, q] = location.hash.replace(/^#/, '').split('?');
  return { ecran: ECRANS[ecran] ? ecran : 'journal', q: new URLSearchParams(q || '') };
}
function allerA(ecran, params) {
  const q = new URLSearchParams(params || {}).toString();
  location.hash = ecran + (q ? '?' + q : '');
}

async function router() {
  clearInterval(minuteur);
  const gen = ++generation;
  const { ecran, q } = parseHash();
  const c = monterCoque();
  for (const a of c.nav.querySelectorAll('a')) a.classList.toggle('actif', a.dataset.ecran === ecran);
  const ctx = {
    q, gen, ecran,
    vivant: () => gen === generation,
    droite(contenu) { c.droite.textContent = ''; c.droite.append(...[contenu || ''].flat(Infinity)); },
    monter(noeud) { c.app.textContent = ''; c.app.append(noeud); },
    suivre(f) { minuteur = setInterval(() => f().catch(() => {}), INTERVALLE_MS); },
  };
  try {
    etat = await api('/api/etat');
    if (gen !== generation) return;
    c.produit.textContent = etat.produit || 'Console de gouvernance';
    document.title = c.produit.textContent;
    alerteChaines();
    ctx.droite('');
    await ECRANS[ecran](ctx);
  } catch (e) {
    ctx.monter(h('div', { class: 'principal' }, h('p', { class: 'erreur' }, 'Erreur : ' + e.message)));
  }
}

// ------------------------------------------------------------------ sélection de session (écrans 1, 2, 4)

// Toutes les sessions de tous les journaux, de la plus récente à la plus ancienne.
function toutesSessions() {
  const out = [];
  for (const j of etat.journaux) for (const s of j.sessions) out.push({ j, s });
  out.sort((a, b) => new Date(b.s.ouverte) - new Date(a.s.ouverte));
  return out;
}
function sessionChoisie(q) {
  const toutes = toutesSessions();
  if (!toutes.length) return null;
  return toutes.find(x => x.j.id === q.get('j') && x.s.id === q.get('s')) || toutes[0];
}
function selecteurSession(ecran, courante, extra) {
  return h('select', { class: 'champ', 'aria-label': 'Session', onchange: e => {
    const [j, s] = e.target.value.split('|');
    allerA(ecran, Object.assign({ j, s }, extra || {}));
  } }, toutesSessions().map(x => h('option', { value: x.j.id + '|' + x.s.id, selected: x.j.id === courante.j.id && x.s.id === courante.s.id },
    nomSession(x.s.id) + ' · ' + heureMin(x.s.ouverte) + ' · ' + x.j.id)));
}
function aucunJournal() {
  return h('div', { class: 'principal ample' }, h('div', { class: 'carte' },
    h('h2', {}, 'Aucune session'),
    h('p', { class: 'second' }, 'Aucun journal de session n\'a été trouvé dans le répertoire de données de la console.')));
}

window.addEventListener('hashchange', router);
