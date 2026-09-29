package journal

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// EtatDepot dit ce que le coffre sait d'une empreinte.
type EtatDepot string

const (
	// Present : les arguments sont là, l'appel est rejouable.
	Present EtatDepot = "present"
	// Purge : les arguments ont été effacés volontairement ; l'entrée du
	// journal reste vérifiable dans la chaîne, l'appel n'est plus rejouable.
	Purge EtatDepot = "purge"
	// Absent : le coffre n'a jamais rien eu, ou l'a perdu sans purge.
	Absent EtatDepot = "absent"
)

var (
	// ErrPurgeSansJournal : une purge doit être chaînée, donc un journal
	// doit être attaché au coffre.
	ErrPurgeSansJournal = errors.New("coffre: purge refusée sans journal attaché")
	// ErrPurgeSansMotif : motif et auteur sont obligatoires.
	ErrPurgeSansMotif = errors.New("coffre: purge refusée sans motif ou sans auteur")
)

// DemandePurge dit pourquoi et par qui une purge est faite. Les deux vont
// dans l'entrée purge du journal.
type DemandePurge struct {
	Motif  string
	Auteur string
}

// Journaliser attache le journal où chaque purge écrit son entrée.
func (c *Coffre) Journaliser(e *Ecrivain) { c.journal = e }

// Horloge remplace l'horloge du coffre, pour les tests.
func (c *Coffre) Horloge(h func() time.Time) { c.horloge = h }

