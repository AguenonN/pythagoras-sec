package directive

// Identifiants du format de directive v1, retiré du dépôt (décision de
// Manu : une seule forme de directive). Aucun n'est plus lu ni produit par
// ce paquet : le chargeur refuse un document v1, Decide n'évalue que la
// forme v2. Ils ne restent que parce que des paquets tenus par d'autres
// sessions les nomment encore (internal/relecture, internal/console,
// internal/gate) ; ils seront supprimés quand ces usages auront disparu.
//
// Deprecated: format v1 retiré.

// Deprecated: format v1 retiré.
type Verrou string

// Deprecated: format v1 retiré.
const (
	Interdiction       Verrou = "interdiction"
	SeuilArgument      Verrou = "seuil_argument"
	ProvenanceSession  Verrou = "provenance_session"
	ApprobationHumaine Verrou = "approbation_humaine"
	ChampsAutorises    Verrou = "champs_autorises_par_destination"
	SourceAttestee     Verrou = "source_attestee"
)

// Deprecated: format v1 retiré ; la v2 écrit "dans" (OpDansV2), "in" est
// refusé au chargement et indéterminé à l'évaluation.
const OpDans Op = "in"

// Deprecated: format v1 retiré ; voir CibleV2.
type Cible struct {
	Serveur string
	Outil   string
}

// Deprecated: format v1 retiré ; voir la feuille session.a_consulte.
type Provenance struct {
	Serveurs []string
	Outils   []string
	Critere  Critere
}

// Deprecated: format v1 retiré ; voir l'effet caviarder.
type Champs struct {
	Autorises []string
	Mode      ModeChamps
}
