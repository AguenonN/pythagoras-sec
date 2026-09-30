// Package registre tient la liste déclarative des serveurs que l'engine
// accepte de joindre : pour chacun, son nom logique, son transport, sa
// cible, l'attestation attendue et le catalogue épinglé des outils qu'il
// peut exposer, chacun classé en lecture ou en action.
//
// Le registre est décidé par l'opérateur, pas par le juriste : ce qu'il
// impose (identité du serveur, schémas épinglés, outils autorisés,
// sampling et elicitation bloqués) est toujours actif et ne dépend
// d'aucune directive. Il est chargé et validé au démarrage ; toute erreur
// arrête le chargement, rendue en bloc, une ligne par défaut.
//
// Comme directive.Charger, ce paquet ne lit aucun fichier : l'appelant
// fournit les octets.
package registre

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"mcprogue/internal/directive"
	"mcprogue/internal/nommage"
	"mcprogue/internal/toolpin"
)

// Transport d'un serveur.
type Transport string

const (
	// Stdio lance un binaire local, après vérification de son empreinte.
	Stdio Transport = "stdio"
	// HTTP joint un serveur distant en HTTP streamable, TLS épinglé.
	HTTP Transport = "http"
	// HTTPSpiffe joint un serveur distant en mTLS SPIFFE. Réservé : le
	// transport n'existe pas encore et le chargeur le refuse.
	HTTPSpiffe Transport = "http-spiffe"
)

// Modes d'attestation, dans le vocabulaire de directive.Attestation.
const (
	ModeBinaire = "binaire"
	ModeTLSPin  = "tls-pin"
	ModeSpiffe  = "spiffe"
)

// Acces classe un outil : lecture seule ou action sur le monde.
type Acces string

const (
	Lecture Acces = "read"
	Action  Acces = "action"
)

// MetaRegistre identifie le registre chargé.
type MetaRegistre struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Auteur  string `json:"auteur"`
	// Empreinte est calculée au chargement sur le tableau serveurs[]
	// canonique. Présente dans le document, elle doit correspondre.
	Empreinte string `json:"empreinte,omitempty"`
}

// Commande est la cible d'un serveur stdio : un chemin absolu, jamais
// résolu dans le PATH, et ses arguments. Le processus est lancé avec un
// environnement réduit aux variables que Env nomme : les autres secrets de
// l'engine ne passent pas au serveur.
type Commande struct {
	Chemin string   `json:"chemin"`
	Args   []string `json:"args,omitempty"`
	// Env nomme les variables transmises au serveur, avec la valeur
	// qu'elles ont dans l'environnement de l'engine au lancement. Le
	// registre ne porte jamais de valeur.
	Env []string `json:"env,omitempty"`
}

// Attestation attendue d'un serveur.
type Attestation struct {
	Mode string `json:"mode"`
	// Attendu : "sha256:<hex>" du binaire (binaire), de la SubjectPublicKeyInfo
	// DER du certificat feuille (tls-pin), ou un SPIFFE ID (spiffe).
	Attendu string `json:"attendu"`
	// ServerName est imposé à la poignée de main TLS et vérifié contre le
	// certificat (tls-pin).
	ServerName string `json:"server_name,omitempty"`
}

