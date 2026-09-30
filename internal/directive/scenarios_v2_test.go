package directive

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Scénarios obligatoires du brief v2, sur les jeux de testdata/v2.

func jeuV2(t *testing.T, nom string) Jeu {
	t.Helper()
	j := lireJeuxV2(t)[nom]
	if j == nil {
		t.Fatalf("jeu %s absent", nom)
	}
	return *j
}

func ts(s string) time.Time {
	h, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return h
}

func consultation(serveur string, r Resultat) Consultation {
	return Consultation{Serveur: serveur, Outil: "consulter", Resultat: r}
}

func appelPret(montant any) Appel {
	return Appel{
		Serveur: "banque-synthetique", Outil: "attribuer_pret",
		Arguments:   map[string]any{"montant": montant, "siren": "552100554"},
		Horodatage:  ts("2026-10-05T10:00:00Z"),
		Attestation: Attestation{Mode: "spiffe", Attendu: "x", Observe: "x", Schemas: true},
	}
}

func appliquees(d Decision) []string {
	var out []string
	for _, a := range d.Applications {
		if a.Appliquee {
			out = append(out, a.ID)
		}
	}
	return out
}

func TestScenario_PolitiqueDeCredit(t *testing.T) {
	j := jeuV2(t, "credit")
	gels := consultation("gels-avoirs", ResultatOK)
	taux := consultation("taux-officiel", ResultatOK)
	cases := []struct {
		nom         string
		etat        EtatSession
		appel       Appel
		effet       Effet
		appliquees  []string
		indetermine string // id d'une directive indéterminée attendue
	}{
		{"sans consultation du registre des gels : refus", EtatSession{}, appelPret(json.Number("100000")), Refuser, []string{"C-001"}, ""},
		{"gels consulté sans données reçues : refus", EtatSession{Consultations: []Consultation{consultation("gels-avoirs", ResultatIndisponible)}}, appelPret(json.Number("100000")), Refuser, []string{"C-001"}, ""},
		{"gels consulté, montant > 500 000 : refus", EtatSession{Consultations: []Consultation{gels}}, appelPret(json.Number("600000")), Refuser, []string{"C-002"}, ""},
		{"gels et taux officiel consultés : attente", EtatSession{Consultations: []Consultation{gels, taux}}, appelPret(json.Number("100000")), AttendreValidation, []string{"C-003"}, ""},
		{"gels consulté, petit montant : autorisé", EtatSession{Consultations: []Consultation{gels}}, appelPret(json.Number("100000")), Autoriser, nil, ""},
		{"montant 500 000 exactement : autorisé", EtatSession{Consultations: []Consultation{gels}}, appelPret(json.Number("500000.000")), Autoriser, nil, ""},
		// En float64, 500000.00000000000001 vaut 500000 : la comparaison
		// exacte seule voit le dépassement.
		{"décimal exact : 500 000,000…01 > 500 000", EtatSession{Consultations: []Consultation{gels}}, appelPret(json.Number("500000.00000000000001")), Refuser, []string{"C-002"}, ""},
		{"montant illisible : indéterminé, refus par défaut", EtatSession{Consultations: []Consultation{gels}}, appelPret("beaucoup"), Refuser, []string{"C-002"}, "C-002"},
		{"refus et attente : le refus l'emporte", EtatSession{Consultations: []Consultation{taux}}, appelPret(json.Number("100000")), Refuser, []string{"C-001", "C-003"}, ""},
	}
	for _, c := range cases {
		t.Run(c.nom, func(t *testing.T) {
			d := Decide(j, c.etat, c.appel)
			if d.Effet != c.effet {
				t.Fatalf("effet %s, attendu %s (motif %q)", d.Effet, c.effet, d.Motif)
			}
			if got := appliquees(d); !reflect.DeepEqual(got, c.appliquees) {
				t.Fatalf("appliquées %v, attendu %v", got, c.appliquees)
			}
			if c.indetermine != "" {
				for _, a := range d.Applications {
					if a.ID == c.indetermine && (!a.Indetermine || a.Resultat != Indetermine) {
						t.Fatalf("%s devrait être indéterminée : %+v", a.ID, a)
					}
				}
			}
			if len(d.Applications) != 3 {
				t.Fatalf("les trois directives crédit doivent être évaluées, %d le sont", len(d.Applications))
			}
		})
	}

	t.Run("caviardage du SIREN vers recherche-entreprises", func(t *testing.T) {
		a := Appel{Serveur: "recherche-entreprises", Outil: "rechercher", Horodatage: ts("2026-10-05T10:00:00Z"),
			Arguments: map[string]any{"siren": "552100554", "dirigeant": "Jeanne", "note": map[string]any{"interne": true}}}
		d := Decide(j, EtatSession{}, a)
		if d.Effet != Caviarder || !reflect.DeepEqual(d.Arguments, map[string]any{"siren": "552100554"}) ||
			!reflect.DeepEqual(d.ChampsRetires, []string{"/dirigeant", "/note"}) || d.EmpreinteOrigine == d.EmpreinteTransmise {
			t.Fatalf("décision %+v", d)
		}
		if _, ok := a.Arguments["dirigeant"]; !ok {
			t.Fatal("le caviardage ne doit pas toucher les arguments de l'appelant")
		}
	})
}

