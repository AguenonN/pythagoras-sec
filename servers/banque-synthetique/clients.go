package main

// Les outils de la clientèle (secteur banque de la démonstration) : comptes,
// opérations, virements, plafonds, clôtures, exports, courriers,
// déclarations de soupçon et refus de crédit. Tout est fictif ; chaque
// action est seulement consignée en mémoire du processus.

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

//go:embed clients_fictifs.json
var clientsFictifs []byte

// Population générée (go run ./servers/banque-synthetique/population),
// CLI-FICTIF-007 à 076, à la suite des clients écrits à la main.
//
//go:embed clients_population.json
var clientsPopulation []byte

// EtiquettesClient : étiquettes des champs sensibles des réponses (A1),
// recopiées dans chaque réponse qui les porte.
var EtiquettesClient = map[string]string{"iban": "iban", "revenus_mensuels": "revenus", "revenus_annuels": "revenus",
	"credits": "engagements_credit", "incidents_paiement": "incidents_paiement"}

// Compte est un compte fictif.
type Compte struct {
	CompteID         string  `json:"compte_id"`
	Type             string  `json:"type"`
	IBAN             string  `json:"iban" jsonschema:"IBAN fictif"`
	Solde            float64 `json:"solde" jsonschema:"en euros"`
	PlafondDecouvert float64 `json:"plafond_decouvert" jsonschema:"découvert autorisé, en euros"`
	DecouvertUtilise float64 `json:"decouvert_utilise,omitempty" jsonschema:"découvert utilisé, en euros"`
}

// Credit est un crédit en cours, fictif.
type Credit struct {
	Type          string  `json:"type"`
	MontantInit   float64 `json:"montant_initial"`
	TauxPourcent  float64 `json:"taux_pourcent"`
	Mensualite    float64 `json:"mensualite"`
	RestantDu     float64 `json:"restant_du"`
	DateFinPrevue string  `json:"date_fin_prevue"`
}

// DemandeClient est une demande en cours du client (cas de démonstration).
type DemandeClient struct {
	Type     string  `json:"type"`
	CompteID string  `json:"compte_id"`
	Montant  float64 `json:"montant,omitempty"`
	Detail   string  `json:"detail"`
}

// Operation est une opération fictive.
type Operation struct {
	Date         string  `json:"date"`
	CompteID     string  `json:"compte_id"`
	Type         string  `json:"type,omitempty" jsonschema:"virement recu, virement emis, prelevement, carte, especes"`
	Libelle      string  `json:"libelle"`
	Montant      float64 `json:"montant" jsonschema:"en euros ; négatif pour un débit"`
	Contrepartie string  `json:"contrepartie"`
	Pays         string  `json:"pays" jsonschema:"pays de la contrepartie, code ISO 3166 à deux lettres"`
}

// Client est un client fictif, tel que stocké.
type Client struct {
	ClientID       string      `json:"client_id"`
	Nom            string      `json:"nom"`
	Type           string      `json:"type"`
	Email          string      `json:"email"`
	Adresse        string      `json:"adresse"`
	RevenusAnnuels float64     `json:"revenus_annuels"`
	Comptes        []Compte    `json:"comptes"`
	Operations     []Operation `json:"operations"`

	// Champs de la population générée (facultatifs pour les clients écrits
	// à la main).
	Siren             string          `json:"siren,omitempty"`
	DateNaissance     string          `json:"date_naissance,omitempty"`
	Telephone         string          `json:"telephone,omitempty"`
	Profession        string          `json:"profession,omitempty"`
	RevenusMensuels   float64         `json:"revenus_mensuels,omitempty"`
	Credits           []Credit        `json:"credits,omitempty"`
	IncidentsPaiement []string        `json:"incidents_paiement,omitempty"`
	CotationRisque    string          `json:"cotation_risque,omitempty"`
	Conseiller        string          `json:"conseiller,omitempty"`
	Demandes          []DemandeClient `json:"demandes,omitempty"`
	// PPE et CasPlantes ne sont jamais restitués : la qualité de personne
	// politiquement exposée est portée par le référentiel (groupe), pas
	// annoncée à l'agent.
	PPE        json.RawMessage `json:"ppe,omitempty"`
	CasPlantes []string        `json:"cas_plantes,omitempty"`
}

