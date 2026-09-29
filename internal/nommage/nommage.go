// Package nommage fixe les règles de nommage partagées par le registre, les
// directives et l'agrégateur : le nom logique d'un serveur, le nom d'un outil
// tel que le serveur l'annonce, et le nom exposé à l'agent qui combine les
// deux.
//
// Une directive cible (serveur, outil) séparément et n'a jamais connaissance
// du nom exposé ; l'agrégateur est le seul à composer et décomposer.
package nommage

import (
	"fmt"
	"regexp"
	"strings"
)

// Separateur joint le nom logique du serveur et le nom de l'outil dans le
// nom exposé à l'agent. Il est interdit dans les noms logiques, donc la
// première occurrence sépare toujours sans ambiguïté, même si l'outil en
// contient un.
const Separateur = "__"

// LongueurMax borne le nom exposé. Mistral ne documente aucune contrainte sur
// les noms de fonctions ; 64 caractères sur [A-Za-z0-9_-] est la convention
// OpenAI-compatible que les clients respectent en pratique. À confirmer par
// un appel réel avant de relâcher la borne.
const LongueurMax = 64

var (
	// Un nom logique est minuscule, sans souligné : le souligné est réservé
	// au séparateur.
	nomLogique = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
	nomOutil   = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

// ValiderNomLogique accepte un nom de serveur du registre.
func ValiderNomLogique(nom string) error {
	if nom == "" {
		return fmt.Errorf("nommage: nom logique vide")
	}
	if strings.Contains(nom, Separateur) {
		return fmt.Errorf("nommage: nom logique %q contient le séparateur %q", nom, Separateur)
	}
	if !nomLogique.MatchString(nom) {
		return fmt.Errorf("nommage: nom logique %q invalide (minuscules, chiffres et tirets, sans tiret en bordure)", nom)
	}
	if len(nom)+len(Separateur) >= LongueurMax {
		return fmt.Errorf("nommage: nom logique %q trop long pour laisser place à un nom d'outil", nom)
	}
	return nil
}

// ValiderNomOutil accepte un nom d'outil tel qu'annoncé par un serveur.
func ValiderNomOutil(nom string) error {
	if nom == "" {
		return fmt.Errorf("nommage: nom d'outil vide")
	}
	if !nomOutil.MatchString(nom) {
		return fmt.Errorf("nommage: nom d'outil %q invalide (lettres, chiffres, souligné et tiret)", nom)
	}
	if len(nom) > LongueurMax {
		return fmt.Errorf("nommage: nom d'outil %q dépasse %d caractères", nom, LongueurMax)
	}
	return nil
}

// Exposer compose le nom vu par l'agent.
func Exposer(serveur, outil string) (string, error) {
	if err := ValiderNomLogique(serveur); err != nil {
		return "", err
	}
	if err := ValiderNomOutil(outil); err != nil {
		return "", err
	}
	expose := serveur + Separateur + outil
	if len(expose) > LongueurMax {
		return "", fmt.Errorf("nommage: nom exposé %q dépasse %d caractères", expose, LongueurMax)
	}
	return expose, nil
}

// Decomposer retrouve (serveur, outil) depuis un nom exposé.
func Decomposer(expose string) (serveur, outil string, err error) {
	i := strings.Index(expose, Separateur)
	if i < 0 {
		return "", "", fmt.Errorf("nommage: %q ne porte pas le séparateur %q", expose, Separateur)
	}
	serveur, outil = expose[:i], expose[i+len(Separateur):]
	if err := ValiderNomLogique(serveur); err != nil {
		return "", "", err
	}
	if err := ValiderNomOutil(outil); err != nil {
		return "", "", err
	}
	return serveur, outil, nil
}
