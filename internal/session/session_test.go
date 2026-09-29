package session

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/internal/chaine"
	"mcprogue/internal/directive"
	"mcprogue/internal/journal"
)

const alice, mallory = "spiffe://lab/agent-a", "spiffe://lab/agent-m"

// tampon est un bytes.Buffer sûr en concurrence : le journal écrit depuis
// les tours pendant que le test relit.
type tampon struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (t *tampon) Write(p []byte) (int, error) { t.mu.Lock(); defer t.mu.Unlock(); return t.b.Write(p) }
func (t *tampon) copie() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]byte(nil), t.b.Bytes()...)
}

type banc struct {
	g   *Gestionnaire
	buf *tampon
	mu  sync.Mutex
	now time.Time
}

func nouveauBanc(t *testing.T, cfg Config) *banc {
	t.Helper()
	b := &banc{buf: &tampon{}, now: time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC)}
	cfg.Horloge = b.horloge
	n := 0
	var mu sync.Mutex
	cfg.NouvelID = func() (string, error) { mu.Lock(); defer mu.Unlock(); n++; return fmt.Sprintf("s%d", n), nil }
	b.g = Nouveau(cfg, journal.Nouveau(chaine.Nouvelle(b.buf, nil)))
	return b
}

func (b *banc) horloge() time.Time      { b.mu.Lock(); defer b.mu.Unlock(); return b.now }
func (b *banc) avancer(d time.Duration) { b.mu.Lock(); b.now = b.now.Add(d); b.mu.Unlock() }
func (b *banc) entrees(t *testing.T) []journal.Entree {
	t.Helper()
	es, err := journal.Lire(bytes.NewReader(b.buf.copie()))
	if err != nil {
		t.Fatal(err)
	}
	return es
}

func ok(ctx context.Context) Retour {
	return Retour{Resultat: directive.ResultatOK, EmpreinteReponse: "r"}
}

func appeler(t *testing.T, g *Gestionnaire, ss *mcp.ServerSession, appelant string, transmettre bool) (*Tour, error) {
	t.Helper()
	tour, err := g.Entrer(context.Background(), ss, appelant)
	if err != nil {
		return nil, err
	}
	defer tour.Terminer()
	if transmettre {
		tour.Transmettre(context.Background(), "taux", "get", "e", ok)
	}
	return tour, nil
}

func TestSession_OuvertureEtatEtFermeture(t *testing.T) {
	b := nouveauBanc(t, Config{})
	ss := new(mcp.ServerSession)

	t1, err := appeler(t, b.g, ss, alice, true)
	if err != nil {
		t.Fatal(err)
	}
	// Un refus de l'engine : tour sans transmission, rang consommé,
	// aucune consultation.
	t2, _ := appeler(t, b.g, ss, alice, false)
	t3, _ := b.g.Entrer(context.Background(), ss, alice)
	etat := t3.Etat()
	t3.Terminer()
	t3.Terminer() // idempotent

	if t1.Seq() != 1 || t2.Seq() != 2 || t3.Seq() != 3 {
		t.Fatalf("seq %d %d %d", t1.Seq(), t2.Seq(), t3.Seq())
	}
	if etat.ID != "s1" || etat.Appelant != alice || len(etat.Consultations) != 1 {
		t.Fatalf("état %+v", etat)
	}
	if c := etat.Consultations[0]; c.Seq != 1 || c.Serveur != "taux" || c.Resultat != directive.ResultatOK || c.EmpreinteReponse != "r" {
		t.Fatalf("consultation %+v", c)
	}
	// L'état rendu est une copie : Decide ne peut pas modifier la session.
	etat.Consultations[0].Resultat = directive.ResultatErreur
	if b.g.Session(ss).Consultations()[0].Resultat != directive.ResultatOK {
		t.Fatal("l'instantané doit être une copie")
	}

	b.g.Fermer(ss, MotifClient)
	b.g.Fermer(ss, MotifClient) // sans effet
	es := b.entrees(t)
	if len(es) != 2 || es[0].Type != journal.SessionOuverte || es[1].Type != journal.SessionFermee {
		t.Fatalf("journal %+v", es)
	}
	if es[0].Session != "s1" || es[0].Appelant != alice {
		t.Fatalf("ouverture %+v", es[0])
	}
	if f := es[1]; f.NbConsultations != 1 || f.DernierSeq != 3 || f.Message != MotifClient {
		t.Fatalf("fermeture %+v", f)
	}
	if b.g.Session(ss) != nil {
		t.Fatal("la session fermée par le client est oubliée")
	}
}

