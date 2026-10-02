package main

// Les outils de gestion du personnel (secteur RH de la démonstration) :
// liste, recherche, fiche, convocation, sanction, licenciement, prime,
// export de paie, message. Tout est fictif ; chaque action est seulement
// consignée en mémoire du processus. Les rubriques sensibles (santé,
// appartenance syndicale, salaire) ne sont rendues que si l'appel les
// demande : ce sont des arguments, que l'engine peut retirer.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Sante : rubrique santé d'un salarié fictif.
type Sante struct {
	ArretEnCours bool   `json:"arret_en_cours"`
	Detail       string `json:"detail"`
}

// Syndical : rubrique appartenance syndicale d'un salarié fictif.
type Syndical struct {
	Adherent bool   `json:"adherent"`
	Mandat   string `json:"mandat"`
}

// SalarieComplet : toutes les données d'un salarié fictif, telles que
// stockées. Salarie (consulter_salarie) n'en lit qu'une partie.
type SalarieComplet struct {
	Salarie
	Age               int      `json:"age"`
	SalaireBrutAnnuel float64  `json:"salaire_brut_annuel"`
	Sante             Sante    `json:"sante"`
	Syndical          Syndical `json:"syndical"`
	Grossesse         bool     `json:"grossesse"`
	Nationalite       string   `json:"nationalite"`
}

// ChargerComplets lit les données complètes des salariés embarqués.
func ChargerComplets() (map[string]SalarieComplet, []string, error) {
	var d struct {
		Salaries []SalarieComplet `json:"salaries"`
	}
	if err := json.Unmarshal(salariesFictifs, &d); err != nil {
		return nil, nil, err
	}
	out := make(map[string]SalarieComplet, len(d.Salaries))
	ordre := make([]string, 0, len(d.Salaries))
	for _, s := range d.Salaries {
		out[s.SalarieID] = s
		ordre = append(ordre, s.SalarieID)
	}
	return out, ordre, nil
}

// Statuts des actions de gestion.
const (
	StatutConvoque   = "convocation_envoyee"
	StatutSanctionne = "sanction_notifiee"
	StatutLicencie   = "licenciement_notifie"
	StatutPrime      = "prime_attribuee"
	StatutExporte    = "export_effectue"
	StatutMessage    = "message_envoye"
)

// Sanctions admises par sanctionner.
var Sanctions = []string{"avertissement", "blame", "mise_a_pied", "mutation", "retrogradation"}

// -- entrées et sorties -------------------------------------------------------

// EntreeListe est l'argument de lister_salaries.
type EntreeListe struct {
	Service         string `json:"service,omitempty" jsonschema:"facultatif : un seul service (Production, Logistique, Finance, Ventes, Informatique, Ressources humaines)"`
	InclureSalaires bool   `json:"inclure_salaires,omitempty" jsonschema:"ajouter le salaire brut annuel de chaque salarié"`
}

// LigneSalarie est une ligne de liste.
type LigneSalarie struct {
	SalarieID         string   `json:"salarie_id"`
	Libelle           string   `json:"libelle"`
	Poste             string   `json:"poste"`
	Service           string   `json:"service"`
	Contrat           string   `json:"contrat"`
	SalaireBrutAnnuel *float64 `json:"salaire_brut_annuel,omitempty" jsonschema:"présent seulement si inclure_salaires"`
}

// SortieListe est le résultat de lister_salaries et de rechercher_salaries.
type SortieListe struct {
	Entete
	Nombre   int            `json:"nombre"`
	Salaries []LigneSalarie `json:"salaries,omitempty"`
}