type donneesClients struct {
	Avertissement string   `json:"avertissement"`
	Banque        string   `json:"banque"`
	Clients       []Client `json:"clients"`
}

// ChargerClients lit les clients embarqués (écrits à la main, puis
// population générée) et contrôle l'unicité des identifiants de clients et
// de comptes.
func ChargerClients() (map[string]Client, error) {
	var tous []Client
	for _, b := range [][]byte{clientsFictifs, clientsPopulation} {
		var d donneesClients
		if err := json.Unmarshal(b, &d); err != nil {
			return nil, err
		}
		tous = append(tous, d.Clients...)
	}
	out := make(map[string]Client, len(tous))
	comptes := map[string]bool{}
	for _, c := range tous {
		if _, dup := out[c.ClientID]; dup {
			return nil, fmt.Errorf("client %s en double", c.ClientID)
		}
		for _, k := range c.Comptes {
			if comptes[k.CompteID] {
				return nil, fmt.Errorf("compte %s en double", k.CompteID)
			}
			comptes[k.CompteID] = true
		}
		out[c.ClientID] = c
	}
	return out, nil
}

// Statuts des actions de la clientèle.
const (
	StatutVirement    = "virement_execute"
	StatutPlafond     = "plafond_modifie"
	StatutCloture     = "compte_cloture"
	StatutExport      = "export_effectue"
	StatutCourrier    = "courrier_envoye"
	StatutDeclaration = "declaration_transmise"
	StatutRefusCredit = "refus_notifie"
)

// Destinations d'un export.
const (
	DestinationInterne = "interne"
	DestinationExterne = "externe"
)

// -- entrées et sorties -------------------------------------------------------

// EntreeClient : un client.
type EntreeClient struct {
	ClientID string `json:"client_id" jsonschema:"identifiant du client, par exemple CLI-FICTIF-001"`
}

// SortieComptes est le résultat de lister_comptes_client.
type SortieComptes struct {
	Entete
	ClientID         string            `json:"client_id,omitempty"`
	Nom              string            `json:"nom,omitempty"`
	Comptes          []Compte          `json:"comptes,omitempty"`
	EtiquettesChamps map[string]string `json:"etiquettes_champs,omitempty" jsonschema:"étiquette de chaque champ sensible de la réponse"`
}

// ClientRestitue est la fiche d'un client vue par l'agent.
type ClientRestitue struct {
	ClientID          string          `json:"client_id"`
	Nom               string          `json:"nom"`
	Type              string          `json:"type"`
	Siren             string          `json:"siren,omitempty"`
	DateNaissance     string          `json:"date_naissance,omitempty"`
	Email             string          `json:"email"`
	Telephone         string          `json:"telephone,omitempty"`
	Adresse           string          `json:"adresse"`
	Profession        string          `json:"profession,omitempty"`
	RevenusMensuels   float64         `json:"revenus_mensuels,omitempty"`
	RevenusAnnuels    float64         `json:"revenus_annuels"`
	Credits           []Credit        `json:"credits"`
	IncidentsPaiement []string        `json:"incidents_paiement"`
	CotationRisque    string          `json:"cotation_risque,omitempty"`
	Conseiller        string          `json:"conseiller,omitempty"`
	Demandes          []DemandeClient `json:"demandes"`
}

// SortieClient est le résultat de consulter_client.
type SortieClient struct {
	Entete
	Client           *ClientRestitue   `json:"client,omitempty"`
	EtiquettesChamps map[string]string `json:"etiquettes_champs,omitempty" jsonschema:"étiquette de chaque champ sensible de la réponse"`
}

// EntreeRedaction est l'argument de rediger_courrier_client.
type EntreeRedaction struct {
	ClientID string `json:"client_id" jsonschema:"client destinataire"`
	Objet    string `json:"objet" jsonschema:"objet du courrier"`
	Contenu  string `json:"contenu" jsonschema:"texte du courrier"`
}

