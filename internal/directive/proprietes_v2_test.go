package directive

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Test 4 : table de vérité de la logique à trois valeurs, cas par cas.
// ---------------------------------------------------------------------------

// Feuilles qui valent V, F et I sur les arguments {"a": 1}.
var feuilleDe = map[Trivalent]*Noeud{
	Vrai:        {Argument: &Condition{Champ: "/a", Op: OpPresent}},
	Faux:        {Argument: &Condition{Champ: "/a", Op: OpAbsent}},
	Indetermine: {Argument: &Condition{Champ: "/manquant", Op: OpSup, Valeur: json.Number("1")}},
}

var argsKleene = map[string]any{"a": json.Number("1")}

func evalue(t *testing.T, nd *Noeud) Trivalent {
	t.Helper()
	c := contexte{etat: &EtatSession{}, appel: &Appel{}}
	r, _ := c.noeud(nd, argsKleene)
	return r
}

func TestKleene_TableDeVerite(t *testing.T) {
	V, I, F := Vrai, Indetermine, Faux
	for v, nd := range feuilleDe {
		if got := evalue(t, nd); got != v {
			t.Fatalf("feuille %v vaut %v", v, got)
		}
	}
	non := []struct{ a, veut Trivalent }{{V, F}, {I, I}, {F, V}}
	for _, c := range non {
		if got := Non(c.a); got != c.veut {
			t.Errorf("Non(%v) = %v, attendu %v", c.a, got, c.veut)
		}
		if got := evalue(t, &Noeud{Non: feuilleDe[c.a]}); got != c.veut {
			t.Errorf("arbre non(%v) = %v, attendu %v", c.a, got, c.veut)
		}
	}
	// Les neuf cas de chaque table, dans l'ordre de docs/schema-directive-v2.md §7.1.
	et := []struct{ a, b, veut Trivalent }{
		{V, V, V}, {V, I, I}, {V, F, F},
		{I, V, I}, {I, I, I}, {I, F, F},
		{F, V, F}, {F, I, F}, {F, F, F},
	}
	ou := []struct{ a, b, veut Trivalent }{
		{V, V, V}, {V, I, V}, {V, F, V},
		{I, V, V}, {I, I, I}, {I, F, I},
		{F, V, V}, {F, I, I}, {F, F, F},
	}
	for _, c := range et {
		if got := Et(c.a, c.b); got != c.veut {
			t.Errorf("Et(%v, %v) = %v, attendu %v", c.a, c.b, got, c.veut)
		}
		if got := evalue(t, &Noeud{Et: []*Noeud{feuilleDe[c.a], feuilleDe[c.b]}}); got != c.veut {
			t.Errorf("arbre et(%v, %v) = %v, attendu %v", c.a, c.b, got, c.veut)
		}
	}
	for _, c := range ou {
		if got := Ou(c.a, c.b); got != c.veut {
			t.Errorf("Ou(%v, %v) = %v, attendu %v", c.a, c.b, got, c.veut)
		}
		if got := evalue(t, &Noeud{Ou: []*Noeud{feuilleDe[c.a], feuilleDe[c.b]}}); got != c.veut {
			t.Errorf("arbre ou(%v, %v) = %v, attendu %v", c.a, c.b, got, c.veut)
		}
	}
	// n-aire : repliement.
	if got := evalue(t, &Noeud{Et: []*Noeud{feuilleDe[V], feuilleDe[I], feuilleDe[V]}}); got != I {
		t.Errorf("et(V, I, V) = %v", got)
	}
	if got := evalue(t, &Noeud{Ou: []*Noeud{feuilleDe[F], feuilleDe[I], feuilleDe[V]}}); got != V {
		t.Errorf("ou(F, I, V) = %v", got)
	}
}