// Etat rend l'état d'une empreinte et, pour une purge, sa date.
//
// La pierre tombale n'est pas chaînée : elle distingue une purge d'une perte
// accidentelle, elle ne prouve pas qu'une suppression était une purge. Dans
// les deux cas, l'appel est honnêtement « non rejouable ».
func (c *Coffre) Etat(empreinte string) (EtatDepot, time.Time) {
	if c == nil {
		return Absent, time.Time{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.index[empreinte]; ok {
		return Present, time.Time{}
	}
	if le, ok := c.purgees[empreinte]; ok {
		return Purge, le
	}
	return Absent, time.Time{}
}

// Purgee dit si une empreinte a été purgée, et quand. C'est la seconde
// moitié de l'interface que le rejeu attend d'un résolveur.
func (c *Coffre) Purgee(empreinte string) (time.Time, bool) {
	etat, le := c.Etat(empreinte)
	return le, etat == Purge
}

// PurgerEmpreintes efface les arguments des empreintes d'origine données et
// rend celles qui ont effectivement été purgées, triées. Une empreinte
// inconnue ou déjà purgée est ignorée.
func (c *Coffre) PurgerEmpreintes(d DemandePurge, empreintes ...string) ([]string, error) {
	if c == nil {
		return nil, nil
	}
	cibles := make(map[string]bool, len(empreintes))
	for _, e := range empreintes {
		cibles[e] = true
	}
	return c.purger(d, nil, func(dep Depot) bool { return cibles[dep.Empreinte] })
}

// PurgerAvant efface les arguments déposés strictement avant limite. L'âge
// d'un dépôt est celui de son premier dépôt : un appel ultérieur aux mêmes
// arguments ne le rajeunit pas, puisque la donnée est détenue depuis lors.
func (c *Coffre) PurgerAvant(limite time.Time, d DemandePurge) ([]string, error) {
	if c == nil {
		return nil, nil
	}
	return c.purger(d, &limite, func(dep Depot) bool { return dep.TS.Before(limite) })
}

// purger réécrit le coffre sans les arguments des dépôts retenus, qui
// deviennent des pierres tombales. La réécriture passe par un fichier
// temporaire synchronisé puis renommé : une coupure laisse l'ancien coffre
// ou le nouveau, jamais un mélange. Les arguments purgés ne restent pas dans
// le fichier ; ce qui reste sur le support physique relève du disque.
//
// L'entrée purge est chaînée avant la réécriture, à la date des pierres
// tombales. Si la réécriture échoue ensuite, une alerte le dit : le
// journal annonce alors une purge que le coffre n'a pas faite, ce que le
// rejeu voit (arguments présents, appel toujours rejouable). L'ordre
// inverse laisserait des pierres tombales sans entrée, que le rejeu
// traiterait comme une altération.
func (c *Coffre) purger(dem DemandePurge, avant *time.Time, retenir func(Depot) bool) ([]string, error) {
	if dem.Motif == "" || dem.Auteur == "" {
		return nil, ErrPurgeSansMotif
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.journal == nil {
		return nil, ErrPurgeSansJournal
	}

	var purgees []string
	for emp, d := range c.index {
		if retenir(d) {
			purgees = append(purgees, emp)
		}
	}
	if len(purgees) == 0 {
		return nil, nil
	}
	sort.Strings(purgees)
	maintenant := c.horloge().UTC()

	nouvIndex := make(map[string]Depot, len(c.index))
	for emp, d := range c.index {
		nouvIndex[emp] = d
	}
	nouvPurgees := make(map[string]time.Time, len(c.purgees)+len(purgees))
	for emp, le := range c.purgees {
		nouvPurgees[emp] = le
	}
	for _, emp := range purgees {
		delete(nouvIndex, emp)
		nouvPurgees[emp] = maintenant
	}

	var limite *time.Time
	if avant != nil {
		l := avant.UTC()
		limite = &l
	}
	if _, err := c.journal.Ecrire(Entree{
		TS: maintenant, Type: PurgeT, Empreintes: purgees, Motif: dem.Motif, Auteur: dem.Auteur, PurgeAvant: limite,
	}); err != nil {
		return nil, fmt.Errorf("coffre: purge non chaînée, rien n'est effacé: %w", err)
	}
	f, err := c.reecrire(nouvIndex, nouvPurgees)
	if err != nil {
		c.journal.Ecrire(Entree{Type: AlerteT, Empreintes: purgees,
			Message: "purge annoncée non appliquée au coffre : " + err.Error()})
		return nil, err
	}
	c.fichier.Close()
	c.fichier, c.index, c.purgees = f, nouvIndex, nouvPurgees
	return purgees, nil
}

// reecrire écrit l'état complet dans un temporaire, le renomme sur le
// coffre et rend le nouveau fichier ouvert en ajout.
func (c *Coffre) reecrire(index map[string]Depot, purgees map[string]time.Time) (*os.File, error) {
	if c.chemin == "" {
		return nil, fmt.Errorf("coffre: purge impossible sans chemin")
	}
	var lignes []Depot
	for _, d := range index {
		lignes = append(lignes, d)
	}
	for emp, le := range purgees {
		le := le
		lignes = append(lignes, Depot{Empreinte: emp, PurgeLe: &le})
	}
	// Ordre stable : par date de dépôt (ou de purge), puis empreinte.
	date := func(d Depot) time.Time {
		if d.PurgeLe != nil {
			return *d.PurgeLe
		}
		return d.TS
	}
	sort.Slice(lignes, func(i, j int) bool {
		if a, b := date(lignes[i]), date(lignes[j]); !a.Equal(b) {
			return a.Before(b)
		}
		return lignes[i].Empreinte < lignes[j].Empreinte
	})

	tmp, err := os.CreateTemp(filepath.Dir(c.chemin), ".coffre-*")
	if err != nil {
		return nil, fmt.Errorf("coffre: purge: %w", err)
	}
	nettoyer := func() { tmp.Close(); os.Remove(tmp.Name()) }
	if err := tmp.Chmod(0o600); err != nil {
		nettoyer()
		return nil, fmt.Errorf("coffre: purge: %w", err)
	}
	for _, d := range lignes {
		b, err := json.Marshal(d)
		if err != nil {
			nettoyer()
			return nil, fmt.Errorf("coffre: purge: encoder: %w", err)
		}
		if _, err := tmp.Write(append(b, '\n')); err != nil {
			nettoyer()
			return nil, fmt.Errorf("coffre: purge: écrire: %w", err)
		}
	}
	if err := tmp.Sync(); err != nil {
		nettoyer()
		return nil, fmt.Errorf("coffre: purge: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return nil, fmt.Errorf("coffre: purge: %w", err)
	}
	// Le fichier d'ajout est ouvert avant le renommage : le descripteur suit
	// l'inode, donc après un renommage réussi il n'y a plus rien qui échoue.
	f, err := os.OpenFile(tmp.Name(), os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		os.Remove(tmp.Name())
		return nil, fmt.Errorf("coffre: purge: %w", err)
	}
	if err := os.Rename(tmp.Name(), c.chemin); err != nil {
		f.Close()
		os.Remove(tmp.Name())
		return nil, fmt.Errorf("coffre: purge: renommer: %w", err)
	}
	if d, err := os.Open(filepath.Dir(c.chemin)); err == nil {
		d.Sync()
		d.Close()
	}
	return f, nil
}
