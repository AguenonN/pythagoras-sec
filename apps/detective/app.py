"""Detective : lecture seule du journal de l'engine versé dans PostgreSQL.

Pour chaque décision d'agent : ce qu'elle a lu, d'où, de quand, et quelles
directives ont joué. Pour chaque changement de donnée ou de règle
(table impacts, écrite par journal-sql impacts) : les décisions passées
qu'il change.

Le service ne connaît pas le mot de passe de la base : la chaîne de
connexion arrive dans DATABASE_URL, injectée à l'exécution par
l'intégration de service d'Aiven Runtime. Il n'écrit rien : uniquement des
SELECT, dans des transactions en lecture seule.

    DATABASE_URL=postgres://… PORT=8080 python app.py
"""

import html
import os
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse

import psycopg

DSN = os.environ.get("DATABASE_URL") or os.environ.get("AIVEN_PG_URI", "")
PORT = int(os.environ.get("PORT", "8080"))

# Effets du moteur, du plus permissif au plus contraignant.
EFFETS = {
    "autoriser": ("autorisé", "ok"),
    "caviarder": ("caviardé", "warn"),
    "attendre_validation": ("en attente de validation", "warn"),
    "refuser": ("refusé", "bad"),
    "non_rejouable": ("non rejouable", "bad"),
}


def e(v):
    return html.escape("" if v is None else str(v))


def effet(v):
    libelle, classe = EFFETS.get(v or "", (v or "—", ""))
    return '<span class="eff %s">%s</span>' % (classe, e(libelle))


def date(v):
    return e(v.strftime("%d/%m/%Y %H:%M:%S UTC")) if v else "—"


def requete(sql, params=()):
    with psycopg.connect(DSN, connect_timeout=10) as conn:
        conn.read_only = True
        with conn.cursor() as cur:
            cur.execute(sql, params)
            return cur.fetchall()


STYLE = """
:root{--fond:#f7f7f5;--carte:#fff;--texte:#1d1d1b;--doux:#6b6b66;--ligne:#e4e4df;
--ok:#1f7a4d;--okf:#e6f4ec;--warn:#8a5a00;--warnf:#fdf1d8;--bad:#a1261d;--badf:#fbe7e5;--lien:#2a4fb8}
@media (prefers-color-scheme:dark){:root{--fond:#151514;--carte:#1f1f1d;--texte:#ecece7;--doux:#a3a39c;
--ligne:#34342f;--ok:#7fd3a5;--okf:#173326;--warn:#f2c46b;--warnf:#3a2d10;--bad:#f39a90;--badf:#3d1c18;--lien:#8eaaf5}}
*{box-sizing:border-box}body{margin:0;background:var(--fond);color:var(--texte);
font:15px/1.5 system-ui,-apple-system,"Segoe UI",sans-serif}
main{max-width:1100px;margin:0 auto;padding:24px 16px 64px}
h1{font-size:22px;margin:0 0 4px}h2{font-size:17px;margin:32px 0 10px}
.sous{color:var(--doux);margin:0 0 20px}a{color:var(--lien)}
.carte{background:var(--carte);border:1px solid var(--ligne);border-radius:10px;padding:14px 16px;margin:10px 0;overflow-x:auto}
table{border-collapse:collapse;width:100%;font-size:14px}th,td{text-align:left;padding:7px 8px;border-bottom:1px solid var(--ligne);vertical-align:top}
th{color:var(--doux);font-weight:600;font-size:12px;text-transform:uppercase;letter-spacing:.03em}
.eff{display:inline-block;padding:1px 8px;border-radius:99px;font-size:13px;white-space:nowrap}
.eff.ok{background:var(--okf);color:var(--ok)}.eff.warn{background:var(--warnf);color:var(--warn)}.eff.bad{background:var(--badf);color:var(--bad)}
.chiffres{display:flex;gap:12px;flex-wrap:wrap}.chiffre{flex:1 1 160px}.chiffre b{display:block;font-size:26px}
.chiffre span{color:var(--doux);font-size:13px}code{font-size:13px}.doux{color:var(--doux)}
.fleche{color:var(--doux);padding:0 4px}
"""


def page(titre, corps):
    return ("<!doctype html><html lang=fr><head><meta charset=utf-8>"
            "<meta name=viewport content='width=device-width,initial-scale=1'>"
            "<title>%s</title><style>%s</style></head><body><main>%s"
            "<p class=doux style='margin-top:40px'>Données fictives (banque synthétique). "
            "La preuve est le journal chaîné par HMAC ; cette base en est une copie "
            "interrogeable, versée après vérification de la chaîne.</p></main></body></html>"
            % (e(titre), STYLE, corps))


