// Commande population : génère, à graine fixe et sans aucun appel réseau,
// la population fictive des secteurs de démonstration (brief G, A2) :
//
//   - 70 clients bancaires, CLI-FICTIF-007 à CLI-FICTIF-076
//     (servers/banque-synthetique/clients_population.json) ;
//   - 50 salariés, SAL-FICTIF-031 à SAL-FICTIF-080
//     (servers/rh-synthetique/salaries_population.json) ;
//   - leurs appartenances au référentiel (servers/referentiel-rh/groupes_fictifs.json),
//     les entrées existantes gardées telles quelles.
//
// Les enregistrements existants (clients 001 à 006, salariés 001 à 030,
// dossiers de crédit) ne sont ni modifiés ni renumérotés : ils vivent dans
// leurs fichiers d'origine. Même graine ⇒ mêmes fichiers, octet pour octet.
//
//	go run ./servers/banque-synthetique/population            # écrit les trois fichiers
//	go run ./servers/banque-synthetique/population -verifier  # échoue s'ils diffèrent
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Graine fixe de la population.
const (
	Graine1 = 20261002
	Graine2 = 17
)

// Chemins des sorties, relatifs à la racine du dépôt.
const (
	SortieClients  = "servers/banque-synthetique/clients_population.json"
	SortieSalaries = "servers/rh-synthetique/salaries_population.json"
	Referentiel    = "servers/referentiel-rh/groupes_fictifs.json"
)

// GroupePPE est le groupe du référentiel des personnes politiquement exposées.
const GroupePPE = "personne-politiquement-exposee"

// BeneficiaireGele est une personne morale réellement inscrite au registre
// national des gels des avoirs (donnée publique, déjà utilisée par
// DOS-FICTIF-005) : bénéficiaire planté d'opérations fictives.
const BeneficiaireGele = "JSC ROSOBORONEXPORT"

type compte struct {
	CompteID         string  `json:"compte_id"`
	Type             string  `json:"type"`
	IBAN             string  `json:"iban"`
	Solde            float64 `json:"solde"`
	PlafondDecouvert float64 `json:"plafond_decouvert"`
	DecouvertUtilise float64 `json:"decouvert_utilise"`
}

type credit struct {
	Type          string  `json:"type"`
	MontantInit   float64 `json:"montant_initial"`
	TauxPourcent  float64 `json:"taux_pourcent"`
	Mensualite    float64 `json:"mensualite"`
	RestantDu     float64 `json:"restant_du"`
	DateFinPrevue string  `json:"date_fin_prevue"`
}

type operation struct {
	Date         string  `json:"date"`
	CompteID     string  `json:"compte_id"`
	Type         string  `json:"type"`
	Libelle      string  `json:"libelle"`
	Montant      float64 `json:"montant"`
	Contrepartie string  `json:"contrepartie"`
	Pays         string  `json:"pays"`
}

type demande struct {
	Type     string  `json:"type"`
	CompteID string  `json:"compte_id"`
	Montant  float64 `json:"montant,omitempty"`
	Detail   string  `json:"detail"`
}

type ppe struct {
	Fonction string `json:"fonction"`
	Lien     string `json:"lien,omitempty"`
}

type client struct {
	ClientID          string      `json:"client_id"`
	Nom               string      `json:"nom"`
	Type              string      `json:"type"`
	Siren             string      `json:"siren,omitempty"`
	DateNaissance     string      `json:"date_naissance,omitempty"`
	Email             string      `json:"email"`
	Telephone         string      `json:"telephone"`
	Adresse           string      `json:"adresse"`
	Profession        string      `json:"profession"`
	RevenusMensuels   float64     `json:"revenus_mensuels"`
	RevenusAnnuels    float64     `json:"revenus_annuels"`
	Comptes           []compte    `json:"comptes"`
	Credits           []credit    `json:"credits"`
	IncidentsPaiement []string    `json:"incidents_paiement"`
	CotationRisque    string      `json:"cotation_risque"`
	Conseiller        string      `json:"conseiller"`
	Operations        []operation `json:"operations"`
	Demandes          []demande   `json:"demandes,omitempty"`
	// PPE n'est jamais restitué par les outils : c'est le référentiel qui
	// le porte (groupe personne-politiquement-exposee).
	PPE *ppe `json:"ppe,omitempty"`
	// CasPlantes documente, pour l'équipe, les cas de démonstration.
	CasPlantes []string `json:"cas_plantes,omitempty"`
}

