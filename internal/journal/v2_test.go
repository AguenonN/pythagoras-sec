package journal

import (
	"encoding/json"
	"reflect"
	"testing"

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
	// Une application d'un journal v1 n'a pas de résultat : faux.
	if (&Decision{Applications: []Application{{ID: "D-1"}}}).VersDirective().Applications[0].Resultat != directive.Faux {
		t.Fatal("résultat absent")
	}
}
