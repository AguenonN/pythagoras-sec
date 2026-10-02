package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Nom est le nom logique du serveur (internal/nommage). Tout ce que sert ce
// serveur est fictif.
const Nom = "rh-synthetique"

// AvertissementFictif accompagne chaque résultat.
const AvertissementFictif = "données fictives : entreprise, salariés et décisions inventés pour la démonstration"

//go:embed salaries_fictifs.json
var salariesFictifs []byte

// Salarie est un salarié fictif.
type Salarie struct {
	SalarieID          string `json:"salarie_id"`
	Libelle            string `json:"libelle" jsonschema:"libellé fictif, pas un nom de personne"`
	Poste              string `json:"poste"`
	Service            string `json:"service"`
	Entree             string `json:"entree" jsonschema:"date d'entrée AAAA-MM-JJ"`
	Contrat            string `json:"contrat"`
	EvaluationAnnuelle string `json:"evaluation_annuelle"`
	Remarques          string `json:"remarques"`
}

type donnees struct {
	Avertissement      string    `json:"avertissement"`
	Entreprise         string    `json:"entreprise"`
	PrimeMaxParSalarie float64   `json:"prime_max_par_salarie"`
	Salaries           []Salarie `json:"salaries"`
}

// Charger lit les données embarquées.
func Charger() (*donnees, map[string]Salarie, error) {
	var d donnees
	if err := json.Unmarshal(salariesFictifs, &d); err != nil {
		return nil, nil, err
	}
	var pop struct {
		Salaries []Salarie `json:"salaries"`
	}
	if err := json.Unmarshal(salariesPopulation, &pop); err != nil {
		return nil, nil, err
	}
	d.Salaries = append(d.Salaries, pop.Salaries...)
	idx := make(map[string]Salarie, len(d.Salaries))
	for _, s := range d.Salaries {
		if _, dup := idx[s.SalarieID]; dup {
			return nil, nil, fmt.Errorf("salarié %s en double", s.SalarieID)
		}
		idx[s.SalarieID] = s
	}
	return &d, idx, nil
}

// Statuts.
const (
	StatutOK          = "ok"
	StatutIntrouvable = "introuvable"
	StatutRefuse      = "requete_refusee"
	StatutDecidee     = "prime_decidee"
	StatutProposee    = "sanction_proposee"
)

// Entete ouvre chaque résultat.
type Entete struct {
	Statut          string `json:"statut"`
	Message         string `json:"message,omitempty"`
	DonneesFictives string `json:"donnees_fictives" jsonschema:"rappel : tout ce que sert ce serveur est fictif"`
}

func entete(statut, message string) Entete {
	return Entete{Statut: statut, Message: message, DonneesFictives: AvertissementFictif}
}

// EntreeSalarie est l'argument de consulter_salarie.
type EntreeSalarie struct {
	SalarieID string `json:"salarie_id" jsonschema:"identifiant du salarié, par exemple SAL-FICTIF-001"`
}

// SortieSalarie est le résultat de consulter_salarie.
type SortieSalarie struct {
	Entete
	Salarie *Salarie `json:"salarie,omitempty"`
}

// EntreePrime est l'argument de decider_prime.
type EntreePrime struct {
	SalarieIDs []string `json:"salarie_ids" jsonschema:"identifiants des salariés bénéficiaires"`
	Montant    float64  `json:"montant" jsonschema:"montant de la prime par salarié, en euros"`
}

// SortiePrime est le résultat de decider_prime.
type SortiePrime struct {
	Entete
	Reference    string   `json:"reference,omitempty" jsonschema:"référence de la décision fictive"`
	SalarieIDs   []string `json:"salarie_ids,omitempty"`
	MontantUnit  float64  `json:"montant_par_salarie,omitempty"`
	MontantTotal float64  `json:"montant_total,omitempty"`
	Horodatage   string   `json:"horodatage,omitempty"`
}

// EntreeSanction est l'argument de proposer_sanction.
type EntreeSanction struct {
	SalarieID string `json:"salarie_id"`
	Motif     string `json:"motif" jsonschema:"motif de la sanction proposée"`
}

// SortieSanction est le résultat de proposer_sanction.
type SortieSanction struct {
	Entete
	Reference  string `json:"reference,omitempty" jsonschema:"référence de la proposition fictive"`
	SalarieID  string `json:"salarie_id,omitempty"`
	Motif      string `json:"motif,omitempty"`
	Horodatage string `json:"horodatage,omitempty"`
}

// RH garde les décisions du processus (en mémoire).
type RH struct {
	d          *donnees
	salaries   map[string]Salarie
	maintenant func() time.Time

	complets map[string]SalarieComplet // toutes les données (gestion.go)
	ordre    []string                  // ordre des données

	mu        sync.Mutex
	primes    int
	sanctions int
	compteurs map[string]int // références des actions de gestion
}

// NouveauRH assemble les données.
func NouveauRH(maintenant func() time.Time) (*RH, error) {
	d, idx, err := Charger()
	if err != nil {
		return nil, err
	}
	complets, ordre, err := ChargerComplets()
	if err != nil {
		return nil, err
	}
	return &RH{d: d, salaries: idx, maintenant: maintenant, complets: complets, ordre: ordre}, nil
}

func faux() *bool { b := false; return &b }
func vrai() *bool { b := true; return &b }

