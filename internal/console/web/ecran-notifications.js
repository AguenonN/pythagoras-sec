'use strict';
// Écran des notifications : ce que l'abonné a reçu (notifications.jsonl de
// chaque session), destinataire, directive, statut de remise.

(function () {
  const RYTHME = { temps_reel: 'en temps réel', synthese: 'en synthèse' };
  const REMISE = { remise: 'Remise', echec: 'Non remise', planifiee: 'Planifiée' };
  const CLASSE = { remise: 'b-autorise', echec: 'b-refuse', planifiee: 'b-indisponible' };

  ECRANS.notifications = async (ctx) => {
    async function charger() {
      const { notifications } = await api('/api/notifications');
      if (!ctx.vivant()) return;
      ctx.droite([h('b', {}, notifications.length + (notifications.length > 1 ? ' notifications' : ' notification'))]);
      ctx.monter(h('div', { class: 'corps' }, h('main', { class: 'principal ample' },
        h('div', { style: 'display:flex;flex-direction:column;gap:4px' },
          h('h1', {}, 'Notifications'),
          h('div', { class: 'second' }, 'Ce que les directives ont demandé de notifier, et ce qu\'il en est advenu (statut de remise consigné au journal).')),
        h('section', { class: 'carte' },
          notifications.length ? [
            h('div', { class: 'grille-notif entete-colonnes', style: 'border-top:0' }, ['Heure de Paris', 'Destinataire (rôle)', 'Directive', 'Remise', 'Détail'].map(t => h('span', {}, t))),
            notifications.map(n => h('div', { class: 'grille-notif' },
              h('span', {}, jourSemaine(n.ts)),
              h('span', { class: 'gras' }, n.destinataire),
              h('span', {}, h('b', {}, n.directive), ', version ' + n.version, n.titre ? h('div', { class: 'petit second' }, n.titre) : null),
              h('span', {}, h('span', { class: 'badge ' + (CLASSE[n.statut] || 'b-indisponible') }, REMISE[n.statut] || n.statut || ND)),
              h('span', { class: 'moyen second' },
                (RYTHME[n.rythme] || n.rythme) + (n.si_echec === 'refuser' ? ' · si la remise échoue, l\'appel est refusé' : '') +
                (n.remis ? ' · ' + n.remis + ' destinataire(s) atteint(s)' : '') + (n.outil ? ' · appel ' + libelleOutil(n.serveur, n.outil) : '') + ' · ',
                lienSession(n.journal, n.session, nomSession(n.session)))))
          ] : h('p', { class: 'second' }, 'Aucune notification pour l\'instant.')))));
    }
    await charger();
    ctx.suivre(charger);
  };
})();
