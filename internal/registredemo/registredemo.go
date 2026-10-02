// Package registredemo construit le registre de démonstration depuis le
// manifeste de construction des serveurs (bin/manifeste.json, écrit par
// scripts/construire-serveurs.sh) et les schema.json versionnés.
//
// Ce que le registre produit tient du manifeste, jamais d'une saisie :
//
//   - huit serveurs, en stdio, dans l'ordre du manifeste ; taux-imposteur
//     n'y figure jamais comme serveur ;
//   - attestation binaire : l'empreinte que le manifeste donne, vérifiée
//     sur le fichier présent ;
//   - catalogue épinglé : chaque outil de tools/list (schema.json, dont
//     l'empreinte est vérifiée) avec son nom, sa description et son
//     inputSchema recopiés tels quels, puisque ce sont eux que le backend
//     compare à ce que le serveur annonce ;
//   - accès : read si l'annotation readOnlyHint est vraie, action sinon,
//     y compris sans annotation (fermé par défaut) ;
//   - environnement transmis par nom : DONNEES_MODE pour tous (gels-avoirs
//     reçoit en plus -mode=REJEU, qui l'emporte : registre local du matin),
//     PISTE_CLIENT_ID et PISTE_CLIENT_SECRET en plus pour legifrance ;
//   - chemins absolus : binaire, enregistrements, instantané ;
//   - étiquettes : celles de demo/etiquettes.json (versionné), posées sur
//     les serveurs et les outils ; elles entrent dans l'empreinte.
//
// Le registre d'attaque est le même document, à une différence près : la
// commande de taux-officiel lance le binaire de taux-imposteur, et
// l'empreinte attendue reste celle de l'officiel. C'est l'entrée
// détournée ; le gate doit refuser de la lancer.
package registredemo

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"mcprogue/internal/registre"
)

// Imposteur est le nom logique de l'artefact d'attaque : présent au
// manifeste, jamais au registre comme serveur.
const Imposteur = "taux-imposteur"

// Detourne est le serveur dont la commande est détournée dans le registre
// d'attaque.
const Detourne = "taux-officiel"

// Serveurs est la liste attendue des huit serveurs du registre, dans
// l'ordre du manifeste.
var Serveurs = []string{
	"taux-officiel", "recherche-entreprises", "gels-avoirs", "eurlex",
	"legifrance", "banque-synthetique", "rh-synthetique", "referentiel-rh",
}

// Variables d'environnement transmises par nom.
const (
	VarMode = "DONNEES_MODE"
)

var envSupplementaire = map[string][]string{
	"legifrance": {"PISTE_CLIENT_ID", "PISTE_CLIENT_SECRET"},
}

// AvecEnregistrements : serveurs dont le répertoire d'enregistrements
// versionné est servers/<nom>/enregistrements. gels-avoirs a le sien
// (Options.Gels).
var AvecEnregistrements = map[string]bool{
	"taux-officiel": true, "recherche-entreprises": true, "eurlex": true, "legifrance": true,
}

// Meta du registre de démonstration.
// Version 2 : les étiquettes de demo/etiquettes.json entrent au registre.
var Meta = registre.MetaRegistre{ID: "demo", Version: 2, Auteur: "ops@cabinet.fr"}

// Manifeste est bin/manifeste.json.
type Manifeste struct {
	Construction map[string]string `json:"construction"`
	Serveurs     []EntreeManifeste `json:"serveurs"`
}

// EntreeManifeste décrit un binaire construit.
type EntreeManifeste struct {
	Nom          string `json:"nom"`
	Binaire      string `json:"binaire"`
	SHA256       string `json:"sha256"`
	Schema       string `json:"schema"`
	SchemaSHA256 string `json:"schema_sha256"`
}

// Options de génération.
type Options struct {
	// Racine est la racine absolue du dépôt : les chemins du manifeste
	// (binaire, schéma) y sont relatifs.
	Racine string
	// Gels est le répertoire d'enregistrements de gels-avoirs, relatif à
	// la racine ou absolu. Défaut : le registre local complet
	// (servers/gels-avoirs/enregistrements_locaux, hors Git). Les tests
	// passent servers/gels-avoirs/enregistrements (registre synthétique).
	Gels string
	// Enregistrements est le répertoire, relatif à la racine ou absolu, qui
	// contient un sous-répertoire par serveur de données officielles
	// (<Enregistrements>/<nom>). Vide : servers/<nom>/enregistrements, les
	// enregistrements versionnés. En DONNEES_MODE=MIXTE, un serveur écrit
	// chaque réponse obtenue dans son répertoire : le lanceur de démo y met
	// une copie de travail, pour que la démo ne réécrive jamais les
	// enregistrements versionnés que les tests rejouent.
	Enregistrements string
	// Etiquettes est le fichier des étiquettes, relatif à la racine ou
	// absolu. Vide : FichierEtiquettes, le fichier versionné.
	Etiquettes string
}

