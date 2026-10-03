package console

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Libellés des outils à l'écran : « Attribution de prêt — banque » plutôt
// que banque-synthetique__attribuer_pret. Le nom technique reste en
// info-bulle, avec la description du catalogue.

// CatalogueOutils est le catalogue des outils (généré par cmd/registre-demo
// depuis le registre de la démonstration), lu en lecture seule pour les
// descriptions.
const CatalogueOutils = "agents/compilateur/exemples/catalogue-demo.json"

// LibelleOutil est ce que l'écran affiche d'un outil.
type LibelleOutil struct {
	Libelle     string `json:"libelle"`     // « Attribution de prêt — banque »
	Technique   string `json:"technique"`   // serveur__outil
	Description string `json:"description"` // catalogue ; vide s'il n'y figure pas
}

// libellesCourts : nom court de chaque outil de la démonstration, tiré de
// sa description au catalogue. Un outil absent de cette table prend le début
// de sa description.
var libellesCourts = map[string]string{
	"banque-synthetique__attribuer_pret":             "Attribution de prêt",
	"banque-synthetique__cloturer_compte":            "Clôture de compte",
	"banque-synthetique__consulter_client":           "Fiche client",
	"banque-synthetique__declarer_soupcon":           "Déclaration de soupçon",
	"banque-synthetique__effectuer_virement":         "Virement",
	"banque-synthetique__envoyer_courrier_client":    "Envoi de courrier au client",
	"banque-synthetique__evaluer_dossier":            "Lecture du dossier de crédit",
	"banque-synthetique__exporter_donnees_client":    "Export des données du client",
	"banque-synthetique__get_taux_interne":           "Seuil de l'usure (base interne)",
	"banque-synthetique__historique_operations":      "Historique des opérations",
	"banque-synthetique__lister_comptes_client":      "Comptes du client",
	"banque-synthetique__modifier_plafond_decouvert": "Modification du découvert autorisé",
	"banque-synthetique__rediger_courrier_client":    "Brouillon de courrier au client",
	"banque-synthetique__refuser_credit":             "Refus de crédit",
	"banque-synthetique__remettre_rapport":           "Remise d'un rapport",
	"rh-synthetique__attribuer_prime":                "Attribution de prime",
	"rh-synthetique__consulter_fiche":                "Fiche complète du salarié",
	"rh-synthetique__consulter_salarie":              "Fiche du salarié",
	"rh-synthetique__convoquer_entretien_prealable":  "Convocation à un entretien préalable",
	"rh-synthetique__decider_prime":                  "Décision de prime",
	"rh-synthetique__envoyer_message_salarie":        "Message au salarié",
	"rh-synthetique__exporter_paie":                  "Export de la paie",
	"rh-synthetique__licencier":                      "Licenciement",
	"rh-synthetique__lister_salaries":                "Liste des salariés",
	"rh-synthetique__proposer_sanction":              "Proposition de sanction",
	"rh-synthetique__rechercher_salaries":            "Recherche de salariés",
	"rh-synthetique__rediger_message":                "Brouillon de message au salarié",
	"rh-synthetique__sanctionner":                    "Sanction disciplinaire",
	"referentiel-rh__groupes_salarie":                "Groupes du salarié",
	"taux-officiel__get_taux_usure":                  "Seuil de l'usure",
	"taux-imposteur__get_taux_usure":                 "Seuil de l'usure",
	"recherche-entreprises__rechercher_entreprise":   "Fiche d'entreprise",
	"gels-avoirs__verifier_gel":                      "Vérification des gels d'avoirs",
	"eurlex__lire_article":                           "Article de droit européen",
	"eurlex__rechercher_acte":                        "Acte de l'Union européenne",
	"legifrance__lire_article":                       "Article de loi",
}

// libellesServeurs : qui répond, en clair.
var libellesServeurs = map[string]string{
	"banque-synthetique":    "banque",
	"rh-synthetique":        "RH",
	"referentiel-rh":        "référentiel RH",
	"taux-officiel":         "Banque de France",
	"taux-imposteur":        "source non officielle",
	"recherche-entreprises": "registre des entreprises",
	"gels-avoirs":           "DG Trésor",
	"eurlex":                "EUR-Lex",
	"legifrance":            "Légifrance",
}

type catalogueOutils struct {
	Serveurs []struct {
		Nom    string `json:"nom"`
		Outils []struct {
			Nom         string `json:"nom"`
			Description string `json:"description"`
		} `json:"outils"`
	} `json:"serveurs"`
}

// LibellesOutils rend le libellé de chaque outil connu : ceux du catalogue
// (s'il est lisible) et ceux de la table, indexés par serveur__outil.
func (d *Depot) LibellesOutils() map[string]LibelleOutil {
	out := map[string]LibelleOutil{}
	descr := map[string]string{}
	var cat catalogueOutils
	if b, err := os.ReadFile(filepath.Join(d.Commandes.Racine, CatalogueOutils)); err == nil && json.Unmarshal(b, &cat) == nil {
		for _, s := range cat.Serveurs {
			for _, o := range s.Outils {
				descr[s.Nom+"__"+o.Nom] = o.Description
			}
		}
	}
	for k := range libellesCourts {
		if _, ok := descr[k]; !ok {
			descr[k] = ""
		}
	}
	for k, dsc := range descr {
		serveur, outil, _ := strings.Cut(k, "__")
		out[k] = LibelleOutil{Libelle: libelleDe(serveur, outil, dsc), Technique: k, Description: dsc}
	}
	return out
}

func libelleDe(serveur, outil, description string) string {
	court := libellesCourts[serveur+"__"+outil]
	if court == "" {
		court = debutDescription(description)
	}
	if court == "" {
		court = strings.ReplaceAll(outil, "_", " ")
		court = strings.ToUpper(court[:1]) + court[1:]
	}
	srv := libellesServeurs[serveur]
	if srv == "" {
		srv = serveur
	}
	return court + " — " + srv
}

// debutDescription : la description jusqu'à la première parenthèse,
// deux-points ou point, au plus 48 caractères (coupés sur un mot).
func debutDescription(d string) string {
	if i := strings.IndexAny(d, "(:.;"); i > 0 {
		d = d[:i]
	}
	d = strings.TrimSpace(d)
	if utf8.RuneCountInString(d) > 48 {
		r := []rune(d)[:48]
		s := string(r)
		if k := strings.LastIndex(s, " "); k > 20 {
			s = s[:k]
		}
		d = s + "…"
	}
	return d
}