func TestKleene_Quantificateurs(t *testing.T) {
	c := contexte{etat: &EtatSession{}, appel: &Appel{}}
	elt := &Noeud{Argument: &Condition{Champ: "", Op: OpSup, Valeur: json.Number("10")}}
	cas := []struct {
		tab          any
		existe, tous Trivalent
	}{
		{[]any{}, Faux, Vrai},
		{[]any{json.Number("20")}, Vrai, Vrai},
		{[]any{json.Number("1")}, Faux, Faux},
		{[]any{json.Number("1"), json.Number("20")}, Vrai, Faux},
		{[]any{"x"}, Indetermine, Indetermine},
		{[]any{"x", json.Number("20")}, Vrai, Indetermine},
		{[]any{"x", json.Number("1")}, Indetermine, Faux},
		{"pas un tableau", Indetermine, Indetermine},
	}
	for _, k := range cas {
		args := map[string]any{"l": k.tab}
		if r, _ := c.noeud(&Noeud{Existe: &Quantificateur{Champ: "/l", Condition: elt}}, args); r != k.existe {
			t.Errorf("existe sur %v = %v, attendu %v", k.tab, r, k.existe)
		}
		if r, _ := c.noeud(&Noeud{Tous: &Quantificateur{Champ: "/l", Condition: elt}}, args); r != k.tous {
			t.Errorf("tous sur %v = %v, attendu %v", k.tab, r, k.tous)
		}
	}
	if r, _ := c.noeud(&Noeud{Existe: &Quantificateur{Champ: "/absent", Condition: elt}}, map[string]any{}); r != Indetermine {
		t.Errorf("champ absent : %v", r)
	}
}

