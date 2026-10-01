// Package console sert les écrans de démonstration : des vues en lecture
// seule construites depuis les fichiers (journal JSONL chaîné, coffre, jeux
// de directives). Rien n'est tenu en mémoire d'une requête à l'autre : chaque
// affichage relit le journal et en vérifie la chaîne.
package console

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"mcprogue/internal/directive"
	"mcprogue/internal/journal"
)

// Depot désigne les fichiers que la console lit. Il ne les écrit jamais.
type Depot struct {
	// Donnees est un répertoire de journaux : chaque sous-répertoire porte
	// journal.jsonl (et coffre.jsonl si présent), et les fichiers *.jsonl à
	// la racine sont des journaux sans coffre. Un chemin vers un fichier
	// est aussi accepté.
	Donnees string
	// Jeux sont des répertoires ou fichiers de jeux de directives (JSON).
	Jeux []string
	// Cle est la clé HMAC des journaux. Vide : la chaîne ne peut pas être
	// vérifiée et l'écran le dit en rouge.
	Cle []byte
}

// Journal est un journal trouvé sur disque.
type Journal struct {
	ID     string `json:"id"`
	Chemin string `json:"-"`
	Coffre string `json:"-"`
}

// Verification est le résultat de la vérification de chaîne, affiché à
// chaque vue.
type Verification struct {
	OK bool `json:"ok"`
	// Lignes est le nombre de lignes dont la chaîne est valide.
	Lignes int `json:"lignes"`
	// Total est le nombre de lignes du journal.
	Total int `json:"total"`
	// Erreur est le message de la chaîne (vide si OK).
	Erreur string `json:"erreur,omitempty"`
	// CleAbsente : aucune clé fournie, rien n'a pu être vérifié.
	CleAbsente bool `json:"cle_absente,omitempty"`
}

// Etat est un journal relu.
type Etat struct {
	Journal Journal
	Entrees []journal.Entree
	Verif   Verification
	Coffre  *coffreLecture
}

