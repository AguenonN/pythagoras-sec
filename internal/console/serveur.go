package console

import (
	"encoding/json"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strconv"
)

// Serveur est le serveur HTTP de la console. Il ne sert que des lectures.
type Serveur struct {
	Depot *Depot
	// Produit est le nom affiché dans l'en-tête (la maquette le laisse en
	// emplacement) ; vide, « Console de gouvernance ».
	Produit string
}

// Handler assemble les routes. Toutes les requêtes doivent viser 127.0.0.1 ou
// localhost : un nom de domaine qui résoudrait vers la machine (rebinding)
// est refusé.
func (s *Serveur) Handler() http.Handler {
	mux := http.NewServeMux()
	sub, _ := fs.Sub(webFS, "web")
	mux.Handle("GET /", http.FileServerFS(sub))
	mux.HandleFunc("GET /api/etat", s.etat)
	mux.HandleFunc("GET /api/journal/{id}/session/{sid}/chaine", s.chaine)
	mux.HandleFunc("GET /api/journal/{id}/session/{sid}/poste", s.poste)
	mux.HandleFunc("GET /api/regles", s.regles)
	mux.HandleFunc("GET /api/avant-apres", s.avantApres)
	mux.HandleFunc("GET /api/file", s.file)
	mux.HandleFunc("POST /api/redaction/relire", s.redaction)
	return gardes(mux)
}

func gardes(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		if host != "127.0.0.1" && host != "localhost" && host != "[::1]" && host != "::1" {
			http.Error(w, "hôte refusé", http.StatusForbidden)
			return
		}
		hd := w.Header()
		hd.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; form-action 'none'")
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}

func ecrire(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v)
}

func erreur(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"erreur": msg})
}

type jeuListe struct {
	Index      int    `json:"index"`
	Libelle    string `json:"libelle"`
	Empreinte  string `json:"empreinte"`
	Source     string `json:"source"`
	Directives int    `json:"directives"`
}

type journalListe struct {
	ID       string          `json:"id"`
	Verif    Verification    `json:"verification"`
	Entrees  int             `json:"entrees"`
	Ecart    int             `json:"premier_ecart"`
	Sessions []SessionResume `json:"sessions"`
}

func (s *Serveur) etat(w http.ResponseWriter, r *http.Request) {
	out := struct {
		Journaux []journalListe `json:"journaux"`
		Jeux     []jeuListe     `json:"jeux"`
		Refus    []string       `json:"jeux_refuses"`
		Cle      bool           `json:"cle_presente"`
		Regles   *Regles        `json:"regles,omitempty"`
		Produit  string         `json:"produit,omitempty"`
	}{Journaux: []journalListe{}, Jeux: []jeuListe{}, Cle: len(s.Depot.Cle) > 0}
	for _, j := range s.Depot.Journaux() {
		st, err := s.Depot.Lire(j)
		if err != nil {
			continue
		}
		out.Journaux = append(out.Journaux, journalListe{ID: j.ID, Verif: st.Verif, Entrees: len(st.Entrees), Ecart: st.Verif.PremierEcart(), Sessions: Sessions(st)})
	}
	jeux, refus := s.Depot.ChargerJeux()
	for i, j := range jeux {
		out.Jeux = append(out.Jeux, jeuListe{Index: i, Libelle: j.Libelle(), Empreinte: j.Jeu.Meta.Empreinte, Source: j.Source, Directives: len(j.Jeu.Directives)})
	}
	out.Produit = s.Produit
	out.Refus = append([]string{}, refus...)
	if jc := EnVigueur(s.tousEtats(), jeux); jc != nil {
		out.Regles = reglesDe(&jc.Jeu)
	}
	ecrire(w, out)
}

func (s *Serveur) ouvrir(w http.ResponseWriter, id string) (*Etat, bool) {
	j, ok := s.Depot.Journal(id)
	if !ok {
		erreur(w, http.StatusNotFound, "journal inconnu")
		return nil, false
	}
	st, err := s.Depot.Lire(j)
	if err != nil {
		erreur(w, http.StatusInternalServerError, "journal illisible : "+err.Error())
		return nil, false
	}
	return st, true
}

func (s *Serveur) chaine(w http.ResponseWriter, r *http.Request) {
	st, ok := s.ouvrir(w, r.PathValue("id"))
	if !ok {
		return
	}
	jeux, _ := s.Depot.ChargerJeux()
	v, ok := Chaine(st, jeux, r.PathValue("sid"))
	if !ok {
		erreur(w, http.StatusNotFound, "session inconnue")
		return
	}
	ecrire(w, v)
}

func (s *Serveur) poste(w http.ResponseWriter, r *http.Request) {
	st, ok := s.ouvrir(w, r.PathValue("id"))
	if !ok {
		return
	}
	jeux, _ := s.Depot.ChargerJeux()
	v, ok := Poste(st, jeux, r.PathValue("sid"))
	if !ok {
		erreur(w, http.StatusNotFound, "session inconnue")
		return
	}
	ecrire(w, v)
}

func (s *Serveur) tousEtats() []*Etat {
	var out []*Etat
	for _, j := range s.Depot.Journaux() {
		if st, err := s.Depot.Lire(j); err == nil {
			out = append(out, st)
		}
	}
	return out
}

func indexJeu(r *http.Request, cle string, n int) (int, bool) {
	i, err := strconv.Atoi(r.URL.Query().Get(cle))
	return i, err == nil && i >= 0 && i < n
}

func (s *Serveur) regles(w http.ResponseWriter, r *http.Request) {
	jeux, _ := s.Depot.ChargerJeux()
	etats := s.tousEtats()
	jc := EnVigueur(etats, jeux)
	if jc == nil {
		erreur(w, http.StatusNotFound, "aucune directive disponible")
		return
	}
	ecrire(w, ReglesEnVigueur(*jc, etats, jeux, r.URL.Query().Get("directive")))
}

func (s *Serveur) avantApres(w http.ResponseWriter, r *http.Request) {
	jeux, _ := s.Depot.ChargerJeux()
	st, ok := s.ouvrir(w, r.URL.Query().Get("journal"))
	if !ok {
		return
	}
	v, err := AvantApres(st, jeux, r.URL.Query().Get("session"), r.URL.Query().Get("directive"))
	if err != nil {
		erreur(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	ecrire(w, v)
}

func (s *Serveur) file(w http.ResponseWriter, r *http.Request) {
	out := struct {
		Attentes []Attente               `json:"attentes"`
		Verif    map[string]Verification `json:"verifications"`
	}{Attentes: []Attente{}, Verif: map[string]Verification{}}
	for _, st := range s.tousEtats() {
		out.Verif[st.Journal.ID] = st.Verif
		out.Attentes = append(out.Attentes, File(st.Journal.ID, st)...)
	}
	ecrire(w, out)
}

func (s *Serveur) redaction(w http.ResponseWriter, r *http.Request) {
	corps, err := io.ReadAll(io.LimitReader(r.Body, 1<<20+1))
	if err != nil || len(corps) > 1<<20 {
		erreur(w, http.StatusRequestEntityTooLarge, "texte trop long")
		return
	}
	var in struct {
		Texte string `json:"texte"`
	}
	if err := json.Unmarshal(corps, &in); err != nil {
		erreur(w, http.StatusBadRequest, "requête illisible")
		return
	}
	ecrire(w, Relire(in.Texte))
}
