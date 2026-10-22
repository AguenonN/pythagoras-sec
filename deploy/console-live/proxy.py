"""Accès à la console « live » sur Aiven Runtime, protégé par mot de passe.

La console n'écoute que sur la boucle locale et refuse tout autre hôte. Ce
proxy la rend joignable sur le port public : toute requête exige le mot de
passe DEMO_MOT_DE_PASSE (authentification HTTP Basic, utilisateur
quelconque) ; il transmet alors GET et POST avec l'hôte local, sans l'en-tête
Origin (la console compare l'origine à l'hôte). Sans mot de passe configuré,
seules les lectures passent.
"""
import base64
import hmac
import http.client
import os
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

AMONT = ("127.0.0.1", int(os.environ.get("CONSOLE_PORT", "8088")))
PORT = int(os.environ.get("PORT", "8080"))
MOT = os.environ.get("DEMO_MOT_DE_PASSE", "")
RETENUS = ("content-type", "content-length", "content-security-policy", "x-content-type-options", "cache-control",
           "last-modified", "etag")


class Proxy(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def autorise(self):
        if not MOT:
            return self.command in ("GET", "HEAD")
        h = self.headers.get("Authorization", "")
        if not h.startswith("Basic "):
            return False
        try:
            _, _, mot = base64.b64decode(h[6:]).decode("utf-8").partition(":")
        except Exception:
            return False
        return hmac.compare_digest(mot.encode(), MOT.encode())

    def repondre(self, code, corps, entetes=()):
        try:
            self.send_response(code)
            for k, v in entetes:
                self.send_header(k, v)
            self.send_header("Content-Length", str(len(corps)))
            self.end_headers()
            if self.command != "HEAD":
                self.wfile.write(corps)
        except (BrokenPipeError, ConnectionResetError):
            pass

    def traiter(self):
        if not self.autorise():
            if MOT:
                return self.repondre(401, b"password required", [("WWW-Authenticate", 'Basic realm="Pythagoras demo"'),
                                                                  ("Content-Type", "text/plain")])
            return self.repondre(403, b'{"erreur":"read-only"}', [("Content-Type", "application/json")])
        n = int(self.headers.get("Content-Length") or 0)
        corps = self.rfile.read(n) if n else None
        entetes = {"Host": "%s:%d" % AMONT}
        for k in ("Content-Type", "Accept"):
            if self.headers.get(k):
                entetes[k] = self.headers[k]
        try:
            c = http.client.HTTPConnection(*AMONT, timeout=300)
            c.request(self.command, self.path, body=corps, headers=entetes)
            r = c.getresponse()
            rep = r.read()
        except OSError as err:
            print("proxy: console injoignable : %s" % err, file=sys.stderr)
            return self.repondre(503, b'{"erreur":"console busy or starting, retry in a moment"}', [("Content-Type", "application/json")])
        self.repondre(r.status, rep, [(k, v) for k, v in r.getheaders() if k.lower() in RETENUS and k.lower() != "content-length"])

    do_GET = do_POST = do_HEAD = traiter

    def log_message(self, fmt, *args):
        print("proxy: " + fmt % args, file=sys.stderr)


if __name__ == "__main__":
    ThreadingHTTPServer(("0.0.0.0", PORT), Proxy).serve_forever()
