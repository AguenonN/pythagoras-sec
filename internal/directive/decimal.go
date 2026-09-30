package directive

import (
	"encoding/json"
	"math"
	"math/big"
	"strconv"
)

// Les nombres sont comparés en décimal exact, jamais en float64 : 0.1 dans
// une directive et 0.1 dans un appel sont égaux, 500000.0000000001 est
// strictement supérieur à 500000.

// Bornes d'un nombre lisible : au-delà, indéterminé (et refusé dans une
// directive). Elles évitent qu'un exposant démesuré ne coûte une allocation
// démesurée.
const (
	chiffresMax = 1000
	exposantMax = 1000
)

// decimal lit un nombre JSON sous les formes qu'un décodeur produit.
func decimal(v any) (*big.Rat, bool) {
	switch n := v.(type) {
	case json.Number:
		return lireDecimal(string(n))
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return nil, false
		}
		// La représentation décimale la plus courte est le texte JSON
		// d'origine : c'est elle qu'on compare, pas la valeur binaire.
		return lireDecimal(strconv.FormatFloat(n, 'g', -1, 64))
	case float32:
		f := float64(n)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, false
		}
		return lireDecimal(strconv.FormatFloat(f, 'g', -1, 32))
	case int:
		return new(big.Rat).SetInt64(int64(n)), true
	case int32:
		return new(big.Rat).SetInt64(int64(n)), true
	case int64:
		return new(big.Rat).SetInt64(n), true
	case uint32:
		return new(big.Rat).SetInt64(int64(n)), true
	case uint64:
		return new(big.Rat).SetInt(new(big.Int).SetUint64(n)), true
	}
	return nil, false
}

// lireDecimal suit la grammaire des nombres JSON (RFC 8259 §6), sans
// expression régulière.
func lireDecimal(s string) (*big.Rat, bool) {
	i := 0
	neg := false
	if i < len(s) && s[i] == '-' {
		neg = true
		i++
	}
	debut := i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	entiers := s[debut:i]
	if len(entiers) == 0 || (len(entiers) > 1 && entiers[0] == '0') {
		return nil, false
	}
	frac := ""
	if i < len(s) && s[i] == '.' {
		i++
		d := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		frac = s[d:i]
		if frac == "" {
			return nil, false
		}
	}
	exp := 0
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		signe := 1
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			if s[i] == '-' {
				signe = -1
			}
			i++
		}
		d := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			if i-d >= 6 {
				return nil, false
			}
			exp = exp*10 + int(s[i]-'0')
			i++
		}
		if i == d {
			return nil, false
		}
		exp *= signe
	}
	if i != len(s) || len(entiers)+len(frac) > chiffresMax || exp > exposantMax || exp < -exposantMax {
		return nil, false
	}
	m, ok := new(big.Int).SetString(entiers+frac, 10)
	if !ok {
		return nil, false
	}
	if neg {
		m.Neg(m)
	}
	echelle := exp - len(frac)
	r := new(big.Rat).SetInt(m)
	if echelle != 0 {
		p := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(abs(echelle))), nil)
		if echelle > 0 {
			r.Mul(r, new(big.Rat).SetInt(p))
		} else {
			r.Quo(r, new(big.Rat).SetInt(p))
		}
	}
	return r, true
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
