package console

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Chemins sont les seuls fichiers que la console lit ou écrit en dehors
// des sessions. Tout est sous la racine de la démonstration.
type Chemins struct {
	Directives   string // demo/directives/demo.json
	Historique   string // demo/directives/historique/ : demo.v<n>.json
	Origines     string // demo/directives/origines.json
	Annuaire     string // demo/annuaire.json
	Cles         string // demo/execution/cles/
	Autorite     string // demo/execution/cles/autorite.pub
	Approbations string // demo/execution/approbations/<empreinte>/
	Propositions string // demo/execution/propositions/
	Compilation  string // demo/execution/compilation/<id>/
	JeuxTravail  string // demo/scenarios/jeux/ : jeux de travail des scénarios (lecture seule)
}

// Chemins rend les chemins à lire ou écrire : ceux de la démonstration,
// sous la racine des commandes (le dépôt) si elle est donnée.
func (d *Depot) Chemins() Chemins {
	c := d.cheminsRel()
	r := d.Commandes.Racine
	if r == "" {
		return c
	}
	j := func(p string) string { return filepath.Join(r, p) }
	return Chemins{Directives: j(c.Directives), Historique: j(c.Historique), Origines: j(c.Origines), Annuaire: j(c.Annuaire),
		Cles: j(c.Cles), Autorite: j(c.Autorite), Approbations: j(c.Approbations), Propositions: j(c.Propositions), Compilation: j(c.Compilation), JeuxTravail: j(c.JeuxTravail)}
}

// cheminsRel calcule les chemins relatifs à la racine de la démonstration :
// c'est sous cette forme que les commandes (qui tournent dans la racine du
// dépôt) les reçoivent.
func (d *Depot) cheminsRel() Chemins {
	demo := d.Demo
	if demo == "" {
		demo = "demo"
	}
	ex := filepath.Join(demo, "execution")
	return Chemins{
		Directives:   filepath.Join(demo, "directives", "demo.json"),
		Historique:   filepath.Join(demo, "directives", "historique"),
		Origines:     filepath.Join(demo, "directives", "origines.json"),
		Annuaire:     filepath.Join(demo, "annuaire.json"),
		Cles:         filepath.Join(ex, "cles"),
		Autorite:     filepath.Join(ex, "cles", "autorite.pub"),
		Approbations: filepath.Join(ex, "approbations"),
		Propositions: filepath.Join(ex, "propositions"),
		Compilation:  filepath.Join(ex, "compilation"),
		JeuxTravail:  filepath.Join(demo, "scenarios", "jeux"),
	}
}

// Commandes sont les programmes appelés par la console. Chacune est une
// ligne de commande déjà découpée ; la console ne signe, ne compile et ne
// lance rien elle-même.
type Commandes struct {
	// Racine est le répertoire de travail des commandes (la racine du
	// dépôt : les chemins de demo/ sont relatifs à elle).
	Racine string
	// Approuver : mcp-approve (sous-commande valider).
	Approuver []string
	// Compilateur : python -m compilateur.service (contrats §4).
	Compilateur []string
	// Lanceur : scripts/lancer-demo.sh (sous-commande demande).
	Lanceur []string
	// Env s'ajoute à l'environnement des commandes (ex. PYTHONPATH).
	Env []string
}

// ecritureMu sérialise les écritures de la console : une activation ne
// s'intercale jamais dans une autre.
var ecritureMu sync.Mutex

// ecrireAtomique écrit un fichier par fichier temporaire du même
// répertoire puis renommage : un lecteur voit l'ancien contenu ou le
// nouveau, jamais un fichier à moitié écrit.
func ecrireAtomique(chemin string, contenu []byte, mode os.FileMode) error {
	dir := filepath.Dir(chemin)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var alea [6]byte
	if _, err := rand.Read(alea[:]); err != nil {
		return err
	}
	tmp := filepath.Join(dir, "."+filepath.Base(chemin)+"."+hex.EncodeToString(alea[:])+".tmp")
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := f.Write(contenu); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, chemin); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("écriture de %s : %w", filepath.Base(chemin), err)
	}
	return nil
}
