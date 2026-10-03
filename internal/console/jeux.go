package console

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"mcprogue/internal/directive"
	"mcprogue/internal/journal"
)

// JeuCharge est l'ensemble des règles lisible, avec sa provenance.
type JeuCharge struct {
	Jeu    directive.Jeu
	Source string
	Brut   []byte
	// Courant : ce sont les règles en vigueur (demo/directives/demo.json),
	// par opposition à une version archivée de l'historique.
	Courant bool
	// Travail : non vide pour un ensemble de travail (jeu d'essai d'une
	// session lancée avec un jeu de travail ou « sans la directive D-00n »),
	// avec son libellé ; ces ensembles ne servent qu'à rejouer.
	Travail string
}

// Libelle est le nom de ces règles dans l'API (jamais affiché tel quel).
func (j JeuCharge) Libelle() string {
	return fmt.Sprintf("%s, version %d", j.Jeu.Meta.ID, j.Jeu.Meta.Version)
}

var reVersionArchive = regexp.MustCompile(`^demo\.v(\d+)\.json$`)

// chargerOctets passe par le chargeur strict, le seul juge, puis refuse
// toute directive qui n'est pas de la forme unique (l'ancienne forme est
// retirée du dépôt : la console ne la charge ni ne l'affiche).
func chargerOctets(brut []byte) (*directive.Jeu, error) {
	j, err := directive.Charger(brut)
	if err != nil {
		return nil, err
	}
	for _, d := range j.Directives {
		if !d.EstV2() {
			return nil, fmt.Errorf("la directive %s n'est pas dans la forme unique des directives", d.ID)
		}
	}
	return j, nil
}

// ChargerJeux lit les règles en vigueur (demo/directives/demo.json) et
// l'historique de leurs versions archivées : la console ne lit aucun autre
// ensemble de règles. Un fichier refusé par le chargeur strict est listé
// dans les refus, jamais utilisé. Résultat trié par version croissante.
func (d *Depot) ChargerJeux() (jeux []JeuCharge, refus []string) {
	ch := d.Chemins()
	vus := map[string]int{}
	ajouter := func(nom string, brut []byte, courant bool) {
		j, err := chargerOctets(brut)
		if err != nil {
			refus = append(refus, fmt.Sprintf("%s : %v", nom, err))
			return
		}
		cle := fmt.Sprintf("%s|%d|%s", j.Meta.ID, j.Meta.Version, j.Meta.Empreinte)
		if i, ok := vus[cle]; ok {
			jeux[i].Courant = jeux[i].Courant || courant
			return
		}
		vus[cle] = len(jeux)
		jeux = append(jeux, JeuCharge{Jeu: *j, Source: nom, Brut: brut, Courant: courant})
	}
	if brut, err := os.ReadFile(ch.Directives); err == nil {
		ajouter(filepath.Base(ch.Directives), brut, true)
	} else {
		refus = append(refus, fmt.Sprintf("%s : introuvable", filepath.Base(ch.Directives)))
	}
	ents, _ := os.ReadDir(ch.Historique)
	for _, e := range ents {
		if e.IsDir() || !reVersionArchive.MatchString(e.Name()) {
			continue
		}
		if brut, err := os.ReadFile(filepath.Join(ch.Historique, e.Name())); err == nil {
			ajouter("historique/"+e.Name(), brut, false)
		}
	}
	sort.SliceStable(jeux, func(i, k int) bool {
		a, b := jeux[i].Jeu.Meta, jeux[k].Jeu.Meta
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		return a.Version < b.Version
	})
	return jeux, refus
}

