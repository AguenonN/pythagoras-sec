package console

import (
	"strings"
	"testing"
)

// Point 4 (console-3) : un rejet se voit dans la frise du Poste et du
// Journal (badge « Rejeté », valideur, rôle, motif, heure, mention), dans la
// liste des demandes, et l'attente humaine est mesurée jusqu'au rejet.
func TestRejetVisible(t *testing.T) {
	b := nouveauBanc(t)
	id, fin := b.demandeAsync(claire, "Peut-on accorder un prêt de 120 000 euros ?")
	b.attendreStatut(id, StatutEnAttente)
	if _, err := b.Dep.Rejeter(id, anna, "directeur-risques", "motif"); err == nil {
		t.Fatal("rôle que le valideur ne détient pas accepté")
	}
	if _, err := b.Dep.Rejeter(id, anna, "analyste-risques", "Montant incohérent avec le dossier"); err != nil {
		t.Fatal(err)
	}
	if err := <-fin; err != nil {
		t.Fatal(err)
	}
	st := b.etat(id)
	sid := Sessions(st)[0].ID
	p, ok := b.Dep.Poste(st, b.jeux(), sid)
	if !ok {
		t.Fatal("poste")
	}
	var rej *Etape
	for i := range p.Etapes {
		if p.Etapes[i].Badge == BadgeRejete {
			rej = &p.Etapes[i]
		}
		if p.Etapes[i].Badge == BadgeAttente {
			t.Errorf("appel n° %d encore « en attente » après le rejet", p.Etapes[i].Seq)
		}
	}
	if rej == nil || rej.Libelle != "Rejeté" || rej.Rejet == nil {
		t.Fatalf("aucun appel rejeté : %+v", p.Etapes)
	}
	r := rej.Rejet
	if r.Valideur != anna || r.Role != "analyste-risques" || r.Nom == "" || !strings.Contains(r.Motif, "incohérent") ||
		r.Date.IsZero() || r.Mention != "décision consignée par la console, hors journal signé" {
		t.Errorf("rejet : %+v", r)
	}
	if p.Rejet == nil || p.Rejet.Seq != rej.Seq {
		t.Errorf("rejet de la demande : %+v", p.Rejet)
	}
	if p.Cout.Attentes != 1 || p.Cout.Rejets != 1 || p.Cout.AttenteSecondes < 0 {
		t.Errorf("attente humaine : %+v", p.Cout)
	}
	if !strings.Contains(p.Reponse.Titre, "Validation rejetée par") {
		t.Errorf("titre : %q", p.Reponse.Titre)
	}
	c, _ := b.Dep.Chaine(st, b.jeux(), sid)
	if c.Rejet == nil || c.Rejet.Seq != rej.Seq {
		t.Errorf("journal : %+v", c.Rejet)
	}
	for _, e := range c.Etapes {
		if e.Seq == rej.Seq && e.Badge != BadgeRejete {
			t.Errorf("journal : appel n° %d %q", e.Seq, e.Badge)
		}
	}
	_, corps := get(t, b.Srv, "/api/etat")
	if !strings.Contains(corps, `"rejetee":true`) {
		t.Errorf("liste des demandes sans le rejet : %.400s", corps)
	}
}
