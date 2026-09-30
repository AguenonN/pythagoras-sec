package main

import (
	"strings"
	"testing"

	"mcprogue/servers/internal/essai"
	"mcprogue/servers/internal/rejeu"
)

// Les tests tournent en REJEU sur les enregistrements versionnés : aucun
// réseau.

func TestActesEnregistres(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau("enregistrements", rejeu.Rejeu)))
	cas := map[string]string{
		"32022R2554": "2022/2554",
		"32016R0679": "2016/679",
		"32024R1689": "2024/1689",
		"32026R1744": "2026/1744",
	}
	for celex, num := range cas {
		var out SortieActe
		res := essai.Appeler(t, cs, "rechercher_acte", map[string]any{"celex": celex}, &out)
		if res.IsError || out.Acte == nil || out.Acte.Statut != "en_vigueur" || !strings.Contains(out.Acte.Titre, num) {
			t.Errorf("%s: %+v", celex, out)
			continue
		}
		if out.Source.Origine != rejeu.OrigineRejeu {
			t.Errorf("%s: origine %q", celex, out.Source.Origine)
		}
	}
	var ia SortieActe
	essai.Appeler(t, cs, "rechercher_acte", map[string]any{"celex": "32024R1689"}, &ia)
	if ia.Acte == nil || len(ia.Acte.ModifiePar) == 0 || ia.Acte.ModifiePar[0] != "32026R1744" {
		t.Errorf("l'AI Act doit être signalé modifié par l'omnibus 2026/1744: %+v", ia.Acte)
	}
}

func TestArticles(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau("enregistrements", rejeu.Rejeu)))
	cas := []struct{ celex, article, titre, extrait string }{
		{"32022R2554", "28", "Principes généraux", "prestataires tiers de services TIC"},
		{"32016R0679", "6", "Licéité du traitement", "le traitement est nécessaire"},
		{"32024R1689", "5", "Pratiques interdites en matière d’IA", "sont interdites"},
		{"32026R1744", "1", "Modifications du règlement (UE) 2024/1689", "1) À l'article 1er, paragraphe 2"},
		// Tout article d'un acte enregistré est lisible en rejeu.
		{"32016R0679", "17", "Droit à l'effacement («droit à l'oubli»)", "effacement"},
	}
	for _, c := range cas {
		var out SortieArticle
		res := essai.Appeler(t, cs, "lire_article", map[string]any{"celex": c.celex, "article": c.article}, &out)
		if res.IsError || out.Statut != rejeu.StatutOK {
			t.Errorf("%s art. %s: %+v", c.celex, c.article, out)
			continue
		}
		if out.Titre != c.titre || !strings.Contains(out.Texte, c.extrait) || out.Version == "" {
			t.Errorf("%s art. %s: titre %q, texte %.200q", c.celex, c.article, out.Titre, out.Texte)
		}
		if out.Source.Origine != rejeu.OrigineRejeu || out.Source.Filtre == "" {
			t.Errorf("%s: source %+v", c.celex, out.Source)
		}
	}
}

func TestErreurs(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau("enregistrements", rejeu.Rejeu)))
	var a SortieArticle
	essai.Appeler(t, cs, "lire_article", map[string]any{"celex": "32016R0679", "article": "999"}, &a)
	if a.Statut != rejeu.StatutIntrouvable {
		t.Errorf("article absent: %+v", a)
	}
	for _, args := range []map[string]any{
		{"celex": "32016R0679\" . ?s ?p ?o", "article": "1"},
		{"celex": "32016R0679", "article": "1; drop"},
	} {
		var o SortieArticle
		if res := essai.Appeler(t, cs, "lire_article", args, &o); !res.IsError || o.Statut != rejeu.StatutRefuse {
			t.Errorf("%v accepté: %+v", args, o)
		}
	}
	var o SortieActe
	if res := essai.Appeler(t, cs, "rechercher_acte", map[string]any{"celex": "32019R2088"}, &o); !res.IsError || o.Statut != rejeu.StatutIndisponible {
		t.Errorf("acte non enregistré: %+v", o)
	}
}

func TestExtraireArticleImbrique(t *testing.T) {
	x := `<html><body><div id="art_2"><p class="oj-ti-art">Article 2</p><p class="oj-sti-art">Objet</p>
	<table><tr><td><p>1)</p></td><td><p>L'article 3 est modifié:</p>
	  <table><tr><td><p>a)</p></td><td><p>le point&nbsp;x</p></td></tr></table></td></tr></table></div>
	<div id="art_3"><p>Article 3</p></div></body></html>`
	titre, texte, err := extraireArticle([]byte(x), "2")
	if err != nil {
		t.Fatal(err)
	}
	attendu := "Article 2\nObjet\n1) L'article 3 est modifié:\na) le point x"
	if titre != "Objet" || texte != attendu {
		t.Fatalf("titre %q texte %q", titre, texte)
	}
}

func TestRetirerSignatures(t *testing.T) {
	x := []byte(`<div id="x"><p>art</p></div><div class="oj-final"><div><p>La présidente</p><p>R. NOM</p></div></div><p>note</p>`)
	f, err := retirerSignatures(x)
	if err != nil {
		t.Fatal(err)
	}
	if string(f) != `<div id="x"><p>art</p></div><p>note</p>` {
		t.Fatalf("%s", f)
	}
	if _, err := retirerSignatures([]byte(`<p>sans bloc</p>`)); err == nil {
		t.Fatal("texte sans bloc de signatures accepté")
	}
}

func TestSchema(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau("enregistrements", rejeu.Rejeu)))
	essai.VerifierSchema(t, cs, "schema.json")
}