func appelWeekend(h string, acces Acces) Appel {
	return Appel{Serveur: "banque-synthetique", Outil: "attribuer_pret", Arguments: map[string]any{},
		Horodatage: ts(h), Acces: acces, VersionRegistre: "registre-demo v3"}
}

// casWeekend : 2026-10-02 est un vendredi ; Paris est en UTC+2 jusqu'au
// 25 octobre 2026 à 01:00 UTC.
var casWeekend = []struct {
	h     string
	acces Acces
	effet Effet
}{
	{"2026-10-02T21:59:00Z", AccesAction, Autoriser},  // vendredi 23:59 à Paris
	{"2026-10-02T22:00:00Z", AccesAction, Refuser},    // samedi 00:00
	{"2026-10-02T22:00:00Z", AccesLecture, Autoriser}, // lecture : hors cible
	{"2026-10-04T21:59:00Z", AccesAction, Refuser},    // dimanche 23:59
	{"2026-10-04T22:00:00Z", AccesAction, Autoriser},  // lundi 00:00
	{"2026-10-24T22:00:00Z", AccesAction, Refuser},    // dimanche 25 00:00, encore UTC+2
}

func TestScenario_WeekEnd(t *testing.T) {
	j := jeuV2(t, "weekend")
	for _, c := range casWeekend {
		d := Decide(j, EtatSession{}, appelWeekend(c.h, c.acces))
		if d.Effet != c.effet {
			t.Errorf("%s %s : %s, attendu %s (%s)", c.h, c.acces, d.Effet, c.effet, d.Motif)
		}
	}
	// Sans version de registre, l'accès est inconnu : la cible est
	// indéterminée, et si_indetermine (refuser) tranche.
	a := appelWeekend("2026-10-02T22:00:00Z", AccesLecture)
	a.VersionRegistre = ""
	if d := Decide(j, EtatSession{}, a); d.Effet != Refuser || !d.Applications[0].Indetermine {
		t.Errorf("accès inconnu : %s, attendu un refus par défaut (%s)", d.Effet, d.Motif)
	}
	// Le fuseau de la machine n'entre pas en jeu, même modifié en cours
	// d'exécution.
	ancien := time.Local
	defer func() { time.Local = ancien }()
	for _, nom := range []string{"Asia/Tokyo", "America/New_York", "UTC"} {
		l, _ := Fuseau(nom)
		time.Local = l
		for _, c := range casWeekend {
			a := appelWeekend(c.h, c.acces)
			a.Horodatage = a.Horodatage.Local()
			if d := Decide(j, EtatSession{}, a); d.Effet != c.effet {
				t.Errorf("time.Local=%s, %s : %s, attendu %s", nom, c.h, d.Effet, c.effet)
			}
		}
	}
}

func TestScenario_TopManagement(t *testing.T) {
	j := jeuV2(t, "top-management")
	d := Decide(j, EtatSession{}, appelPret(json.Number("2500000")))
	if d.Effet != AttendreValidation {
		t.Fatalf("effet %s, attendu attendre_validation", d.Effet)
	}
	if !reflect.DeepEqual(d.Validation, &Validation{Roles: []string{"directeur-risques"}, QuatreYeux: true}) {
		t.Fatalf("validation %+v", d.Validation)
	}
	veut := []Notification{{Directive: "T-001", Version: 1, DestinataireRole: "juridique", Rythme: TempsReel, SiEchec: EchecRefuser}}
	if !reflect.DeepEqual(d.Notifications, veut) {
		t.Fatalf("notifications %+v", d.Notifications)
	}
	if !strings.Contains(d.Motif, "directeur-risques") || !strings.Contains(d.Motif, "quatre yeux") {
		t.Fatalf("motif %q", d.Motif)
	}
	if len(d.Applications[0].Fondements) != 2 || d.Applications[0].Fondement.Texte != "Politique de crédit" {
		t.Fatalf("fondements %+v", d.Applications[0])
	}
	d = Decide(j, EtatSession{}, appelPret(json.Number("2000000")))
	if d.Effet != Autoriser || d.Notifications != nil || d.Validation != nil {
		t.Fatalf("2 000 000 exactement : %+v", d)
	}
}

