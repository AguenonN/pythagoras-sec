package annuaire

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// Deux jetons, tous deux des JWT EdDSA signés par la clé d'une identité de
// l'annuaire, et vérifiés contre la clé que l'annuaire lui connaît :
//
//   - le jeton d'identité du demandeur, remis à la session au lancement :
//     « sub est la personne pour qui l'agent travaille », signé par sub
//     lui-même ou par une identité qui détient le rôle emetteur-identite ;
//   - le jeton d'approbation d'un valideur, remis à la reprise d'un appel
//     en attente : « iss, qui détient le rôle role, valide l'appel dont
//     l'empreinte transmise est empreinte_transmise ».
//
// Chaque jeton porte une audience propre : l'un ne vaut jamais pour
// l'autre.

const (
	AudienceDemandeur  = "mcp-gate/demandeur"
	AudienceValidation = "mcp-gate/validation"
	// RoleEmetteurIdentite permet de signer le jeton d'identité d'un autre
	// (un portail d'authentification, par exemple).
	RoleEmetteurIdentite = "emetteur-identite"
	// DureeMaxIdentite borne la vie d'un jeton d'identité : une journée de
	// travail, pas une habilitation permanente.
	DureeMaxIdentite = 12 * time.Hour
	// DureeMaxApprobation borne la vie d'un jeton d'approbation : un humain
	// qui valide et l'agent qui reprend, pas un blanc-seing.
	DureeMaxApprobation = 15 * time.Minute
	// Decalage tolère une horloge de signataire en avance.
	Decalage = time.Minute
)

// Erreurs de vérification, testables par errors.Is.
var (
	ErrJetonIllisible = errors.New("jeton illisible")
	ErrSignataire     = errors.New("signataire inconnu de l'annuaire ou signature invalide")
	ErrJetonPerime    = errors.New("jeton hors de sa période de validité")
	ErrAudience       = errors.New("jeton émis pour un autre usage")
	ErrHabilitation   = errors.New("le signataire n'est pas habilité")
	ErrAutreAppel     = errors.New("jeton émis pour un autre appel")
)

// ClaimsApprobation : iss et sub sont le valideur, qui signe pour lui-même.
type ClaimsApprobation struct {
	jwt.Claims
	Role               string `json:"role"`
	EmpreinteTransmise string `json:"empreinte_transmise"`
}

// Identification est ce qu'un jeton d'identité vérifié établit.
type Identification struct {
	Demandeur string
	Emetteur  string
	Emis      time.Time
}

// Approbation est ce qu'un jeton d'approbation vérifié établit. Date est
// la date de validation (iat) : c'est à elle que le rôle est vérifié.
type Approbation struct {
	Valideur       string
	Role           string
	Date           time.Time
	EmpreinteJeton string
}

// EmpreinteJeton hache un jeton : le journal ne le porte jamais en clair.
func EmpreinteJeton(jeton string) string {
	s := sha256.Sum256([]byte(jeton))
	return "sha256:" + hex.EncodeToString(s[:])
}

func signer(cle ed25519.PrivateKey, claims any) (string, error) {
	s, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.EdDSA, Key: cle}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		return "", err
	}
	return jwt.Signed(s).Claims(claims).Serialize()
}

// SignerIdentite signe un jeton d'identité du demandeur.
func SignerIdentite(cle ed25519.PrivateKey, emetteur, demandeur string, emis time.Time, duree time.Duration) (string, error) {
	return signer(cle, jwt.Claims{
		Issuer: emetteur, Subject: demandeur, Audience: jwt.Audience{AudienceDemandeur},
		IssuedAt: jwt.NewNumericDate(emis), Expiry: jwt.NewNumericDate(emis.Add(duree)),
	})
}

// SignerApprobation signe un jeton d'approbation : valideur, rôle au titre
// duquel il valide, empreinte transmise de l'appel.
func SignerApprobation(cle ed25519.PrivateKey, valideur, role, empreinteTransmise string, emis time.Time, duree time.Duration) (string, error) {
	return signer(cle, ClaimsApprobation{
		Claims: jwt.Claims{
			Issuer: valideur, Subject: valideur, Audience: jwt.Audience{AudienceValidation},
			IssuedAt: jwt.NewNumericDate(emis), Expiry: jwt.NewNumericDate(emis.Add(duree)),
		},
		Role: role, EmpreinteTransmise: empreinteTransmise,
	})
}

