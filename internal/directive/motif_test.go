package directive

import (
	"strings"
	"testing"
	"time"
)

// Aucun motif ne cite une valeur d'argument : seulement le champ,
// l'opérateur et la valeur de seuil issue de la directive. Les arguments
// peuvent porter des données personnelles ; le motif va au journal, à
// l'agent et aux écrans.
func TestMotifs_SansValeurDArgument(t *testing.T) {
	const sentinelle = "VALEUR-SECRETE-7f3a"
	args := map[string]any{
		"montant":    sentinelle, // chaîne là où un nombre est attendu : indéterminé
		"nom":        sentinelle, // comparé par ==, in
		"siren":      sentinelle, // caviardé
		"emprunteur": map[string]any{"adresse": sentinelle},
		"liste":      []any{sentinelle},
		sentinelle:   1.0, // même une clé ne doit pas fuir dans un motif (retirée comme pointeur)
	}
	ds := []Directive{
		seuil("S1", Refuser, nil),
		seuil("S2", Refuser, func(d *Directive) {
			d.Condition = []Condition{
				{Champ: "/nom", Op: OpEgal, Valeur: "autre"},
				{Champ: "/nom", Op: OpDans, Valeur: []any{"a", "b"}},
				{Champ: "/liste/0", Op: OpDiff, Valeur: "x"},
			}
		}),
		seuil("S3", AttendreValidation, func(d *Directive) {
			d.SiIndetermine = IndetermineIgnorer
			d.SiIndetermineValidePar = "juriste"
		}),
		champs("C1", ModeCaviarder, "/siren"),
		champs("C2", ModeRefuser, "/emprunteur/adresse"),
		dir("I", Interdiction, Refuser, nil),
		dir("A", ApprobationHumaine, AttendreValidation, nil),
		provenance("P", Refuser, Transmis, []string{"taux-officiel"}, nil),
		dir("T", SourceAttestee, Refuser, func(d *Directive) { d.Cible = Cible{Serveur: "banque", Outil: "*"} }),
		dir("O", Interdiction, Refuser, func(d *Directive) {
			d.Statut = Observation
			d.Perimetre = &Perimetre{Appelants: []string{"spiffe://lab/x"}}
			r := t0.Add(-time.Minute)
			d.DateEffet = t0.Add(-time.Hour)
			d.RevisionAvant = &r
		}),
	}
	a := appel(args)
	a.Attestation = Attestation{Mode: "binaire", Attendu: "aaa", Observe: "bbb"}
	etat := EtatSession{Consultations: []Consultation{{Serveur: "taux-officiel", Outil: "get", Resultat: ResultatErreur}}}
	d := Decide(jeu(ds...), etat, a)

	// Les motifs et alertes ne citent jamais ce que l'agent a envoyé. Les
	// pointeurs retirés (ChampsRetires) sont des noms de champs destinés au
	// journal ; ils ne passent pas par les motifs.
	textes := []string{d.Motif}
	textes = append(textes, d.Alertes...)
	for _, app := range d.Applications {
		textes = append(textes, app.Motif)
	}
	if len(d.ChampsRetires) == 0 {
		t.Fatal("le caviardage doit avoir retiré des champs")
	}
	if len(d.Applications) != len(ds) {
		t.Fatalf("%d applications pour %d directives : le test ne couvre pas tous les verrous", len(d.Applications), len(ds))
	}
	for _, s := range textes {
		if strings.Contains(s, sentinelle) {
			t.Errorf("une valeur d'argument apparaît dans un motif : %q", s)
		}
	}
	// Ce que le motif cite, en revanche, doit y être : le champ, l'opérateur
	// et le seuil de la directive.
	trouve := false
	for _, app := range d.Applications {
		if app.ID == "S1" && strings.Contains(app.Motif, "/montant > 500000") {
			trouve = true
		}
	}
	if !trouve {
		t.Error("le motif du seuil doit citer champ, opérateur et seuil de la directive")
	}
}

// Même garantie pour chaque feuille et chaque effet v2 : les motifs ne citent
// que la directive. Les sentinelles sont posées dans les valeurs, les clés,
// les éléments de tableaux et les identifiants de personnes concernées.
func TestMotifsV2_SansValeurDArgument(t *testing.T) {
	const s = "VALEUR-SECRETE-7f3a"
	var ds []Directive
	for _, nom := range []string{"credit", "weekend", "top-management", "rh", "demandeur", "complet"} {
		for _, d := range jeuV2(t, nom).Directives {
			if d.EstV2() && d.Declencheur.Evenement == EvAppelOutil {
				ds = append(ds, d)
			}
		}
	}
	// Sans ignorer, pour que chaque indétermination produise son motif.
	ds[len(ds)-1].SiIndetermine, ds[len(ds)-1].SiIndetermineValidePar = "", ""
	// Toutes ciblent l'appel. Un caviardage exige une cible nommée au
	// chargement ; Decide ne relit pas ce contrôle.
	for i := range ds {
		c := *ds[i].Declencheur.Cible
		c.Serveur, c.Outil, c.Etiquette = "*", "*", ""
		ds[i].Declencheur = &Declencheur{Evenement: EvAppelOutil, Cible: &c}
	}
	args := map[string]any{
		"montant": s, "devise": s, "code": s, "client_id": s, "siren": s, "note": s,
		"salarie_ids": []any{s, s + "-2"},
		"lignes":      []any{map[string]any{"beneficiaires": []any{s}}},
		s:             map[string]any{s: s},
	}
	a := Appel{Serveur: "banque-synthetique", Outil: "attribuer_pret", Arguments: args, Horodatage: ts("2026-10-03T21:30:00Z"),
		Acces: AccesAction, EtiquettesCible: []string{"sensible"}, VersionRegistre: "r", SousAgentDeclare: s,
		Demandeur:   &Demandeur{Identite: s, Groupes: []string{"x"}},
		Sujets:      []Sujet{{Empreinte: EmpreinteSujet(s), Groupes: []string{"contentieux-en-cours", "vip"}}},
		Attestation: Attestation{Mode: "spiffe", Attendu: "a", Observe: "a", Schemas: true}}
	etat := EtatSession{Appelant: "spiffe://lab/agents/x", Consultations: []Consultation{
		{Serveur: "taux-officiel", Outil: "get_taux", Resultat: ResultatOK, VersionRegistre: "r", Etiquettes: []string{"officiel"},
			Source: &SourceReponse{Origine: s, Date: s}}}}
	d := Decide(jeu(ds...), etat, a)
	if len(d.Applications) != len(ds) {
		t.Fatalf("%d applications pour %d directives", len(d.Applications), len(ds))
	}
	textes := append([]string{d.Motif}, d.Alertes...)
	for _, app := range d.Applications {
		textes = append(textes, app.Motif)
	}
	for _, m := range textes {
		if strings.Contains(m, s) {
			t.Errorf("une valeur d'argument apparaît dans un motif : %q", m)
		}
	}
}
