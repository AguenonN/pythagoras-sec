package gatee2e

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"mcprogue/internal/journal"
	"mcprogue/internal/rejeu"
)

// lignes rend les lignes du journal écrit par le scénario du point de
// contrôle, et le rang de l'entrée appel de rang seq.
func lignes(t *testing.T, chemin string, seq int) ([][]byte, int) {
	t.Helper()
	brut, err := os.ReadFile(chemin)
	if err != nil {
		t.Fatal(err)
	}
	ls := bytes.Split(bytes.TrimSpace(brut), []byte("\n"))
	for i, l := range ls {
		var e journal.Entree
		if err := json.Unmarshal(l, &e); err != nil {
			t.Fatal(err)
		}
		if e.Type == journal.AppelT && e.Seq == seq {
			return ls, i
		}
	}
	t.Fatalf("pas d'appel %d", seq)
	return nil, 0
}

// verifierEtRejouer rend le rejeu et les entrées d'un journal donné en
// octets, et l'erreur de vérification de sa chaîne.
func verifierEtRejouer(t *testing.T, b *banc, brut []byte) ([]rejeu.Rejeu, []journal.Entree, error) {
	t.Helper()
	_, errV := journal.Verifier(bytes.NewReader(brut), cleTest)
	entrees, err := journal.Lire(bytes.NewReader(brut))
	if err != nil {
		t.Fatal(err)
	}
	rs, err := rejeu.RejouerJournal(entrees, b.coffre, rejeu.Catalogue{b.jeu}, rejeu.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return rs, entrees, errV
}

func alterations(rs []rejeu.Rejeu, seq int) []rejeu.Ecart {
	var out []rejeu.Ecart
	for _, r := range rs {
		if r.Seq == seq {
			for _, e := range r.Ecarts {
				if e.Cause == rejeu.CauseAlteration {
					out = append(out, e)
				}
			}
		}
	}
	return out
}

// Le journal honnête du point de contrôle, puis altéré de deux façons :
// la vérification de la chaîne échoue, et le rejeu nomme l'altération.
func TestAlteration_JournalFalsifie(t *testing.T) {
	b := demarrer(t, registreSimple(t), jeuProvenance)
	scenario(t, b.cs)

	brut, err := os.ReadFile(b.cheminJ)
	if err != nil {
		t.Fatal(err)
	}
	if rs, _, errV := verifierEtRejouer(t, b, brut); errV != nil || len(rs) != 3 || len(alterations(rs, 3)) != 0 {
		t.Fatalf("journal honnête : %v, %d rejeu(x)", errV, len(rs))
	}

	t.Run("refus maquillé en autorisation", func(t *testing.T) {
		ls, i := lignes(t, b.cheminJ, 3)
		var e map[string]any
		if err := json.Unmarshal(ls[i], &e); err != nil {
			t.Fatal(err)
		}
		dec := e["decision"].(map[string]any)
		if dec["effet"] != "refuser" {
			t.Fatalf("effet %v", dec["effet"])
		}
		dec["effet"] = "autoriser" // prev et mac inchangés
		ls[i], _ = json.Marshal(e)

		rs, _, errV := verifierEtRejouer(t, b, bytes.Join(ls, []byte("\n")))
		if errV == nil {
			t.Fatal("la chaîne altérée se vérifie")
		}
		var controles []string
		for _, e := range alterations(rs, 3) {
			controles = append(controles, e.Controle)
		}
		if got := strings.Join(controles, ","); got != "statut,decision" {
			t.Fatalf("écarts de l'appel 3 : %v", got)
		}
	})

	t.Run("entrée appel supprimée", func(t *testing.T) {
		ls, i := lignes(t, b.cheminJ, 2)
		ls = append(ls[:i:i], ls[i+1:]...)
		rs, entrees, errV := verifierEtRejouer(t, b, bytes.Join(ls, []byte("\n")))
		if errV == nil {
			t.Fatal("la chaîne amputée se vérifie")
		}
		// L'appel parti a disparu du journal : sa transmission reste
		// orpheline, et l'appel suivant ne retrouve plus son état.
		if orph := rejeu.TransmissionsSansAppel(entrees); len(orph) != 1 || orph[0].Seq != 2 {
			t.Fatalf("orphelines %+v", orph)
		}
		if a := alterations(rs, 3); len(a) != 1 || a[0].Controle != rejeu.ControleEtat {
			t.Fatalf("écarts de l'appel 3 : %+v", a)
		}
	})
}