def accueil():
    impacts = requete("""
        SELECT evenement, count(*), count(*) FILTER (WHERE change), max(calcule_le)
        FROM impacts GROUP BY evenement ORDER BY max(calcule_le) DESC""")
    changees = requete("""
        SELECT i.evenement, i.session, i.seq, i.serveur || '/' || i.outil, i.avant, i.apres, e.journal, e.rang
        FROM impacts i
        LEFT JOIN entrees e ON e.journal = i.journal AND e.session = i.session AND e.seq = i.seq AND e.type = 'appel'
        WHERE i.change ORDER BY i.calcule_le DESC, i.session, i.seq""")
    decisions = requete("""
        SELECT e.journal, e.rang, e.ts, e.session, e.seq, e.sous_agent, e.serveur || '/' || e.outil, e.effet, e.motif,
               (SELECT count(*) FROM consultations c WHERE c.journal = e.journal AND c.rang = e.rang AND c.resultat = 'ok')
        FROM entrees e
        WHERE e.type = 'appel' AND (e.effet <> 'autoriser' OR e.outil IN ('attribuer_pret', 'effectuer_virement'))
        ORDER BY e.ts DESC LIMIT 50""")
    nb = requete("SELECT count(*) FILTER (WHERE type = 'appel'), count(DISTINCT journal) FROM entrees")[0]

    out = ["<h1>Detective</h1><p class=sous>Sur quelles données les agents ont-ils décidé, "
           "et quelles décisions passées tombent quand une donnée ou une règle change.</p>"]
    total_changees = sum(r[2] for r in impacts)
    out.append("<div class=chiffres>"
               "<div class='carte chiffre'><b>%d</b><span>décisions d'agent journalisées</span></div>"
               "<div class='carte chiffre'><b>%d</b><span>journaux vérifiés et versés</span></div>"
               "<div class='carte chiffre'><b>%d</b><span>décisions passées changées par un rejeu</span></div></div>"
               % (nb[0], nb[1], total_changees))

    out.append("<h2>Rejeux : données ou règles qui changent</h2><div class=carte>")
    if impacts:
        out.append("<table><tr><th>Évènement</th><th>Décisions rejouées</th><th>Changées</th><th>Calculé le</th></tr>")
        for ev, n, c, quand in impacts:
            out.append("<tr><td>%s</td><td>%d</td><td><b>%d</b></td><td>%s</td></tr>" % (e(ev), n, c, date(quand)))
        out.append("</table>")
    else:
        out.append("<p class=doux>Aucun rejeu. <code>journal-sql impacts … | psql</code></p>")
    out.append("</div>")

    if changees:
        out.append("<h2>Décisions que ces changements invalident</h2><div class=carte><table>"
                   "<tr><th>Évènement</th><th>Session</th><th>Action</th><th>Effet journalisé → rejoué</th><th></th></tr>")
        for ev, sess, seq, action, avant, apres, journal, rang in changees:
            lien = ("<a href='/decision?j=%s&r=%s'>ce qu'elle a lu</a>" % (e(journal), e(rang))) if journal else ""
            out.append("<tr><td>%s</td><td><code>%s</code> #%s</td><td><code>%s</code></td><td>%s<span class=fleche>→</span>%s</td><td>%s</td></tr>"
                       % (e(ev), e((sess or "")[:8]), e(seq), e(action), effet(avant), effet(apres), lien))
        out.append("</table></div>")

    out.append("<h2>Décisions d'action</h2><div class=carte>")
    if decisions:
        out.append("<table><tr><th>Date</th><th>Sous-agent</th><th>Action</th><th>Décision</th><th>Motif</th><th>Lu</th></tr>")
        for journal, rang, ts, sess, seq, agent, action, eff, motif, lus in decisions:
            out.append("<tr><td>%s</td><td>%s</td><td><a href='/decision?j=%s&r=%s'><code>%s</code></a></td><td>%s</td><td>%s</td><td>%d source(s)</td></tr>"
                       % (date(ts), e(agent or "—"), e(journal), e(rang), e(action), effet(eff), e(motif), lus))
        out.append("</table>")
    else:
        out.append("<p class=doux>Aucun journal versé. <code>journal-sql exporter journal.jsonl | psql</code></p>")
    out.append("</div>")
    return page("Detective", "".join(out))