func appelRH(acces Acces, ids ...any) Appel {
	return Appel{Serveur: "rh-synthetique", Outil: "modifier_contrat", Horodatage: ts("2026-10-05T10:00:00Z"),
		Arguments: map[string]any{"salarie_ids": ids}, Acces: acces, VersionRegistre: "registre-demo v3",
		Sujets: []Sujet{
			{Empreinte: EmpreinteSujet("S-1"), Groupes: []string{"contentieux-en-cours"}},
			{Empreinte: EmpreinteSujet("S-2"), Groupes: []string{"salaries-proteges"}},
			{Empreinte: EmpreinteSujet("S-3"), Groupes: []string{"cadres"}},
			{Empreinte: EmpreinteSujet("1042"), Groupes: []string{"contentieux-en-cours"}},
		}}
}

func TestScenario_RH(t *testing.T) {
	j := jeuV2(t, "rh")
	cases := []struct {
		nom   string
		appel Appel
		effet Effet
	}{
		{"un salarié en contentieux : refus", appelRH(AccesAction, "S-3", "S-1"), Refuser},
		{"salarié protégé et action : attente", appelRH(AccesAction, "S-3", "S-2"), AttendreValidation},
		{"salarié protégé en lecture : autorisé", appelRH(AccesLecture, "S-2"), Autoriser},
		{"les deux : refus (précédence)", appelRH(AccesAction, "S-2", "S-1"), Refuser},
		{"aucun sujet visé : autorisé", appelRH(AccesAction, "S-3"), Autoriser},
		{"liste vide : autorisé (existe sur vide = faux)", appelRH(AccesAction), Autoriser},
		{"identifiant entier : résolu", appelRH(AccesAction, json.Number("1042")), Refuser},
		{"sujet non résolu : indéterminé, refus par défaut", appelRH(AccesAction, "S-9"), Refuser},
	}
	for _, c := range cases {
		t.Run(c.nom, func(t *testing.T) {
			d := Decide(j, EtatSession{}, c.appel)
			if d.Effet != c.effet {
				t.Fatalf("effet %s, attendu %s (%s)", d.Effet, c.effet, d.Motif)
			}
			for _, a := range d.Applications {
				for _, id := range []string{"S-1", "S-2", "S-3", "S-9", "1042"} {
					if strings.Contains(a.Motif, id) {
						t.Fatalf("un identifiant de sujet apparaît dans un motif : %q", a.Motif)
					}
				}
			}
		})
	}
	if d := Decide(j, EtatSession{}, appelRH(AccesAction, "S-2")); d.Validation == nil || !reflect.DeepEqual(d.Validation.Roles, []string{"drh"}) {
		t.Fatalf("validation %+v", d.Validation)
	}
}

func TestScenario_Demandeur(t *testing.T) {
	j := jeuV2(t, "demandeur")
	base := func() (EtatSession, Appel) {
		etat := EtatSession{Consultations: []Consultation{{Serveur: "crm", Outil: "fiche", Resultat: ResultatOK,
			Etiquettes: []string{"donnees_client"}, VersionRegistre: "registre-demo v3"}}}
		a := Appel{Serveur: "rapports", Outil: "produire", Horodatage: ts("2026-10-05T10:00:00Z"), Arguments: map[string]any{},
			EtiquettesCible: []string{"production_rapport"}, VersionRegistre: "registre-demo v3",
			Demandeur: &Demandeur{Identite: "bob@cabinet.fr", Groupes: []string{"prestataires-externes"}}}
		return etat, a
	}
	etat, a := base()
	d := Decide(j, etat, a)
	veut := []Notification{{Directive: "U-001", Version: 1, DestinataireRole: "dpo", Rythme: TempsReel, SiEchec: EchecContinuer}}
	if d.Effet != Refuser || !reflect.DeepEqual(d.Notifications, veut) {
		t.Fatalf("refus et notification attendus : %+v", d)
	}

	etat, a = base()
	a.Demandeur.Groupes = []string{"salaries"}
	if d := Decide(j, etat, a); d.Effet != Autoriser || d.Notifications != nil {
		t.Fatalf("autre groupe : %+v", d)
	}
	etat, a = base()
	etat.Consultations[0].Resultat = ResultatErreur
	if d := Decide(j, etat, a); d.Effet != Autoriser {
		t.Fatalf("aucune donnée client reçue : %s", d.Effet)
	}
	etat, a = base()
	a.EtiquettesCible = []string{"lecture"}
	if d := Decide(j, etat, a); d.Effet != Autoriser || len(d.Applications) != 0 {
		t.Fatalf("cible non étiquetée production_rapport : %+v", d)
	}
	etat, a = base()
	a.Demandeur = nil
	if d := Decide(j, etat, a); d.Effet != Refuser || !d.Applications[0].Indetermine {
		t.Fatalf("demandeur non résolu : refus par défaut attendu, %+v", d)
	}
	// Directive en observation : journalisée, sans refus ni notification.
	etat, a = base()
	obs := j
	obs.Directives = append([]Directive(nil), j.Directives...)
	obs.Directives[0].Statut = Observation
	if d := Decide(obs, etat, a); d.Effet != Autoriser || d.Notifications != nil || !d.Applications[0].Declenchee {
		t.Fatalf("observation : %+v", d)
	}
}