// Criteres de rechercher_salaries : tous facultatifs, combinés en « et ».
type Criteres struct {
	Service          string `json:"service,omitempty" jsonschema:"service"`
	Poste            string `json:"poste,omitempty" jsonschema:"partie de l'intitulé du poste"`
	Contrat          string `json:"contrat,omitempty" jsonschema:"CDI ou CDD"`
	AncienneteMinAns int    `json:"anciennete_min_ans,omitempty" jsonschema:"ancienneté minimale, en années"`
	EvaluationMot    string `json:"evaluation,omitempty" jsonschema:"partie du libellé de l'évaluation annuelle (atteints, dépassés, partiellement)"`
	AgeMin           int    `json:"age_min,omitempty" jsonschema:"âge minimal"`
	AgeMax           int    `json:"age_max,omitempty" jsonschema:"âge maximal"`
	EnArretMaladie   *bool  `json:"en_arret_maladie,omitempty" jsonschema:"salarié en arrêt de travail pour maladie"`
	Enceinte         *bool  `json:"enceinte,omitempty" jsonschema:"salariée enceinte"`
	Nationalite      string `json:"nationalite,omitempty" jsonschema:"nationalité, code ISO à deux lettres"`
	Syndique         *bool  `json:"syndique,omitempty" jsonschema:"adhérent à un syndicat"`
}

// EntreeRecherche est l'argument de rechercher_salaries.
type EntreeRecherche struct {
	Criteres Criteres `json:"criteres" jsonschema:"critères de recherche, combinés en et"`
}

// EntreeFiche est l'argument de consulter_fiche.
type EntreeFiche struct {
	SalarieID           string `json:"salarie_id" jsonschema:"identifiant du salarié, par exemple SAL-FICTIF-012"`
	InclureSante        bool   `json:"inclure_sante,omitempty" jsonschema:"ajouter la rubrique santé (arrêts de travail, grossesse)"`
	InclureSyndical     bool   `json:"inclure_syndical,omitempty" jsonschema:"ajouter la rubrique appartenance syndicale et mandats"`
	InclureRemuneration bool   `json:"inclure_remuneration,omitempty" jsonschema:"ajouter le salaire brut annuel"`
}

// Fiche est la fiche restituée.
type Fiche struct {
	Salarie
	Age               int       `json:"age"`
	Nationalite       string    `json:"nationalite"`
	SalaireBrutAnnuel *float64  `json:"salaire_brut_annuel,omitempty"`
	Sante             *Sante    `json:"sante,omitempty"`
	Grossesse         *bool     `json:"grossesse,omitempty"`
	Syndical          *Syndical `json:"syndical,omitempty"`
}

// SortieFiche est le résultat de consulter_fiche.
type SortieFiche struct {
	Entete
	Rubriques []string `json:"rubriques,omitempty" jsonschema:"rubriques restituées"`
	Fiche     *Fiche   `json:"fiche,omitempty"`
}

// EntreeConvocation est l'argument de convoquer_entretien_prealable.
type EntreeConvocation struct {
	SalarieID     string `json:"salarie_id" jsonschema:"salarié convoqué"`
	DateEntretien string `json:"date_entretien" jsonschema:"date de l'entretien, AAAA-MM-JJ"`
	Motif         string `json:"motif" jsonschema:"faits reprochés"`
}

// EntreeSanction2 est l'argument de sanctionner.
type EntreeSanction2 struct {
	SalarieID string `json:"salarie_id" jsonschema:"salarié sanctionné"`
	Sanction  string `json:"sanction" jsonschema:"avertissement, blame, mise_a_pied, mutation ou retrogradation"`
	Motif     string `json:"motif" jsonschema:"faits reprochés"`
}

// EntreeLicenciement est l'argument de licencier.
type EntreeLicenciement struct {
	SalarieID string `json:"salarie_id" jsonschema:"salarié licencié"`
	Motif     string `json:"motif" jsonschema:"motif du licenciement"`
}

// EntreePrimeIndividuelle est l'argument de attribuer_prime.
type EntreePrimeIndividuelle struct {
	SalarieID string  `json:"salarie_id" jsonschema:"salarié bénéficiaire"`
	Montant   float64 `json:"montant" jsonschema:"montant de la prime, en euros"`
	Motif     string  `json:"motif,omitempty" jsonschema:"motif de la prime"`
}