// Journaux liste les journaux, triés par identifiant.
func (d *Depot) Journaux() []Journal {
	var out []Journal
	info, err := os.Stat(d.Donnees)
	if err != nil {
		return nil
	}
	if !info.IsDir() {
		base := strings.TrimSuffix(filepath.Base(d.Donnees), ".jsonl")
		return []Journal{{ID: base, Chemin: d.Donnees, Coffre: coffreVoisin(d.Donnees)}}
	}
	if _, err := os.Stat(filepath.Join(d.Donnees, "journal.jsonl")); err == nil {
		out = append(out, Journal{ID: filepath.Base(d.Donnees), Chemin: filepath.Join(d.Donnees, "journal.jsonl"),
			Coffre: filepath.Join(d.Donnees, "coffre.jsonl")})
	}
	ents, _ := os.ReadDir(d.Donnees)
	for _, e := range ents {
		p := filepath.Join(d.Donnees, e.Name())
		switch {
		case e.IsDir():
			j := filepath.Join(p, "journal.jsonl")
			if _, err := os.Stat(j); err == nil {
				out = append(out, Journal{ID: e.Name(), Chemin: j, Coffre: filepath.Join(p, "coffre.jsonl")})
			}
		case strings.HasSuffix(e.Name(), ".jsonl") && !strings.HasPrefix(e.Name(), "coffre") && e.Name() != "journal.jsonl":
			out = append(out, Journal{ID: strings.TrimSuffix(e.Name(), ".jsonl"), Chemin: p, Coffre: coffreVoisin(p)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func coffreVoisin(chemin string) string {
	return filepath.Join(filepath.Dir(chemin), "coffre.jsonl")
}

// Journal retrouve un journal par identifiant. L'identifiant n'est jamais
// utilisé pour construire un chemin : il ne sert qu'à choisir dans la liste.
func (d *Depot) Journal(id string) (Journal, bool) {
	for _, j := range d.Journaux() {
		if j.ID == id {
			return j, true
		}
	}
	return Journal{}, false
}

var reLigne = regexp.MustCompile(`ligne (\d+)`)

// Lire relit le journal, vérifie sa chaîne et ouvre le coffre en lecture.
// Une dernière ligne sans saut de ligne est en cours d'écriture par le gate :
// elle est ignorée, ni lue ni comptée en écart.
func (d *Depot) Lire(j Journal) (*Etat, error) {
	brut, err := os.ReadFile(j.Chemin)
	if err != nil {
		return nil, err
	}
	if i := bytes.LastIndexByte(brut, '\n'); i >= 0 {
		brut = brut[:i+1]
	} else {
		brut = nil
	}
	st := &Etat{Journal: j}
	st.Verif.Total = bytes.Count(brut, []byte{'\n'})
	entrees, errLire := journal.Lire(bytes.NewReader(brut))
	st.Entrees = entrees
	switch {
	case len(d.Cle) == 0:
		st.Verif.CleAbsente = true
		st.Verif.Erreur = "clé du journal absente : la chaîne ne peut pas être vérifiée"
	default:
		n, err := journal.Verifier(bytes.NewReader(brut), d.Cle)
		st.Verif.Lignes = n
		if err != nil {
			st.Verif.Erreur = err.Error()
		} else {
			st.Verif.OK = true
		}
	}
	if errLire != nil {
		st.Verif.OK = false
		if st.Verif.Erreur == "" {
			st.Verif.Erreur = errLire.Error()
		}
	}
	st.Coffre = ouvrirCoffreLecture(j.Coffre)
	return st, nil
}

// PremierEcart rend l'indice (base 0) de la première entrée dont la chaîne
// n'est pas vérifiée, ou -1.
func (v Verification) PremierEcart() int {
	if v.OK {
		return -1
	}
	if m := reLigne.FindStringSubmatch(v.Erreur); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n >= 1 {
			return n - 1
		}
	}
	return v.Lignes
}

// ---------------------------------------------------------------------------
// Coffre en lecture seule : les arguments ne sont jamais affichés, ils ne
// servent qu'au rejeu.
// ---------------------------------------------------------------------------

type coffreLecture struct {
	index    map[string]map[string]any
	transmis map[string]map[string]any
	purgees  map[string]time.Time
}

func ouvrirCoffreLecture(chemin string) *coffreLecture {
	c := &coffreLecture{index: map[string]map[string]any{}, transmis: map[string]map[string]any{}, purgees: map[string]time.Time{}}
	f, err := os.Open(chemin)
	if err != nil {
		return c
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 16<<20)
	for sc.Scan() {
		ligne := bytes.TrimSpace(sc.Bytes())
		if len(ligne) == 0 {
			continue
		}
		var dep journal.Depot
		if json.Unmarshal(ligne, &dep) != nil {
			continue // ligne en cours d'écriture
		}
		if dep.PurgeLe != nil {
			delete(c.index, dep.Empreinte)
			delete(c.transmis, dep.Empreinte)
			c.purgees[dep.Empreinte] = *dep.PurgeLe
			continue
		}
		delete(c.purgees, dep.Empreinte)
		c.index[dep.Empreinte] = dep.ArgumentsOrigine
		if dep.ArgumentsTransmis != nil {
			c.transmis[dep.Empreinte] = dep.ArgumentsTransmis
		}
	}
	return c
}

func (c *coffreLecture) Arguments(e string) (map[string]any, bool) {
	a, ok := c.index[e]
	return a, ok
}

func (c *coffreLecture) Purgee(e string) (time.Time, bool) {
	t, ok := c.purgees[e]
	return t, ok
}

// ---------------------------------------------------------------------------
// Jeux de directives
// ---------------------------------------------------------------------------

// JeuCharge est un jeu lisible, avec sa provenance.
type JeuCharge struct {
	Jeu    directive.Jeu
	Source string
	Brut   []byte
}

// Libelle est le nom d'un jeu à l'écran.
func (j JeuCharge) Libelle() string {
	return fmt.Sprintf("%s, version %d", j.Jeu.Meta.ID, j.Jeu.Meta.Version)
}

// ChargerJeux lit les jeux des chemins donnés, puis les jeux intégrés qui
// ne sont pas déjà présents (même identifiant, version et empreinte). Un
// fichier refusé par le chargeur strict est listé dans les refus, jamais
// utilisé.
func (d *Depot) ChargerJeux() (jeux []JeuCharge, refus []string) {
	vus := map[string]bool{}
	ajouter := func(nom string, brut []byte) {
		j, err := directive.Charger(brut)
		if err != nil {
			refus = append(refus, fmt.Sprintf("%s : %v", nom, err))
			return
		}
		cle := fmt.Sprintf("%s|%d|%s", j.Meta.ID, j.Meta.Version, j.Meta.Empreinte)
		if vus[cle] {
			return
		}
		vus[cle] = true
		jeux = append(jeux, JeuCharge{Jeu: *j, Source: nom, Brut: brut})
	}
	for _, chemin := range d.Jeux {
		info, err := os.Stat(chemin)
		if err != nil {
			continue
		}
		var fichiers []string
		if info.IsDir() {
			ents, _ := os.ReadDir(chemin)
			for _, e := range ents {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
					fichiers = append(fichiers, filepath.Join(chemin, e.Name()))
				}
			}
		} else {
			fichiers = []string{chemin}
		}
		for _, f := range fichiers {
			if brut, err := os.ReadFile(f); err == nil {
				ajouter(filepath.Base(f), brut)
			}
		}
	}
	for _, nom := range nomsExemples() {
		if brut, err := exemples.ReadFile("exemples/" + nom); err == nil {
			ajouter("intégré : "+nom, brut)
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
