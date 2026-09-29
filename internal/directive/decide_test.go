package directive

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"mcprogue/internal/approval"
)

var t0 = time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC)

func dir(id string, verrou Verrou, effet Effet, mod func(*Directive)) Directive {
	d := Directive{
		ID: id, Version: 1, Statut: Active, DateEffet: t0.Add(-time.Hour),
		TexteFR:   "texte " + id,
		Fondement: Fondement{Texte: "RGPD", Article: "22"},
		Verrou:    verrou,
		Cible:     Cible{Serveur: "banque", Outil: "attribuer_pret"},
		Effet:     effet,
	}
	if mod != nil {
		mod(&d)
	}
	return d
}

func jeu(ds ...Directive) Jeu {
	return Jeu{Meta: MetaJeu{ID: "test", Version: 1, DateEffet: t0.Add(-time.Hour), Auteur: "t"}, Directives: ds}
}

func appel(args map[string]any) Appel {
	return Appel{
		Serveur: "banque", Outil: "attribuer_pret", Arguments: args, Horodatage: t0,
		Attestation: Attestation{Mode: "binaire", Attendu: "a", Observe: "a", Schemas: true, Verifiee: true},
	}
}

func seuil(id string, effet Effet, mod func(*Directive)) Directive {
	return dir(id, SeuilArgument, effet, func(d *Directive) {
		d.Condition = []Condition{{Champ: "/montant", Op: OpSup, Valeur: 500000.0}}
		if mod != nil {
			mod(d)
		}
	})
}

func champs(id string, mode ModeChamps, autorises ...string) Directive {
	effet := Caviarder
	if mode == ModeRefuser {
		effet = Refuser
	}
	return dir(id, ChampsAutorises, effet, func(d *Directive) {
		d.Champs = &Champs{Autorises: autorises, Mode: mode}
	})
}

func provenance(id string, effet Effet, critere Critere, serveurs []string, outils []string) Directive {
	return dir(id, ProvenanceSession, effet, func(d *Directive) {
		d.Provenance = &Provenance{Serveurs: serveurs, Outils: outils, Critere: critere}
	})
}

type cas struct {
	nom        string
	jeu        Jeu
	etat       EtatSession
	appel      Appel
	effet      Effet
	args       map[string]any // nil : non vérifié
	retires    []string
	appliquees []string // ids attendus avec Appliquee
	verif      func(t *testing.T, d Decision)
}