// Outil est une entrée du catalogue épinglé. Nom, description et schéma
// d'entrée sont ce que l'agent lit ; ce sont eux que l'engine expose, et
// eux que l'engine compare à ce que le serveur annonce.
type Outil struct {
	Nom         string         `json:"nom"`
	Acces       Acces          `json:"acces"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// Empreinte est le pin de l'outil, calculé comme toolpin.Hash sur les trois
// champs que le modèle lit.
func (o Outil) Empreinte() (string, error) {
	return toolpin.Hash(map[string]any{"name": o.Nom, "description": o.Description, "inputSchema": o.InputSchema})
}

// Serveur est une entrée du registre.
type Serveur struct {
	Nom         string      `json:"nom"`
	Transport   Transport   `json:"transport"`
	Commande    *Commande   `json:"commande,omitempty"`
	URL         string      `json:"url,omitempty"`
	Attestation Attestation `json:"attestation"`
	Outils      []Outil     `json:"outils"`
}

// Outil rend l'entrée épinglée d'un outil.
func (s Serveur) Outil(nom string) (Outil, bool) {
	for _, o := range s.Outils {
		if o.Nom == nom {
			return o, true
		}
	}
	return Outil{}, false
}

// Registre est le document chargé.
type Registre struct {
	Meta     MetaRegistre `json:"registre"`
	Serveurs []Serveur    `json:"serveurs"`
}

// Serveur rend l'entrée d'un nom logique.
func (r *Registre) Serveur(nom string) (Serveur, bool) {
	for _, s := range r.Serveurs {
		if s.Nom == nom {
			return s, true
		}
	}
	return Serveur{}, false
}

// Entrees rend la vue que directive.ControlesEnVigueur affiche au juriste.
// Sampling et elicitation sont toujours bloqués : l'engine ne câble aucun
// gestionnaire.
func (r *Registre) Entrees() []directive.EntreeRegistre {
	out := make([]directive.EntreeRegistre, 0, len(r.Serveurs))
	for _, s := range r.Serveurs {
		outils := make(map[string]string, len(s.Outils))
		for _, o := range s.Outils {
			outils[o.Nom] = string(o.Acces)
		}
		out = append(out, directive.EntreeRegistre{
			Serveur: s.Nom, Transport: string(s.Transport), ModeAttestation: s.Attestation.Mode,
			Attendu: s.Attestation.Attendu, Outils: outils, SchemasEpingles: true,
		})
	}
	return out
}

// Charger lit un registre depuis ses octets et le valide strictement.
func Charger(donnees []byte) (*Registre, error) {
	dec := json.NewDecoder(bytes.NewReader(donnees))
	dec.DisallowUnknownFields()
	var r Registre
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("registre: document illisible: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("registre: contenu inattendu après le registre")
	}
	if err := Valider(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

var (
	empreinteHex = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	// nomVariable : un nom de variable d'environnement, sans valeur.
	nomVariable = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// Valider applique le schéma strict et fixe l'empreinte.
func Valider(r *Registre) error {
	var v validation
	if r.Meta.ID == "" {
		v.f("registre: id manquant")
	}
	if r.Meta.Version < 1 {
		v.f("registre: version doit être >= 1")
	}
	if r.Meta.Auteur == "" {
		v.f("registre: auteur manquant")
	}
	if len(r.Serveurs) == 0 {
		v.f("registre: aucun serveur")
	}
	vus := map[string]bool{}
	for i := range r.Serveurs {
		s := &r.Serveurs[i]
		p := fmt.Sprintf("serveur %d", i+1)
		if s.Nom != "" {
			p = "serveur " + s.Nom
		}
		if vus[s.Nom] {
			v.f("%s: nom logique en double", p)
		}
		vus[s.Nom] = true
		validerServeur(&v, p, s)
	}
	if err := v.err(); err != nil {
		return err
	}
	empreinte, err := Empreinte(r.Serveurs)
	if err != nil {
		return err
	}
	if r.Meta.Empreinte != "" && r.Meta.Empreinte != empreinte {
		return fmt.Errorf("registre: l'empreinte déclarée %s ne correspond pas au contenu (%s)", r.Meta.Empreinte, empreinte)
	}
	r.Meta.Empreinte = empreinte
	return nil
}

// Empreinte calcule sha256 du tableau serveurs[] en JSON canonique.
func Empreinte(serveurs []Serveur) (string, error) {
	b, err := json.Marshal(serveurs)
	if err != nil {
		return "", fmt.Errorf("registre: encoder pour empreinte: %w", err)
	}
	var generique any
	if err := json.Unmarshal(b, &generique); err != nil {
		return "", fmt.Errorf("registre: canoniser pour empreinte: %w", err)
	}
	canon, err := json.Marshal(generique)
	if err != nil {
		return "", fmt.Errorf("registre: canoniser pour empreinte: %w", err)
	}
	sum := sha256.Sum256(canon)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func validerServeur(v *validation, p string, s *Serveur) {
	if err := nommage.ValiderNomLogique(s.Nom); err != nil {
		v.f("%s: %v", p, err)
	}
	a := s.Attestation
	switch s.Transport {
	case Stdio:
		if s.Commande == nil || s.Commande.Chemin == "" {
			v.f("%s: le transport stdio exige commande.chemin", p)
		} else if !filepath.IsAbs(s.Commande.Chemin) {
			v.f("%s: commande.chemin %q doit être absolu (aucune résolution dans le PATH)", p, s.Commande.Chemin)
		}
		if s.URL != "" {
			v.f("%s: le transport stdio n'admet pas d'url", p)
		}
		if s.Commande != nil {
			vues := map[string]bool{}
			for _, nom := range s.Commande.Env {
				switch {
				case strings.Contains(nom, "="):
					// Ne pas citer l'entrée : elle porte peut-être une valeur.
					v.f("%s: commande.env ne contient que des noms de variables, jamais de valeur (entrée avec « = » refusée)", p)
				case !nomVariable.MatchString(nom):
					v.f("%s: commande.env : nom de variable %q invalide", p, nom)
				case vues[nom]:
					v.f("%s: commande.env : variable %s en double", p, nom)
				}
				vues[nom] = true
			}
		}
		if a.Mode != ModeBinaire {
			v.f("%s: le transport stdio exige l'attestation binaire (mode %q)", p, a.Mode)
		}
	case HTTP:
		if s.Commande != nil {
			v.f("%s: le transport http n'admet pas de commande", p)
		}
		if !strings.HasPrefix(s.URL, "https://") {
			v.f("%s: le transport http exige une url https:// (url %q)", p, s.URL)
		}
		if a.Mode != ModeTLSPin {
			v.f("%s: le transport http exige l'attestation tls-pin (mode %q)", p, a.Mode)
		}
	case HTTPSpiffe:
		v.f("%s: le transport http-spiffe n'est pas encore disponible ; retirer le serveur ou changer de transport", p)
	default:
		v.f("%s: transport %q inconnu", p, s.Transport)
	}
	switch a.Mode {
	case ModeBinaire, ModeTLSPin:
		if !empreinteHex.MatchString(a.Attendu) {
			v.f("%s: attestation.attendu %q doit s'écrire sha256:<64 hexadécimaux minuscules>", p, a.Attendu)
		}
	case ModeSpiffe:
		if !strings.HasPrefix(a.Attendu, "spiffe://") {
			v.f("%s: attestation.attendu %q doit être un SPIFFE ID", p, a.Attendu)
		}
	default:
		v.f("%s: mode d'attestation %q inconnu ou non admis (aucune attestation n'est jamais admise)", p, a.Mode)
	}
	if a.Mode == ModeTLSPin && a.ServerName == "" {
		v.f("%s: l'attestation tls-pin exige server_name", p)
	}
	if a.Mode != ModeTLSPin && a.ServerName != "" {
		v.f("%s: server_name n'a de sens qu'avec tls-pin", p)
	}

	if len(s.Outils) == 0 {
		v.f("%s: aucun outil épinglé", p)
	}
	outils := map[string]bool{}
	for i, o := range s.Outils {
		q := fmt.Sprintf("%s: outil %d", p, i+1)
		if o.Nom != "" {
			q = p + ": outil " + o.Nom
		}
		if outils[o.Nom] {
			v.f("%s: nom d'outil en double", q)
		}
		outils[o.Nom] = true
		if _, err := nommage.Exposer(s.Nom, o.Nom); err != nil {
			v.f("%s: %v", q, err)
		}
		switch o.Acces {
		case Lecture, Action:
		default:
			v.f("%s: acces %q inconnu (read ou action)", q, o.Acces)
		}
		if strings.TrimSpace(o.Description) == "" {
			v.f("%s: description manquante", q)
		}
		if o.InputSchema == nil {
			v.f("%s: input_schema manquant", q)
		} else if o.InputSchema["type"] != "object" {
			v.f("%s: input_schema doit être de type object", q)
		}
	}
}

// Noms rend les noms logiques, triés.
func (r *Registre) Noms() []string {
	out := make([]string, 0, len(r.Serveurs))
	for _, s := range r.Serveurs {
		out = append(out, s.Nom)
	}
	sort.Strings(out)
	return out
}

type validation struct{ errs []error }

func (v *validation) f(format string, args ...any) {
	v.errs = append(v.errs, fmt.Errorf("registre: "+format, args...))
}

func (v *validation) err() error { return errors.Join(v.errs...) }