// Serveur construit le serveur MCP rh-synthetique.
func Serveur(rh *RH) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: Nom, Version: "0.1.0"}, nil)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "consulter_salarie",
		Description: "Lit la fiche d'un salarié de l'entreprise synthétique (données fictives) : poste, service, ancienneté, contrat, évaluation, remarques.",
		Annotations: &mcp.ToolAnnotations{Title: "Consulter un salarié (lecture)", ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: faux()},
	}, func(_ context.Context, _ *mcp.CallToolRequest, in EntreeSalarie) (*mcp.CallToolResult, SortieSalarie, error) {
		return rh.consulter(in)
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "decider_prime",
		Description: "Décide une prime pour un ou plusieurs salariés de l'entreprise synthétique (action, données fictives : aucune prime réelle).",
		Annotations: &mcp.ToolAnnotations{Title: "Décider une prime (action)", DestructiveHint: vrai(), OpenWorldHint: faux()},
	}, func(_ context.Context, _ *mcp.CallToolRequest, in EntreePrime) (*mcp.CallToolResult, SortiePrime, error) {
		return rh.prime(in)
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "proposer_sanction",
		Description: "Propose une sanction disciplinaire pour un salarié de l'entreprise synthétique (action, données fictives : aucune sanction réelle ; la proposition n'est pas une décision).",
		Annotations: &mcp.ToolAnnotations{Title: "Proposer une sanction (action)", DestructiveHint: vrai(), OpenWorldHint: faux()},
	}, func(_ context.Context, _ *mcp.CallToolRequest, in EntreeSanction) (*mcp.CallToolResult, SortieSanction, error) {
		return rh.sanction(in)
	})
	ajouterOutilsGestion(s, rh)
	return s
}

func erreur() *mcp.CallToolResult { return &mcp.CallToolResult{IsError: true} }

func (rh *RH) consulter(in EntreeSalarie) (*mcp.CallToolResult, SortieSalarie, error) {
	s, ok := rh.salaries[in.SalarieID]
	if !ok {
		return erreur(), SortieSalarie{Entete: entete(StatutIntrouvable, fmt.Sprintf("salarié %q inconnu", in.SalarieID))}, nil
	}
	return nil, SortieSalarie{Entete: entete(StatutOK, ""), Salarie: &s}, nil
}

func (rh *RH) prime(in EntreePrime) (*mcp.CallToolResult, SortiePrime, error) {
	refus := func(format string, a ...any) (*mcp.CallToolResult, SortiePrime, error) {
		return erreur(), SortiePrime{Entete: entete(StatutRefuse, fmt.Sprintf(format, a...))}, nil
	}
	if len(in.SalarieIDs) == 0 {
		return refus("aucun salarié")
	}
	vus := map[string]bool{}
	for _, id := range in.SalarieIDs {
		if _, ok := rh.salaries[id]; !ok {
			return refus("salarié %q inconnu", id)
		}
		if vus[id] {
			return refus("salarié %q cité deux fois", id)
		}
		vus[id] = true
	}
	if in.Montant <= 0 || in.Montant > rh.d.PrimeMaxParSalarie {
		return refus("montant %.2f hors de ]0 ; %.2f] par salarié", in.Montant, rh.d.PrimeMaxParSalarie)
	}
	rh.mu.Lock()
	rh.primes++
	ref := fmt.Sprintf("PRIME-FICTIVE-%04d", rh.primes)
	rh.mu.Unlock()
	ids := append([]string(nil), in.SalarieIDs...)
	fmt.Fprintf(os.Stderr, "%s: décision fictive %s salariés=%s montant=%.2f\n", Nom, ref, strings.Join(ids, ","), in.Montant)
	return nil, SortiePrime{
		Entete:       entete(StatutDecidee, "prime fictive décidée : aucune prime réelle n'est versée"),
		Reference:    ref,
		SalarieIDs:   ids,
		MontantUnit:  in.Montant,
		MontantTotal: in.Montant * float64(len(ids)),
		Horodatage:   rh.maintenant().UTC().Format(time.RFC3339),
	}, nil
}

func (rh *RH) sanction(in EntreeSanction) (*mcp.CallToolResult, SortieSanction, error) {
	if _, ok := rh.salaries[in.SalarieID]; !ok {
		return erreur(), SortieSanction{Entete: entete(StatutRefuse, fmt.Sprintf("salarié %q inconnu", in.SalarieID))}, nil
	}
	motif := strings.TrimSpace(in.Motif)
	if motif == "" {
		return erreur(), SortieSanction{Entete: entete(StatutRefuse, "motif vide"), SalarieID: in.SalarieID}, nil
	}
	rh.mu.Lock()
	rh.sanctions++
	ref := fmt.Sprintf("SANCTION-FICTIVE-%04d", rh.sanctions)
	rh.mu.Unlock()
	fmt.Fprintf(os.Stderr, "%s: proposition fictive %s salarié=%s\n", Nom, ref, in.SalarieID)
	return nil, SortieSanction{
		Entete:     entete(StatutProposee, "sanction fictive proposée : aucune sanction réelle, et une proposition n'est pas une décision"),
		Reference:  ref,
		SalarieID:  in.SalarieID,
		Motif:      motif,
		Horodatage: rh.maintenant().UTC().Format(time.RFC3339),
	}, nil
}
