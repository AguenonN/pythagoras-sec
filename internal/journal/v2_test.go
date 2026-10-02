package journal

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"mcprogue/internal/chaine"
	"mcprogue/internal/directive"
)

// Les sorties v2 d'une décision passent par le journal et en reviennent
// à l'identique.
func TestDecision_AllerRetourV2(t *testing.T) {
	d := directive.Decision{
		Effet: directive.AttendreValidation, Motif: "m",
		Applications: []directive.Application{{
			ID: "T-001", Version: 1, Fondement: directive.Fondement{Texte: "a"}, Statut: directive.Active,
			Effet: directive.AttendreValidation, Declenchee: true, Appliquee: true, Motif: "x",
			Resultat: directive.Indetermine, Fondements: []directive.Fondement{{Texte: "a"}, {Texte: "b", Article: "2"}},
		}},
		Notifications: []directive.Notification{{Directive: "T-001", Version: 1, DestinataireRole: "juridique",
			Rythme: directive.TempsReel, SiEchec: directive.EchecRefuser}},
		Marques:    []directive.Marque{{Etiquette: "revue", Directive: "T-001", Version: 1}},
		Validation: &directive.Validation{Roles: []string{"directeur-risques"}, QuatreYeux: true},
	}
	b, err := json.Marshal(DepuisDecision(d))
	if err != nil {
		t.Fatal(err)
	}
	var relue Decision
	if err := json.Unmarshal(b, &relue); err != nil {
		t.Fatal(err)
	}
	if got := relue.VersDirective(); !reflect.DeepEqual(got, d) {
		t.Fatalf("aller-retour\n%+v\n%+v", got, d)
	}
	// Une application sans résultat se relit faux.
	if (&Decision{Applications: []Application{{ID: "D-1"}}}).VersDirective().Applications[0].Resultat != directive.Faux {
		t.Fatal("résultat absent")
	}
}

// La remise compte les abonnés qui avaient la place ; un abonné saturé
// ne compte pas. Synthèse sans conséquence : planifiée.
func TestEcrireRemise(t *testing.T) {
	e := Nouveau(chaine.Nouvelle(&bytes.Buffer{}, bytes.Repeat([]byte("k"), 32)))
	n := Notification{Directive: "N", Version: 1, DestinataireRole: "juridique", Rythme: "temps_reel", SiEchec: "refuser"}
	en, err := e.EcrireRemise(Entree{Notification: &Remise{Notification: n}})
	if err != nil || en.Type != NotificationT || en.Notification.Statut != RemiseEchec || en.Notification.Remis != 0 || en.MAC == "" {
		t.Fatalf("sans abonné : %+v %v", en.Notification, err)
	}
	plein, fin1 := e.Abonner(1)
	defer fin1()
	e.Ecrire(Entree{Type: AlerteT}) // remplit l'abonné
	ok, fin2 := e.Abonner(4)
	defer fin2()
	en, _ = e.EcrireRemise(Entree{Notification: &Remise{Notification: n}})
	if en.Notification.Statut != RemiseFaite || en.Notification.Remis != 1 || len(ok) != 1 || len(plein) != 1 {
		t.Fatalf("un abonné libre : %+v", en.Notification)
	}
	n.Rythme, n.SiEchec = "synthese", "continuer"
	en, _ = e.EcrireRemise(Entree{Notification: &Remise{Notification: n}})
	if en.Notification.Statut != RemisePlanifiee {
		t.Fatalf("synthèse : %+v", en.Notification)
	}
	if _, err := e.EcrireRemise(Entree{}); err == nil {
		t.Fatal("entrée sans notification acceptée")
	}
}

func TestControleNotifications(t *testing.T) {
	d := directive.Decision{Effet: directive.Autoriser, Motif: "m"}
	r := Remise{Notification: Notification{Directive: "N", SiEchec: "refuser"}, Statut: RemiseEchec}
	if got, par := ControleNotifications(d, []Remise{r}); !par || got.Effet != directive.Refuser {
		t.Fatal("échec si_echec refuser non refusé")
	}
	r.SiEchec = "continuer"
	if _, par := ControleNotifications(d, []Remise{r}); par {
		t.Fatal("continuer refusé")
	}
	r.SiEchec, r.Statut = "refuser", RemiseFaite
	if _, par := ControleNotifications(d, []Remise{r}); par {
		t.Fatal("remise refusée")
	}
	d.Effet = directive.Refuser
	r.Statut = RemiseEchec
	if got, par := ControleNotifications(d, []Remise{r}); par || got.Motif != "m" {
		t.Fatal("un refus de directive garde la main")
	}
}