func TestDecide_Table(t *testing.T) {
	base := map[string]any{"montant": 600000.0, "siren": "552100554", "nom": "X", "adresse": "Y"}
	consulteOK := EtatSession{Consultations: []Consultation{{Serveur: "taux-officiel", Outil: "get_taux", Resultat: ResultatOK}}}
	consulteErr := EtatSession{Consultations: []Consultation{{Serveur: "taux-officiel", Outil: "get_taux", Resultat: ResultatErreur}}}

	cases := []cas{
		{
			nom: "aucune directive : autoriser", jeu: jeu(), appel: appel(base), effet: Autoriser,
			verif: func(t *testing.T, d Decision) {
				if d.EmpreinteOrigine != d.EmpreinteTransmise || d.Motif != "autorisé : aucune directive applicable" {
					t.Errorf("décision %+v", d)
				}
			},
		},

		// Précédence.
		{
			nom:   "précédence : refuser > attendre > caviarder",
			jeu:   jeu(champs("C", ModeCaviarder, "/siren"), dir("A", ApprobationHumaine, AttendreValidation, nil), dir("I", Interdiction, Refuser, nil)),
			appel: appel(base), effet: Refuser, appliquees: []string{"C", "A", "I"},
			args: map[string]any{"siren": "552100554"},
			verif: func(t *testing.T, d Decision) {
				if !strings.HasPrefix(d.Motif, "refusé par I v1 (interdiction, RGPD art. 22)") {
					t.Errorf("motif %q", d.Motif)
				}
			},
		},
		{
			nom:   "précédence : attendre > caviarder",
			jeu:   jeu(champs("C", ModeCaviarder, "/siren"), dir("A", ApprobationHumaine, AttendreValidation, nil)),
			appel: appel(base), effet: AttendreValidation, appliquees: []string{"C", "A"},
		},
		{
			nom: "précédence : caviarder seul",
			jeu: jeu(champs("C", ModeCaviarder, "/siren")), appel: appel(base), effet: Caviarder, appliquees: []string{"C"},
			retires: []string{"/adresse", "/montant", "/nom"},
		},

		// Seuil.
		{nom: "seuil dépassé refuse", jeu: jeu(seuil("S", Refuser, nil)), appel: appel(base), effet: Refuser, appliquees: []string{"S"}},
		{nom: "seuil non atteint", jeu: jeu(seuil("S", Refuser, nil)), appel: appel(map[string]any{"montant": 400000.0}), effet: Autoriser,
			verif: func(t *testing.T, d Decision) {
				if len(d.Applications) != 1 || d.Applications[0].Declenchee {
					t.Errorf("la directive doit être journalisée non déclenchée: %+v", d.Applications)
				}
			}},
		{nom: "seuil strict : égalité n'est pas au-delà", jeu: jeu(seuil("S", Refuser, nil)), appel: appel(map[string]any{"montant": 500000.0}), effet: Autoriser},
		{nom: "seuil en attente", jeu: jeu(seuil("S", AttendreValidation, nil)), appel: appel(base), effet: AttendreValidation, appliquees: []string{"S"}},
		{nom: "seuil : conditions en ET", jeu: jeu(seuil("S", Refuser, func(d *Directive) {
			d.Condition = append(d.Condition, Condition{Champ: "/nom", Op: OpEgal, Valeur: "autre"})
		})), appel: appel(base), effet: Autoriser},
		{nom: "seuil : in / present / absent", jeu: jeu(seuil("S", Refuser, func(d *Directive) {
			d.Condition = []Condition{
				{Champ: "/nom", Op: OpDans, Valeur: []any{"X", "Z"}},
				{Champ: "/siren", Op: OpPresent},
				{Champ: "/inconnu", Op: OpAbsent},
			}
		})), appel: appel(base), effet: Refuser, appliquees: []string{"S"}},

		// Indéterminé.
		{
			nom: "indéterminé : champ absent, refus par défaut même si effet attendre",
			jeu: jeu(seuil("S", AttendreValidation, nil)), appel: appel(map[string]any{"siren": "1"}), effet: Refuser, appliquees: []string{"S"},
			verif: func(t *testing.T, d Decision) {
				a := d.Applications[0]
				if !a.Indetermine || a.Effet != Refuser || !strings.Contains(a.Motif, "indéterminée") {
					t.Errorf("application %+v", a)
				}
			},
		},
		{
			nom: "indéterminé : type inattendu (chaîne)",
			jeu: jeu(seuil("S", Refuser, nil)), appel: appel(map[string]any{"montant": "600000"}), effet: Refuser, appliquees: []string{"S"},
			verif: func(t *testing.T, d Decision) {
				if !d.Applications[0].Indetermine {
					t.Error("une chaîne comparée à un nombre doit être indéterminée")
				}
			},
		},
		{
			nom: "indéterminé : ignorer, validé par le juriste",
			jeu: jeu(seuil("S", Refuser, func(d *Directive) {
				d.SiIndetermine = IndetermineIgnorer
				d.SiIndetermineValidePar = "juriste@cabinet.fr"
			})), appel: appel(map[string]any{"siren": "1"}), effet: Autoriser,
			verif: func(t *testing.T, d Decision) {
				a := d.Applications[0]
				if !a.Indetermine || a.Declenchee || !strings.Contains(a.Motif, "juriste@cabinet.fr") {
					t.Errorf("application %+v", a)
				}
			},
		},
		{
			nom: "indéterminé : une condition fausse déterminée l'emporte",
			jeu: jeu(seuil("S", Refuser, func(d *Directive) {
				d.Condition = append(d.Condition, Condition{Champ: "/absent", Op: OpSup, Valeur: 1.0})
			})), appel: appel(map[string]any{"montant": 100.0}), effet: Autoriser,
			verif: func(t *testing.T, d Decision) {
				if d.Applications[0].Indetermine {
					t.Error("faux ET indéterminé doit être faux")
				}
			},
		},

		// Non-rétroactivité et statuts.
		{nom: "non-rétroactif : date_effet future ignorée", jeu: jeu(dir("I", Interdiction, Refuser, func(d *Directive) { d.DateEffet = t0.Add(time.Minute) })),
			appel: appel(base), effet: Autoriser, verif: func(t *testing.T, d Decision) {
				if len(d.Applications) != 0 {
					t.Error("une directive future ne doit pas être journalisée comme applicable")
				}
			}},
		{nom: "date_effet égale à l'appel s'applique", jeu: jeu(dir("I", Interdiction, Refuser, func(d *Directive) { d.DateEffet = t0 })),
			appel: appel(base), effet: Refuser, appliquees: []string{"I"}},
		{nom: "brouillon et retirée ignorés", jeu: jeu(
			dir("B", Interdiction, Refuser, func(d *Directive) { d.Statut = Brouillon }),
			dir("R", Interdiction, Refuser, func(d *Directive) { d.Statut = Retiree }),
		), appel: appel(base), effet: Autoriser},
		{nom: "observation : journalisée, non appliquée", jeu: jeu(dir("O", Interdiction, Refuser, func(d *Directive) { d.Statut = Observation })),
			appel: appel(base), effet: Autoriser, verif: func(t *testing.T, d Decision) {
				a := d.Applications[0]
				if !a.Declenchee || a.Appliquee || !strings.HasPrefix(a.Motif, "observation") {
					t.Errorf("application %+v", a)
				}
			}},
		{nom: "observation : ne caviarde pas", jeu: jeu(dir("O", ChampsAutorises, Caviarder, func(d *Directive) {
			d.Statut = Observation
			d.Champs = &Champs{Autorises: []string{"/siren"}, Mode: ModeCaviarder}
		})), appel: appel(base), effet: Autoriser, args: base, verif: func(t *testing.T, d Decision) {
			if len(d.ChampsRetires) != 0 || len(d.Applications[0].ChampsRetires) != 3 {
				t.Errorf("décision %+v", d)
			}
		}},

		// Caviardage.
		{
			nom:   "caviardage cumulé : intersection des listes",
			jeu:   jeu(champs("C1", ModeCaviarder, "/siren", "/nom"), champs("C2", ModeCaviarder, "/siren", "/adresse")),
			appel: appel(base), effet: Caviarder, appliquees: []string{"C1", "C2"},
			args: map[string]any{"siren": "552100554"}, retires: []string{"/adresse", "/montant", "/nom"},
			verif: func(t *testing.T, d Decision) {
				if !reflect.DeepEqual(d.Applications[0].ChampsRetires, []string{"/adresse", "/montant"}) ||
					!reflect.DeepEqual(d.Applications[1].ChampsRetires, []string{"/nom"}) {
					t.Errorf("retraits par directive: %+v", d.Applications)
				}
			},
		},
		{
			nom:   "caviardage + validation humaine : attente, arguments déjà caviardés, jeton sur l'empreinte transmise",
			jeu:   jeu(champs("C", ModeCaviarder, "/siren"), dir("A", ApprobationHumaine, AttendreValidation, nil)),
			appel: appel(base), effet: AttendreValidation, appliquees: []string{"C", "A"},
			args: map[string]any{"siren": "552100554"}, retires: []string{"/adresse", "/montant", "/nom"},
			verif: func(t *testing.T, d Decision) {
				want := approval.Digest("attribuer_pret", map[string]any{"siren": "552100554"})
				if d.EmpreinteTransmise != want {
					t.Errorf("empreinte transmise %s, attendu %s", d.EmpreinteTransmise, want)
				}
				if d.EmpreinteOrigine == d.EmpreinteTransmise {
					t.Error("l'empreinte d'origine doit différer après caviardage")
				}
				if d.EmpreinteOrigine != approval.Digest("attribuer_pret", base) {
					t.Error("l'empreinte d'origine doit porter sur les arguments reçus")
				}
				if !strings.Contains(d.Motif, "caviardés") {
					t.Errorf("motif %q", d.Motif)
				}
			},
		},
		{
			nom:   "caviardage imbriqué : préfixe et objet sur le chemin",
			jeu:   jeu(champs("C", ModeCaviarder, "/emprunteur/siren")),
			appel: appel(map[string]any{"emprunteur": map[string]any{"siren": "1", "nom": "N"}, "montant": 1.0, "liste": []any{1.0}}),
			effet: Caviarder, appliquees: []string{"C"}, args: map[string]any{"emprunteur": map[string]any{"siren": "1"}},
			retires: []string{"/emprunteur/nom", "/liste", "/montant"},
		},
		{nom: "champs mode refuser : champ en trop refuse", jeu: jeu(champs("C", ModeRefuser, "/siren")), appel: appel(base), effet: Refuser, appliquees: []string{"C"}, args: base},
		{nom: "champs mode refuser : rien en trop", jeu: jeu(champs("C", ModeRefuser, "/siren")), appel: appel(map[string]any{"siren": "1"}), effet: Autoriser},
		{
			nom: "les arguments de l'appelant ne sont pas modifiés",
			jeu: jeu(champs("C", ModeCaviarder, "/siren")), appel: appel(base), effet: Caviarder, appliquees: []string{"C"},
			verif: func(t *testing.T, d Decision) {
				if len(base) != 4 {
					t.Error("Decide a muté les arguments de l'appel")
				}
			},
		},

		// Provenance.
		{nom: "provenance donnees_recues : consultation ok déclenche", jeu: jeu(provenance("P", AttendreValidation, DonneesRecues, []string{"taux-officiel"}, nil)),
			etat: consulteOK, appel: appel(base), effet: AttendreValidation, appliquees: []string{"P"}},
		{nom: "provenance donnees_recues : consultation en erreur ne déclenche pas", jeu: jeu(provenance("P", AttendreValidation, DonneesRecues, []string{"taux-officiel"}, nil)),
			etat: consulteErr, appel: appel(base), effet: Autoriser},
		{nom: "provenance transmis : consultation en erreur déclenche", jeu: jeu(provenance("P", Refuser, Transmis, []string{"taux-officiel"}, nil)),
			etat: consulteErr, appel: appel(base), effet: Refuser, appliquees: []string{"P"}},
		{nom: "provenance par outil", jeu: jeu(provenance("P", Refuser, DonneesRecues, nil, []string{"taux-officiel/get_taux"})),
			etat: consulteOK, appel: appel(base), effet: Refuser, appliquees: []string{"P"}},
		{nom: "provenance par outil : autre outil", jeu: jeu(provenance("P", Refuser, DonneesRecues, nil, []string{"taux-officiel/autre"})),
			etat: consulteOK, appel: appel(base), effet: Autoriser},
		{nom: "provenance : session vierge", jeu: jeu(provenance("P", Refuser, DonneesRecues, []string{"taux-officiel"}, nil)),
			appel: appel(base), effet: Autoriser},

		// Source attestée.
		{
			nom: "source_attestee : attestation en échec refuse en citant le fondement",
			jeu: jeu(dir("T", SourceAttestee, Refuser, func(d *Directive) { d.Cible = Cible{Serveur: "taux-officiel", Outil: "*"} })),
			appel: func() Appel {
				a := appel(nil)
				a.Serveur, a.Outil = "taux-officiel", "get_taux"
				a.Attestation = Attestation{Mode: "binaire", Attendu: "aaa", Observe: "bbb", Schemas: false, Verifiee: false}
				return a
			}(), effet: Refuser, appliquees: []string{"T"},
			verif: func(t *testing.T, d Decision) {
				if !strings.Contains(d.Motif, "RGPD art. 22") || !strings.Contains(d.Applications[0].Motif, "attendu aaa observé bbb") {
					t.Errorf("motif %q / %q", d.Motif, d.Applications[0].Motif)
				}
			},
		},
		{
			nom: "source_attestee : Verifiee déclaré vrai mais attendu != observé, refus",
			jeu: jeu(dir("T", SourceAttestee, Refuser, func(d *Directive) { d.Cible = Cible{Serveur: "banque", Outil: "*"} })),
			appel: func() Appel {
				a := appel(base)
				a.Attestation = Attestation{Mode: "spiffe", Attendu: "spiffe://lab/taux", Observe: "spiffe://lab/autre", Schemas: true, Verifiee: true}
				return a
			}(), effet: Refuser, appliquees: []string{"T"},
		},
		{
			nom: "source_attestee : Verifiee déclaré vrai mais schémas non conformes, refus",
			jeu: jeu(dir("T", SourceAttestee, Refuser, func(d *Directive) { d.Cible = Cible{Serveur: "banque", Outil: "*"} })),
			appel: func() Appel {
				a := appel(base)
				a.Attestation = Attestation{Mode: "binaire", Attendu: "a", Observe: "a", Schemas: false, Verifiee: true}
				return a
			}(), effet: Refuser, appliquees: []string{"T"},
		},
		{
			nom: "source_attestee : Verifiee déclaré vrai mais mode inconnu, refus",
			jeu: jeu(dir("T", SourceAttestee, Refuser, func(d *Directive) { d.Cible = Cible{Serveur: "banque", Outil: "*"} })),
			appel: func() Appel {
				a := appel(base)
				a.Attestation = Attestation{Mode: "aucune", Attendu: "a", Observe: "a", Schemas: true, Verifiee: true}
				return a
			}(), effet: Refuser, appliquees: []string{"T"},
		},
		{
			nom: "source_attestee : Verifiee faux mais attestation cohérente, autorisé",
			jeu: jeu(dir("T", SourceAttestee, Refuser, func(d *Directive) { d.Cible = Cible{Serveur: "banque", Outil: "*"} })),
			appel: func() Appel {
				a := appel(base)
				a.Attestation = Attestation{Mode: "binaire", Attendu: "a", Observe: "a", Schemas: true, Verifiee: false}
				return a
			}(), effet: Autoriser,
		},
		{
			nom:   "source_attestee : serveur attesté",
			jeu:   jeu(dir("T", SourceAttestee, Refuser, func(d *Directive) { d.Cible = Cible{Serveur: "banque", Outil: "*"} })),
			appel: appel(base), effet: Autoriser,
		},

		// Périmètre.
		{nom: "périmètre : appelant hors liste, directive ignorée", jeu: jeu(dir("I", Interdiction, Refuser, func(d *Directive) {
			d.Perimetre = &Perimetre{Appelants: []string{"spiffe://lab/a"}}
		})), etat: EtatSession{Appelant: "spiffe://lab/b"}, appel: appel(base), effet: Autoriser,
			verif: func(t *testing.T, d Decision) {
				if len(d.Applications) != 0 {
					t.Error("hors périmètre ne doit pas être journalisé comme applicable")
				}
			}},
		{nom: "périmètre : appelant dans la liste", jeu: jeu(dir("I", Interdiction, Refuser, func(d *Directive) {
			d.Perimetre = &Perimetre{Appelants: []string{"spiffe://lab/a"}}
		})), etat: EtatSession{Appelant: "spiffe://lab/a"}, appel: appel(base), effet: Refuser, appliquees: []string{"I"}},
		{nom: "périmètre : appelant non prouvé, appliqué par défaut", jeu: jeu(dir("I", Interdiction, Refuser, func(d *Directive) {
			d.Perimetre = &Perimetre{Appelants: []string{"spiffe://lab/a"}}
		})), appel: appel(base), effet: Refuser, appliquees: []string{"I"},
			verif: func(t *testing.T, d Decision) {
				if !strings.Contains(d.Applications[0].Motif, "non prouvé") {
					t.Errorf("motif %q", d.Applications[0].Motif)
				}
			}},

		// Échéance de relecture.
		{nom: "révision échue : alerte, directive maintenue", jeu: jeu(dir("I", Interdiction, Refuser, func(d *Directive) {
			d.DateEffet = t0.Add(-2 * time.Hour)
			r := t0.Add(-time.Hour)
			d.RevisionAvant = &r
		})), appel: appel(base), effet: Refuser, appliquees: []string{"I"},
			verif: func(t *testing.T, d Decision) {
				if len(d.Alertes) != 1 || !d.Applications[0].RevisionEchue {
					t.Errorf("alertes %v, application %+v", d.Alertes, d.Applications[0])
				}
			}},

		// Cible.
		{nom: "cible joker", jeu: jeu(dir("I", Interdiction, Refuser, func(d *Directive) { d.Cible = Cible{Serveur: "*", Outil: "*"} })),
			appel: appel(base), effet: Refuser, appliquees: []string{"I"}},
		{nom: "cible autre outil", jeu: jeu(dir("I", Interdiction, Refuser, func(d *Directive) { d.Cible.Outil = "autre" })),
			appel: appel(base), effet: Autoriser},
	}

	for _, c := range cases {
		t.Run(c.nom, func(t *testing.T) {
			d := Decide(c.jeu, c.etat, c.appel)
			if d.Effet != c.effet {
				t.Fatalf("effet %q, attendu %q (motif %q)", d.Effet, c.effet, d.Motif)
			}
			if c.args != nil && !reflect.DeepEqual(d.Arguments, c.args) {
				t.Errorf("arguments %v, attendu %v", d.Arguments, c.args)
			}
			if c.retires != nil && !reflect.DeepEqual(d.ChampsRetires, c.retires) {
				t.Errorf("champs retirés %v, attendu %v", d.ChampsRetires, c.retires)
			}
			var got []string
			for _, a := range d.Applications {
				if a.Appliquee {
					got = append(got, a.ID)
				}
			}
			if len(c.appliquees) > 0 || len(got) > 0 {
				if !reflect.DeepEqual(got, c.appliquees) {
					t.Errorf("appliquées %v, attendu %v", got, c.appliquees)
				}
			}
			if d.EmpreinteTransmise != approval.Digest(c.appel.Outil, d.Arguments) {
				t.Error("EmpreinteTransmise doit porter sur Decision.Arguments")
			}
			if c.verif != nil {
				c.verif(t, d)
			}
		})
	}
}