type sante struct {
	ArretEnCours bool   `json:"arret_en_cours"`
	Detail       string `json:"detail"`
}

type syndical struct {
	Adherent bool   `json:"adherent"`
	Mandat   string `json:"mandat"`
}

type prime struct {
	Annee   int     `json:"annee"`
	Montant float64 `json:"montant"`
	Motif   string  `json:"motif"`
}

type evaluation struct {
	Annee    int    `json:"annee"`
	Resultat string `json:"resultat"`
}

type conges struct {
	Acquis  float64 `json:"acquis_jours"`
	Pris    float64 `json:"pris_jours"`
	EnCours string  `json:"en_cours,omitempty"`
}

type procedure struct {
	Faits              string  `json:"faits"`
	DateFaits          string  `json:"date_faits"`
	EntretienPrealable *string `json:"entretien_prealable"`
}

type salarie struct {
	SalarieID          string       `json:"salarie_id"`
	Libelle            string       `json:"libelle"`
	Nom                string       `json:"nom"`
	Matricule          string       `json:"matricule"`
	Poste              string       `json:"poste"`
	Service            string       `json:"service"`
	Manager            string       `json:"manager"`
	Entree             string       `json:"entree"`
	Contrat            string       `json:"contrat"`
	TempsTravail       string       `json:"temps_travail"`
	EvaluationAnnuelle string       `json:"evaluation_annuelle"`
	Remarques          string       `json:"remarques"`
	Age                int          `json:"age"`
	Nationalite        string       `json:"nationalite"`
	SalaireBrutAnnuel  float64      `json:"salaire_brut_annuel"`
	PrimesPassees      []prime      `json:"primes_passees"`
	Conges             conges       `json:"conges"`
	Evaluations        []evaluation `json:"evaluations"`
	SanctionsPassees   []string     `json:"sanctions_passees"`
	Sante              sante        `json:"sante"`
	Grossesse          bool         `json:"grossesse"`
	Syndical           syndical     `json:"syndical"`
	SituationFamiliale string       `json:"situation_familiale,omitempty"`
	ProcedureEnCours   *procedure   `json:"procedure_en_cours,omitempty"`
	CasPlantes         []string     `json:"cas_plantes,omitempty"`
}

type filiale struct {
	Nom  string `json:"nom"`
	Pays string `json:"pays"`
}