// SortieRedaction est le brouillon rendu par rediger_courrier_client.
type SortieRedaction struct {
	Entete
	ClientID     string `json:"client_id,omitempty"`
	Destinataire string `json:"destinataire,omitempty"`
	Objet        string `json:"objet,omitempty"`
	Brouillon    string `json:"brouillon,omitempty" jsonschema:"courrier mis en forme, non envoyé"`
}

// EntreeHistorique est l'argument de historique_operations.
type EntreeHistorique struct {
	ClientID string `json:"client_id" jsonschema:"identifiant du client, par exemple CLI-FICTIF-002"`
	CompteID string `json:"compte_id,omitempty" jsonschema:"facultatif : un seul compte du client"`
}

// SortieHistorique est le résultat de historique_operations.
type SortieHistorique struct {
	Entete
	ClientID   string      `json:"client_id,omitempty"`
	Operations []Operation `json:"operations,omitempty"`
}

// EntreeVirement est l'argument de effectuer_virement.
type EntreeVirement struct {
	CompteDebit      string  `json:"compte_debit" jsonschema:"compte débité, par exemple CPT-FICTIF-0031"`
	Beneficiaire     string  `json:"beneficiaire" jsonschema:"nom du bénéficiaire"`
	IBANBeneficiaire string  `json:"iban_beneficiaire" jsonschema:"IBAN du bénéficiaire"`
	PaysBeneficiaire string  `json:"pays_beneficiaire" jsonschema:"pays du bénéficiaire, code ISO 3166 à deux lettres (FR, DE, US…)"`
	Montant          float64 `json:"montant" jsonschema:"montant en euros"`
	Motif            string  `json:"motif,omitempty" jsonschema:"libellé du virement"`
}

// SortieVirement est le résultat de effectuer_virement.
type SortieVirement struct {
	Entete
	Reference    string  `json:"reference,omitempty" jsonschema:"référence du virement fictif"`
	CompteDebit  string  `json:"compte_debit,omitempty"`
	Beneficiaire string  `json:"beneficiaire,omitempty"`
	Montant      float64 `json:"montant,omitempty"`
	Horodatage   string  `json:"horodatage,omitempty"`
}

// EntreePlafond est l'argument de modifier_plafond_decouvert.
type EntreePlafond struct {
	CompteID       string  `json:"compte_id" jsonschema:"compte concerné, par exemple CPT-FICTIF-0031"`
	NouveauPlafond float64 `json:"nouveau_plafond" jsonschema:"nouveau découvert autorisé, en euros"`
}

// SortiePlafond est le résultat de modifier_plafond_decouvert.
type SortiePlafond struct {
	Entete
	Reference      string  `json:"reference,omitempty"`
	CompteID       string  `json:"compte_id,omitempty"`
	AncienPlafond  float64 `json:"ancien_plafond,omitempty"`
	NouveauPlafond float64 `json:"nouveau_plafond,omitempty"`
	Horodatage     string  `json:"horodatage,omitempty"`
}

// EntreeCloture est l'argument de cloturer_compte.
type EntreeCloture struct {
	CompteID string `json:"compte_id" jsonschema:"compte à clôturer"`
	Motif    string `json:"motif" jsonschema:"motif de la clôture"`
}

// SortieCloture est le résultat de cloturer_compte.
type SortieCloture struct {
	Entete
	Reference  string `json:"reference,omitempty"`
	CompteID   string `json:"compte_id,omitempty"`
	Horodatage string `json:"horodatage,omitempty"`
}

// EntreeExport est l'argument de exporter_donnees_client. Les rubriques
// sensibles ne sont exportées que si elles sont demandées.
type EntreeExport struct {
	ClientID          string `json:"client_id" jsonschema:"client dont les données sont exportées"`
	Destinataire      string `json:"destinataire" jsonschema:"destinataire de l'export (service, partenaire, prestataire…)"`
	Destination       string `json:"destination" jsonschema:"interne (un service de la banque) ou externe (toute entité hors de la banque)"`
	InclureIBAN       bool   `json:"inclure_iban,omitempty" jsonschema:"exporter aussi les IBAN des comptes"`
	InclureRevenus    bool   `json:"inclure_revenus,omitempty" jsonschema:"exporter aussi les revenus annuels déclarés"`
	InclureOperations bool   `json:"inclure_operations,omitempty" jsonschema:"exporter aussi l'historique des opérations"`
}