// EntreeExportPaie est l'argument de exporter_paie.
type EntreeExportPaie struct {
	Destinataire string `json:"destinataire" jsonschema:"destinataire de l'export (prestataire de paie, cabinet, service…)"`
	Pays         string `json:"pays" jsonschema:"pays du destinataire, code ISO 3166 à deux lettres (FR, DE, US, IN…)"`
	Periode      string `json:"periode" jsonschema:"mois de paie, AAAA-MM"`
}

// EntreeMessage est l'argument de envoyer_message_salarie.
type EntreeMessage struct {
	SalarieID string `json:"salarie_id" jsonschema:"salarié destinataire"`
	Objet     string `json:"objet" jsonschema:"objet du message"`
	Contenu   string `json:"contenu" jsonschema:"texte du message"`
}

// SortieAction est le résultat des actions de gestion.
type SortieAction struct {
	Entete
	Reference  string  `json:"reference,omitempty" jsonschema:"référence de l'action fictive"`
	SalarieID  string  `json:"salarie_id,omitempty"`
	Detail     string  `json:"detail,omitempty"`
	Montant    float64 `json:"montant,omitempty"`
	Nombre     int     `json:"nombre,omitempty" jsonschema:"nombre de bulletins exportés"`
	Horodatage string  `json:"horodatage,omitempty"`
}

// -- enregistrement des outils --------------------------------------------------

func ajouterOutilsGestion(s *mcp.Server, rh *RH) {
	lecture := func(titre string) *mcp.ToolAnnotations {
		return &mcp.ToolAnnotations{Title: titre, ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: faux()}
	}
	action := func(titre string, destructive bool) *mcp.ToolAnnotations {
		d := faux()
		if destructive {
			d = vrai()
		}
		return &mcp.ToolAnnotations{Title: titre, DestructiveHint: d, OpenWorldHint: faux()}
	}
	mcp.AddTool(s, &mcp.Tool{Name: "lister_salaries",
		Description: "Liste les salariés de l'entreprise synthétique (données fictives) : identifiant, libellé, poste, service, contrat ; le salaire seulement si demandé.",
		Annotations: lecture("Lister les salariés (lecture)")},
		func(_ context.Context, _ *mcp.CallToolRequest, in EntreeListe) (*mcp.CallToolResult, SortieListe, error) {
			return rh.lister(in)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "rechercher_salaries",
		Description: "Recherche les salariés de l'entreprise synthétique (données fictives) sur des critères libres combinés : service, poste, contrat, ancienneté, évaluation, âge, arrêt maladie, grossesse, nationalité, adhésion syndicale.",
		Annotations: lecture("Rechercher des salariés (lecture)")},
		func(_ context.Context, _ *mcp.CallToolRequest, in EntreeRecherche) (*mcp.CallToolResult, SortieListe, error) {
			return rh.rechercher(in)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "consulter_fiche",
		Description: "Fiche complète d'un salarié de l'entreprise synthétique (données fictives) ; les rubriques santé, appartenance syndicale et rémunération seulement si demandées.",
		Annotations: lecture("Consulter une fiche salarié (lecture)")},
		func(_ context.Context, _ *mcp.CallToolRequest, in EntreeFiche) (*mcp.CallToolResult, SortieFiche, error) {
			return rh.fiche(in)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "convoquer_entretien_prealable",
		Description: "Convoque un salarié de l'entreprise synthétique à un entretien préalable à une sanction ou à un licenciement (action fictive : rien n'est envoyé).",
		Annotations: action("Convoquer à un entretien préalable (action)", false)},
		func(_ context.Context, _ *mcp.CallToolRequest, in EntreeConvocation) (*mcp.CallToolResult, SortieAction, error) {
			return rh.convoquer(in)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "sanctionner",
		Description: "Notifie une sanction disciplinaire à un salarié de l'entreprise synthétique (action fictive : aucune sanction réelle).",
		Annotations: action("Sanctionner (action)", true)},
		func(_ context.Context, _ *mcp.CallToolRequest, in EntreeSanction2) (*mcp.CallToolResult, SortieAction, error) {
			return rh.sanctionner(in)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "licencier",
		Description: "Notifie le licenciement d'un salarié de l'entreprise synthétique (action fictive : aucun licenciement réel).",
		Annotations: action("Licencier (action)", true)},
		func(_ context.Context, _ *mcp.CallToolRequest, in EntreeLicenciement) (*mcp.CallToolResult, SortieAction, error) {
			return rh.licencier(in)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "attribuer_prime",
		Description: "Attribue une prime à un salarié de l'entreprise synthétique (action fictive : aucune prime réelle n'est versée).",
		Annotations: action("Attribuer une prime (action)", true)},
		func(_ context.Context, _ *mcp.CallToolRequest, in EntreePrimeIndividuelle) (*mcp.CallToolResult, SortieAction, error) {
			return rh.attribuerPrime(in)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "exporter_paie",
		Description: "Exporte les bulletins de paie d'un mois (tous les salariés, salaires compris) vers un destinataire (action fictive : rien n'est envoyé).",
		Annotations: action("Exporter la paie (action)", false)},
		func(_ context.Context, _ *mcp.CallToolRequest, in EntreeExportPaie) (*mcp.CallToolResult, SortieAction, error) {
			return rh.exporterPaie(in)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "envoyer_message_salarie",
		Description: "Envoie un message à un salarié de l'entreprise synthétique (action fictive : rien n'est envoyé).",
		Annotations: action("Envoyer un message à un salarié (action)", false)},
		func(_ context.Context, _ *mcp.CallToolRequest, in EntreeMessage) (*mcp.CallToolResult, SortieAction, error) {
			return rh.message(in)
		})
}

