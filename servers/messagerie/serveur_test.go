package main

import (
	"context"
	"testing"

	"mcprogue/internal/nommage"
	"mcprogue/servers/internal/essai"
	modele "mcprogue/servers/modele-synthetique"
)

func TestNom(t *testing.T) {
	if err := nommage.ValiderNomLogique(Nom); err != nil {
		t.Fatal(err)
	}
}

// Les outils déclarés dans outils.json sont ceux du serveur, avec le bon
// classement lecture / action.
func TestOutilsDeclares(t *testing.T) {
	o, _, err := modele.LireFichiers("outils.json", "donnees.json")
	if err != nil {
		t.Fatal(err)
	}
	srv, err := modele.Serveur(Nom, outils, donnees)
	if err != nil {
		t.Fatal(err)
	}
	cs := essai.Connecter(t, srv)
	liste, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	vus := map[string]bool{}
	for _, v := range liste.Tools {
		vus[v.Name] = v.Annotations != nil && v.Annotations.ReadOnlyHint
	}
	for _, d := range o.Outils {
		lecture, ok := vus[d.Nom]
		if !ok || lecture != *d.ReadOnlyHint {
			t.Errorf("outil %s : annoncé=%v lecture=%v, déclaré lecture=%v", d.Nom, ok, lecture, *d.ReadOnlyHint)
		}
	}
	if len(vus) != len(o.Outils) {
		t.Errorf("%d outils annoncés, %d déclarés", len(vus), len(o.Outils))
	}
}

// schema.json est la sortie brute de tools/list : à régénérer après toute
// modification de outils.json (scripts/nouveau-serveur.sh --regenerer messagerie).
func TestSchema(t *testing.T) {
	srv, err := modele.Serveur(Nom, outils, donnees)
	if err != nil {
		t.Fatal(err)
	}
	essai.VerifierSchema(t, essai.Connecter(t, srv), "schema.json")
}
