package console

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

// Point 2 (console-3) : le mode du compilateur est transmis au service, et
// une panne du service s'affiche en français, sans nom d'exception.
func TestCompilateurModeEtPannes(t *testing.T) {
	b := nouveauBanc(t)
	for _, mode := range []string{"reel", "rejeu"} {
		b.Dep.Commandes.ModeCompilateur = mode
		c, err := b.Dep.Compiler("Quel mode ? "+mode, "", "", nil, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(c.Erreurs) != 1 || c.Erreurs[0] != "mode : "+mode {
			t.Errorf("mode %s : le service a reçu %v", mode, c.Erreurs)
		}
	}
	rec := httptest.NewRecorder()
	b.Srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "http://127.0.0.1/api/redaction", nil))
	var red struct {
		Mode string `json:"mode_compilateur"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &red); err != nil || red.Mode != "rejeu" {
		t.Errorf("mode affiché : %q (%v)", red.Mode, err)
	}

	panne := func(texte string, argv []string, attendu string) {
		t.Helper()
		sauf := b.Dep.Commandes.Compilateur
		if argv != nil {
			b.Dep.Commandes.Compilateur = argv
		}
		defer func() { b.Dep.Commandes.Compilateur = sauf }()
		_, err := b.Dep.Compiler(texte, "", "", nil, "")
		var ea *ErreurAction
		if !errors.As(err, &ea) {
			t.Fatalf("%q : erreur %v", texte, err)
		}
		if !strings.HasPrefix(ea.Message, "Le compilateur n'a pas pu répondre : ") || !strings.Contains(ea.Message, attendu) {
			t.Errorf("%q : %q", texte, ea.Message)
		}
		for _, nom := range []string{"EnregistrementAbsent", "ErreurMistral", "Error", "Traceback", "exec:"} {
			if strings.Contains(ea.Message, nom) {
				t.Errorf("%q : nom technique %q affiché : %q", texte, nom, ea.Message)
			}
		}
	}
	panne("panne-enregistrement", nil, "aucune réponse enregistrée pour ce texte (compilation rejouée)")
	panne("panne-cle", nil, "la clé de l'API Mistral est absente")
	panne("un texte", []string{"programme-introuvable-console3"}, "« programme-introuvable-console3 » est introuvable")
	panne("un texte", []string{"python3", "-c", "raise ValueError('catalogue vide')"}, "catalogue vide")
	panne("un texte", []string{"python3", "-c", "import compilateur_absent_console3"}, "le module du service est introuvable")

	// Une remarque sur le texte n'est pas une panne : elle reste affichée.
	c, err := b.Dep.Compiler("Un outil inconnu", "", "", nil, "")
	if err != nil || c.Etat != "refuse" {
		t.Errorf("refus motivé : %v %v", c, err)
	}
}