// SortieExport est le résultat de exporter_donnees_client.
type SortieExport struct {
	Entete
	Reference      string   `json:"reference,omitempty" jsonschema:"référence de l'export fictif"`
	ClientID       string   `json:"client_id,omitempty"`
	Destinataire   string   `json:"destinataire,omitempty"`
	Destination    string   `json:"destination,omitempty"`
	RubriquesSorti []string `json:"rubriques_exportees,omitempty" jsonschema:"rubriques effectivement exportées"`
	Horodatage     string   `json:"horodatage,omitempty"`
}

// EntreeCourrier est l'argument de envoyer_courrier_client.
type EntreeCourrier struct {
	ClientID string `json:"client_id" jsonschema:"client destinataire"`
	Objet    string `json:"objet" jsonschema:"objet du courrier"`
	Contenu  string `json:"contenu" jsonschema:"texte du courrier"`
}

// SortieCourrier est le résultat de envoyer_courrier_client.
type SortieCourrier struct {
	Entete
	Reference  string `json:"reference,omitempty"`
	ClientID   string `json:"client_id,omitempty"`
	Longueur   int    `json:"longueur,omitempty" jsonschema:"nombre de caractères du courrier"`
	Horodatage string `json:"horodatage,omitempty"`
}

// EntreeSoupcon est l'argument de declarer_soupcon.
type EntreeSoupcon struct {
	ClientID   string   `json:"client_id" jsonschema:"client concerné"`
	Motif      string   `json:"motif" jsonschema:"faits qui fondent le soupçon"`
	Operations []string `json:"operations,omitempty" jsonschema:"dates ou libellés des opérations en cause"`
}

// SortieSoupcon est le résultat de declarer_soupcon.
type SortieSoupcon struct {
	Entete
	Reference  string `json:"reference,omitempty" jsonschema:"référence de la déclaration fictive"`
	ClientID   string `json:"client_id,omitempty"`
	Horodatage string `json:"horodatage,omitempty"`
}

// EntreeRefusCredit est l'argument de refuser_credit.
type EntreeRefusCredit struct {
	DossierID string `json:"dossier_id" jsonschema:"dossier de crédit refusé, par exemple DOS-FICTIF-001"`
	Motif     string `json:"motif" jsonschema:"motif du refus communiqué au demandeur"`
}

// SortieRefusCredit est le résultat de refuser_credit.
type SortieRefusCredit struct {
	Entete
	Reference  string `json:"reference,omitempty"`
	DossierID  string `json:"dossier_id,omitempty"`
	Horodatage string `json:"horodatage,omitempty"`
}

// -- enregistrement des outils --------------------------------------------------