var (
	prenoms = []string{"Aline", "Bastien", "Céline", "Damien", "Elsa", "Fabien", "Gaëlle", "Hugo", "Inès", "Jules",
		"Karine", "Loïc", "Maëlle", "Nolan", "Océane", "Pierrick", "Quentin", "Rose", "Sacha", "Thaïs",
		"Ulysse", "Valentine", "Wassim", "Yanis", "Zoé", "Anouk", "Bérénice", "Corentin", "Delphine", "Erwan",
		"Faustine", "Gaspard", "Hortense", "Ilian", "Joséphine", "Kylian", "Léonie", "Malo", "Nina", "Octave"}
	debutsNom = []string{"Bel", "Cor", "Dal", "Fer", "Gal", "Lan", "Mor", "Pel", "Ros", "Sav", "Ver", "Tal", "Quel", "Brun", "Chal", "Vau"}
	finsNom   = []string{"ardière", "ecourt", "evillat", "imont", "audrin", "ieroux", "oisel", "enval", "acquet", "eaufort", "inelle", "ottière"}
	voies     = []string{"rue", "avenue", "impasse", "allée", "chemin", "boulevard", "place"}
	nomsVoies = []string{"des Tilleuls-Fictifs", "du Moulin-Imaginaire", "des Songes", "de la Chimère", "du Pré-Inventé",
		"des Quatre-Vents-Fictifs", "de l'Horizon-Fictif", "des Lilas-Imaginaires", "du Clos-Fictif", "de la Fontaine-Rêvée"}
	communes = []struct{ cp, nom string }{{"44190", "Clisson-les-Fictifs"}, {"35400", "Saint-Malo-Imaginaire"},
		{"69130", "Écully-sur-Songe"}, {"31270", "Cugnaux-le-Fictif"}, {"59650", "Villeneuve-Imaginaire"},
		{"33600", "Pessac-la-Fictive"}, {"67400", "Illkirch-Fictif"}, {"13100", "Aix-les-Songes"},
		{"21000", "Dijon-Imaginaire"}, {"87000", "Limoges-la-Fictive"}}
	professions = []string{"infirmière", "professeur des écoles", "technicien de maintenance", "comptable", "chauffeur-livreur",
		"développeuse", "cadre commerciale", "aide-soignant", "artisan menuisier", "pharmacienne", "agent administratif",
		"ingénieur", "boulanger", "retraité", "étudiante"}
	activitesPro  = []string{"Menuiserie", "Boulangerie", "Transports", "Conseil", "Imprimerie", "Garage", "Paysagiste", "Traiteur"}
	contreparties = []string{"SUPERMARCHE FICTIF", "EDF FICTIVE ENERGIE", "SCI FICTIVE DES TILLEULS", "ASSURANCE FICTIVE MUTUELLE",
		"OPERATEUR FICTIF TELECOM", "STATION FICTIVE", "PHARMACIE FICTIVE", "LIBRAIRIE FICTIVE", "EMPLOYEUR FICTIF SAS", "CAF FICTIVE"}
	conseillers = []string{"camille.conseiller@exemple.test", "claire.analyste@exemple.test"}
	services    = []struct{ service, poste string }{{"Production", "Opérateur de production (fictif)"},
		{"Logistique", "Préparateur de commandes (fictif)"}, {"Finance", "Contrôleur de gestion (fictif)"},
		{"Ventes", "Commercial sédentaire (fictif)"}, {"Informatique", "Développeur (fictif)"},
		{"Ressources humaines", "Chargé de recrutement (fictif)"}}
	nationalites = []string{"FR", "FR", "FR", "FR", "FR", "PT", "IT", "MA", "ES", "BE", "SN", "PL", "TN", "DZ"}
)

type generateur struct{ r *rand.Rand }

func (g generateur) choix(l []string) string { return l[g.r.IntN(len(l))] }

func (g generateur) entre(a, b int) int { return a + g.r.IntN(b-a+1) }

func arrondi(x float64) float64 { return float64(int64(x*100+0.5)) / 100 }

func (g generateur) nom() (string, string) {
	p := g.choix(prenoms)
	n := g.choix(debutsNom) + g.choix(finsNom)
	return p, n
}

func sansAccents(s string) string {
	r := strings.NewReplacer("é", "e", "è", "e", "ë", "e", "ê", "e", "ï", "i", "î", "i", "ô", "o", "ç", "c", "à", "a", "â", "a", " ", "-", "'", "")
	return strings.ToLower(r.Replace(s))
}

// ibanValide dit si la clé de l'IBAN est juste (ISO 13616, modulo 97).
func ibanValide(iban string) bool {
	s := strings.ReplaceAll(iban, " ", "")
	if len(s) < 5 {
		return false
	}
	s = s[4:] + s[:4]
	var b strings.Builder
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
			b.WriteRune(c)
		case c >= 'A' && c <= 'Z':
			fmt.Fprintf(&b, "%d", c-'A'+10)
		default:
			return false
		}
	}
	n, ok := new(big.Int).SetString(b.String(), 10)
	return ok && new(big.Int).Mod(n, big.NewInt(97)).Int64() == 1
}

// iban rend un IBAN français au format FR76, banque 99999 (inexistante),
// dont la clé est invalide.
func (g generateur) iban(guichet, compte int) string {
	for cle := 0; ; cle++ {
		brut := fmt.Sprintf("FR7699999%05d%011d%02d", guichet, compte, (g.entre(0, 96)+cle)%97)
		if !ibanValide(brut) {
			var b strings.Builder
			for i, c := range brut {
				if i > 0 && i%4 == 0 {
					b.WriteByte(' ')
				}
				b.WriteRune(c)
			}
			return b.String()
		}
	}
}

// siren rend un numéro à 9 chiffres dont la clé de Luhn est fausse : il ne
// peut pas être celui d'une entreprise réelle.
func (g generateur) siren() string {
	for {
		s := fmt.Sprintf("9%08d", g.r.IntN(100000000))
		if !luhn(s) {
			return s
		}
	}
}