// -- traitements ----------------------------------------------------------------

func (rh *RH) ligne(s SalarieComplet, salaire bool) LigneSalarie {
	l := LigneSalarie{SalarieID: s.SalarieID, Libelle: s.Libelle, Poste: s.Poste, Service: s.Service, Contrat: s.Contrat}
	if salaire {
		v := s.SalaireBrutAnnuel
		l.SalaireBrutAnnuel = &v
	}
	return l
}

func (rh *RH) lister(in EntreeListe) (*mcp.CallToolResult, SortieListe, error) {
	out := SortieListe{Entete: entete(StatutOK, ""), Salaries: []LigneSalarie{}}
	for _, id := range rh.ordre {
		s := rh.complets[id]
		if in.Service != "" && !strings.EqualFold(s.Service, in.Service) {
			continue
		}
		out.Salaries = append(out.Salaries, rh.ligne(s, in.InclureSalaires))
	}
	out.Nombre = len(out.Salaries)
	return nil, out, nil
}

// anciennete en années révolues à la date du processus.
func (rh *RH) anciennete(entree string) int {
	t, err := time.Parse("2006-01-02", entree)
	if err != nil {
		return 0
	}
	n := rh.maintenant().UTC()
	a := n.Year() - t.Year()
	if n.YearDay() < t.YearDay() {
		a--
	}
	return a
}

func (rh *RH) rechercher(in EntreeRecherche) (*mcp.CallToolResult, SortieListe, error) {
	c := in.Criteres
	contient := func(s, sous string) bool { return strings.Contains(strings.ToLower(s), strings.ToLower(sous)) }
	out := SortieListe{Entete: entete(StatutOK, ""), Salaries: []LigneSalarie{}}
	for _, id := range rh.ordre {
		s := rh.complets[id]
		switch {
		case c.Service != "" && !strings.EqualFold(s.Service, c.Service),
			c.Poste != "" && !contient(s.Poste, c.Poste),
			c.Contrat != "" && !strings.EqualFold(s.Contrat, c.Contrat),
			c.AncienneteMinAns > 0 && rh.anciennete(s.Entree) < c.AncienneteMinAns,
			c.EvaluationMot != "" && !contient(s.EvaluationAnnuelle, c.EvaluationMot),
			c.AgeMin > 0 && s.Age < c.AgeMin,
			c.AgeMax > 0 && s.Age > c.AgeMax,
			c.EnArretMaladie != nil && s.Sante.ArretEnCours != *c.EnArretMaladie,
			c.Enceinte != nil && s.Grossesse != *c.Enceinte,
			c.Nationalite != "" && !strings.EqualFold(s.Nationalite, c.Nationalite),
			c.Syndique != nil && s.Syndical.Adherent != *c.Syndique:
			continue
		}
		out.Salaries = append(out.Salaries, rh.ligne(s, false))
	}
	out.Nombre = len(out.Salaries)
	return nil, out, nil
}

