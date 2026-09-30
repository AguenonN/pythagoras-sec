package gate

import (
	"errors"
	"fmt"
	"os"

	"mcprogue/internal/directive"
	"mcprogue/internal/journal"
)

// versionJeu identifie un jeu comme le fait le rejeu : (id, version).
type versionJeu struct {
	id      string
	version int
}

var (
	// ErrJeuAutreID : le jeu rechargé n'a pas l'identifiant du jeu actif.
	ErrJeuAutreID = errors.New("gate: le jeu rechargé n'a pas l'identifiant du jeu actif")
	// ErrVersionReecrite : une version déjà chargée revient avec un autre
	// contenu ; le rejeu ne saurait plus lequel des deux a décidé.
	ErrVersionReecrite = errors.New("gate: version déjà chargée avec une autre empreinte")
)

// Rechargement dit ce qu'un rechargement a fait.
type Rechargement struct {
	Bascule bool          // un nouveau jeu est actif
	Jeu     directive.Jeu // le jeu actif après le rechargement
}

// Recharger remplace le jeu actif sans redémarrer. Le document passe par
// le chargeur strict, ses directives ne visent que des cibles du
// registre ; il doit garder l'identifiant du jeu actif, et une
// version déjà chargée doit revenir à l'identique (c'est ce qui permet le
// retour arrière). En cas d'échec, l'ancien jeu reste actif et une entrée
// alerte est écrite. En cas de succès, l'entrée jeu_charge est écrite
// avant la bascule, puis la bascule est atomique : chaque appel décide
// sous le jeu qu'il a lu avant Decide, et son entrée appel cite ce jeu.
// Un jeu identique au jeu actif ne bascule rien et n'écrit rien. L'entrée
// jeu_charge d'une bascule porte la version de départ et celle d'arrivée ;
// le retour à une version inférieure est permis et porte en plus
// retour_arriere.
func (g *Gate) Recharger(donnees []byte) (Rechargement, error) {
	g.recharge.Lock()
	defer g.recharge.Unlock()
	actif := *g.jeu.Load()

	nouveau, err := directive.Charger(donnees)
	if err == nil {
		err = verifierCibles(*nouveau, g.cfg.Registre)
	}
	if err == nil && nouveau.Meta.ID != actif.Meta.ID {
		err = fmt.Errorf("%w (%q au lieu de %q)", ErrJeuAutreID, nouveau.Meta.ID, actif.Meta.ID)
	}
	if err == nil {
		v := versionJeu{nouveau.Meta.ID, nouveau.Meta.Version}
		if emp, vu := g.charges[v]; vu && emp != nouveau.Meta.Empreinte {
			err = fmt.Errorf("%w : %s v%d était %s, le document donne %s", ErrVersionReecrite,
				v.id, v.version, emp, nouveau.Meta.Empreinte)
		}
	}
	if err != nil {
		g.alerte("", "", tronquer("rechargement du jeu refusé, "+fmt.Sprintf("%s v%d", actif.Meta.ID, actif.Meta.Version)+
			" reste actif : "+err.Error()))
		return Rechargement{Jeu: actif}, err
	}
	if nouveau.Meta.Empreinte == actif.Meta.Empreinte {
		return Rechargement{Jeu: actif}, nil
	}
	contexte := fmt.Sprintf("rechargé à chaud, remplace %s v%d %s", actif.Meta.ID, actif.Meta.Version, actif.Meta.Empreinte)
	// Toute bascule à chaud dit d'où elle part et où elle arrive ; un retour
	// à une version inférieure, autorisé, est de plus marqué comme tel.
	base := journal.Entree{VersionDepart: actif.Meta.Version, VersionArrivee: nouveau.Meta.Version}
	if nouveau.Meta.Version < actif.Meta.Version {
		base.RetourArriere = true
		contexte = fmt.Sprintf("retour arrière de v%d à v%d, ", actif.Meta.Version, nouveau.Meta.Version) + contexte
	}
	if err := g.journaliserJeu(base, *nouveau, contexte); err != nil {
		// Sans trace de la bascule, pas de bascule.
		g.enPanne.Store(true)
		g.log.Error("journal indisponible, l'engine refuse désormais tout appel", "erreur", err)
		return Rechargement{Jeu: actif}, err
	}
	g.charges[versionJeu{nouveau.Meta.ID, nouveau.Meta.Version}] = nouveau.Meta.Empreinte
	g.jeu.Store(nouveau)
	g.log.Info("jeu rechargé", "id", nouveau.Meta.ID, "version", nouveau.Meta.Version, "empreinte", nouveau.Meta.Empreinte)
	return Rechargement{Bascule: true, Jeu: *nouveau}, nil
}

// RechargerFichier lit le document puis Recharger. Un fichier illisible
// est un rechargement refusé comme un autre : alerte, ancien jeu actif.
func (g *Gate) RechargerFichier(chemin string) (Rechargement, error) {
	b, err := os.ReadFile(chemin)
	if err != nil {
		g.recharge.Lock()
		actif := *g.jeu.Load()
		g.recharge.Unlock()
		g.alerte("", "", tronquer(fmt.Sprintf("rechargement du jeu refusé, %s v%d reste actif : fichier illisible : %v",
			actif.Meta.ID, actif.Meta.Version, err)))
		return Rechargement{Jeu: actif}, err
	}
	return g.Recharger(b)
}

// Jeu rend le jeu actif.
func (g *Gate) Jeu() directive.Jeu { return *g.jeu.Load() }

// tronquer borne un message d'alerte : le chargeur rend toutes ses erreurs
// en bloc, et une ligne de journal est bornée.
func tronquer(s string) string {
	const max = 4000
	if len(s) <= max {
		return s
	}
	return s[:max] + " […]"
}