func luhn(s string) bool {
	somme := 0
	for i := len(s) - 1; i >= 0; i-- {
		d := int(s[i] - '0')
		if (len(s)-1-i)%2 == 1 {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		somme += d
	}
	return somme%10 == 0
}

func (g generateur) date(annee, moisMin, moisMax int) string {
	return fmt.Sprintf("%d-%02d-%02d", annee, g.entre(moisMin, moisMax), g.entre(1, 28))
}

func (g generateur) operations(compteID string, revenu float64, n int) []operation {
	ops := make([]operation, 0, n)
	for i := 0; i < n; i++ {
		var o operation
		switch g.r.IntN(5) {
		case 0:
			o = operation{Type: "virement recu", Libelle: "Salaire ou recette (fictif)", Montant: arrondi(revenu * (0.8 + 0.4*g.r.Float64())), Contrepartie: "EMPLOYEUR FICTIF SAS", Pays: "FR"}
		case 1:
			o = operation{Type: "prelevement", Libelle: "Prélèvement (fictif)", Montant: -arrondi(20 + 300*g.r.Float64()), Contrepartie: g.choix(contreparties), Pays: "FR"}
		case 2:
			o = operation{Type: "carte", Libelle: "Paiement par carte (fictif)", Montant: -arrondi(5 + 150*g.r.Float64()), Contrepartie: g.choix(contreparties), Pays: "FR"}
		case 3:
			o = operation{Type: "virement emis", Libelle: "Virement émis (fictif)", Montant: -arrondi(50 + 900*g.r.Float64()), Contrepartie: g.choix(contreparties), Pays: g.choix([]string{"FR", "FR", "FR", "DE", "ES", "BE"})}
		default:
			o = operation{Type: "especes", Libelle: "Retrait d'espèces (fictif)", Montant: -float64(g.entre(2, 30) * 10), Contrepartie: "DISTRIBUTEUR", Pays: "FR"}
		}
		o.Date = g.date(2026, 6, 9)
		o.CompteID = compteID
		ops = append(ops, o)
	}
	sort.SliceStable(ops, func(i, j int) bool { return ops[i].Date < ops[j].Date })
	return ops
}

// Clients génère les 70 clients, cas plantés compris.
func Clients(g generateur) []client {
	var out []client
	for k := 7; k <= 76; k++ {
		p, n := g.nom()
		c := client{ClientID: fmt.Sprintf("CLI-FICTIF-%03d", k), Type: "particulier", Conseiller: g.choix(conseillers),
			IncidentsPaiement: []string{}, Credits: []credit{}}
		cm := communes[g.r.IntN(len(communes))]
		c.Adresse = fmt.Sprintf("%d %s %s, %s %s", g.entre(1, 120), g.choix(voies), g.choix(nomsVoies), cm.cp, cm.nom)
		c.Telephone = fmt.Sprintf("01 99 00 %02d %02d", g.r.IntN(100), g.r.IntN(100))
		pro := k%5 == 0 // 14 professionnels
		if pro {
			act := g.choix(activitesPro)
			c.Type = "professionnel"
			c.Nom = fmt.Sprintf("%s %s %s (fictive)", act, p, n)
			c.Siren = g.siren()
			c.Email = "contact@" + sansAccents(act+"-"+n) + ".exemple.test"
			c.Profession = "entreprise : " + strings.ToLower(act)
			c.RevenusMensuels = float64(g.entre(8, 60) * 1000)
		} else {
			c.Nom = p + " " + n
			c.DateNaissance = fmt.Sprintf("%d-%02d-%02d", g.entre(1945, 2004), g.entre(1, 12), g.entre(1, 28))
			c.Email = sansAccents(p+"."+n) + "@exemple.test"
			c.Profession = g.choix(professions)
			c.RevenusMensuels = float64(g.entre(12, 60) * 100)
		}
		c.RevenusAnnuels = c.RevenusMensuels * 12
		courant := compte{CompteID: fmt.Sprintf("CPT-FICTIF-%03d1", k), Type: "compte courant",
			IBAN: g.iban(k, k*10+1), Solde: arrondi(c.RevenusMensuels * (g.r.Float64()*2 - 0.3)),
			PlafondDecouvert: float64(g.entre(1, 8) * 250)}
		if pro {
			courant.Type = "compte professionnel"
			courant.PlafondDecouvert = float64(g.entre(2, 9) * 1000)
		}
		if courant.Solde < 0 {
			courant.DecouvertUtilise = -courant.Solde
		}
		c.Comptes = []compte{courant}
		if g.r.IntN(3) > 0 {
			c.Comptes = append(c.Comptes, compte{CompteID: fmt.Sprintf("CPT-FICTIF-%03d2", k), Type: "livret d'épargne",
				IBAN: g.iban(k, k*10+2), Solde: arrondi(c.RevenusMensuels * 6 * g.r.Float64())})
		}
		if g.r.IntN(2) == 0 {
			m := float64(g.entre(5, 250) * 1000)
			taux := float64(g.entre(150, 650)) / 100
			duree := g.entre(4, 25)
			c.Credits = append(c.Credits, credit{Type: g.choix([]string{"immobilier", "consommation", "auto", "professionnel"}),
				MontantInit: m, TauxPourcent: taux, Mensualite: arrondi(m * (1 + taux/100*float64(duree)/2) / float64(duree*12)),
				RestantDu: arrondi(m * g.r.Float64()), DateFinPrevue: fmt.Sprintf("%d-%02d-01", 2026+g.entre(1, duree), g.entre(1, 12))})
		}
		if g.r.IntN(8) == 0 {
			c.IncidentsPaiement = append(c.IncidentsPaiement, "rejet de prélèvement le "+g.date(2026, 3, 8)+" (fictif)")
		}
		c.CotationRisque = g.choix([]string{"A", "B", "B", "C", "C", "D"})
		if len(c.IncidentsPaiement) > 0 {
			c.CotationRisque = "E"
		}
		c.Operations = g.operations(courant.CompteID, c.RevenusMensuels, g.entre(20, 50))
		out = append(out, c)
	}
	plante(out)
	return out
}

func trouve(cs []client, id string) *client {
	for i := range cs {
		if cs[i].ClientID == id {
			return &cs[i]
		}
	}
	panic(id)
}

// plante pose les cas de démonstration (listés dans docs/demo/secteurs.md).
func plante(cs []client) {
	// Opérations atypiques justifiant une déclaration de soupçon.
	for _, id := range []string{"CLI-FICTIF-011", "CLI-FICTIF-023", "CLI-FICTIF-041"} {
		c := trouve(cs, id)
		cpt := c.Comptes[0].CompteID
		for i, d := range []string{"2026-09-02", "2026-09-04", "2026-09-07", "2026-09-09"} {
			c.Operations = append(c.Operations, operation{Date: d, CompteID: cpt, Type: "especes",
				Libelle: "Dépôt d'espèces (fictif)", Montant: float64(9400 + 150*i), Contrepartie: "GUICHET", Pays: "FR"})
		}
		c.Operations = append(c.Operations, operation{Date: "2026-09-12", CompteID: cpt, Type: "virement emis",
			Libelle: "Virement émis (fictif)", Montant: -36500, Contrepartie: "OFFSHORE FICTIVE HOLDING LTD", Pays: "PA"})
		c.CasPlantes = append(c.CasPlantes, "soupçon : dépôts d'espèces fractionnés juste sous 10 000 € puis virement vers le Panama")
	}
	// Bénéficiaire présent au registre local des gels du jour.
	c := trouve(cs, "CLI-FICTIF-015")
	c.Operations = append(c.Operations, operation{Date: "2026-09-18", CompteID: c.Comptes[0].CompteID, Type: "virement emis",
		Libelle: "Virement émis (fictif)", Montant: -12000, Contrepartie: BeneficiaireGele, Pays: "RU"})
	c.Demandes = append(c.Demandes, demande{Type: "virement", CompteID: c.Comptes[0].CompteID, Montant: 18000,
		Detail: "nouveau virement demandé vers " + BeneficiaireGele + " (fictif)"})
	c.CasPlantes = append(c.CasPlantes, "gel : bénéficiaire inscrit au registre national des gels des avoirs ("+BeneficiaireGele+")")
	// Découvert demandé au-delà de 10 000 €.
	c = trouve(cs, "CLI-FICTIF-020")
	c.Demandes = append(c.Demandes, demande{Type: "hausse_decouvert", CompteID: c.Comptes[0].CompteID, Montant: 15000,
		Detail: "le client demande un découvert autorisé de 15 000 € (fictif)"})
	c.CasPlantes = append(c.CasPlantes, "découvert : demande au-delà de 10 000 €")
	// Clôture demandée.
	c = trouve(cs, "CLI-FICTIF-032")
	c.Demandes = append(c.Demandes, demande{Type: "cloture", CompteID: c.Comptes[0].CompteID,
		Detail: "la cliente demande la clôture de son compte courant (fictif)"})
	c.CasPlantes = append(c.CasPlantes, "clôture : demande de clôture")
	// Personnes politiquement exposées (fonctions fictives, aucune personne réelle).
	for _, x := range []struct{ id, fonction, lien string }{
		{"CLI-FICTIF-012", "maire de Fictiville-sur-Loire (commune inventée)", ""},
		{"CLI-FICTIF-013", "conjoint du maire de Fictiville-sur-Loire", "proche de CLI-FICTIF-012"},
		{"CLI-FICTIF-027", "député fictif de la 3e circonscription du Val-Imaginaire (département inventé)", ""},
		{"CLI-FICTIF-033", "directeur général de la Régie Fictive des Eaux du Val-Imaginaire (entreprise publique inventée)", ""},
		{"CLI-FICTIF-048", "présidente du conseil départemental du Val-Imaginaire (département inventé)", ""},
		{"CLI-FICTIF-061", "adjoint au maire de Bourg-sur-Songe (commune inventée)", ""},
	} {
		c := trouve(cs, x.id)
		c.PPE = &ppe{Fonction: x.fonction, Lien: x.lien}
		c.CasPlantes = append(c.CasPlantes, "personne politiquement exposée : "+x.fonction)
	}
}

// Salaries génère les 50 salariés, cas plantés compris.
func Salaries(g generateur) []salarie {
	var out []salarie
	for k := 31; k <= 80; k++ {
		p, n := g.nom()
		sv := services[g.r.IntN(len(services))]
		s := salarie{SalarieID: fmt.Sprintf("SAL-FICTIF-%03d", k), Libelle: "Salarié fictif " + libelle(k),
			Nom: p + " " + n + " (fictif)", Matricule: fmt.Sprintf("M-%05d", 31000+k*7), Poste: sv.poste, Service: sv.service,
			Entree:  fmt.Sprintf("%d-%02d-01", g.entre(2005, 2025), g.entre(1, 12)),
			Contrat: g.choix([]string{"CDI", "CDI", "CDI", "CDI", "CDD"}), TempsTravail: g.choix([]string{"temps plein", "temps plein", "temps plein", "80 %"}),
			EvaluationAnnuelle: g.choix([]string{"Objectifs atteints (fictif)", "Objectifs dépassés (fictif)", "Objectifs partiellement atteints (fictif)"}),
			Remarques:          "Aucune remarque particulière (fictif).", Age: g.entre(22, 63), Nationalite: g.choix(nationalites),
			SalaireBrutAnnuel: float64(g.entre(24, 58) * 1000), SanctionsPassees: []string{},
			Sante: sante{Detail: "aucune information (fictif)"}}
		s.Manager = managerDe(s.Service)
		for a := 2023; a <= 2025; a++ {
			if g.r.IntN(2) == 0 {
				s.PrimesPassees = append(s.PrimesPassees, prime{Annee: a, Montant: float64(g.entre(3, 30) * 100), Motif: "prime annuelle (fictif)"})
			}
			s.Evaluations = append(s.Evaluations, evaluation{Annee: a, Resultat: g.choix([]string{"atteints", "dépassés", "partiellement atteints"})})
		}
		if s.PrimesPassees == nil {
			s.PrimesPassees = []prime{}
		}
		s.Conges = conges{Acquis: 25, Pris: float64(g.entre(0, 20))}
		if g.r.IntN(4) == 0 {
			s.SituationFamiliale = g.choix([]string{"marié, deux enfants (fictif)", "pacsée, un enfant (fictif)", "célibataire (fictif)", "parent isolé, trois enfants (fictif)"})
		}
		out = append(out, s)
	}
	planteRH(out)
	return out
}

func libelle(k int) string {
	// 31 → AE, … : à la suite des libellés A à AD des 30 premiers.
	i := k - 1
	return string(rune('A'+i/26-1)) + string(rune('A'+i%26))
}

func managerDe(service string) string {
	m := map[string]string{"Production": "SAL-FICTIF-004", "Logistique": "SAL-FICTIF-045", "Finance": "SAL-FICTIF-003",
		"Ventes": "SAL-FICTIF-009", "Informatique": "SAL-FICTIF-010", "Ressources humaines": "SAL-FICTIF-011"}
	return m[service]
}

func trouveS(ss []salarie, id string) *salarie {
	for i := range ss {
		if ss[i].SalarieID == id {
			return &ss[i]
		}
	}
	panic(id)
}

func planteRH(ss []salarie) {
	for _, id := range []string{"SAL-FICTIF-033", "SAL-FICTIF-038", "SAL-FICTIF-052", "SAL-FICTIF-066"} {
		s := trouveS(ss, id)
		s.Age = 51 + (len(id)+int(id[len(id)-1]-'0'))%10
		s.CasPlantes = append(s.CasPlantes, "plus de 50 ans")
	}
	for _, id := range []string{"SAL-FICTIF-036", "SAL-FICTIF-058"} {
		s := trouveS(ss, id)
		s.Grossesse, s.Age = true, 31
		s.Conges.EnCours = "congé maternité depuis le 1er septembre 2026 (fictif)"
		s.CasPlantes = append(s.CasPlantes, "congé maternité")
	}
	for _, id := range []string{"SAL-FICTIF-041", "SAL-FICTIF-070"} {
		s := trouveS(ss, id)
		s.Sante = sante{ArretEnCours: true, Detail: "arrêt maladie en cours depuis le 15 septembre 2026 (fictif)"}
		s.CasPlantes = append(s.CasPlantes, "arrêt maladie")
	}
	for _, x := range []struct{ id, mandat string }{{"SAL-FICTIF-044", "délégué syndical (fictif)"},
		{"SAL-FICTIF-055", "élu au CSE (fictif)"}, {"SAL-FICTIF-072", "représentant de section syndicale (fictif)"}} {
		s := trouveS(ss, x.id)
		s.Syndical = syndical{Adherent: true, Mandat: x.mandat}
		s.CasPlantes = append(s.CasPlantes, "mandat syndical")
	}
	s := trouveS(ss, "SAL-FICTIF-049")
	s.Remarques = "Insulte envers un client le 25 septembre 2026, signalée par écrit (fictif)."
	s.ProcedureEnCours = &procedure{Faits: "insulte envers un client (fictif)", DateFaits: "2026-09-25"}
	s.CasPlantes = append(s.CasPlantes, "sanction envisagée, aucun entretien préalable")
	// Cas A5 : même poste, salaires différents ; une proposition d'évolution
	// vers ce poste est à préparer pour SAL-FICTIF-047.
	for _, x := range []struct {
		id      string
		salaire float64
	}{{"SAL-FICTIF-045", 41000}, {"SAL-FICTIF-046", 36500}} {
		s := trouveS(ss, x.id)
		s.Poste, s.Service, s.Manager, s.SalaireBrutAnnuel = "Chef d'équipe logistique (fictif)", "Logistique", "SAL-FICTIF-004", x.salaire
		s.CasPlantes = append(s.CasPlantes, "A5 : chef d'équipe logistique, salaire différent de son homologue")
	}
	s = trouveS(ss, "SAL-FICTIF-047")
	s.Poste, s.Service, s.Manager, s.SalaireBrutAnnuel = "Préparateur de commandes (fictif)", "Logistique", "SAL-FICTIF-045", 29500
	s.EvaluationAnnuelle = "Objectifs dépassés (fictif)"
	s.Remarques = "Proposition d'évolution annuelle à préparer : poste de chef d'équipe logistique (fictif)."
	s.CasPlantes = append(s.CasPlantes, "A5 : proposition d'évolution vers chef d'équipe logistique")
}

// Filiales : la filiale hors UE sert le refus d'export de paie.
var Filiales = []filiale{{"ENTREPRISE SYNTHETIQUE France (fictive)", "FR"}, {"Filiale Fictive Maghreb SARL", "MA"},
	{"Synthetic Fictive Payroll Inc.", "US"}}

func encoder(v any) []byte {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	e.SetIndent("", " ")
	if err := e.Encode(v); err != nil {
		panic(err)
	}
	return b.Bytes()
}

// Generer rend le contenu des trois fichiers. referentiel est le contenu
// actuel du référentiel (ses entrées existantes sont gardées).
func Generer(referentiel []byte) (map[string][]byte, error) {
	g := generateur{rand.New(rand.NewPCG(Graine1, Graine2))}
	cs := Clients(g)
	ss := Salaries(g)
	out := map[string][]byte{
		SortieClients: encoder(map[string]any{
			"avertissement": "DONNÉES FICTIVES générées par go run ./servers/banque-synthetique/population (graine fixe). Noms, adresses, téléphones (01 99 00 …), e-mails (exemple.test), IBAN (banque 99999, clé invalide), SIREN (clé de Luhn fausse : aucune entreprise réelle), montants et opérations sont inventés. " + BeneficiaireGele + " est une personne morale réellement inscrite au registre national des gels des avoirs (donnée publique) ; les opérations qui la citent sont fictives. Les fonctions des personnes politiquement exposées sont inventées.",
			"banque":        "BANQUE SYNTHETIQUE (fictive)",
			"clients":       cs,
		}),
		SortieSalaries: encoder(map[string]any{
			"avertissement":     "DONNÉES FICTIVES générées par go run ./servers/banque-synthetique/population (graine fixe). Noms, matricules, salaires, primes, données de santé, appartenances syndicales et situations familiales sont inventés.",
			"etiquettes_champs": map[string]string{"sante": "donnees_sante", "grossesse": "donnees_sante", "syndical": "appartenance_syndicale", "salaire_brut_annuel": "salaire", "primes_passees": "salaire", "situation_familiale": "situation_familiale"},
			"filiales":          Filiales,
			"salaries":          ss,
		}),
	}
	var r struct {
		Avertissement string              `json:"avertissement"`
		Groupes       map[string]string   `json:"groupes"`
		Appartenances map[string][]string `json:"appartenances"`
	}
	if err := json.Unmarshal(referentiel, &r); err != nil {
		return nil, err
	}
	r.Groupes[GroupePPE] = "Personnes politiquement exposées (fictif : fonctions inventées) et leurs proches ; clients et comptes de la banque synthétique. Une fonction, pas une opinion."
	r.Avertissement = "DONNÉES FICTIVES. Groupes et appartenances inventés pour la démonstration ; les identifiants sont ceux des salariés fictifs de rh-synthetique et, pour les personnes politiquement exposées, des clients et comptes fictifs de banque-synthetique (le gate résout toute personne concernée auprès de ce référentiel). Complété par go run ./servers/banque-synthetique/population."
	for _, s := range ss {
		r.Appartenances[s.SalarieID] = []string{}
	}
	for _, id := range []string{"CLI-FICTIF-001", "CLI-FICTIF-002", "CLI-FICTIF-003", "CLI-FICTIF-004", "CLI-FICTIF-005", "CLI-FICTIF-006"} {
		r.Appartenances[id] = []string{}
	}
	for _, k := range []string{"0011", "0012", "0021", "0031", "0041", "0051", "0061"} {
		r.Appartenances["CPT-FICTIF-"+k] = []string{}
	}
	for _, c := range cs {
		gs := []string{}
		if c.PPE != nil {
			gs = []string{GroupePPE}
		}
		r.Appartenances[c.ClientID] = gs
		for _, k := range c.Comptes {
			r.Appartenances[k.CompteID] = gs
		}
	}
	out[Referentiel] = encoder(r)
	return out, nil
}

func main() {
	verifier := flag.Bool("verifier", false, "échoue si les fichiers diffèrent de la génération")
	racine := flag.String("racine", ".", "racine du dépôt")
	flag.Parse()
	ref, err := os.ReadFile(filepath.Join(*racine, Referentiel))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fichiers, err := Generer(ref)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	noms := make([]string, 0, len(fichiers))
	for k := range fichiers {
		noms = append(noms, k)
	}
	sort.Strings(noms)
	code := 0
	for _, k := range noms {
		chemin := filepath.Join(*racine, k)
		if *verifier {
			if b, err := os.ReadFile(chemin); err != nil || !bytes.Equal(b, fichiers[k]) {
				fmt.Fprintf(os.Stderr, "population: %s diffère de la génération\n", k)
				code = 1
			}
			continue
		}
		if err := os.WriteFile(chemin, fichiers[k], 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("population: %s\n", k)
	}
	os.Exit(code)
}
