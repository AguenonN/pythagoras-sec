package controle

import (
	"testing"

	"mcprogue/internal/directive"
)

func TestRegistre(t *testing.T) {
	valide := directive.Attestation{Mode: "binaire", Attendu: "a", Observe: "a", Schemas: true}
	echec := directive.Attestation{Mode: "binaire", Attendu: "a", Observe: "b", Detail: "empreinte différente"}
	// Verifiee n'est jamais cru : seul Valide compte.
	menteuse := echec
	menteuse.Verifiee = true

	for _, c := range []struct {
		nom    string
		effet  directive.Effet
		att    directive.Attestation
		veut   directive.Effet
		impose bool
	}{
		{"attesté, autorisé", directive.Autoriser, valide, directive.Autoriser, false},
		{"non attesté, autorisé", directive.Autoriser, echec, directive.Refuser, true},
		{"non attesté, caviardé", directive.Caviarder, echec, directive.Refuser, true},
		{"non attesté, en attente", directive.AttendreValidation, echec, directive.Refuser, true},
		{"non attesté, déjà refusé par une directive", directive.Refuser, echec, directive.Refuser, false},
		{"Verifiee posé à la main", directive.Autoriser, menteuse, directive.Refuser, true},
	} {
		dec, impose := Registre(directive.Decision{Effet: c.effet, Motif: "m"}, "taux", c.att)
		if dec.Effet != c.veut || impose != c.impose {
			t.Errorf("%s : %s, imposé %v", c.nom, dec.Effet, impose)
		}
		if impose && dec.Motif != "refusé par le registre : le serveur taux n'est pas attesté (empreinte différente)" {
			t.Errorf("%s : motif %q", c.nom, dec.Motif)
		}
		if !impose && dec.Motif != "m" {
			t.Errorf("%s : motif modifié", c.nom)
		}
	}
}