// DossierGelsLocal est le registre des gels complet, préparé par
// « gels-avoirs preparer ».
const DossierGelsLocal = "servers/gels-avoirs/enregistrements_locaux"

// DossierGelsSynthetique est le registre des gels synthétique versionné.
const DossierGelsSynthetique = "servers/gels-avoirs/enregistrements"

// LireManifeste lit et décode le manifeste, strictement.
func LireManifeste(chemin string) (*Manifeste, error) {
	b, err := os.ReadFile(chemin)
	if err != nil {
		return nil, fmt.Errorf("registredemo: manifeste: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var m Manifeste
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("registredemo: manifeste illisible: %w", err)
	}
	return &m, nil
}

// Generer construit les deux registres, validés par registre.Valider
// (empreinte comprise). Toute incohérence entre le manifeste, les
// binaires et les schémas est une erreur.
func Generer(m *Manifeste, o Options) (demo, attaque *registre.Registre, err error) {
	if !filepath.IsAbs(o.Racine) {
		return nil, nil, fmt.Errorf("registredemo: la racine %q doit être absolue", o.Racine)
	}
	if o.Gels == "" {
		o.Gels = DossierGelsLocal
	}
	entrees := map[string]EntreeManifeste{}
	for _, e := range m.Serveurs {
		if _, deja := entrees[e.Nom]; deja {
			return nil, nil, fmt.Errorf("registredemo: %s en double au manifeste", e.Nom)
		}
		entrees[e.Nom] = e
	}
	attendus := append(append([]string{}, Serveurs...), Imposteur)
	var errs []error
	for _, nom := range attendus {
		if _, ok := entrees[nom]; !ok {
			errs = append(errs, fmt.Errorf("registredemo: %s absent du manifeste", nom))
		}
	}
	if len(entrees) != len(attendus) {
		var inconnus []string
		for nom := range entrees {
			if !contient(attendus, nom) {
				inconnus = append(inconnus, nom)
			}
		}
		sort.Strings(inconnus)
		if len(inconnus) > 0 {
			errs = append(errs, fmt.Errorf("registredemo: serveur(s) inattendu(s) au manifeste : %s", strings.Join(inconnus, ", ")))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, nil, err
	}
	// Les binaires sont vérifiés tous, imposteur compris : le registre
	// d'attaque le désigne.
	for _, nom := range attendus {
		if err := verifierFichier(o.Racine, entrees[nom].Binaire, entrees[nom].SHA256); err != nil {
			errs = append(errs, fmt.Errorf("registredemo: %s: binaire: %w", nom, err))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, nil, err
	}

	demo = &registre.Registre{Meta: Meta}
	for _, nom := range Serveurs {
		e := entrees[nom]
		outils, err := OutilsDepuisSchema(o.Racine, e)
		if err != nil {
			return nil, nil, err
		}
		demo.Serveurs = append(demo.Serveurs, registre.Serveur{
			Nom:         nom,
			Transport:   registre.Stdio,
			Commande:    commande(o, nom, abs(o.Racine, e.Binaire)),
			Attestation: registre.Attestation{Mode: registre.ModeBinaire, Attendu: "sha256:" + e.SHA256},
			Outils:      outils,
		})
	}
	chemin := o.Etiquettes
	if chemin == "" {
		chemin = FichierEtiquettes
	}
	etiquettes, err := LireEtiquettes(abs(o.Racine, chemin))
	if err != nil {
		return nil, nil, err
	}
	if err := etiquettes.Appliquer(demo); err != nil {
		return nil, nil, err
	}
	if err := registre.Valider(demo); err != nil {
		return nil, nil, err
	}

	// Le registre d'attaque : une copie profonde, dont seule la commande
	// de taux-officiel change.
	attaque, err = copier(demo)
	if err != nil {
		return nil, nil, err
	}
	attaque.Meta = registre.MetaRegistre{ID: Meta.ID + "-attaque", Version: Meta.Version, Auteur: Meta.Auteur}
	for i := range attaque.Serveurs {
		if attaque.Serveurs[i].Nom == Detourne {
			attaque.Serveurs[i].Commande.Chemin = abs(o.Racine, entrees[Imposteur].Binaire)
		}
	}
	if err := registre.Valider(attaque); err != nil {
		return nil, nil, err
	}
	return demo, attaque, nil
}

// commande rend la commande d'un serveur : chemin absolu, arguments
// absolus, variables nommées.
func commande(o Options, nom, binaire string) *registre.Commande {
	c := &registre.Commande{Chemin: binaire, Env: append([]string{VarMode}, envSupplementaire[nom]...)}
	switch {
	case AvecEnregistrements[nom]:
		dir := filepath.Join("servers", nom, "enregistrements")
		if o.Enregistrements != "" {
			dir = filepath.Join(o.Enregistrements, nom)
		}
		c.Args = []string{"-enregistrements", abs(o.Racine, dir)}
	case nom == "gels-avoirs":
		// Toujours en REJEU sur le registre local (gels-avoirs preparer, le
		// matin de la démo) : le registre complet met jusqu'à 90 s à venir
		// de l'API, aucune demande ne doit l'attendre.
		c.Args = []string{"-enregistrements", abs(o.Racine, o.Gels), "-mode=REJEU"}
	case nom == "banque-synthetique":
		c.Args = []string{"-instantane", abs(o.Racine, "servers/taux-officiel/instantane_trimestre_precedent.json")}
	}
	return c
}

// OutilsDepuisSchema lit le schema.json d'une entrée du manifeste, vérifie
// son empreinte et le convertit du format tools/list vers le format du
// registre.
func OutilsDepuisSchema(racine string, e EntreeManifeste) ([]registre.Outil, error) {
	b, err := lireVerifie(racine, e.Schema, e.SchemaSHA256)
	if err != nil {
		return nil, fmt.Errorf("registredemo: %s: schéma: %w", e.Nom, err)
	}
	outils, err := Convertir(b)
	if err != nil {
		return nil, fmt.Errorf("registredemo: %s: %w", e.Nom, err)
	}
	return outils, nil
}

// outilListe est un outil tel que tools/list le rend. Seuls les champs
// lus sont décodés ; les autres (outputSchema, title…) n'entrent pas au
// registre.
type outilListe struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations *struct {
		ReadOnlyHint *bool `json:"readOnlyHint"`
	} `json:"annotations"`
}

// Convertir passe du format tools/list (tableau d'outils) au catalogue du
// registre, dans l'ordre de la liste. L'accès est read si et seulement si
// readOnlyHint est vrai.
func Convertir(toolsList []byte) ([]registre.Outil, error) {
	var liste []outilListe
	if err := json.Unmarshal(toolsList, &liste); err != nil {
		return nil, fmt.Errorf("tools/list illisible: %w", err)
	}
	if len(liste) == 0 {
		return nil, errors.New("tools/list vide")
	}
	out := make([]registre.Outil, 0, len(liste))
	for _, t := range liste {
		if t.Name == "" || t.InputSchema == nil {
			return nil, fmt.Errorf("outil %q sans nom ou sans inputSchema", t.Name)
		}
		acces := registre.Action
		if t.Annotations != nil && t.Annotations.ReadOnlyHint != nil && *t.Annotations.ReadOnlyHint {
			acces = registre.Lecture
		}
		out = append(out, registre.Outil{Nom: t.Name, Acces: acces, Description: t.Description, InputSchema: t.InputSchema})
	}
	return out, nil
}

// Encoder rend le document JSON d'un registre, indenté, terminé par un
// saut de ligne. encoding/json trie les clés des objets : deux
// générations sur les mêmes entrées rendent les mêmes octets.
func Encoder(r *registre.Registre) ([]byte, error) {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	b = append(b, '\n')
	// Relu par le chargeur strict, comme le gate le relira.
	if _, err := registre.Charger(b); err != nil {
		return nil, fmt.Errorf("registredemo: le registre produit ne se recharge pas: %w", err)
	}
	return b, nil
}

func abs(racine, chemin string) string {
	if filepath.IsAbs(chemin) {
		return filepath.Clean(chemin)
	}
	return filepath.Join(racine, chemin)
}

func verifierFichier(racine, chemin, attendu string) error {
	f, err := os.Open(abs(racine, chemin))
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if vu := hex.EncodeToString(h.Sum(nil)); vu != attendu {
		return fmt.Errorf("%s : empreinte %s, le manifeste dit %s (reconstruire : scripts/construire-serveurs.sh)", chemin, vu, attendu)
	}
	return nil
}

func lireVerifie(racine, chemin, attendu string) ([]byte, error) {
	b, err := os.ReadFile(abs(racine, chemin))
	if err != nil {
		return nil, err
	}
	somme := sha256.Sum256(b)
	if vu := hex.EncodeToString(somme[:]); vu != attendu {
		return nil, fmt.Errorf("%s : empreinte %s, le manifeste dit %s (reconstruire : scripts/construire-serveurs.sh)", chemin, vu, attendu)
	}
	return b, nil
}

func copier(r *registre.Registre) (*registre.Registre, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	var c registre.Registre
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func contient(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}