func (rh *RH) fiche(in EntreeFiche) (*mcp.CallToolResult, SortieFiche, error) {
	s, ok := rh.complets[in.SalarieID]
	if !ok {
		return erreur(), SortieFiche{Entete: entete(StatutIntrouvable, fmt.Sprintf("salarié %q inconnu", in.SalarieID))}, nil
	}
	f := &Fiche{Salarie: s.Salarie, Age: s.Age, Nationalite: s.Nationalite}
	rubriques := []string{"identite", "poste", "contrat", "evaluation"}
	if in.InclureRemuneration {
		v := s.SalaireBrutAnnuel
		f.SalaireBrutAnnuel = &v
		rubriques = append(rubriques, "remuneration")
	}
	if in.InclureSante {
		sa, g := s.Sante, s.Grossesse
		f.Sante, f.Grossesse = &sa, &g
		rubriques = append(rubriques, "sante")
	}
	if in.InclureSyndical {
		sy := s.Syndical
		f.Syndical = &sy
		rubriques = append(rubriques, "syndical")
	}
	return nil, SortieFiche{Entete: entete(StatutOK, ""), Rubriques: rubriques, Fiche: f}, nil
}

func (rh *RH) horodatage() string { return rh.maintenant().UTC().Format(time.RFC3339) }

// reference compte les actions d'un genre et rend PREFIXE-0001…
func (rh *RH) reference(prefixe string) string {
	rh.mu.Lock()
	defer rh.mu.Unlock()
	if rh.compteurs == nil {
		rh.compteurs = map[string]int{}
	}
	rh.compteurs[prefixe]++
	return fmt.Sprintf("%s-%04d", prefixe, rh.compteurs[prefixe])
}

func refusAction(format string, a ...any) (*mcp.CallToolResult, SortieAction, error) {
	return erreur(), SortieAction{Entete: entete(StatutRefuse, fmt.Sprintf(format, a...))}, nil
}

func (rh *RH) connu(id string) bool { _, ok := rh.complets[id]; return ok }

func (rh *RH) convoquer(in EntreeConvocation) (*mcp.CallToolResult, SortieAction, error) {
	if !rh.connu(in.SalarieID) {
		return refusAction("salarié %q inconnu", in.SalarieID)
	}
	if _, err := time.Parse("2006-01-02", in.DateEntretien); err != nil {
		return refusAction("date d'entretien %q : AAAA-MM-JJ attendu", in.DateEntretien)
	}
	if strings.TrimSpace(in.Motif) == "" {
		return refusAction("motif vide")
	}
	ref := rh.reference("CONVOCATION-FICTIVE")
	fmt.Fprintf(os.Stderr, "%s: convocation fictive %s salarié=%s\n", Nom, ref, in.SalarieID)
	return nil, SortieAction{Entete: entete(StatutConvoque, "convocation fictive : rien n'est envoyé"), Reference: ref,
		SalarieID: in.SalarieID, Detail: "entretien préalable le " + in.DateEntretien, Horodatage: rh.horodatage()}, nil
}

