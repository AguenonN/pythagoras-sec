// Commande spiffe-proxy est le sidecar du lab : il donne une identité SPIFFE
// à un serveur MCP qui n'en connaît pas (banque-synthetique ne parle que
// TLS avec certificat). Il termine le mTLS SPIFFE côté réseau (SVID obtenue
// de la Workload API SPIRE, appelants limités aux SPIFFE ID listés) et
// relaie en HTTPS vers le serveur, dans le même pod, sur la boucle locale.
//
//	spiffe-proxy gencert -dir /tls
//	spiffe-proxy -ecoute :8443 -amont https://127.0.0.1:9443 \
//	    -appelants spiffe://mcp.gcp.lab/gate
//
// gencert écrit un certificat auto-signé jetable (tls.crt, tls.key) pour le
// serveur, qui n'est joignable que depuis la boucle locale du pod : le
// sidecar ne le vérifie pas, et refuse tout amont qui n'est pas en boucle
// locale. Lab seulement : en production le serveur porterait lui-même sa SVID.
package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"mcprogue/internal/identity"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "gencert" {
		fs := flag.NewFlagSet("gencert", flag.ExitOnError)
		dir := fs.String("dir", "", "répertoire de sortie")
		fs.Parse(os.Args[2:])
		if *dir == "" {
			log.Fatal("spiffe-proxy gencert: -dir obligatoire")
		}
		if err := genererCert(*dir); err != nil {
			log.Fatalf("spiffe-proxy gencert: %v", err)
		}
		return
	}
	ecoute := flag.String("ecoute", ":8443", "adresse d'écoute mTLS SPIFFE")
	amont := flag.String("amont", "", "url https du serveur, en boucle locale")
	appelants := flag.String("appelants", "", "SPIFFE ID admis, séparés par des virgules")
	socket := flag.String("spiffe-socket", "", "Workload API (défaut : SPIFFE_ENDPOINT_SOCKET)")
	flag.Parse()
	if err := servir(*ecoute, *amont, *appelants, *socket); err != nil {
		log.Fatalf("spiffe-proxy: %v", err)
	}
}

func servir(ecoute, amont, appelants, socket string) error {
	h, err := Proxy(amont)
	if err != nil {
		return err
	}
	autorisant, err := identity.OnlyIDs(strings.Split(appelants, ",")...)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	src, err := identity.NewSource(ctx, socket)
	if err != nil {
		return err
	}
	defer src.Close()
	l, err := net.Listen("tcp", ecoute)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	go func() { <-ctx.Done(); srv.Close() }()
	log.Printf("spiffe-proxy: %s -> %s, appelants %s", l.Addr(), amont, appelants)
	if err := srv.Serve(tls.NewListener(l, src.ServerTLS(autorisant))); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Proxy relaie vers l'amont, qui doit être une url https en boucle locale.
// Les flux SSE du HTTP streamable passent sans tampon.
func Proxy(amont string) (http.Handler, error) {
	u, err := url.Parse(amont)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Errorf("-amont %q : url https attendue", amont)
	}
	if ip := net.ParseIP(u.Hostname()); u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil, fmt.Errorf("-amont %q : le sidecar ne relaie que vers la boucle locale", amont)
	}
	p := httputil.NewSingleHostReverseProxy(u)
	p.FlushInterval = -1
	p.Transport = &http.Transport{
		// Boucle locale du pod, certificat jetable : voir l'en-tête.
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}
	return p, nil
}

func genererCert(dir string) error {
	cle, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	modele := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "serveur du lab (boucle locale)"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(48 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, modele, modele, &cle.PublicKey, cle)
	if err != nil {
		return err
	}
	kb, err := x509.MarshalPKCS8PrivateKey(cle)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "tls.crt"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return err
	}
	// Lisible par le conteneur du serveur (autre utilisateur possible) :
	// la clé ne vit que dans l'emptyDir du pod.
	return os.WriteFile(filepath.Join(dir, "tls.key"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kb}), 0o644)
}