// AvecTravail complète les ensembles de règles connus (en vigueur et
// historique) des ensembles de travail sous lesquels des sessions ont tourné,
// retrouvés par l'empreinte consignée au journal :
//   - les fichiers de demo/directives/ et de demo/scenarios/jeux/ ;
//   - chacun des ensembles connus ou de ces fichiers privé d'une directive
//     (« --sans-directive D-00n »).
//
// Rien n'est lu si tous les journaux portent un ensemble déjà connu. Les
// ensembles de travail suivent les autres, sans jamais être « en vigueur » :
// listes, activations et règles en vigueur ne les voient pas.
func (d *Depot) AvecTravail(base []JeuCharge, etats []*Etat) []JeuCharge {
	manque := false
	for _, st := range etats {
		for _, e := range st.Entrees {
			if e.Type == journal.AppelT && e.Jeu != nil {
				if _, ok := trouverJeu(base, e.Jeu); !ok {
					manque = true
				}
			}
		}
	}
	if !manque {
		return base
	}
	vus := map[string]bool{}
	cle := func(m directive.MetaJeu) string { return fmt.Sprintf("%s|%d|%s", m.ID, m.Version, m.Empreinte) }
	for _, j := range base {
		vus[cle(j.Jeu.Meta)] = true
	}
	out := append([]JeuCharge(nil), base...)
	fichiers := []JeuCharge{}
	for _, dir := range []string{filepath.Dir(d.Chemins().Directives), d.Chemins().JeuxTravail} {
		ents, _ := os.ReadDir(dir)
		for _, e := range ents {
			if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
				continue
			}
			brut, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			j, err := chargerOctets(brut)
			if err != nil || vus[cle(j.Meta)] {
				continue
			}
			vus[cle(j.Meta)] = true
			fichiers = append(fichiers, JeuCharge{Jeu: *j, Source: filepath.Base(dir) + "/" + e.Name(), Brut: brut,
				Travail: "règles de travail « " + strings.TrimSuffix(e.Name(), ".json") + " »"})
		}
	}
	out = append(out, fichiers...)
	for _, j := range out {
		for _, dd := range j.Jeu.Directives {
			s := sansDirective(j.Jeu, dd.ID)
			if len(s.Directives) == 0 {
				continue
			}
			s.Directives = append([]directive.Directive(nil), s.Directives...)
			s.Meta.Empreinte, s.Meta.Signature = "", ""
			if directive.Valider(&s) != nil || vus[cle(s.Meta)] {
				continue
			}
			vus[cle(s.Meta)] = true
			out = append(out, JeuCharge{Jeu: s, Source: j.Source, Travail: "sans la directive " + dd.ID})
		}
	}
	return out
}

// jeuChargeDe retrouve l'ensemble sous lequel une entrée a été journalisée.
func jeuChargeDe(jeux []JeuCharge, ref *journal.RefJeu) *JeuCharge {
	if ref == nil {
		return nil
	}
	for i := range jeux {
		m := jeux[i].Jeu.Meta
		if m.ID == ref.ID && m.Version == ref.Version && m.Empreinte == ref.Empreinte {
			return &jeux[i]
		}
	}
	return nil
}

// Courant rend les règles en vigueur, ou nil.
func Courant(jeux []JeuCharge) *JeuCharge {
	for i := range jeux {
		if jeux[i].Courant {
			return &jeux[i]
		}
	}
	return nil
}

// Activation dit quand et par qui une directive est entrée en vigueur : la
// première version des règles où elle est en observation ou active.
type Activation struct {
	Auteur string    `json:"auteur"`
	Date   time.Time `json:"date"`
	Statut string    `json:"statut"` // statut dans la version la plus récente
}

// Activations retrouve, pour chaque directive, l'auteur et la date de son
// activation dans l'historique des règles.
func Activations(jeux []JeuCharge) map[string]Activation {
	out := map[string]Activation{}
	for _, j := range jeux { // triés par version croissante
		for _, d := range j.Jeu.Directives {
			a, vu := out[d.ID]
			if !vu && (d.Statut == directive.Active || d.Statut == directive.Observation) {
				a = Activation{Auteur: j.Jeu.Meta.Auteur, Date: j.Jeu.Meta.DateEffet}
				vu = true
			}
			if vu {
				a.Statut = string(d.Statut)
				out[d.ID] = a
			}
		}
	}
	return out
}

// Origine d'une directive (contrats §6), tenue dans origines.json.
type Origine struct {
	Proposition string `json:"proposition,omitempty"`
	Decision    string `json:"decision,omitempty"` // reprise_modifiee | reprise_telle_quelle
}

// LireOrigines lit demo/directives/origines.json (absent : aucune).
func (d *Depot) LireOrigines() map[string]Origine {
	out := map[string]Origine{}
	if b, err := os.ReadFile(d.Chemins().Origines); err == nil {
		_ = json.Unmarshal(b, &out)
	}
	return out
}

// LibelleOrigine écrit l'origine en français : « écrite par J. Juriste » ou
// « proposée par l'Analyste le …, reprise [telle quelle | modifiée] par … ».
func (d *Depot) LibelleOrigine(id, auteur string, o map[string]Origine) string {
	nom := d.NomIdentite(auteur)
	or, ok := o[id]
	if !ok || or.Proposition == "" {
		return "écrite par " + nom
	}
	reprise := "reprise telle quelle"
	if or.Decision == "reprise_modifiee" {
		reprise = "reprise modifiée"
	}
	prop, err := d.LireProposition(or.Proposition)
	if err != nil {
		return "proposée par l'Analyste, " + reprise + " par " + nom
	}
	return "proposée par l'Analyste le " + prop.Date.In(Paris()).Format("02/01/2006 à 15 h 04") + ", " + reprise + " par " + nom
}

// Paris est le fuseau d'affichage.
func Paris() *time.Location {
	l, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		return time.FixedZone("CET", 3600)
	}
	return l
}
