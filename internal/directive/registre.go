package directive

import (
	"sort"
	"strings"
)

// EntreeRegistre est la vue qu'un registre de serveurs donne de lui-même à ce
// paquet : assez pour afficher à l'auteur, en lecture seule, les contrôles
// que l'engine applique sans qu'aucune directive ne les demande. Le paquet
// registre la remplit ; celui-ci ne fait que la lire.
type EntreeRegistre struct {
	Serveur         string
	Transport       string            // stdio | http | http-spiffe
	ModeAttestation string            // spiffe | tls-pin | binaire | aucune
	Attendu         string            // SPIFFE ID, SPKI ou sha256 du binaire
	Outils          map[string]string // nom d'outil → read | action
	SchemasEpingles bool
	Sampling        bool // sampling/createMessage autorisé
	Elicitation     bool // elicitation/create autorisé
}

// ControleRegistre est une ligne de la relecture : un contrôle en vigueur sur
// un serveur, marqué comme venant du registre, jamais d'une directive.
type ControleRegistre struct {
	Serveur  string
	Controle string
	Valeur   string
	Origine  string // toujours "registre"
}

// ControlesEnVigueur rend, triés par serveur puis contrôle, les contrôles
// que le registre impose. La relecture les affiche sous les
// directives, en lecture seule.
func ControlesEnVigueur(entrees []EntreeRegistre) []ControleRegistre {
	var out []ControleRegistre
	ajouter := func(serveur, controle, valeur string) {
		out = append(out, ControleRegistre{Serveur: serveur, Controle: controle, Valeur: valeur, Origine: "registre"})
	}
	for _, e := range entrees {
		mode := e.ModeAttestation
		if mode == "" {
			mode = "aucune"
		}
		ident := mode
		if e.Attendu != "" {
			ident += " " + e.Attendu
		}
		ajouter(e.Serveur, "identite_serveur", ident)
		ajouter(e.Serveur, "transport", e.Transport)
		ajouter(e.Serveur, "schemas_epingles", ouiNon(e.SchemasEpingles))
		ajouter(e.Serveur, "sampling_serveur", autoriseBloque(e.Sampling))
		ajouter(e.Serveur, "elicitation_serveur", autoriseBloque(e.Elicitation))
		noms := make([]string, 0, len(e.Outils))
		for n := range e.Outils {
			noms = append(noms, n)
		}
		sort.Strings(noms)
		for _, n := range noms {
			ajouter(e.Serveur, "outil_autorise", n+" ("+e.Outils[n]+")")
		}
		if len(noms) == 0 {
			ajouter(e.Serveur, "outil_autorise", "aucun : tout appel refusé")
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Serveur != out[j].Serveur {
			return out[i].Serveur < out[j].Serveur
		}
		if out[i].Controle != out[j].Controle {
			return out[i].Controle < out[j].Controle
		}
		return strings.Compare(out[i].Valeur, out[j].Valeur) < 0
	})
	return out
}

func ouiNon(b bool) string {
	if b {
		return "oui"
	}
	return "non"
}

func autoriseBloque(b bool) string {
	if b {
		return "autorisé"
	}
	return "bloqué"
}
