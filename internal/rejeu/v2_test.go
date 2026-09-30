package rejeu

import (
	"encoding/json"
	"reflect"
	"testing"

	"mcprogue/internal/directive"
	"mcprogue/internal/journal"
)

// AppelDepuis reconstitue tous les champs de l'appel que Decide a lus, v2
// compris, après un aller-retour par le JSON du journal.
func TestAppelDepuis_ChampsV2(t *testing.T) {
	attendu := directive.Appel{
		Serveur: "rh-synthetique", Outil: "decider_prime", Arguments: map[string]any{"montant": 10.0},
		Attestation: directive.Attestation{Mode: "binaire", Attendu: "a", Observe: "a", Schemas: true, Verifiee: true},
		Horodatage:  t0,
		Acces:       directive.AccesAction, EtiquettesCible: []string{"donnees_personnelles"}, VersionRegistre: "demo v2 sha256:x",
		Demandeur:        &directive.Demandeur{Identite: "alice@exemple.test", Groupes: []string{"drh"}},
		Sujets:           []directive.Sujet{{Empreinte: directive.EmpreinteSujet("SAL-1"), Groupes: []string{"salaries-proteges"}}},
		SousAgentDeclare: "redacteur",
	}
	en := journal.Entree{
		TS: t0, Type: journal.AppelT, Serveur: attendu.Serveur, Outil: attendu.Outil,
		Attestation: journal.DepuisAttestation(attendu.Attestation),
		Acces:       string(attendu.Acces), EtiquettesCible: attendu.EtiquettesCible, VersionRegistre: attendu.VersionRegistre,
		Demandeur: journal.DepuisDemandeur(attendu.Demandeur), Sujets: journal.DepuisSujets(attendu.Sujets),
		SousAgentDeclare: attendu.SousAgentDeclare,
	}
	b, err := json.Marshal(en)
	if err != nil {
		t.Fatal(err)
	}
	var relue journal.Entree
	if err := json.Unmarshal(b, &relue); err != nil {
		t.Fatal(err)
	}
	if got := AppelDepuis(relue, attendu.Arguments); !reflect.DeepEqual(got, attendu) {
		t.Fatalf("appel reconstitué\n%+v\nattendu\n%+v", got, attendu)
	}
	// Un journal v1 ne porte aucun champ v2 : l'appel reconstitué n'en a
	// pas non plus (registre inconnu, demandeur non résolu).
	v1 := AppelDepuis(journal.Entree{TS: t0, Serveur: "s", Outil: "o"}, nil)
	if v1.VersionRegistre != "" || v1.Acces != "" || v1.Demandeur != nil || v1.Sujets != nil || v1.EtiquettesCible != nil {
		t.Fatalf("appel v1 : %+v", v1)
	}
}

// La consultation qu'une entrée appel produit porte les compléments v2 de
// cette entrée, sous la forme même que l'instantané de session.
func TestConsultation_ComplementsV2(t *testing.T) {
	en := journal.Entree{
		TS: t0, Type: journal.AppelT, Session: "s", Seq: 2, Serveur: "taux-officiel", Outil: "get_taux",
		Appel:           &journal.Appel{EmpreinteTransmise: "e"},
		EtiquettesCible: []string{"externe"}, VersionRegistre: "v",
		Resultat: &journal.Resultat{Statut: journal.StatutOK, Source: &journal.Source{Origine: "rejeu", Date: "2026-10-01T00:00:00Z", Empreinte: "x"}},
	}
	c, ok := consultation(en)
	if !ok {
		t.Fatal("pas de consultation")
	}
	d := journal.VersConsultations([]journal.Consultation{c})[0]
	if !reflect.DeepEqual(d.Etiquettes, []string{"externe"}) || d.VersionRegistre != "v" ||
		d.Source == nil || *d.Source != (directive.SourceReponse{Origine: "rejeu", Date: "2026-10-01T00:00:00Z"}) {
		t.Fatalf("consultation %+v", d)
	}
	en.Resultat.Source = &journal.Source{Origine: journal.OrigineNonDeclaree}
	c, _ = consultation(en)
	if c.Source != nil {
		t.Fatalf("non_declaree présentée comme une source : %+v", c.Source)
	}
}
