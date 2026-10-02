package main

import (
	"os"
	"path/filepath"
	"testing"

	"mcprogue/internal/directive"
	"mcprogue/internal/registre"
)

// Le registre et le jeu du lab sont acceptés par les chargeurs du gate.
func TestRegistreEtJeuDuLab(t *testing.T) {
	b, err := Generer(filepath.Join("..", "..", "..", "servers", "banque-synthetique", "schema.json"),
		"https://banque-synthetique.mcp-lab.svc.cluster.local:8443/mcp", "spiffe://mcp.gcp.lab/banque-synthetique")
	if err != nil {
		t.Fatal(err)
	}
	r, err := registre.Charger(b)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := r.Serveur("banque-synthetique")
	if !ok || s.Transport != registre.HTTPSpiffe || s.Attestation.Attendu != "spiffe://mcp.gcp.lab/banque-synthetique" {
		t.Fatalf("%+v", s)
	}
	if _, ok := s.Outil("attribuer_pret"); !ok {
		t.Fatal("attribuer_pret absent du registre du lab")
	}
	doc, err := os.ReadFile(filepath.Join("..", "directives-lab.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := directive.Charger(doc); err != nil {
		t.Fatal(err)
	}
}