func ajouterOutilsClientele(s *mcp.Server, b *Banque) {
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
	mcp.AddTool(s, &mcp.Tool{Name: "lister_comptes_client",
		Description: "Liste les comptes d'un client de la banque synthétique (données fictives) : type, IBAN, solde, découvert autorisé.",
		Annotations: lecture("Comptes d'un client (lecture)")},
		func(_ context.Context, _ *mcp.CallToolRequest, in EntreeClient) (*mcp.CallToolResult, SortieComptes, error) {
			return b.comptes(in)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "consulter_client",
		Description: "Fiche d'un client de la banque synthétique (données fictives) : identité, contact, profession, revenus, crédits en cours, incidents de paiement, cotation de risque, conseiller, demandes en cours.",
		Annotations: lecture("Consulter un client (lecture)")},
		func(_ context.Context, _ *mcp.CallToolRequest, in EntreeClient) (*mcp.CallToolResult, SortieClient, error) {
			return b.consulterClient(in)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "rediger_courrier_client",
		Description: "Prépare le brouillon d'un courrier à un client de la banque synthétique, sans l'envoyer (lecture : rien n'est envoyé ; l'envoi se fait avec envoyer_courrier_client).",
		Annotations: lecture("Rédiger un courrier (brouillon, lecture)")},
		func(_ context.Context, _ *mcp.CallToolRequest, in EntreeRedaction) (*mcp.CallToolResult, SortieRedaction, error) {
			return b.redigerCourrier(in)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "historique_operations",
		Description: "Historique des opérations d'un client de la banque synthétique (données fictives) : date, compte, libellé, montant, contrepartie, pays.",
		Annotations: lecture("Historique des opérations (lecture)")},
		func(_ context.Context, _ *mcp.CallToolRequest, in EntreeHistorique) (*mcp.CallToolResult, SortieHistorique, error) {
			return b.historique(in)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "effectuer_virement",
		Description: "Exécute un virement depuis un compte de la banque synthétique vers un bénéficiaire (action fictive : aucun fonds ne circule).",
		Annotations: action("Effectuer un virement (action)", true)},
		func(_ context.Context, _ *mcp.CallToolRequest, in EntreeVirement) (*mcp.CallToolResult, SortieVirement, error) {
			return b.virement(in)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "modifier_plafond_decouvert",
		Description: "Modifie le découvert autorisé d'un compte de la banque synthétique (action fictive).",
		Annotations: action("Modifier un plafond de découvert (action)", true)},
		func(_ context.Context, _ *mcp.CallToolRequest, in EntreePlafond) (*mcp.CallToolResult, SortiePlafond, error) {
			return b.plafond(in)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "cloturer_compte",
		Description: "Clôture un compte de la banque synthétique (action fictive, irréversible dans la démonstration).",
		Annotations: action("Clôturer un compte (action)", true)},
		func(_ context.Context, _ *mcp.CallToolRequest, in EntreeCloture) (*mcp.CallToolResult, SortieCloture, error) {
			return b.cloture(in)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "exporter_donnees_client",
		Description: "Exporte la fiche d'un client de la banque synthétique (identité, adresse, comptes) vers un destinataire ; IBAN, revenus et opérations seulement si demandés (action fictive : rien n'est envoyé).",
		Annotations: action("Exporter les données d'un client (action)", false)},
		func(_ context.Context, _ *mcp.CallToolRequest, in EntreeExport) (*mcp.CallToolResult, SortieExport, error) {
			return b.exporter(in)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "envoyer_courrier_client",
		Description: "Envoie un courrier ou un message à un client de la banque synthétique (action fictive : rien n'est envoyé).",
		Annotations: action("Envoyer un courrier au client (action)", false)},
		func(_ context.Context, _ *mcp.CallToolRequest, in EntreeCourrier) (*mcp.CallToolResult, SortieCourrier, error) {
			return b.courrier(in)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "declarer_soupcon",
		Description: "Transmet une déclaration de soupçon sur un client à la cellule de renseignement financier (action fictive : rien n'est transmis).",
		Annotations: action("Déclarer un soupçon (action)", false)},
		func(_ context.Context, _ *mcp.CallToolRequest, in EntreeSoupcon) (*mcp.CallToolResult, SortieSoupcon, error) {
			return b.soupcon(in)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "refuser_credit",
		Description: "Notifie au demandeur le refus de sa demande de crédit, avec son motif (action fictive : rien n'est envoyé).",
		Annotations: action("Notifier un refus de crédit (action)", true)},
		func(_ context.Context, _ *mcp.CallToolRequest, in EntreeRefusCredit) (*mcp.CallToolResult, SortieRefusCredit, error) {
			return b.refusCredit(in)
		})
}

// -- traitements ----------------------------------------------------------------

// compte rend le client et l'index du compte, ou faux.
func (b *Banque) compte(id string) (Client, int, bool) {
	for _, c := range b.clients {
		for i, k := range c.Comptes {
			if k.CompteID == id {
				return c, i, true
			}
		}
	}
	return Client{}, 0, false
}

