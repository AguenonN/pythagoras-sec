"""Vue publique en lecture seule de la console, pour Aiven Runtime.

La console n'écoute que sur la boucle locale et refuse tout autre hôte
(protection contre le DNS rebinding). Ce proxy la rend joignable sur le port
public en ne transmettant que GET et HEAD, avec l'hôte local : les actions
(valider, compiler, lancer une demande) restent impossibles depuis Internet.
"""
import http.client
import os
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

AMONT = ("127.0.0.1", int(os.environ.get("CONSOLE_PORT", "8088")))
PORT = int(os.environ.get("PORT", "8080"))
ENTETES_RETENUS = ("content-type", "content-length", "content-security-policy", "x-content-type-options",
                   "cache-control", "last-modified", "etag")


class Proxy(BaseHTTPRequestHandler):
    def transmettre(self, avec_corps):
        try:
            c = http.client.HTTPConnection(*AMONT, timeout=180)
            c.request(self.command, self.path, headers={"Host": "%s:%d" % AMONT, "Accept": self.headers.get("Accept", "*/*")})
            r = c.getresponse()
            corps = r.read()
        except OSError as err:
            print("proxy: console injoignable : %s" % err, file=sys.stderr)
            return self.repondre(503, b'{"erreur":"console busy or starting, retry in a moment"}')
        try:
            self.send_response(r.status)
            for k, v in r.getheaders():
                if k.lower() in ENTETES_RETENUS:
                    self.send_header(k, v)
            self.end_headers()
            if avec_corps:
                self.wfile.write(corps)
        except (BrokenPipeError, ConnectionResetError):
            pass  # le navigateur a abandonné la requête

    def repondre(self, code, corps):
        try:
            self._repondre(code, corps)
        except (BrokenPipeError, ConnectionResetError):
            pass  # le navigateur a abandonné la requête

    def _repondre(self, code, corps):
        self.send_response(code)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(corps)))
        self.end_headers()
        self.wfile.write(corps)

    def do_GET(self):
        self.transmettre(True)

    def do_HEAD(self):
        self.transmettre(False)

    def refuser(self):
        self.repondre(403, '{"erreur":"Public read-only view: actions run on the bank\'s own console."}'.encode())

    do_POST = do_PUT = do_PATCH = do_DELETE = refuser

    def log_message(self, fmt, *args):
        print("proxy: " + fmt % args, file=sys.stderr)


if __name__ == "__main__":
    ThreadingHTTPServer(("0.0.0.0", PORT), Proxy).serve_forever()
