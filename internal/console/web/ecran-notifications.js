'use strict';
// Écran des notifications : ce que l'abonné a reçu (notifications.jsonl de
// chaque session), destinataire, directive, statut de remise.

(function () {
  const RYTHME = { temps_reel: 'in real time', synthese: 'in a digest' };
  const REMISE = { remise: 'Delivery', echec: 'Not delivered', planifiee: 'Scheduled' };
  const CLASSE = { remise: 'b-autorise', echec: 'b-refuse', planifiee: 'b-indisponible' };

  ECRANS.notifications = async (ctx) => {
    async function charger() {
      const { notifications } = await api('/api/notifications');
      if (!ctx.vivant()) return;
      ctx.droite([h('b', {}, notifications.length + (notifications.length > 1 ? ' notifications' : ' notification'))]);
      ctx.monter(h('div', { class: 'corps' }, h('main', { class: 'principal ample' },
        h('div', { style: 'display:flex;flex-direction:column;gap:4px' },
          h('h1', {}, 'Notifications'),
          h('div', { class: 'second' }, 'What the rules asked to notify, and what happened (delivery status recorded in the journal).')),
        h('section', { class: 'carte' },
          notifications.length ? [
            h('div', { class: 'grille-notif entete-colonnes', style: 'border-top:0' }, ['Paris time', 'Recipient (role)', 'Rule', 'Delivery', 'Detail'].map(t => h('span', {}, t))),
            notifications.map(n => h('div', { class: 'grille-notif' },
              h('span', {}, jourSemaine(n.ts)),
              h('span', { class: 'gras' }, n.destinataire),
              h('span', {}, h('b', {}, n.directive), ', version ' + n.version, n.titre ? h('div', { class: 'petit second' }, n.titre) : null),
              h('span', {}, h('span', { class: 'badge ' + (CLASSE[n.statut] || 'b-indisponible') }, REMISE[n.statut] || n.statut || ND)),
              h('span', { class: 'moyen second' },
                (RYTHME[n.rythme] || n.rythme) + (n.si_echec === 'refuser' ? ' · if delivery fails, the call is refused' : '') +
                (n.remis ? ' · ' + n.remis + ' destinataire(s) atteint(s)' : '') + (n.outil ? ' · call ' + libelleOutil(n.serveur, n.outil) : '') + ' · ',
                lienSession(n.journal, n.session, nomSession(n.session)))))
          ] : h('p', { class: 'second' }, 'No notification yet.')))));
    }
    await charger();
    ctx.suivre(charger);
  };
})();