def decision(journal, rang):
    lignes = requete("""
        SELECT ts, session, seq, sous_agent, serveur || '/' || outil, effet, motif, statut, jeu_id, jeu_version
        FROM entrees WHERE journal = %s AND rang = %s AND type = 'appel'""", (journal, rang))
    if not lignes:
        return None
    ts, sess, seq, agent, action, eff, motif, statut, jeu_id, jeu_v = lignes[0]
    lus = requete("""
        SELECT seq, serveur || '/' || outil, resultat, source_origine, source_date
        FROM consultations WHERE journal = %s AND rang = %s ORDER BY i""", (journal, rang))
    regles = requete("""
        SELECT directive, version, effet, declenchee, appliquee, resultat, motif
        FROM applications WHERE journal = %s AND rang = %s ORDER BY directive""", (journal, rang))
    rejeux = requete("""
        SELECT evenement, avant, apres, change FROM impacts
        WHERE journal = %s AND session = %s AND seq = %s ORDER BY calcule_le DESC""", (journal, sess, seq))

    out = ["<p><a href='/'>← toutes les décisions</a></p>",
           "<h1><code>%s</code></h1><p class=sous>%s · sous-agent %s · session <code>%s</code> #%s · jeu %s v%s</p>"
           % (e(action), date(ts), e(agent or "—"), e((sess or "")[:8]), e(seq), e(jeu_id), e(jeu_v)),
           "<div class=carte><p style='margin:0'>Décision : %s · statut <code>%s</code></p><p class=doux style='margin:6px 0 0'>%s</p></div>"
           % (effet(eff), e(statut), e(motif))]

    out.append("<h2>Ce que la décision a lu</h2><div class=carte><table>"
               "<tr><th>#</th><th>Donnée lue</th><th>Résultat</th><th>Origine</th><th>Date de la source</th></tr>")
    for s, outil, res, origine, quand in lus:
        classe = "ok" if res == "ok" else "bad"
        out.append("<tr><td>%s</td><td><code>%s</code></td><td><span class='eff %s'>%s</span></td><td>%s</td><td>%s</td></tr>"
                   % (e(s), e(outil), classe, e(res), e(origine or "non déclarée"), date(quand)))
    if not lus:
        out.append("<tr><td colspan=5 class=doux>Aucune consultation avant cet appel.</td></tr>")
    out.append("</table></div>")

    out.append("<h2>Règles évaluées</h2><div class=carte><table>"
               "<tr><th>Directive</th><th>Déclenchée</th><th>Appliquée</th><th>Condition</th><th>Effet</th><th>Motif</th></tr>")
    for d, v, eff_d, decl, appl, res, mot in regles:
        out.append("<tr><td><b>%s</b> v%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>"
                   % (e(d), e(v), "oui" if decl else "non", "oui" if appl else "non", e(res), effet(eff_d), e(mot)))
    if not regles:
        out.append("<tr><td colspan=6 class=doux>Aucune directive ne visait cet appel.</td></tr>")
    out.append("</table></div>")

    if rejeux:
        out.append("<h2>Rejeux de cette décision</h2><div class=carte><table>"
                   "<tr><th>Évènement</th><th>Journalisé → rejoué</th></tr>")
        for ev, avant, apres, change in rejeux:
            out.append("<tr><td>%s</td><td>%s<span class=fleche>→</span>%s%s</td></tr>"
                       % (e(ev), effet(avant), effet(apres), " <b>changée</b>" if change else ""))
        out.append("</table></div>")
    return page(action, "".join(out))


class Handler(BaseHTTPRequestHandler):
    def envoyer(self, code, corps, type_="text/html; charset=utf-8"):
        b = corps.encode("utf-8")
        self.send_response(code)
        self.send_header("Content-Type", type_)
        self.send_header("Content-Length", str(len(b)))
        self.send_header("X-Content-Type-Options", "nosniff")
        self.send_header("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
        self.end_headers()
        self.wfile.write(b)

    def do_GET(self):
        url = urlparse(self.path)
        try:
            if url.path == "/healthz":
                requete("SELECT 1")
                return self.envoyer(200, "ok", "text/plain; charset=utf-8")
            if url.path == "/":
                return self.envoyer(200, accueil())
            if url.path == "/decision":
                q = parse_qs(url.query)
                j, r = (q.get("j") or [""])[0], (q.get("r") or [""])[0]
                if not j or not r.isdigit():
                    return self.envoyer(400, page("Requête invalide", "<p>Paramètres j et r attendus.</p>"))
                corps = decision(j, int(r))
                if corps is None:
                    return self.envoyer(404, page("Introuvable", "<p>Décision introuvable. <a href='/'>Retour</a></p>"))
                return self.envoyer(200, corps)
            return self.envoyer(404, page("Introuvable", "<p><a href='/'>Retour</a></p>"))
        except psycopg.Error as err:
            # Le message de la base peut contenir l'hôte : il reste dans les traces du service.
            print("detective: base : %s" % err, file=sys.stderr)
            return self.envoyer(503, page("Base indisponible", "<p>La base ne répond pas. Réessayer dans un instant.</p>"))

    def log_message(self, fmt, *args):
        print("detective: " + fmt % args, file=sys.stderr)


def main():
    if not DSN:
        sys.exit("detective: DATABASE_URL absente (injectée par l'intégration de service Aiven)")
    print("detective: écoute sur :%d" % PORT, file=sys.stderr)
    ThreadingHTTPServer(("0.0.0.0", PORT), Handler).serve_forever()


if __name__ == "__main__":
    main()