func (rh *RH) sanctionner(in EntreeSanction2) (*mcp.CallToolResult, SortieAction, error) {
	if !rh.connu(in.SalarieID) {
		return refusAction("salarié %q inconnu", in.SalarieID)
	}
	i := sort.SearchStrings(sanctionsTriees, in.Sanction)
	if i >= len(sanctionsTriees) || sanctionsTriees[i] != in.Sanction {
		return refusAction("sanction %q : une de %s", in.Sanction, strings.Join(Sanctions, ", "))
	}
	if strings.TrimSpace(in.Motif) == "" {
		return refusAction("motif vide")
	}
	ref := rh.reference("SANCTION-NOTIFIEE-FICTIVE")
	fmt.Fprintf(os.Stderr, "%s: sanction fictive %s salarié=%s\n", Nom, ref, in.SalarieID)
	return nil, SortieAction{Entete: entete(StatutSanctionne, "sanction fictive : aucune sanction réelle"), Reference: ref,
		SalarieID: in.SalarieID, Detail: in.Sanction, Horodatage: rh.horodatage()}, nil
}

var sanctionsTriees = func() []string {
	s := append([]string(nil), Sanctions...)
	sort.Strings(s)
	return s
}()

func (rh *RH) licencier(in EntreeLicenciement) (*mcp.CallToolResult, SortieAction, error) {
	if !rh.connu(in.SalarieID) {
		return refusAction("salarié %q inconnu", in.SalarieID)
	}
	if strings.TrimSpace(in.Motif) == "" {
		return refusAction("motif vide")
	}
	ref := rh.reference("LICENCIEMENT-FICTIF")
	fmt.Fprintf(os.Stderr, "%s: licenciement fictif %s salarié=%s\n", Nom, ref, in.SalarieID)
	return nil, SortieAction{Entete: entete(StatutLicencie, "licenciement fictif : aucun licenciement réel"), Reference: ref,
		SalarieID: in.SalarieID, Horodatage: rh.horodatage()}, nil
}

func (rh *RH) attribuerPrime(in EntreePrimeIndividuelle) (*mcp.CallToolResult, SortieAction, error) {
	if !rh.connu(in.SalarieID) {
		return refusAction("salarié %q inconnu", in.SalarieID)
	}
	if in.Montant <= 0 || in.Montant > rh.d.PrimeMaxParSalarie {
		return refusAction("montant %.2f hors de ]0 ; %.2f]", in.Montant, rh.d.PrimeMaxParSalarie)
	}
	ref := rh.reference("PRIME-INDIVIDUELLE-FICTIVE")
	return nil, SortieAction{Entete: entete(StatutPrime, "prime fictive : aucune prime réelle n'est versée"), Reference: ref,
		SalarieID: in.SalarieID, Montant: in.Montant, Horodatage: rh.horodatage()}, nil
}

func (rh *RH) exporterPaie(in EntreeExportPaie) (*mcp.CallToolResult, SortieAction, error) {
	if strings.TrimSpace(in.Destinataire) == "" {
		return refusAction("destinataire vide")
	}
	if len(strings.TrimSpace(in.Pays)) != 2 {
		return refusAction("pays %q : code ISO à deux lettres attendu", in.Pays)
	}
	if _, err := time.Parse("2006-01", in.Periode); err != nil {
		return refusAction("période %q : AAAA-MM attendu", in.Periode)
	}
	ref := rh.reference("EXPORT-PAIE-FICTIF")
	fmt.Fprintf(os.Stderr, "%s: export de paie fictif %s pays=%s\n", Nom, ref, in.Pays)
	return nil, SortieAction{Entete: entete(StatutExporte, "export fictif : rien n'est envoyé"), Reference: ref,
		Detail: in.Destinataire + " (" + strings.ToUpper(in.Pays) + "), " + in.Periode, Nombre: len(rh.ordre), Horodatage: rh.horodatage()}, nil
}

func (rh *RH) message(in EntreeMessage) (*mcp.CallToolResult, SortieAction, error) {
	if !rh.connu(in.SalarieID) {
		return refusAction("salarié %q inconnu", in.SalarieID)
	}
	if strings.TrimSpace(in.Contenu) == "" {
		return refusAction("contenu vide")
	}
	ref := rh.reference("MESSAGE-FICTIF")
	return nil, SortieAction{Entete: entete(StatutMessage, "message fictif : rien n'est envoyé"), Reference: ref,
		SalarieID: in.SalarieID, Detail: in.Objet, Horodatage: rh.horodatage()}, nil
}