func TestSession_ChangementIdentiteRefuseEtJournalise(t *testing.T) {
	b := nouveauBanc(t, Config{})
	ss := new(mcp.ServerSession)
	if _, err := appeler(t, b.g, ss, alice, true); err != nil {
		t.Fatal(err)
	}
	if _, err := b.g.Entrer(context.Background(), ss, mallory); !errors.Is(err, ErrIdentiteChangee) {
		t.Fatalf("attendu ErrIdentiteChangee, reçu %v", err)
	}
	// Un appelant non prouvé ne l'est pas davantage.
	if _, err := b.g.Entrer(context.Background(), ss, ""); !errors.Is(err, ErrIdentiteChangee) {
		t.Fatalf("appelant vide: %v", err)
	}
	// La session est intacte et continue pour son appelant.
	tour, err := appeler(t, b.g, ss, alice, false)
	if err != nil || tour.Seq() != 2 {
		t.Fatalf("après tentative: %v %v", err, tour)
	}
	if s := b.g.Session(ss); s.Appelant != alice || len(s.Consultations()) != 1 {
		t.Fatalf("session %+v", s)
	}
	var changes []journal.Entree
	for _, e := range b.entrees(t) {
		if e.Type == journal.IdentiteChangee {
			changes = append(changes, e)
		}
	}
	if len(changes) != 2 || changes[0].Session != "s1" || changes[0].Appelant != mallory || changes[0].Message == "" {
		t.Fatalf("identite_changee %+v", changes)
	}
}

func TestSession_BackendMuet(t *testing.T) {
	b := nouveauBanc(t, Config{DelaiBackend: 50 * time.Millisecond})
	ss := new(mcp.ServerSession)
	libere := make(chan struct{})
	defer close(libere)
	muet := func(ctx context.Context) Retour { <-libere; return Retour{Resultat: directive.ResultatOK} } // ignore ctx

	tour, err := b.g.Entrer(context.Background(), ss, alice)
	if err != nil {
		t.Fatal(err)
	}
	debut := time.Now()
	c, r := tour.Transmettre(context.Background(), "lent", "get", "e", muet)
	tour.Terminer()
	if d := time.Since(debut); d > 2*time.Second {
		t.Fatalf("Transmettre a attendu %v", d)
	}
	if c.Resultat != directive.ResultatIndisponible || r.Err == nil || c.EmpreinteReponse != "" {
		t.Fatalf("consultation %+v retour %+v", c, r)
	}

	// Le verrou est libéré : l'appel suivant entre aussitôt et voit la
	// consultation indisponible (critère « transmis »).
	ctx, annuler := context.WithTimeout(context.Background(), time.Second)
	defer annuler()
	t2, err := b.g.Entrer(ctx, ss, alice)
	if err != nil {
		t.Fatalf("verrou non libéré: %v", err)
	}
	if cs := t2.Etat().Consultations; len(cs) != 1 || cs[0].Resultat != directive.ResultatIndisponible {
		t.Fatalf("état %+v", cs)
	}
	// Une seconde transmission dans le même tour n'appelle rien.
	t2.Transmettre(context.Background(), "a", "b", "c", ok)
	if _, r := t2.Transmettre(context.Background(), "a", "b", "c", ok); !errors.Is(r.Err, ErrDejaTransmis) {
		t.Fatalf("double transmission: %v", r.Err)
	}
	t2.Terminer()
	if n := len(b.g.Session(ss).Consultations()); n != 2 {
		t.Fatalf("%d consultations", n)
	}
}

func TestSession_AttenteDuVerrouAnnulable(t *testing.T) {
	b := nouveauBanc(t, Config{})
	ss := new(mcp.ServerSession)
	tour, _ := b.g.Entrer(context.Background(), ss, alice)
	defer tour.Terminer()
	ctx, annuler := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer annuler()
	if _, err := b.g.Entrer(ctx, ss, alice); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("attendu l'expiration du contexte, reçu %v", err)
	}
}

