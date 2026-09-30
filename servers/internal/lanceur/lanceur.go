// Package lanceur factorise le main des serveurs de données : nom logique,
// mode de rejeu, répertoire d'enregistrements, transport.
//
// Deux transports : stdio (par défaut) et HTTP streamable sur TLS (-http,
// avec -tls-cert et -tls-key obligatoires : jamais de HTTP en clair).
package lanceur

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/internal/nommage"
	"mcprogue/servers/internal/rejeu"
)

// Config est ce que le main transmet au constructeur du serveur.
type Config struct {
	Mode            rejeu.Mode
	Enregistrements string
}

// Client crée un client de rejeu sur le répertoire du serveur.
func (c Config) Client() *rejeu.Client { return rejeu.Nouveau(c.Enregistrements, c.Mode) }

// Lancer lit les options, construit le serveur et le sert. Il ne rend la
// main qu'à la fin ; il quitte le processus en cas d'erreur.
// Enregistrements par défaut : servers/<nom>/enregistrements.
func Lancer(nom string, construire func(Config) (*mcp.Server, error)) {
	LancerDepuis(nom, "servers/"+nom+"/enregistrements", construire)
}

// LancerDepuis est Lancer avec un autre répertoire d'enregistrements par
// défaut.
func LancerDepuis(nom, dirDefaut string, construire func(Config) (*mcp.Server, error)) {
	if err := nommage.ValiderNomLogique(nom); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	dir := flag.String("enregistrements", dirDefaut, "répertoire des enregistrements")
	adresse := flag.String("http", "", "adresse d'écoute HTTP streamable sur TLS (ex. 127.0.0.1:8443) ; stdio si vide")
	cert := flag.String("tls-cert", "", "certificat TLS (PEM), obligatoire avec -http")
	cle := flag.String("tls-key", "", "clé privée TLS (PEM), obligatoire avec -http")
	flag.Parse()

	mode, err := rejeu.ModeDepuisEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	var conf *tls.Config
	if *adresse != "" {
		if *cert == "" || *cle == "" {
			fmt.Fprintf(os.Stderr, "%s: -http exige -tls-cert et -tls-key (pas de HTTP en clair)\n", nom)
			os.Exit(2)
		}
		paire, err := tls.LoadX509KeyPair(*cert, *cle)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: TLS: %v\n", nom, err)
			os.Exit(2)
		}
		conf = ConfigTLS(paire)
	}
	srv, err := construire(Config{Mode: mode, Enregistrements: *dir})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", nom, err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if conf == nil {
		fmt.Fprintf(os.Stderr, "%s: mode %s, enregistrements %s, stdio\n", nom, mode, *dir)
		err = srv.Run(ctx, &mcp.StdioTransport{})
	} else {
		var ln net.Listener
		ln, err = net.Listen("tcp", *adresse)
		if err == nil {
			fmt.Fprintf(os.Stderr, "%s: mode %s, enregistrements %s, https://%s/mcp\n", nom, mode, *dir, ln.Addr())
			err = ServirTLS(ctx, ln, srv, conf)
		}
	}
	if err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", nom, err)
		os.Exit(1)
	}
}

// ConfigTLS : TLS 1.2 au moins, certificat du serveur.
func ConfigTLS(paire tls.Certificate) *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{paire}}
}

// ServirTLS sert srv en HTTP streamable sur TLS, chemin /mcp, jusqu'à
// l'annulation de ctx.
func ServirTLS(ctx context.Context, ln net.Listener, srv *mcp.Server, conf *tls.Config) error {
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil))
	h := &http.Server{
		Handler:           mux,
		TLSConfig:         conf,
		ReadHeaderTimeout: 10 * time.Second,
	}
	arret := make(chan struct{})
	go func() {
		defer close(arret)
		<-ctx.Done()
		c, annuler := context.WithTimeout(context.Background(), 5*time.Second)
		defer annuler()
		h.Shutdown(c)
	}()
	err := h.Serve(tls.NewListener(ln, conf))
	if errors.Is(err, http.ErrServerClosed) {
		<-arret
		return nil
	}
	return err
}
