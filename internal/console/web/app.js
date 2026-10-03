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
  ['notifications', 'Notifications'],
];
const LIBELLE_BADGE = {
  autorise: 'Autorisé', caviarde: 'Caviardé', en_attente: 'En attente', refuse: 'Refusé',
  indisponible: 'Indisponible', source_non_attestee: 'Source non attestée', rejete: 'Rejeté',
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
const NOMBRE_ONGLET = { validation: 'attentes', redaction: 'propositions' };
// Nom du valideur d'après l'annuaire, à défaut son identité.
const nomValideur = (v) => v.nom || v.identite;
const nd = (texte) => h('span', { class: 'nd' }, texte || ND);

async function api(chemin, options) {
  const r = await fetch(chemin, options);
  const corps = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(corps.erreur || ('erreur ' + r.status));
  return corps;
}

// Écriture : corps JSON ; une erreur 4xx rend le message de la console.
function post(chemin, corps) {
  return api(chemin, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(corps || {}) });
}
// Zone de message d'erreur d'action, affichée en clair dans l'écran.
function zoneErreur() {
  const z = h('p', { class: 'erreur gras', role: 'alert', hidden: true });
  z.dire = (m) => { z.hidden = !m; z.textContent = m || ''; };
  return z;
}
function lienSession(j, s, texte) {
  return h('a', { href: '#journal?' + new URLSearchParams({ j, s }) }, texte);
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
// Le nom court d'une session : la fin de son identifiant quand celui-ci porte
// une date (AAAAMMJJ-HHMMSS-court), sinon ses quatre premiers caractères.
const nomSession = (id) => {
  const m = /^\d{8}-\d{6}-([0-9a-z]+)$/i.exec(String(id));
  return 'S-' + (m ? m[1].slice(0, 4) : String(id).slice(0, 4));
};
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
// directive s'écrit en toutes lettres.
function motifAffiche(m) {
  return String(m || '')
    .replace(/sha256:([0-9a-f]{8})[0-9a-f]{8,}/g, 'sha256:$1…')
    .replace(/\b([A-Z]+-\d+) v(\d+)\b/g, '$1, version $2');
}
// Un rejet consigné par la console (approbations/<empreinte>/rejet.json).
function texteRejet(r) {
  return 'Rejeté par ' + (r.nom || r.valideur) + (r.role ? ' (rôle « ' + r.role + ' »)' : '') +
    (r.date ? ' le ' + jourComplet(r.date) + ' à ' + heureSec(r.date) : '') + ' : ' + r.motif;
}
const parQui = (r) => (r.nom || r.valideur) + (r.role ? ' (' + r.role + ')' : '');
function humanise(nom) { return String(nom || '').replace(/_/g, ' '); }

// ------------------------------------------------------------------ outils

// Libellés des outils (GET /api/outils), chargés une fois : à l'écran,
// « Attribution de prêt — banque » ; le nom technique serveur__outil et la
// description du catalogue en info-bulle.
let OUTILS = null;
async function chargerOutils() {
  if (OUTILS) return;
  try { OUTILS = (await api('/api/outils')).outils || {}; } catch (e) { OUTILS = {}; }
}
function cleOutil(serveur, outil) {
  outil = String(outil || '');
  return outil.includes('__') || !serveur ? outil : serveur + '__' + outil;
}
function libelleOutil(serveur, outil) {
  const k = cleOutil(serveur, outil), l = OUTILS && OUTILS[k];
  if (l) return l.libelle;
  const [s, o] = k.includes('__') ? k.split('__') : ['', k];
  const t = humanise(o);
  return (t.charAt(0).toUpperCase() + t.slice(1)) + (s ? ' — ' + s : '');
}
function outilLisible(serveur, outil, attrs) {
  const k = cleOutil(serveur, outil), l = OUTILS && OUTILS[k];
  return h('span', Object.assign({ class: 'outil', title: k + (l && l.description ? '\n' + l.description : '') }, attrs || {}), libelleOutil(serveur, outil));
}
// Remplace, dans un texte libre, chaque nom serveur__outil par son libellé.
const RE_OUTIL = /\b[a-z0-9][a-z0-9-]*__[a-z0-9_]+\b/g;
function avecOutils(texte) {
  const out = [];
  let i = 0;
  for (const m of String(texte).matchAll(RE_OUTIL)) {
    if (m.index > i) out.push(texte.slice(i, m.index));
    out.push(outilLisible('', m[0]));
    i = m.index + m[0].length;
  }
  if (i < texte.length) out.push(texte.slice(i));
  return out;
}

// ------------------------------------------------------------------ Markdown

// Markdown minimal et sûr : paragraphes, listes (« - », « * », « 1. »),
// gras (**…**), titres (#…) rendus en gras. Le texte n'est jamais interprété
// comme du HTML : chaque morceau devient un nœud texte ; seules les balises
// p, ul, ol, li, strong et br sont créées, par ce code.
function enLigne(texte) {
  const out = [];
  String(texte).split(/(\*\*[^*\n]+?\*\*)/).forEach((m, i) => {
    if (i % 2 === 1) out.push(h('strong', {}, avecOutils(m.slice(2, -2))));
    else if (m) out.push(...avecOutils(m));
  });
  return out;
}
function markdown(texte) {
  const blocs = [];
  const lignes = String(texte || '').replace(/\r\n?/g, '\n').split('\n');
  let para = [], liste = null;
  const finPara = () => {
    if (!para.length) return;
    blocs.push(h('p', {}, para.map((l, i) => [i ? h('br') : null, enLigne(l)])));
    para = [];
  };
  const finListe = () => { if (liste) { blocs.push(liste); liste = null; } };
  for (const brute of lignes) {
    const l = brute.trim();
    const puce = /^[-*•]\s+(.*)$/.exec(l), num = /^\d+[.)]\s+(.*)$/.exec(l), titre = /^#{1,6}\s+(.*)$/.exec(l);
    if (!l) { finPara(); finListe(); continue; }
    if (puce || num) {
      finPara();
      const tag = puce ? 'ul' : 'ol';
      if (!liste || liste.tagName.toLowerCase() !== tag) { finListe(); liste = h(tag, { class: 'md-liste' }); }
      liste.append(h('li', {}, enLigne((puce || num)[1])));
      continue;
    }
    finListe();
    if (titre) { finPara(); blocs.push(h('p', {}, h('strong', {}, enLigne(titre[1].replace(/\*\*/g, ''))))); continue; }
    para.push(l);
  }
  finPara(); finListe();
  return h('div', { class: 'md' }, blocs);
}

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
    [etat] = await Promise.all([api('/api/etat'), chargerOutils()]);
    if (gen !== generation) return;
    c.produit.textContent = etat.produit || 'Console de gouvernance';
    for (const a of c.nav.querySelectorAll('a')) {
      const k = NOMBRE_ONGLET[a.dataset.ecran], n = k ? etat[k] : 0;
      a.textContent = NAV.find(x => x[0] === a.dataset.ecran)[1] + (n ? ' · ' + n : '');
    }
    document.title = c.produit.textContent;
    alerteChaines();
    ctx.droite('');
    await ECRANS[ecran](ctx);
  } catch (e) {
    ctx.monter(h('div', { class: 'principal' }, h('p', { class: 'erreur' }, 'Erreur : ' + e.message)));
  }
}

