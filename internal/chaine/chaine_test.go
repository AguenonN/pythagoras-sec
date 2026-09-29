package chaine

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type entree struct {
	Type   string  `json:"type"`
	N      int     `json:"n"`
	Grand  int64   `json:"grand,omitempty"`
	Prix   float64 `json:"prix,omitempty"`
	Prev   string  `json:"prev,omitempty"`
	MAC    string  `json:"mac,omitempty"`
	Texte  string  `json:"texte,omitempty"`
	Objets []any   `json:"objets,omitempty"`
}

func TestChaine_EcritEtVerifie(t *testing.T) {
	cle := []byte(strings.Repeat("k", 32))
	var buf bytes.Buffer
	c := Nouvelle(&buf, cle)
	for i := 1; i <= 3; i++ {
		ligne, err := c.Ajouter(entree{Type: "t", N: i, Grand: 1 << 60, Prix: 0.1, Texte: "é\"/\\", Objets: []any{map[string]any{"b": 1, "a": 2}}})
		if err != nil {
			t.Fatal(err)
		}
		var e entree
		if err := json.Unmarshal(ligne, &e); err != nil || e.MAC == "" || e.Prev == "" {
			t.Fatalf("ligne rendue illisible: %s (%v)", ligne, err)
		}
	}
	n, err := Verifier(bytes.NewReader(buf.Bytes()), cle)
	if err != nil || n != 3 {
		t.Fatalf("vérification: %d, %v", n, err)
	}
	if _, err := Verifier(bytes.NewReader(buf.Bytes()), []byte("autre-cle-autre-cle-autre-cle-xx")); err == nil {
		t.Fatal("une autre clé doit faire échouer la vérification")
	}
	prev, err := DernierMAC(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	var derniere entree
	lignes := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	_ = json.Unmarshal(lignes[2], &derniere)
	if prev != derniere.MAC {
		t.Fatalf("DernierMAC %s, dernière ligne %s", prev, derniere.MAC)
	}
}

func TestChaine_DetecteAlterationSuppressionReordonnancement(t *testing.T) {
	var buf bytes.Buffer
	c := Nouvelle(&buf, nil)
	for i := 1; i <= 4; i++ {
		if _, err := c.Ajouter(entree{Type: "t", N: i}); err != nil {
			t.Fatal(err)
		}
	}
	lignes := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	cas := map[string][][]byte{
		"altération":       {lignes[0], bytes.Replace(lignes[1], []byte(`"n":2`), []byte(`"n":20`), 1), lignes[2], lignes[3]},
		"suppression":      {lignes[0], lignes[2], lignes[3]},
		"réordonnancement": {lignes[1], lignes[0], lignes[2], lignes[3]},
		"troncature":       {lignes[0], lignes[1][:len(lignes[1])/2]},
	}
	for nom, ls := range cas {
		t.Run(nom, func(t *testing.T) {
			if _, err := Verifier(bytes.NewReader(bytes.Join(ls, []byte("\n"))), nil); err == nil {
				t.Fatal("altération non détectée")
			}
		})
	}
	// La chaîne intacte, elle, passe, y compris avec des lignes vides.
	intact := bytes.Join([][]byte{lignes[0], {}, lignes[1], lignes[2], lignes[3]}, []byte("\n"))
	if n, err := Verifier(bytes.NewReader(intact), nil); err != nil || n != 4 {
		t.Fatalf("chaîne intacte refusée: %d %v", n, err)
	}
}

func TestChaine_OuvrirReprend(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "j.jsonl")
	cle := []byte(strings.Repeat("c", 32))
	c, f, err := Ouvrir(chemin, cle)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Ajouter(entree{Type: "a", N: 1}); err != nil {
		t.Fatal(err)
	}
	f.Close()
	// Réouverture : la seconde ligne doit s'accrocher à la première.
	c, f, err = Ouvrir(chemin, cle)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Ajouter(entree{Type: "a", N: 2}); err != nil {
		t.Fatal(err)
	}
	f.Close()
	b, _ := os.ReadFile(chemin)
	if n, err := Verifier(bytes.NewReader(b), cle); err != nil || n != 2 {
		t.Fatalf("après reprise: %d %v\n%s", n, err, b)
	}
	// Un fichier tronqué n'est pas prolongé.
	if err := os.WriteFile(chemin, b[:len(b)-10], 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Ouvrir(chemin, cle); err == nil {
		t.Fatal("un fichier tronqué doit refuser la reprise")
	}
}

func TestChaine_RefuseNonObjet(t *testing.T) {
	c := Nouvelle(&bytes.Buffer{}, nil)
	if _, err := c.Ajouter([]byte(`[1,2]`)); err == nil {
		t.Fatal("un tableau n'est pas une entrée")
	}
	if _, err := c.Ajouter([]byte(`null`)); err == nil {
		t.Fatal("null n'est pas une entrée")
	}
	var nulle *Chaine
	if ligne, err := nulle.Ajouter(entree{}); err != nil || ligne != nil {
		t.Fatal("une chaîne nulle n'écrit rien et ne se plaint pas")
	}
}

func TestChaine_Concurrence(t *testing.T) {
	var buf bytes.Buffer
	c := Nouvelle(&buf, []byte(strings.Repeat("z", 32)))
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := c.Ajouter(entree{Type: "p", N: i}); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if n, err := Verifier(bytes.NewReader(buf.Bytes()), []byte(strings.Repeat("z", 32))); err != nil || n != 50 {
		t.Fatalf("%d %v", n, err)
	}
}