func TestSession_ConcurrenceSerialiseeParSession(t *testing.T) {
	b := nouveauBanc(t, Config{})
	sessions := []*mcp.ServerSession{new(mcp.ServerSession), new(mcp.ServerSession), new(mcp.ServerSession)}
	var dedans [3]atomic.Int32
	var chevauchement atomic.Bool
	var maxGlobal, courant atomic.Int32

	const parSession = 40
	var wg sync.WaitGroup
	for i, ss := range sessions {
		for k := 0; k < parSession; k++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				tour, err := b.g.Entrer(context.Background(), ss, alice)
				if err != nil {
					t.Error(err)
					return
				}
				defer tour.Terminer()
				if k%4 == 0 {
					return // refus : pas de consultation
				}
				tour.Transmettre(context.Background(), "s", "o", "e", func(ctx context.Context) Retour {
					if dedans[i].Add(1) > 1 {
						chevauchement.Store(true)
					}
					n := courant.Add(1)
					for {
						m := maxGlobal.Load()
						if n <= m || maxGlobal.CompareAndSwap(m, n) {
							break
						}
					}
					time.Sleep(time.Millisecond)
					courant.Add(-1)
					dedans[i].Add(-1)
					return ok(ctx)
				})
			}()
		}
	}
	wg.Wait()
	if chevauchement.Load() {
		t.Fatal("deux appels d'une même session ont été au backend en même temps")
	}
	if maxGlobal.Load() < 2 {
		t.Log("aucun parallélisme observé entre sessions (machine chargée ?)")
	}
	for _, ss := range sessions {
		cs := b.g.Session(ss).Consultations()
		if len(cs) != parSession*3/4 {
			t.Fatalf("%d consultations, attendu %d", len(cs), parSession*3/4)
		}
		for j := 1; j < len(cs); j++ {
			if cs[j].Seq <= cs[j-1].Seq {
				t.Fatalf("rangs non croissants: %d puis %d", cs[j-1].Seq, cs[j].Seq)
			}
		}
	}
	// Trois sessions, trois identifiants distincts, trois ouvertures.
	ids := map[string]bool{}
	for _, e := range b.entrees(t) {
		if e.Type == journal.SessionOuverte {
			ids[e.Session] = true
		}
	}
	if len(ids) != 3 {
		t.Fatalf("ouvertures %v", ids)
	}
}

func TestSession_UneSessionBloqueeNeBloquePasLesAutres(t *testing.T) {
	b := nouveauBanc(t, Config{DelaiBackend: time.Minute})
	a, autre := new(mcp.ServerSession), new(mcp.ServerSession)
	libere := make(chan struct{})
	entre := make(chan struct{})
	go func() {
		tour, _ := b.g.Entrer(context.Background(), a, alice)
		defer tour.Terminer()
		tour.Transmettre(context.Background(), "s", "o", "e", func(ctx context.Context) Retour {
			close(entre)
			<-libere
			return ok(ctx)
		})
	}()
	<-entre
	ctx, annuler := context.WithTimeout(context.Background(), time.Second)
	defer annuler()
	tour, err := b.g.Entrer(ctx, autre, alice)
	if err != nil {
		t.Fatalf("une autre session doit avancer: %v", err)
	}
	tour.Terminer()
	close(libere)
}

func TestSession_BorneConsultations(t *testing.T) {
	var fermees []string
	var mu sync.Mutex
	b := nouveauBanc(t, Config{MaxConsultations: 2, AFermeture: func(_ *mcp.ServerSession, motif string) {
		mu.Lock()
		fermees = append(fermees, motif)
		mu.Unlock()
	}})
	ss := new(mcp.ServerSession)
	appeler(t, b.g, ss, alice, true)
	appeler(t, b.g, ss, alice, false) // un refus ne compte pas
	appeler(t, b.g, ss, alice, true)  // deuxième consultation : fermeture en fin de tour
	if _, err := b.g.Entrer(context.Background(), ss, alice); !errors.Is(err, ErrFermee) {
		t.Fatalf("attendu ErrFermee, reçu %v", err)
	}
	es := b.entrees(t)
	f := es[len(es)-1]
	if f.Type != journal.SessionFermee || f.NbConsultations != 2 || f.DernierSeq != 3 || f.Message != MotifConsultationMax {
		t.Fatalf("fermeture %+v", f)
	}
	if len(fermees) != 1 || fermees[0] != MotifConsultationMax {
		t.Fatalf("AFermeture %v", fermees)
	}
	// Le départ du client ensuite n'écrit pas une seconde fermeture.
	b.g.Fermer(ss, MotifClient)
	if n := len(b.entrees(t)); n != len(es) {
		t.Fatalf("seconde fermeture écrite (%d entrées)", n)
	}
}

func TestSession_AgeMax(t *testing.T) {
	b := nouveauBanc(t, Config{AgeMax: time.Hour})
	ss, autre := new(mcp.ServerSession), new(mcp.ServerSession)
	appeler(t, b.g, ss, alice, true)
	appeler(t, b.g, autre, alice, false)

	// À l'entrée : la session trop vieille est fermée et l'appel refusé.
	b.avancer(time.Hour)
	if _, err := b.g.Entrer(context.Background(), ss, alice); !errors.Is(err, ErrFermee) {
		t.Fatalf("attendu ErrFermee, reçu %v", err)
	}
	// Par balayage : l'autre session, jamais rappelée, est fermée aussi.
	if n := b.g.Balayer(); n != 1 {
		t.Fatalf("balayées %d", n)
	}
	var motifs []string
	for _, e := range b.entrees(t) {
		if e.Type == journal.SessionFermee {
			motifs = append(motifs, e.Session+":"+e.Message)
		}
	}
	if len(motifs) != 2 || motifs[0] != "s1:"+MotifAgeMax || motifs[1] != "s2:"+MotifAgeMax {
		t.Fatalf("fermetures %v", motifs)
	}
}

