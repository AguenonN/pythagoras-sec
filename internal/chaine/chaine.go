// Package chaine écrit des lignes JSON chaînées par MAC : chaque ligne porte
// prev, le MAC de la précédente, et mac, le MAC de son propre contenu joint
// à prev. Retirer, réordonner ou modifier une ligne casse tous les MAC qui
// la suivent. Avec une clé, le MAC est un HMAC-SHA256 que seul le porteur de
// la clé peut produire ; sans clé, un SHA-256, qui n'atteste que l'absence
// d'accident.
//
// C'est la généralisation de internal/approval.Audit à un corps quelconque :
// le journal des décisions y met des entrées de plusieurs types dans une
// seule chaîne. approval.Audit reste tel quel pour mcp-proxy.
//
// Forme canonique : le corps est décodé en objet générique avec UseNumber,
// prev est posé, mac retiré, et encoding/json trie les clés à tous les
// niveaux. La même forme sert à sceller et à vérifier.
package chaine

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"os"
	"sync"
)

const (
	champPrev = "prev"
	champMAC  = "mac"
	// LigneMax borne une ligne lue. Un motif cite des noms de champs et des
	// directives, jamais des valeurs, donc les lignes restent courtes ; la
	// borne protège le lecteur d'un fichier corrompu.
	LigneMax = 1 << 20
)

var genese = hex.EncodeToString(make([]byte, sha256.Size))

// Genese est le prev de la première ligne d'une chaîne.
func Genese() string { return genese }

// Chaine écrit dans w. Une *Chaine nulle est valide et n'écrit rien.
type Chaine struct {
	mu   sync.Mutex
	w    io.Writer
	cle  []byte
	prev string
}

// Nouvelle démarre à la genèse. Pour continuer un fichier existant, lire
// DernierMAC puis Reprendre, ou passer par Ouvrir qui fait les deux.
func Nouvelle(w io.Writer, cle []byte) *Chaine {
	return &Chaine{w: w, cle: cle, prev: genese}
}

// Reprendre pointe la chaîne sur le MAC de la dernière ligne déjà écrite.
func (c *Chaine) Reprendre(prev string) {
	if c == nil || prev == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prev = prev
}

// Ajouter scelle et écrit une entrée. entree est encodée en JSON (un objet),
// ses champs prev et mac sont écrasés. La ligne écrite est rendue, sans le
// saut de ligne final, pour que l'appelant relise prev et mac.
func (c *Chaine) Ajouter(entree any) ([]byte, error) {
	if c == nil {
		return nil, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	corps, err := objet(entree)
	if err != nil {
		return nil, err
	}
	corps[champPrev] = c.prev
	delete(corps, champMAC)
	canon, err := json.Marshal(corps)
	if err != nil {
		return nil, fmt.Errorf("chaine: encoder: %w", err)
	}
	mac := c.sceller(c.prev, canon)
	corps[champMAC] = mac
	ligne, err := json.Marshal(corps)
	if err != nil {
		return nil, fmt.Errorf("chaine: encoder: %w", err)
	}
	if _, err := c.w.Write(append(ligne, '\n')); err != nil {
		return nil, fmt.Errorf("chaine: écrire: %w", err)
	}
	c.prev = mac
	return ligne, nil
}

func (c *Chaine) sceller(prev string, canon []byte) string {
	return sceller(c.cle, prev, canon)
}

func sceller(cle []byte, prev string, canon []byte) string {
	var h hash.Hash
	if len(cle) > 0 {
		h = hmac.New(sha256.New, cle)
	} else {
		h = sha256.New()
	}
	h.Write([]byte(prev))
	h.Write(canon)
	return hex.EncodeToString(h.Sum(nil))
}

// objet met une entrée sous forme d'objet générique, nombres préservés.
func objet(entree any) (map[string]any, error) {
	var b []byte
	switch e := entree.(type) {
	case []byte:
		b = e
	case json.RawMessage:
		b = e
	default:
		var err error
		if b, err = json.Marshal(entree); err != nil {
			return nil, fmt.Errorf("chaine: encoder l'entrée: %w", err)
		}
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("chaine: l'entrée n'est pas un objet JSON: %w", err)
	}
	if m == nil {
		return nil, fmt.Errorf("chaine: l'entrée est null")
	}
	return m, nil
}

// DernierMAC lit une chaîne existante et rend le MAC sur lequel la ligne
// suivante doit s'accrocher. Vide ou absent : la genèse. Une ligne illisible
// est une erreur, pas un nouveau départ.
func DernierMAC(r io.Reader) (string, error) {
	dernier := genese
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), LigneMax)
	for n := 1; sc.Scan(); n++ {
		ligne := bytes.TrimSpace(sc.Bytes())
		if len(ligne) == 0 {
			continue
		}
		m, err := objet(ligne)
		if err != nil {
			return "", fmt.Errorf("chaine: ligne %d: %w", n, err)
		}
		mac, _ := m[champMAC].(string)
		if mac == "" {
			return "", fmt.Errorf("chaine: ligne %d sans mac", n)
		}
		dernier = mac
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("chaine: lire: %w", err)
	}
	return dernier, nil
}

// Verifier parcourt une chaîne et contrôle chaque maillon. Il rend le nombre
// de lignes valides et, en cas d'erreur, la ligne fautive dans le message.
// Sans clé, la vérification n'a de valeur que contre les accidents.
func Verifier(r io.Reader, cle []byte) (int, error) {
	prev := genese
	n := 0
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), LigneMax)
	for ligne := 1; sc.Scan(); ligne++ {
		raw := bytes.TrimSpace(sc.Bytes())
		if len(raw) == 0 {
			continue
		}
		m, err := objet(raw)
		if err != nil {
			return n, fmt.Errorf("chaine: ligne %d: %w", ligne, err)
		}
		p, _ := m[champPrev].(string)
		if p != prev {
			return n, fmt.Errorf("chaine: ligne %d rompt la chaîne : elle suit %s, la précédente scelle à %s",
				ligne, court(p), court(prev))
		}
		veut, _ := m[champMAC].(string)
		delete(m, champMAC)
		canon, err := json.Marshal(m)
		if err != nil {
			return n, fmt.Errorf("chaine: ligne %d: %w", ligne, err)
		}
		if a := sceller(cle, prev, canon); a != veut {
			return n, fmt.Errorf("chaine: ligne %d altérée : elle scelle à %s, elle porte %s",
				ligne, court(a), court(veut))
		}
		prev = veut
		n++
	}
	if err := sc.Err(); err != nil {
		return n, fmt.Errorf("chaine: lire: %w", err)
	}
	return n, nil
}

// Ouvrir ouvre ou crée un fichier de chaîne et reprend au dernier MAC. Le
// fichier est ouvert en lecture-écriture pour être relu avant d'être
// prolongé ; un fichier illisible n'est pas prolongé.
func Ouvrir(chemin string, cle []byte) (*Chaine, *os.File, error) {
	f, err := os.OpenFile(chemin, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("chaine: ouvrir %s: %w", chemin, err)
	}
	prev, err := DernierMAC(f)
	if err != nil {
		f.Close()
		return nil, nil, fmt.Errorf("chaine: %s ne peut pas être prolongé: %w", chemin, err)
	}
	c := Nouvelle(f, cle)
	c.Reprendre(prev)
	return c, f, nil
}

func court(mac string) string {
	if len(mac) > 12 {
		return mac[:12]
	}
	return mac
}
