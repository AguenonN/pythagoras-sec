"""Client MCP générique sur stdio, côté hôte.

Un message JSON-RPC par ligne. Les réponses sont indexées par id ; les
notifications du serveur sont ignorées ; toute requête du serveur vers
l'hôte (sampling, elicitation, roots…) est refusée : cet hôte ne prête
jamais son modèle ni son utilisateur à un serveur.

Deux façons de refuser un appel coexistent et sont distinguées :

- une erreur JSON-RPC, levée en ``ErreurMCP`` (``code`` -32001 : refus de
  politique, convention de cmd/mcp-proxy) ;
- un résultat ``isError`` : l'outil a décliné ; c'est une valeur, rendue
  telle quelle à l'appelant.
"""

import json
import subprocess

VERSION_PROTOCOLE = "2025-06-18"
CODE_REFUS = -32001


class ErreurMCP(Exception):
    def __init__(self, message, code=None, data=None):
        super().__init__(message)
        self.message = message
        self.code = code
        self.data = data


class ClientMCP:
    def __init__(self, argv, env=None, nom="agents-boucle", version="0.1.0"):
        self.argv = argv
        self.env = env
        self.nom = nom
        self.version = version
        self.proc = None
        self._id = 0

    def __enter__(self):
        self.demarrer()
        return self

    def __exit__(self, *exc):
        self.fermer()

    def demarrer(self):
        self.proc = subprocess.Popen(
            self.argv, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL, env=self.env, text=True, encoding="utf-8", bufsize=1)
        self._requete("initialize", {
            "protocolVersion": VERSION_PROTOCOLE,
            "capabilities": {},
            "clientInfo": {"name": self.nom, "version": self.version},
        })
        self._envoyer({"jsonrpc": "2.0", "method": "notifications/initialized"})

    def fermer(self):
        if self.proc is None:
            return
        try:
            self.proc.stdin.close()
        except OSError:
            pass
        try:
            self.proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            self.proc.kill()
            self.proc.wait()
        self.proc.stdout.close()
        self.proc = None

    # -- JSON-RPC -------------------------------------------------------------

    def _envoyer(self, message):
        self.proc.stdin.write(json.dumps(message, ensure_ascii=False) + "\n")
        self.proc.stdin.flush()

    def _requete(self, methode, params):
        self._id += 1
        attendu = self._id
        self._envoyer({"jsonrpc": "2.0", "id": attendu, "method": methode, "params": params})
        while True:
            ligne = self.proc.stdout.readline()
            if not ligne:
                raise ErreurMCP("le serveur MCP s'est arrêté pendant %s" % methode)
            ligne = ligne.strip()
            if not ligne:
                continue
            try:
                msg = json.loads(ligne)
            except ValueError:
                raise ErreurMCP("ligne illisible du serveur MCP") from None
            if "method" in msg:
                if "id" in msg:  # requête du serveur : toujours refusée
                    self._envoyer({"jsonrpc": "2.0", "id": msg["id"],
                                   "error": {"code": -32601, "message": "refusé par l'hôte"}})
                continue  # notification
            if msg.get("id") != attendu:
                continue
            if "error" in msg:
                e = msg["error"] or {}
                raise ErreurMCP(e.get("message", ""), e.get("code"), e.get("data"))
            return msg.get("result") or {}

    # -- MCP ------------------------------------------------------------------

    def lister_outils(self):
        outils, curseur = [], None
        while True:
            params = {"cursor": curseur} if curseur else {}
            r = self._requete("tools/list", params)
            outils += r.get("tools", [])
            curseur = r.get("nextCursor")
            if not curseur:
                return outils

    def appeler(self, nom, arguments, meta=None):
        params = {"name": nom, "arguments": arguments}
        if meta:
            params["_meta"] = meta
        return self._requete("tools/call", params)