func TestSession_BalayageAttendLaFinDuTour(t *testing.T) {
	b := nouveauBanc(t, Config{AgeMax: time.Hour, DelaiBackend: time.Minute})
	ss := new(mcp.ServerSession)
	tour, _ := b.g.Entrer(context.Background(), ss, alice)
	b.avancer(2 * time.Hour)
	if n := b.g.Balayer(); n != 1 {
		t.Fatal("la session doit être marquée")
	}
	// Le tour en cours termine sa consultation ; la fermeture la compte.
	tour.Transmettre(context.Background(), "s", "o", "e", ok)
	tour.Terminer()
	es := b.entrees(t)
	f := es[len(es)-1]
	if f.Type != journal.SessionFermee || f.NbConsultations != 1 || f.Message != MotifAgeMax {
		t.Fatalf("fermeture %+v", f)
	}
}

// Avec une vraie ServerSession du go-sdk : le départ du client ferme la
// session dans le journal.
func TestSession_SuivreUneVraieSession(t *testing.T) {
	b := nouveauBanc(t, Config{})
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	serveur := mcp.NewServer(&mcp.Implementation{Name: "engine", Version: "t"}, nil)
	ss, err := serveur.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "t"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	b.g.Suivre(ss)
	appeler(t, b.g, ss, alice, true)
	cs.Close()

	fin := time.After(5 * time.Second)
	var es []journal.Entree
	for {
		es = b.entrees(t)
		if es[len(es)-1].Type == journal.SessionFermee {
			break
		}
		select {
		case <-fin:
			t.Fatal("la session n'a pas été fermée au départ du client")
		case <-time.After(5 * time.Millisecond):
		}
	}
	if b.g.Session(ss) != nil {
		t.Fatal("la session doit être oubliée")
	}
	if f := es[len(es)-1]; f.Type != journal.SessionFermee || f.Message != MotifClient || f.NbConsultations != 1 {
		t.Fatalf("fermeture %+v", f)
	}
}

// Aucun chemin atteignable par un appel ne panique : chaque cas rend une
// erreur typée.
func TestSession_AucunePanique(t *testing.T) {
	ctx := context.Background()
	var gNul *Gestionnaire
	if _, err := gNul.Entrer(ctx, new(mcp.ServerSession), alice); !errors.Is(err, ErrGestionnaireNul) {
		t.Fatal(err)
	}
	b := nouveauBanc(t, Config{})
	if _, err := b.g.Entrer(ctx, nil, alice); !errors.Is(err, ErrSessionNulle) {
		t.Fatal(err)
	}
	b.g.Suivre(nil)

	// Tour nul : l'engine a différé Terminer avant de tester l'erreur.
	var nul *Tour
	nul.Terminer()
	if nul.Seq() != 0 || nul.SessionID() != "" || len(nul.Etat().Consultations) != 0 {
		t.Fatal()
	}
	if _, r := nul.Transmettre(ctx, "s", "o", "e", ok); !errors.Is(r.Err, ErrTourNul) {
		t.Fatal(r.Err)
	}

	ss := new(mcp.ServerSession)
	tour, _ := b.g.Entrer(ctx, ss, alice)
	if _, r := tour.Transmettre(ctx, "s", "o", "e", nil); !errors.Is(r.Err, ErrBackendNul) {
		t.Fatal(r.Err)
	}
	if n := len(b.g.Session(ss).Consultations()); n != 0 {
		t.Fatal("un backend nul ne consulte rien")
	}
	tour.Terminer()

	tour, _ = b.g.Entrer(ctx, ss, alice)
	c, r := tour.Transmettre(ctx, "s", "o", "e", func(context.Context) Retour { panic("boum") })
	tour.Terminer()
	if !errors.Is(r.Err, ErrBackendPanique) || c.Resultat != directive.ResultatErreur {
		t.Fatalf("panique du backend: %+v %v", c, r.Err)
	}

	// Identifiant impossible : la session n'est pas ouverte.
	g := Nouveau(Config{NouvelID: func() (string, error) { return "", errors.New("aléa") }}, nil)
	if _, err := g.Entrer(ctx, new(mcp.ServerSession), alice); !errors.Is(err, ErrIdentifiant) {
		t.Fatal(err)
	}
}