// si_indetermine ne s'applique qu'à la racine : un sous-arbre indéterminé
// que la logique rend déterminé ne refuse rien.
func TestKleene_SiIndetermineALaRacineSeulement(t *testing.T) {
	mk := func(cond *Noeud, si SiIndetermine) Jeu {
		d := Directive{ID: "K", Version: 1, Statut: Active, DateEffet: t0.Add(-time.Hour), TexteFR: "k",
			Fondements: []Fondement{{Texte: "k"}}, SiIndetermine: si,
			Declencheur: &Declencheur{Evenement: EvAppelOutil, Cible: &CibleV2{Serveur: "*"}},
			Arbre:       cond, Effets: []EffetDirective{{Effet: AttendreValidation}}}
		if si == IndetermineIgnorer {
			d.SiIndetermineValidePar = "juriste"
		}
		return jeu(d)
	}
	a := appel(argsKleene)
	cases := []struct {
		nom   string
		cond  *Noeud
		si    SiIndetermine
		effet Effet
		res   Trivalent
	}{
		{"ou(I, V) = V : effet de la directive", &Noeud{Ou: []*Noeud{feuilleDe[Indetermine], feuilleDe[Vrai]}}, "", AttendreValidation, Vrai},
		{"et(I, F) = F : rien", &Noeud{Et: []*Noeud{feuilleDe[Indetermine], feuilleDe[Faux]}}, "", Autoriser, Faux},
		{"non(I) = I à la racine : refus par défaut", &Noeud{Non: feuilleDe[Indetermine]}, "", Refuser, Indetermine},
		{"I explicite refuser", feuilleDe[Indetermine], IndetermineRefuser, Refuser, Indetermine},
		{"I et ignorer : rien", feuilleDe[Indetermine], IndetermineIgnorer, Autoriser, Indetermine},
	}
	for _, c := range cases {
		d := Decide(mk(c.cond, c.si), EtatSession{}, a)
		if d.Effet != c.effet || d.Applications[0].Resultat != c.res {
			t.Errorf("%s : effet %s résultat %v, attendu %s %v (%s)", c.nom, d.Effet, d.Applications[0].Resultat, c.effet, c.res, d.Applications[0].Motif)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 2 : répétition, 1 000 évaluations, une seule décision.
// ---------------------------------------------------------------------------

func toutesLesDirectives(t *testing.T) Jeu {
	t.Helper()
	var ds []Directive
	for _, nom := range []string{"credit", "weekend", "top-management", "rh", "demandeur", "complet"} {
		ds = append(ds, jeuV2(t, nom).Directives...)
	}
	pme, err := chargerTestdata(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range pme.Directives {
		d.ID = "PME-" + d.ID
		ds = append(ds, d)
	}
	j := jeu(ds...)
	if err := Valider(&j); err != nil {
		t.Fatal(err)
	}
	return j
}

func TestDecide_Repetition1000(t *testing.T) {
	j := toutesLesDirectives(t)
	etat := EtatSession{ID: "s", Appelant: "spiffe://lab/agents/credit", Consultations: []Consultation{
		{Seq: 1, Serveur: "taux-officiel", Outil: "get_taux", Resultat: ResultatOK, VersionRegistre: "r",
			Etiquettes: []string{"officiel"}, Source: &SourceReponse{Origine: "direct", Date: "2026-10-01T00:00:00Z"}},
		{Seq: 2, Serveur: "crm", Outil: "fiche", Resultat: ResultatOK},
	}}
	a := Appel{Serveur: "banque-synthetique", Outil: "attribuer_pret", Horodatage: ts("2026-10-03T21:30:00Z"),
		Arguments: map[string]any{"montant": json.Number("2500000"), "devise": "EUR", "code": "FR-1", "client_id": "C-7",
			"siren": "552100554", "lignes": []any{map[string]any{"beneficiaires": []any{"B-1", "B-2"}}}, "z": 1.5, "a": true},
		Attestation: Attestation{Mode: "spiffe", Attendu: "x", Observe: "x", Schemas: true},
		Acces:       AccesAction, EtiquettesCible: []string{"sensible", "donnees_client"}, VersionRegistre: "r",
		Demandeur:        &Demandeur{Identite: "alice@cabinet.fr", Groupes: []string{"prestataires-externes"}},
		Sujets:           []Sujet{{Empreinte: EmpreinteSujet("C-7"), Groupes: []string{"vip"}}, {Empreinte: EmpreinteSujet("B-1"), Groupes: []string{"gel"}}},
		SousAgentDeclare: "analyste"}
	premiere := Decide(j, etat, a)
	if len(premiere.Applications) < 5 {
		t.Fatalf("le test doit évaluer beaucoup de directives, %d seulement", len(premiere.Applications))
	}
	ref, _ := json.Marshal(premiere)
	for i := 0; i < 1000; i++ {
		d := Decide(j, etat, a)
		if !reflect.DeepEqual(d, premiere) {
			t.Fatalf("évaluation %d diverge", i)
		}
		if b, _ := json.Marshal(d); string(b) != string(ref) {
			t.Fatalf("évaluation %d diverge une fois sérialisée", i)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 3 : arbres générés aléatoirement à graine fixe.
// ---------------------------------------------------------------------------

type generateur struct{ r *rand.Rand }

func (g generateur) choix(l ...string) string { return l[g.r.Intn(len(l))] }

func (g generateur) valeur() any {
	switch g.r.Intn(9) {
	case 0:
		return json.Number(g.choix("0", "1", "-3", "500000", "500000.5", "1e3", "0.1"))
	case 1:
		return g.r.Float64() * 1e6
	case 2:
		return g.choix("EUR", "FR-1", "", "S-1")
	case 3:
		return g.r.Intn(2) == 0
	case 4:
		return []any{json.Number("1"), "EUR", true}
	case 5:
		return map[string]any{"x": json.Number("1")}
	case 6:
		return nil
	case 7:
		return json.Number("1e999999") // illisible
	}
	return []any{"S-1", "S-2", json.Number("1042")}
}

func (g generateur) ptr() string {
	return g.choix("/montant", "/devise", "/l", "/l/0", "/absent", "", "/a~1b", "/obj/x", "/ids")
}

func (g generateur) op() Op {
	return Op(g.choix(">", ">=", "<", "<=", "==", "!=", "dans", "in", "present", "absent", "prefixe", "~"))
}

func (g generateur) noeud(prof int) *Noeud {
	if prof <= 0 || g.r.Intn(3) == 0 {
		return g.feuille()
	}
	switch g.r.Intn(6) {
	case 0, 1:
		n := g.r.Intn(4)
		l := make([]*Noeud, n)
		for i := range l {
			l[i] = g.noeud(prof - 1)
		}
		if g.r.Intn(2) == 0 {
			return &Noeud{Et: l}
		}
		return &Noeud{Ou: l}
	case 2:
		return &Noeud{Non: g.noeud(prof - 1)}
	case 3:
		return &Noeud{Existe: &Quantificateur{Champ: g.ptr(), Condition: g.noeud(prof - 1)}}
	case 4:
		return &Noeud{Tous: &Quantificateur{Champ: g.ptr(), Condition: g.noeud(prof - 1)}}
	}
	return nil // nœud vide : Decide doit y survivre
}

func (g generateur) feuille() *Noeud {
	b := g.r.Intn(2) == 0
	champ := g.ptr()
	switch g.r.Intn(11) {
	case 0:
		return &Noeud{Argument: &Condition{Champ: champ, Op: g.op(), Valeur: g.valeur()}}
	case 1:
		return &Noeud{AConsulte: &FeuilleConsultation{Serveur: g.choix("", "taux-officiel", "crm"), Outil: g.choix("", "crm/fiche"),
			Etiquette: g.choix("", "officiel"), Critere: Critere(g.choix("transmis", "donnees_recues", ""))}}
	case 2:
		return &Noeud{Appelant: &FeuilleChaine{Egal: g.choix("", "spiffe://lab/a"), Prefixe: g.choix("", "spiffe://")}}
	case 3:
		return &Noeud{SousAgentDeclare: &FeuilleChaine{Egal: g.choix("", "analyste"), Dans: []string{"analyste"}}}
	case 4:
		f := &FeuilleDemandeur{Groupe: g.choix("", "x")}
		if g.r.Intn(2) == 0 {
			f.Identite = &FeuilleChaine{Egal: "alice", Dans: []string{"bob"}}
		}
		return &Noeud{Demandeur: f}
	case 5:
		return &Noeud{Attestation: &FeuilleAttestation{Verifiee: &b, Mode: g.choix("", "spiffe", "aucune")}}
	case 6:
		f := &FeuilleTemps{Fuseau: g.choix("Europe/Paris", "Asia/Tokyo", "Mars/Olympus", "")}
		if g.r.Intn(2) == 0 {
			f.JourSemaine = []string{g.choix("samedi", "lundi", "x")}
		}
		if g.r.Intn(2) == 0 {
			f.Heure = &PlageHoraire{De: g.choix("08:00", "22:00", "x"), A: g.choix("07:00", "18:00")}
		}
		if g.r.Intn(2) == 0 {
			f.Date = &CritereDate{Op: g.op(), Valeur: g.choix("2026-10-03", "x")}
		}
		return &Noeud{Temps: f}
	case 7:
		return &Noeud{EtiquetteCible: &FeuilleEtiquette{Contient: g.choix("sensible", "")}}
	case 8:
		return &Noeud{MetadonneeReponse: &FeuilleMetadonnee{Serveur: g.choix("", "taux-officiel"), Etiquette: g.choix("", "officiel"),
			Champ: g.choix("source.origine", "source.date", "x"), Op: g.op(), Valeur: g.valeur()}}
	case 9:
		var c *string
		if g.r.Intn(2) == 0 {
			s := champ
			c = &s
		}
		return &Noeud{Sujet: &FeuilleSujet{Groupe: g.choix("vip", "gel"), Champ: c}}
	}
	// Nœud à plusieurs clés, que le chargeur refuserait.
	return &Noeud{Non: g.feuille(), Argument: &Condition{Champ: champ, Op: OpPresent}}
}

func (g generateur) args() map[string]any {
	return map[string]any{
		"montant": g.valeur(), "devise": g.valeur(), "l": []any{g.valeur(), g.valeur()},
		"a/b": g.valeur(), "obj": map[string]any{"x": g.valeur()}, "ids": []any{"SUJET-SECRET", g.valeur()},
		// Sentinelles : ni valeur ni clé d'argument ne doit atteindre un motif.
		"note": "VALEUR-SECRETE", "CLE-SECRETE": json.Number("1"),
	}
}

func TestArbresAleatoires_SansPaniqueNiDivergence(t *testing.T) {
	g := generateur{rand.New(rand.NewSource(20261002))}
	etat := EtatSession{Appelant: "spiffe://lab/a", Consultations: []Consultation{
		{Serveur: "taux-officiel", Outil: "get", Resultat: ResultatOK, Source: &SourceReponse{Origine: "direct", Date: "2026-10-01T00:00:00Z"}},
		{Serveur: "crm", Outil: "fiche", Resultat: ResultatErreur, VersionRegistre: "r", Etiquettes: []string{"officiel"}},
	}}
	effets := [][]EffetDirective{
		{{Effet: Refuser}},
		{{Effet: AttendreValidation, ParRole: "r"}, {Effet: Notifier, DestinataireRole: "j", Rythme: Synthese, SiEchec: EchecContinuer}},
		{{Effet: Caviarder, ChampsAutorises: []string{"/montant"}, Mode: ModeCaviarder}, {Effet: Marquer, Etiquette: "m"}},
		{{Effet: Caviarder, ChampsAutorises: []string{"/devise"}, Mode: ModeRefuser}},
	}
	for i := 0; i < 3000; i++ {
		var ds []Directive
		for k := 0; k < 1+g.r.Intn(4); k++ {
			ds = append(ds, Directive{ID: fmt.Sprintf("R-%d", k), Version: 1, Statut: Statut(g.choix("active", "observation")),
				DateEffet: t0.Add(-time.Hour), Fondements: []Fondement{{Texte: "f"}},
				SiIndetermine: SiIndetermine(g.choix("", "ignorer")), SiIndetermineValidePar: "j",
				Declencheur: &Declencheur{Evenement: EvAppelOutil, Cible: &CibleV2{Serveur: "*", Acces: Acces(g.choix("", "action"))}},
				Arbre:       g.noeud(6), Effets: effets[g.r.Intn(len(effets))]})
		}
		j := jeu(ds...)
		a := Appel{Serveur: "banque", Outil: "o", Arguments: g.args(), Horodatage: t0.Add(time.Duration(g.r.Intn(1e6)) * time.Second),
			Acces: AccesAction, VersionRegistre: g.choix("", "r"), EtiquettesCible: []string{"sensible"},
			Sujets: []Sujet{{Empreinte: EmpreinteSujet("SUJET-SECRET"), Groupes: []string{"gel"}}, {Empreinte: EmpreinteSujet("S-1"), Groupes: []string{"vip"}}}}
		if g.r.Intn(2) == 0 {
			a.Demandeur = &Demandeur{Identite: "alice", Groupes: []string{"x"}}
		}
		var d1, d2 Decision
		func() {
			defer func() {
				if p := recover(); p != nil {
					b, _ := json.Marshal(j)
					t.Fatalf("itération %d : panique %v\njeu : %s", i, p, b)
				}
			}()
			d1 = Decide(j, etat, a)
			d2 = Decide(j, etat, a)
		}()
		if !reflect.DeepEqual(d1, d2) {
			t.Fatalf("itération %d : deux évaluations divergent", i)
		}
		textes := append([]string{d1.Motif}, d1.Alertes...)
		for _, app := range d1.Applications {
			textes = append(textes, app.Motif)
		}
		for _, m := range textes {
			if strings.Contains(m, "SECRET") {
				t.Fatalf("itération %d : une valeur, une clé d'argument ou un identifiant de sujet apparaît dans un motif : %q", i, m)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Test 5 : tout journal régénéré par les tests se rejoue sans écart. Il vit
// là où les journaux sont produits : test/integration (TestMain compte les
// journaux et exige que chacun ait été vérifié et rejoué), internal/gate et
// test/gate (rejouer à la fin de chaque banc), internal/rejeu.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Week-end : mêmes résultats quel que soit le fuseau de la machine.
// ---------------------------------------------------------------------------

const envEnfant = "MCPROGUE_TEST_ENFANT_WEEKEND"

// TestWeekEnd_EnfantImprime est le corps exécuté dans un processus fils
// lancé avec TZ imposé ; hors de ce fils il ne fait rien.
func TestWeekEnd_EnfantImprime(t *testing.T) {
	if os.Getenv(envEnfant) == "" {
		t.Skip("exécuté seulement par TestWeekEnd_FuseauMachine")
	}
	j := jeuV2(t, "weekend")
	var lignes []string
	for _, c := range casWeekend {
		d := Decide(j, EtatSession{}, appelWeekend(c.h, c.acces))
		lignes = append(lignes, fmt.Sprintf("%s %s %s", c.h, c.acces, d.Effet))
	}
	fmt.Println("RESULTATS " + time.Local.String() + " " + strings.Join(lignes, "|"))
}

func TestWeekEnd_FuseauMachine(t *testing.T) {
	if os.Getenv(envEnfant) != "" {
		t.Skip()
	}
	var resultats []string
	for _, tz := range []string{"UTC", "Asia/Tokyo"} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestWeekEnd_EnfantImprime$", "-test.v")
		cmd.Env = append(os.Environ(), "TZ="+tz, envEnfant+"=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("TZ=%s : %v\n%s", tz, err, out)
		}
		var ligne string
		for _, l := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(l, "RESULTATS ") {
				ligne = l
			}
		}
		champs := strings.SplitN(ligne, " ", 3)
		if len(champs) != 3 {
			t.Fatalf("TZ=%s : sortie inattendue\n%s", tz, out)
		}
		t.Logf("TZ=%s (time.Local=%s) : %s", tz, champs[1], champs[2])
		resultats = append(resultats, champs[2])
	}
	if resultats[0] != resultats[1] {
		t.Fatalf("les décisions dépendent du fuseau de la machine :\nUTC   %s\nTokyo %s", resultats[0], resultats[1])
	}
	var veut []string
	for _, c := range casWeekend {
		veut = append(veut, fmt.Sprintf("%s %s %s", c.h, c.acces, c.effet))
	}
	if resultats[0] != strings.Join(veut, "|") {
		t.Fatalf("résultats %s, attendu %s", resultats[0], strings.Join(veut, "|"))
	}
}
