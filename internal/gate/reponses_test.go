package gate

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/internal/directive"
)

// structure relit le structuredContent tel que l'agent le reçoit.
func structure[T any](t *testing.T, r *mcp.CallToolResult) T {
	t.Helper()
	var out T
	if !r.IsError {
		t.Fatalf("isError attendu : %q", texte(r))
	}
	b, err := json.Marshal(r.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("%s : %v", b, err)
	}
	return out
}

// Les champs du contrat sont exactement ceux documentés.
func TestContrat_Champs(t *testing.T) {
	for _, c := range []struct {
		v    any
		veut []string
	}{
		{Refus{}, []string{"statut", "origine", "directive", "version", "fondement", "motif"}},
		{Attente{}, []string{"statut", "empreinte_transmise", "directive", "version"}},
	} {
		b, _ := json.Marshal(c.v)
		var m map[string]any
		json.Unmarshal(b, &m)
		if len(m) != len(c.veut) {
			t.Fatalf("%s", b)
		}
		for _, k := range c.veut {
			if _, ok := m[k]; !ok {
				t.Fatalf("%s sans %s", b, k)
			}
		}
	}
}

func TestContrat_RefusParDirective(t *testing.T) {
	b := monter(t, nil)
	cs := b.agent(t)
	appelerAgent(t, cs, "taux-officiel__get_taux", map[string]any{"devise": "EUR"})
	r := structure[Refus](t, appelerAgent(t, cs, "banque-synthetique__attribuer_pret", map[string]any{"montant": 1.0, "siren": "1"}))
	veut := directive.Fondement{Texte: "RGPD", Article: "22"}
	if r.Statut != StatutRefuse || r.Origine != OrigineRefusDirective || r.Directive != "D-001" || r.Version != 1 ||
		r.Fondement != veut || r.Motif != b.appels(t)[1].Decision.Motif {
		t.Fatalf("%+v", r)
	}
}

func TestContrat_RefusParRegistre(t *testing.T) {
	b := monter(t, func(b *banc) {
		b.banque.att = directive.Attestation{Mode: "tls-pin", Attendu: h64, Observe: "sha256:autre"}
	})
	r := structure[Refus](t, appelerAgent(t, b.agent(t), "banque-synthetique__attribuer_pret", map[string]any{"montant": 1.0, "siren": "1"}))
	if r.Statut != StatutRefuse || r.Origine != OrigineRefusRegistre || r.Directive != "test" || r.Version != 1 ||
		r.Fondement.Texte == "" || r.Motif != b.appels(t)[0].Decision.Motif {
		t.Fatalf("%+v", r)
	}
}

func TestContrat_RefusParEngine(t *testing.T) {
	b := monter(t, nil)
	for _, res := range []*mcp.CallToolResult{
		b.g.Traiter(context.Background(), nil, "taux-officiel__get_taux", []byte(`[1]`), nil),
		b.g.Traiter(context.Background(), nil, "inconnu__x", nil, nil),
	} {
		r := structure[Refus](t, res)
		if r.Statut != StatutRefuse || r.Origine != OrigineRefusEngine || r.Directive != "" || r.Version != 0 ||
			r.Fondement.Texte != "Contrat de l'engine" || r.Motif != texte(res) {
			t.Fatalf("%+v", r)
		}
	}
	// Journal en panne : même forme.
	cs := b.agent(t)
	b.journal.casser(errors.New("disque plein"))
	if r := structure[Refus](t, appelerAgent(t, cs, "taux-officiel__get_taux", map[string]any{"devise": "EUR"})); r.Origine != OrigineRefusEngine {
		t.Fatalf("%+v", r)
	}
}

func TestContrat_Attente(t *testing.T) {
	b := monter(t, nil)
	a := structure[Attente](t, appelerAgent(t, b.agent(t), "banque-synthetique__attribuer_pret", map[string]any{"montant": 600000.0, "siren": "1"}))
	if a.Statut != StatutEnAttente || a.Directive != "D-002" || a.Version != 1 || a.EmpreinteTransmise != b.appels(t)[0].Appel.EmpreinteTransmise {
		t.Fatalf("%+v", a)
	}
}

// Une source injoignable n'est pas un refus : isError sans structure.
func TestContrat_EchecNestPasUnRefus(t *testing.T) {
	b := monter(t, func(b *banc) { b.taux.err = errors.New("injoignable") })
	r := appelerAgent(t, b.agent(t), "taux-officiel__get_taux", map[string]any{"devise": "EUR"})
	if !r.IsError || r.StructuredContent != nil {
		t.Fatalf("%+v", r)
	}
}