// Le scénario de la démo, sur le jeu d'exemple : A passe, B attend, C est
// bloqué, l'imposteur est rejeté, le partenaire ne reçoit que le SIREN.
func TestDecide_ScenarioDemo(t *testing.T) {
	jeu, err := chargerTestdata(t)
	if err != nil {
		t.Fatal(err)
	}
	pret := func(montant float64) Appel {
		return Appel{Serveur: "banque-synthetique", Outil: "attribuer_pret",
			Arguments:   map[string]any{"montant": montant, "siren": "552100554"},
			Attestation: Attestation{Mode: "binaire", Verifiee: true, Schemas: true}, Horodatage: t0}
	}
	vierge := EtatSession{ID: "s1", Appelant: "spiffe://lab/sous-agent-credit"}
	apresTaux := vierge
	apresTaux.Consultations = []Consultation{{Serveur: "taux-officiel", Outil: "get_taux_usure", Resultat: ResultatOK, Empreinte: "x", EmpreinteReponse: "y"}}

	if d := Decide(*jeu, vierge, pret(200000)); d.Effet != Autoriser {
		t.Errorf("A: %q %s", d.Effet, d.Motif)
	}
	if d := Decide(*jeu, apresTaux, pret(200000)); d.Effet != AttendreValidation || !strings.Contains(d.Motif, "D-001") {
		t.Errorf("B: %q %s", d.Effet, d.Motif)
	}
	if d := Decide(*jeu, apresTaux, pret(800000)); d.Effet != Refuser || !strings.Contains(d.Motif, "D-002") {
		t.Errorf("C: %q %s", d.Effet, d.Motif)
	}
	imposteur := Appel{Serveur: "taux-officiel", Outil: "get_taux_usure", Horodatage: t0,
		Attestation: Attestation{Mode: "binaire", Attendu: "sha256:bon", Observe: "sha256:autre"}}
	if d := Decide(*jeu, vierge, imposteur); d.Effet != Refuser || !strings.Contains(d.Motif, "D-003") {
		t.Errorf("imposteur: %q %s", d.Effet, d.Motif)
	}
	partenaire := Appel{Serveur: "recherche-entreprises", Outil: "verifier", Horodatage: t0,
		Arguments:   map[string]any{"siren": "552100554", "nom": "Dupont", "montant": 200000.0},
		Attestation: Attestation{Verifiee: true, Schemas: true}}
	d := Decide(*jeu, vierge, partenaire)
	if d.Effet != Caviarder || !reflect.DeepEqual(d.Arguments, map[string]any{"siren": "552100554"}) {
		t.Errorf("partenaire: %q %v %s", d.Effet, d.Arguments, d.Motif)
	}
}

func TestControlesEnVigueur(t *testing.T) {
	c := ControlesEnVigueur([]EntreeRegistre{
		{Serveur: "taux-officiel", Transport: "stdio", ModeAttestation: "binaire", Attendu: "sha256:ab", Outils: map[string]string{"get_taux_usure": "read"}, SchemasEpingles: true},
		{Serveur: "banque-synthetique", Transport: "stdio", ModeAttestation: "binaire", Attendu: "sha256:cd", Outils: map[string]string{"attribuer_pret": "action", "evaluer": "read"}},
	})
	if len(c) == 0 || c[0].Serveur != "banque-synthetique" || c[0].Origine != "registre" {
		t.Fatalf("contrôles %+v", c)
	}
	var outils []string
	for _, x := range c {
		if x.Serveur == "banque-synthetique" && x.Controle == "outil_autorise" {
			outils = append(outils, x.Valeur)
		}
	}
	if !reflect.DeepEqual(outils, []string{"attribuer_pret (action)", "evaluer (read)"}) {
		t.Errorf("outils %v", outils)
	}
}
