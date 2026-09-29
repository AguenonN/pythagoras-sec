package journal

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"mcprogue/internal/approval"
	"mcprogue/internal/chaine"
)

func deposer(t *testing.T, c *Coffre, outil string, args map[string]any) string {
	t.Helper()
	emp := approval.Digest(outil, args)
	if err := c.Deposer(Depot{Empreinte: emp, Outil: outil, ArgumentsOrigine: args}); err != nil {
		t.Fatal(err)
	}
	return emp
}

func TestCoffre_PurgeParEmpreinteLaisseLeJournalVerifiable(t *testing.T) {
	dir := t.TempDir()
	chemin := filepath.Join(dir, "coffre.jsonl")
	c, err := OuvrirCoffre(chemin)
	if err != nil {
		t.Fatal(err)
	}
	c.Horloge(func() time.Time { return t0 })
	a := deposer(t, c, "verifier", map[string]any{"siren": "552100554", "nom": "Dupont"})
	b := deposer(t, c, "verifier", map[string]any{"siren": "732829320"})

	// Le journal référence les deux empreintes.
	var buf bytes.Buffer
	cle := []byte(strings.Repeat("k", 32))
	e := Nouveau(chaine.Nouvelle(&buf, cle))
	for i, emp := range []string{a, b} {
		if _, err := e.Ecrire(Entree{Type: AppelT, Seq: i + 1, Outil: "verifier",
			Appel: &Appel{EmpreinteOrigine: emp, EmpreinteTransmise: emp}}); err != nil {
			t.Fatal(err)
		}
	}
	avant := append([]byte(nil), buf.Bytes()...)

	c.Horloge(func() time.Time { return t0.Add(24 * time.Hour) })
	purgees, err := c.PurgerEmpreintes(a, "inconnue")
	if err != nil {
		t.Fatal(err)
	}
	if len(purgees) != 1 || purgees[0] != a {
		t.Fatalf("purgées %v", purgees)
	}
	if again, _ := c.PurgerEmpreintes(a); len(again) != 0 {
		t.Fatal("une empreinte déjà purgée est ignorée")
	}

	if _, ok := c.Arguments(a); ok {
		t.Fatal("les arguments purgés ne doivent plus être rendus")
	}
	if etat, le := c.Etat(a); etat != Purge || !le.Equal(t0.Add(24*time.Hour)) {
		t.Fatalf("état %s %v", etat, le)
	}
	if etat, _ := c.Etat(b); etat != Present {
		t.Fatalf("b doit rester présent: %s", etat)
	}
	if etat, _ := c.Etat("jamais"); etat != Absent {
		t.Fatal()
	}

	// Le journal n'a pas bougé et reste vérifiable ; l'entrée purgée y est.
	if !bytes.Equal(avant, buf.Bytes()) {
		t.Fatal("la purge du coffre ne doit pas toucher le journal")
	}
	if n, err := Verifier(bytes.NewReader(buf.Bytes()), cle); err != nil || n != 2 {
		t.Fatalf("vérification %d %v", n, err)
	}

	// Le fichier ne contient plus la valeur purgée, a des permissions
	// restreintes, et aucun temporaire ne traîne.
	brut, _ := os.ReadFile(chemin)
	if bytes.Contains(brut, []byte("Dupont")) || bytes.Contains(brut, []byte("552100554")) {
		t.Fatalf("une valeur purgée est restée dans le coffre:\n%s", brut)
	}
	if fi, _ := os.Stat(chemin); fi.Mode().Perm() != 0o600 {
		t.Errorf("permissions %v", fi.Mode().Perm())
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 1 {
		t.Errorf("fichiers restants %v", ents)
	}

	// On peut encore déposer après la purge (le fichier a été remplacé).
	d := deposer(t, c, "verifier", map[string]any{"siren": "1"})
	c.Fermer()

	// La pierre tombale survit à la réouverture.
	c, err = OuvrirCoffre(chemin)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Fermer()
	if etat, le := c.Etat(a); etat != Purge || !le.Equal(t0.Add(24*time.Hour)) {
		t.Fatalf("après réouverture: %s %v", etat, le)
	}
	for _, emp := range []string{b, d} {
		if _, ok := c.Arguments(emp); !ok {
			t.Fatalf("%s doit rester rejouable", emp)
		}
	}
	if le, ok := c.Purgee(a); !ok || le.IsZero() {
		t.Fatal("Purgee doit rendre la date")
	}
}

func TestCoffre_PurgeParAnciennete(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "coffre.jsonl")
	c, err := OuvrirCoffre(chemin)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Fermer()
	var vieux, recents []string
	for i := 0; i < 3; i++ {
		c.Horloge(func() time.Time { return t0.Add(time.Duration(i) * time.Hour) })
		vieux = append(vieux, deposer(t, c, "o", map[string]any{"i": float64(i)}))
	}
	c.Horloge(func() time.Time { return t0.Add(48 * time.Hour) })
	recents = append(recents, deposer(t, c, "o", map[string]any{"i": "r"}))

	// Redéposer un vieux dépôt ne le rajeunit pas.
	deposer(t, c, "o", map[string]any{"i": float64(0)})

	purgees, err := c.PurgerAvant(t0.Add(24 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(purgees) != 3 {
		t.Fatalf("purgées %v", purgees)
	}
	for _, emp := range vieux {
		if etat, _ := c.Etat(emp); etat != Purge {
			t.Fatalf("%s: %s", emp, etat)
		}
	}
	if etat, _ := c.Etat(recents[0]); etat != Present {
		t.Fatal("le dépôt récent doit rester")
	}
	// Limite stricte : un dépôt exactement à la limite reste.
	if p, _ := c.PurgerAvant(t0.Add(48 * time.Hour)); len(p) != 0 {
		t.Fatalf("limite stricte: %v", p)
	}

	// Un nouvel appel aux mêmes arguments les redépose : rejouable à nouveau.
	c.Horloge(func() time.Time { return t0.Add(72 * time.Hour) })
	deposer(t, c, "o", map[string]any{"i": float64(1)})
	if etat, _ := c.Etat(vieux[1]); etat != Present {
		t.Fatalf("redépôt après purge: %s", etat)
	}
	c.Fermer()
	c2, err := OuvrirCoffre(chemin)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Fermer()
	if etat, _ := c2.Etat(vieux[1]); etat != Present {
		t.Fatalf("redépôt après purge, relu: %s", etat)
	}
	if etat, _ := c2.Etat(vieux[2]); etat != Purge {
		t.Fatalf("relu: %s", etat)
	}
}

func TestCoffre_PurgeConcurrenteAvecDepots(t *testing.T) {
	c, err := OuvrirCoffre(filepath.Join(t.TempDir(), "coffre.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Fermer()
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				args := map[string]any{"g": float64(g), "i": float64(i)}
				emp := approval.Digest("o", args)
				if err := c.Deposer(Depot{Empreinte: emp, Outil: "o", ArgumentsOrigine: args}); err != nil {
					t.Error(err)
					return
				}
				if i%10 == 0 {
					if _, err := c.PurgerEmpreintes(emp); err != nil {
						t.Error(err)
						return
					}
				}
			}
		}(g)
	}
	wg.Wait()
	presents, purges := 0, 0
	for g := 0; g < 4; g++ {
		for i := 0; i < 50; i++ {
			switch etat, _ := c.Etat(approval.Digest("o", map[string]any{"g": float64(g), "i": float64(i)})); etat {
			case Present:
				presents++
			case Purge:
				purges++
			}
		}
	}
	if presents != 180 || purges != 20 {
		t.Fatalf("présents %d purgés %d", presents, purges)
	}
}

func TestCoffre_NulPurge(t *testing.T) {
	var c *Coffre
	if p, err := c.PurgerAvant(t0); err != nil || p != nil {
		t.Fatal()
	}
	if etat, _ := c.Etat("x"); etat != Absent {
		t.Fatal()
	}
}

// Un abonné lent perd des entrées sans ralentir les autres ni l'engine :
// seules ses pertes sont comptées, et le compte est lisible.
func TestEcrivain_PertesCompteesParAbonneLent(t *testing.T) {
	e := Nouveau(chaine.Nouvelle(&bytes.Buffer{}, nil))
	rapide, stopR := e.Abonner(100)
	defer stopR()
	lent, stopL := e.Abonner(2)
	defer stopL()

	const n = 10
	for i := 0; i < n; i++ {
		if _, err := e.Ecrire(Entree{Type: AlerteT, Seq: i + 1}); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(rapide); got != n {
		t.Fatalf("l'abonné rapide doit tout recevoir: %d", got)
	}
	if e.Perdues() != n-2 {
		t.Fatalf("perdues %d, attendu %d", e.Perdues(), n-2)
	}
	// Le lent a reçu les premières, dans l'ordre : la perte est en queue.
	if a, b := <-lent, <-lent; a.Seq != 1 || b.Seq != 2 {
		t.Fatalf("reçues %d %d", a.Seq, b.Seq)
	}
}

func TestEcrivain_PertesSousConcurrence(t *testing.T) {
	e := Nouveau(chaine.Nouvelle(&bytes.Buffer{}, nil))
	ch, stop := e.Abonner(8)
	recues := 0
	fini := make(chan struct{})
	go func() {
		for range ch {
			recues++
			time.Sleep(50 * time.Microsecond) // lecteur lent
		}
		close(fini)
	}()
	const ecrivains, parEcrivain = 8, 100
	var wg sync.WaitGroup
	for g := 0; g < ecrivains; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < parEcrivain; i++ {
				if _, err := e.Ecrire(Entree{Type: AlerteT}); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	stop()
	<-fini
	if total := int64(recues) + e.Perdues(); total != ecrivains*parEcrivain {
		t.Fatalf("reçues %d + perdues %d != %d", recues, e.Perdues(), ecrivains*parEcrivain)
	}
}
