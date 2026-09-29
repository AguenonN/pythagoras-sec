package backend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connecterStdio vérifie l'empreinte du binaire puis le lance. Un binaire
// non conforme n'est jamais exécuté.
//
// Limite connue : entre le calcul de l'empreinte et l'exec, le fichier
// pourrait être remplacé par qui a le droit d'écrire dans son répertoire.
// Le registre exige un chemin absolu ; le répertoire doit n'être inscriptible
// que par l'opérateur.
func (c *client) connecterStdio(ctx context.Context) error {
	chemin := c.s.Commande.Chemin
	observe, err := empreinteFichier(chemin)
	if err != nil {
		return c.echec("", "binaire illisible, processus non lancé", err)
	}
	if observe != c.s.Attestation.Attendu {
		return c.echec(observe, "empreinte du binaire différente de celle du registre, processus non lancé", nil)
	}
	cmd := exec.Command(chemin, c.s.Commande.Args...)
	// Environnement vide : les secrets de l'engine ne passent pas au serveur.
	cmd.Env = []string{}
	cmd.Stderr = c.opts.Stderr
	return c.etablir(ctx, &mcp.CommandTransport{Command: cmd}, func() string { return observe })
}

func empreinteFichier(chemin string) (string, error) {
	f, err := os.Open(chemin)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