// verifierSignature lit le jeton, trouve son signataire (iss) dans
// l'annuaire à la date a, et vérifie la signature avec sa clé. Seul EdDSA
// est admis : l'algorithme ne se choisit pas dans le jeton.
func verifierSignature(src Source, jeton string, a time.Time, claims any) (Fiche, error) {
	tok, err := jwt.ParseSigned(jeton, []jose.SignatureAlgorithm{jose.EdDSA})
	if err != nil {
		return Fiche{}, ErrJetonIllisible
	}
	var brut jwt.Claims
	if err := tok.UnsafeClaimsWithoutVerification(&brut); err != nil || brut.Issuer == "" {
		return Fiche{}, ErrJetonIllisible
	}
	fiche, ok := src.Resoudre(brut.Issuer, a)
	if !ok {
		return Fiche{}, ErrSignataire
	}
	if err := tok.Claims(fiche.Cle, claims); err != nil {
		return Fiche{}, ErrSignataire
	}
	return fiche, nil
}

// periode contrôle audience, dates et durée à la date a.
func periode(c jwt.Claims, audience string, a time.Time, dureeMax time.Duration) error {
	if !c.Audience.Contains(audience) {
		return ErrAudience
	}
	if c.IssuedAt == nil || c.Expiry == nil {
		return fmt.Errorf("%w : iat et exp obligatoires", ErrJetonPerime)
	}
	emis, fin := c.IssuedAt.Time(), c.Expiry.Time()
	if fin.Sub(emis) > dureeMax {
		return fmt.Errorf("%w : durée de vie supérieure à %s", ErrJetonPerime, dureeMax)
	}
	if a.Before(emis.Add(-Decalage)) || !a.Before(fin) {
		return ErrJetonPerime
	}
	return nil
}

// VerifierIdentite vérifie un jeton d'identité du demandeur à la date a.
// Le demandeur doit être dans l'annuaire ; le signataire doit être le
// demandeur lui-même, ou détenir le rôle emetteur-identite à la date
// d'émission.
func VerifierIdentite(src Source, jeton string, a time.Time) (Identification, error) {
	var c jwt.Claims
	fiche, err := verifierSignature(src, jeton, a, &c)
	if err != nil {
		return Identification{}, err
	}
	if err := periode(c, AudienceDemandeur, a, DureeMaxIdentite); err != nil {
		return Identification{}, err
	}
	if c.Subject == "" {
		return Identification{}, fmt.Errorf("%w : aucun demandeur (sub vide)", ErrJetonIllisible)
	}
	if _, ok := src.Resoudre(c.Subject, a); !ok {
		return Identification{}, fmt.Errorf("%w : demandeur inconnu de l'annuaire", ErrHabilitation)
	}
	emis := c.IssuedAt.Time()
	if c.Subject != c.Issuer {
		if f, ok := src.Resoudre(fiche.Identite, emis); !ok || !f.DetientRole(RoleEmetteurIdentite) {
			return Identification{}, fmt.Errorf("%w : %s ne peut signer que sa propre identité", ErrHabilitation, c.Issuer)
		}
	}
	return Identification{Demandeur: c.Subject, Emetteur: c.Issuer, Emis: emis.UTC()}, nil
}

// VerifierApprobation vérifie un jeton d'approbation à la date a, pour
// l'appel d'empreinte transmise donnée : signature du valideur, période,
// appel visé, et rôle détenu à la date de validation (iat). Les règles qui
// portent sur plusieurs jetons (un par rôle, quatre yeux) sont celles de
// l'engine.
func VerifierApprobation(src Source, jeton, empreinteTransmise string, a time.Time) (Approbation, error) {
	var c ClaimsApprobation
	if _, err := verifierSignature(src, jeton, a, &c); err != nil {
		return Approbation{}, err
	}
	if err := periode(c.Claims, AudienceValidation, a, DureeMaxApprobation); err != nil {
		return Approbation{}, err
	}
	if c.Subject != c.Issuer {
		return Approbation{}, fmt.Errorf("%w : un valideur ne signe que pour lui-même", ErrHabilitation)
	}
	if c.EmpreinteTransmise != empreinteTransmise {
		return Approbation{}, ErrAutreAppel
	}
	date := c.IssuedAt.Time().UTC()
	f, ok := src.Resoudre(c.Issuer, date)
	if c.Role == "" || !ok || !f.DetientRole(c.Role) {
		return Approbation{}, fmt.Errorf("%w : %s ne détenait pas le rôle %q à la date de validation", ErrHabilitation, c.Issuer, c.Role)
	}
	return Approbation{Valideur: c.Issuer, Role: c.Role, Date: date, EmpreinteJeton: EmpreinteJeton(jeton)}, nil
}