// ------------------------------------------------------------------ sélection de session (écrans 1, 2, 4)

// Une ligne par demande (dossier de session), de la plus récente à la plus
// ancienne ; ses sessions d'outils en dessous, éventuellement aucune (refus,
// question de clarification, demande hors périmètre).
const dateDemande = (j) => new Date(j.debut || (j.sessions[0] && j.sessions[0].ouverte) || 0);
function toutesDemandes() {
  return [...(etat.journaux || [])].sort((a, b) => dateDemande(b) - dateDemande(a) || (a.id < b.id ? 1 : -1));
}
// Un choix : { j: la demande, s: sa session d'outils ou null }.
function choixDe(j, sid) {
  return { j, s: j.sessions.find(x => x.id === sid) || j.sessions[0] || null };
}
function toutesSessions() {
  return toutesDemandes().map(j => choixDe(j));
}
function sessionChoisie(q) {
  const demandes = toutesDemandes();
  if (!demandes.length) return null;
  const j = demandes.find(x => x.id === q.get('j')) || demandes[0];
  return choixDe(j, q.get('s'));
}
const nomChoix = (x) => nomSession(x.s ? x.s.id : x.j.id);
// Adresse de l'API pour une demande et, si elle en a une, sa session d'outils.
const cheminJournal = (x, ecran) => '/api/journal/' + encodeURIComponent(x.j.id) +
  (x.s ? '/session/' + encodeURIComponent(x.s.id) : '') + '/' + ecran;
function selecteurSession(ecran, courante, extra) {
  return h('select', { class: 'champ', 'aria-label': 'Demande', onchange: e => {
    const [j, s] = e.target.value.split('|');
    allerA(ecran, Object.assign({ j, s }, extra || {}));
  } }, toutesSessions().map(x => h('option', { value: x.j.id + '|' + (x.s ? x.s.id : ''), selected: x.j.id === courante.j.id },
    nomChoix(x) + ' · ' + heureMin(dateDemande(x.j)) + ' · ' + x.j.id)));
}
function aucunJournal() {
  return h('div', { class: 'principal ample' }, h('div', { class: 'carte' },
    h('h2', {}, 'Aucune session'),
    h('p', { class: 'second' }, 'Aucun journal de session n\'a été trouvé dans le répertoire de données de la console.')));
}

window.addEventListener('hashchange', router);