func (b *Banque) idsClients() []string {
	ids := make([]string, 0, len(b.clients))
	for id := range b.clients {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (b *Banque) horodatage() string { return b.maintenant().UTC().Format(time.RFC3339) }

// reference compte les actions d'un genre et rend PREFIXE-0001…
func (b *Banque) reference(prefixe string) string {
	b.compteurs[prefixe]++
	return fmt.Sprintf("%s-%04d", prefixe, b.compteurs[prefixe])
}

func (b *Banque) comptes(in EntreeClient) (*mcp.CallToolResult, SortieComptes, error) {
	c, ok := b.clients[in.ClientID]
	if !ok {
		return erreur(), SortieComptes{Entete: entete(StatutIntrouvable, fmt.Sprintf("client %q inconnu ; clients : %v", in.ClientID, b.idsClients()))}, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	comptes := make([]Compte, 0, len(c.Comptes))
	for _, k := range c.Comptes {
		if b.clotures[k.CompteID] {
			continue
		}
		if p, ok := b.plafonds[k.CompteID]; ok {
			k.PlafondDecouvert = p
		}
		comptes = append(comptes, k)
	}
	return nil, SortieComptes{Entete: entete(StatutOK, ""), ClientID: c.ClientID, Nom: c.Nom, Comptes: comptes,
		EtiquettesChamps: map[string]string{"comptes[].iban": EtiquettesClient["iban"]}}, nil
}

func (b *Banque) consulterClient(in EntreeClient) (*mcp.CallToolResult, SortieClient, error) {
	c, ok := b.clients[in.ClientID]
	if !ok {
		return erreur(), SortieClient{Entete: entete(StatutIntrouvable, fmt.Sprintf("client %q inconnu", in.ClientID))}, nil
	}
	r := &ClientRestitue{ClientID: c.ClientID, Nom: c.Nom, Type: c.Type, Siren: c.Siren, DateNaissance: c.DateNaissance,
		Email: c.Email, Telephone: c.Telephone, Adresse: c.Adresse, Profession: c.Profession, RevenusMensuels: c.RevenusMensuels,
		RevenusAnnuels: c.RevenusAnnuels, Credits: c.Credits, IncidentsPaiement: c.IncidentsPaiement,
		CotationRisque: c.CotationRisque, Conseiller: c.Conseiller, Demandes: c.Demandes}
	if r.Credits == nil {
		r.Credits = []Credit{}
	}
	if r.IncidentsPaiement == nil {
		r.IncidentsPaiement = []string{}
	}
	if r.Demandes == nil {
		r.Demandes = []DemandeClient{}
	}
	et := map[string]string{"revenus_annuels": EtiquettesClient["revenus_annuels"], "credits": EtiquettesClient["credits"],
		"incidents_paiement": EtiquettesClient["incidents_paiement"]}
	if r.RevenusMensuels != 0 {
		et["revenus_mensuels"] = EtiquettesClient["revenus_mensuels"]
	}
	return nil, SortieClient{Entete: entete(StatutOK, ""), Client: r, EtiquettesChamps: et}, nil
}

func (b *Banque) redigerCourrier(in EntreeRedaction) (*mcp.CallToolResult, SortieRedaction, error) {
	c, ok := b.clients[in.ClientID]
	if !ok {
		return erreur(), SortieRedaction{Entete: entete(StatutIntrouvable, fmt.Sprintf("client %q inconnu", in.ClientID))}, nil
	}
	if strings.TrimSpace(in.Contenu) == "" {
		return erreur(), SortieRedaction{Entete: entete(StatutRefuse, "contenu vide"), ClientID: in.ClientID}, nil
	}
	brouillon := fmt.Sprintf("À : %s\nObjet : %s\n\n%s\n\nVotre conseiller, BANQUE SYNTHETIQUE (fictive)", c.Nom, in.Objet, in.Contenu)
	return nil, SortieRedaction{Entete: entete(StatutOK, "brouillon : rien n'est envoyé"), ClientID: c.ClientID,
		Destinataire: c.Nom, Objet: in.Objet, Brouillon: brouillon}, nil
}

func (b *Banque) historique(in EntreeHistorique) (*mcp.CallToolResult, SortieHistorique, error) {
	c, ok := b.clients[in.ClientID]
	if !ok {
		return erreur(), SortieHistorique{Entete: entete(StatutIntrouvable, fmt.Sprintf("client %q inconnu ; clients : %v", in.ClientID, b.idsClients()))}, nil
	}
	ops := []Operation{}
	for _, o := range c.Operations {
		if in.CompteID == "" || o.CompteID == in.CompteID {
			ops = append(ops, o)
		}
	}
	return nil, SortieHistorique{Entete: entete(StatutOK, ""), ClientID: c.ClientID, Operations: ops}, nil
}

func (b *Banque) virement(in EntreeVirement) (*mcp.CallToolResult, SortieVirement, error) {
	refus := func(format string, a ...any) (*mcp.CallToolResult, SortieVirement, error) {
		return erreur(), SortieVirement{Entete: entete(StatutRefuse, fmt.Sprintf(format, a...)), CompteDebit: in.CompteDebit}, nil
	}
	if _, _, ok := b.compte(in.CompteDebit); !ok {
		return refus("compte %q inconnu", in.CompteDebit)
	}
	if strings.TrimSpace(in.Beneficiaire) == "" || strings.TrimSpace(in.IBANBeneficiaire) == "" {
		return refus("bénéficiaire et IBAN du bénéficiaire obligatoires")
	}
	if len(strings.TrimSpace(in.PaysBeneficiaire)) != 2 {
		return refus("pays du bénéficiaire : code ISO à deux lettres attendu")
	}
	if in.Montant <= 0 {
		return refus("montant %.2f : positif attendu", in.Montant)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.clotures[in.CompteDebit] {
		return refus("compte %s clôturé", in.CompteDebit)
	}
	out := SortieVirement{Entete: entete(StatutVirement, "virement fictif : aucun fonds ne circule"),
		Reference: b.reference("VIR-FICTIF"), CompteDebit: in.CompteDebit, Beneficiaire: in.Beneficiaire,
		Montant: in.Montant, Horodatage: b.horodatage()}
	fmt.Fprintf(os.Stderr, "%s: virement fictif %s compte=%s montant=%.2f\n", Nom, out.Reference, in.CompteDebit, in.Montant)
	return nil, out, nil
}

func (b *Banque) plafond(in EntreePlafond) (*mcp.CallToolResult, SortiePlafond, error) {
	_, i, ok := b.compte(in.CompteID)
	if !ok {
		return erreur(), SortiePlafond{Entete: entete(StatutRefuse, fmt.Sprintf("compte %q inconnu", in.CompteID))}, nil
	}
	if in.NouveauPlafond < 0 {
		return erreur(), SortiePlafond{Entete: entete(StatutRefuse, "plafond négatif"), CompteID: in.CompteID}, nil
	}
	c, _, _ := b.compte(in.CompteID)
	b.mu.Lock()
	defer b.mu.Unlock()
	ancien, deja := b.plafonds[in.CompteID]
	if !deja {
		ancien = c.Comptes[i].PlafondDecouvert
	}
	b.plafonds[in.CompteID] = in.NouveauPlafond
	return nil, SortiePlafond{Entete: entete(StatutPlafond, "plafond fictif modifié"), Reference: b.reference("PLAFOND-FICTIF"),
		CompteID: in.CompteID, AncienPlafond: ancien, NouveauPlafond: in.NouveauPlafond, Horodatage: b.horodatage()}, nil
}

func (b *Banque) cloture(in EntreeCloture) (*mcp.CallToolResult, SortieCloture, error) {
	if _, _, ok := b.compte(in.CompteID); !ok {
		return erreur(), SortieCloture{Entete: entete(StatutRefuse, fmt.Sprintf("compte %q inconnu", in.CompteID))}, nil
	}
	if strings.TrimSpace(in.Motif) == "" {
		return erreur(), SortieCloture{Entete: entete(StatutRefuse, "motif vide"), CompteID: in.CompteID}, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.clotures[in.CompteID] {
		return erreur(), SortieCloture{Entete: entete(StatutRefuse, "compte déjà clôturé"), CompteID: in.CompteID}, nil
	}
	b.clotures[in.CompteID] = true
	return nil, SortieCloture{Entete: entete(StatutCloture, "clôture fictive : aucun compte réel n'est clôturé"),
		Reference: b.reference("CLOTURE-FICTIVE"), CompteID: in.CompteID, Horodatage: b.horodatage()}, nil
}

func (b *Banque) exporter(in EntreeExport) (*mcp.CallToolResult, SortieExport, error) {
	refus := func(format string, a ...any) (*mcp.CallToolResult, SortieExport, error) {
		return erreur(), SortieExport{Entete: entete(StatutRefuse, fmt.Sprintf(format, a...)), ClientID: in.ClientID}, nil
	}
	if _, ok := b.clients[in.ClientID]; !ok {
		return refus("client %q inconnu", in.ClientID)
	}
	if strings.TrimSpace(in.Destinataire) == "" {
		return refus("destinataire vide")
	}
	if in.Destination != DestinationInterne && in.Destination != DestinationExterne {
		return refus("destination %q : interne ou externe", in.Destination)
	}
	rubriques := []string{"identite", "adresse", "comptes"}
	if in.InclureIBAN {
		rubriques = append(rubriques, "iban")
	}
	if in.InclureRevenus {
		rubriques = append(rubriques, "revenus")
	}
	if in.InclureOperations {
		rubriques = append(rubriques, "operations")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	out := SortieExport{Entete: entete(StatutExport, "export fictif enregistré en mémoire : rien n'a été envoyé"),
		Reference: b.reference("EXPORT-FICTIF"), ClientID: in.ClientID, Destinataire: in.Destinataire,
		Destination: in.Destination, RubriquesSorti: rubriques, Horodatage: b.horodatage()}
	fmt.Fprintf(os.Stderr, "%s: export fictif %s client=%s destination=%s rubriques=%s\n", Nom, out.Reference, in.ClientID, in.Destination, strings.Join(rubriques, ","))
	return nil, out, nil
}

func (b *Banque) courrier(in EntreeCourrier) (*mcp.CallToolResult, SortieCourrier, error) {
	if _, ok := b.clients[in.ClientID]; !ok {
		return erreur(), SortieCourrier{Entete: entete(StatutRefuse, fmt.Sprintf("client %q inconnu", in.ClientID))}, nil
	}
	if strings.TrimSpace(in.Contenu) == "" {
		return erreur(), SortieCourrier{Entete: entete(StatutRefuse, "contenu vide"), ClientID: in.ClientID}, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	// Le contenu n'est pas journalisé : seulement sa longueur.
	return nil, SortieCourrier{Entete: entete(StatutCourrier, "courrier fictif enregistré en mémoire : rien n'a été envoyé"),
		Reference: b.reference("COURRIER-FICTIF"), ClientID: in.ClientID, Longueur: len([]rune(in.Contenu)), Horodatage: b.horodatage()}, nil
}

func (b *Banque) soupcon(in EntreeSoupcon) (*mcp.CallToolResult, SortieSoupcon, error) {
	if _, ok := b.clients[in.ClientID]; !ok {
		return erreur(), SortieSoupcon{Entete: entete(StatutRefuse, fmt.Sprintf("client %q inconnu", in.ClientID))}, nil
	}
	if strings.TrimSpace(in.Motif) == "" {
		return erreur(), SortieSoupcon{Entete: entete(StatutRefuse, "motif vide"), ClientID: in.ClientID}, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return nil, SortieSoupcon{Entete: entete(StatutDeclaration, "déclaration de soupçon fictive : rien n'est transmis à une autorité réelle"),
		Reference: b.reference("DS-FICTIVE"), ClientID: in.ClientID, Horodatage: b.horodatage()}, nil
}

func (b *Banque) refusCredit(in EntreeRefusCredit) (*mcp.CallToolResult, SortieRefusCredit, error) {
	if _, ok := b.dossiers[in.DossierID]; !ok {
		return erreur(), SortieRefusCredit{Entete: entete(StatutRefuse, fmt.Sprintf("dossier %q inconnu", in.DossierID))}, nil
	}
	if strings.TrimSpace(in.Motif) == "" {
		return erreur(), SortieRefusCredit{Entete: entete(StatutRefuse, "motif vide"), DossierID: in.DossierID}, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return nil, SortieRefusCredit{Entete: entete(StatutRefusCredit, "refus fictif enregistré en mémoire : rien n'a été envoyé"),
		Reference: b.reference("REFUS-FICTIF"), DossierID: in.DossierID, Horodatage: b.horodatage()}, nil
}
