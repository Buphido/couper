package accesscontrol

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go/v4"
	"github.com/hashicorp/hcl/v2"

	acjwt "github.com/avenga/couper/accesscontrol/jwt"
	"github.com/avenga/couper/config/request"
	"github.com/avenga/couper/errors"
	"github.com/avenga/couper/eval/content"
	"github.com/avenga/couper/internal/seetie"
)

const (
	Invalid JWTSourceType = iota
	Cookie
	Header
)

var _ AccessControl = &JWT{}

type (
	JWTSourceType uint8
	JWTSource     struct {
		Name string
		Type JWTSourceType
	}
)

type JWT struct {
	algorithm      acjwt.Algorithm
	claims         hcl.Expression
	claimsRequired []string
	source         JWTSource
	hmacSecret     []byte
	name           string
	pubKey         *rsa.PublicKey
}

type JWTOptions struct {
	Algorithm      string
	Claims         hcl.Expression
	ClaimsRequired []string
	Name           string // TODO: more generic (validate)
	Source         JWTSource
	Key            []byte
}

func NewJWTSource(cookie, header string) JWTSource {
	c, h := strings.TrimSpace(cookie), strings.TrimSpace(header)
	if c != "" && h != "" { // both are invalid
		return JWTSource{}
	}

	if c != "" {
		return JWTSource{
			Name: c,
			Type: Cookie,
		}
	} else if h != "" {
		return JWTSource{
			Name: h,
			Type: Header,
		}
	}
	return JWTSource{}
}

// NewJWT parses the key and creates Validation obj which can be referenced in related handlers.
func NewJWT(options *JWTOptions) (*JWT, error) {
	jwtAC := &JWT{
		algorithm:      acjwt.NewAlgorithm(options.Algorithm),
		claims:         options.Claims,
		claimsRequired: options.ClaimsRequired,
		name:           options.Name,
		source:         options.Source,
	}

	if jwtAC.source.Type == Invalid {
		return nil, fmt.Errorf("token source is invalid")
	}

	if jwtAC.algorithm == acjwt.AlgorithmUnknown {
		return nil, fmt.Errorf("algorithm is not supported")
	}

	if jwtAC.algorithm.IsHMAC() {
		jwtAC.hmacSecret = options.Key
		return jwtAC, nil
	}

	pubKey, err := parsePublicPEMKey(options.Key)
	if err != nil {
		return nil, err
	}

	jwtAC.pubKey = pubKey
	return jwtAC, nil
}

// Validate reading the token from configured source and validates against the key.
func (j *JWT) Validate(req *http.Request) error {
	ctx := req.Context()
	cctx := ctx.Value(request.ContextType).(content.Context)
	evalCtx := cctx.HCLContext()
	claims, diags := seetie.ExpToMap(evalCtx, j.claims)
	if diags != nil {
		return diags
	}

	var tokenValue string
	var err error

	switch j.source.Type {
	case Cookie:
		cookie, cerr := req.Cookie(j.source.Name)
		if cerr != http.ErrNoCookie && cookie != nil {
			tokenValue = cookie.Value
		}
	case Header:
		if strings.ToLower(j.source.Name) == "authorization" {
			if tokenValue = req.Header.Get(j.source.Name); tokenValue != "" {
				if tokenValue, err = getBearer(tokenValue); err != nil {
					return err
				}
			}
		} else {
			tokenValue = req.Header.Get(j.source.Name)
		}
	}

	// TODO j.PostParam, j.QueryParam
	if tokenValue == "" {
		return errors.JwtTokenMissing.Message("token required")
	}

	parser, err := newParser(j.algorithm, claims)
	if err != nil {
		return err
	}

	token, err := parser.Parse(tokenValue, j.getValidationKey)
	if err != nil {
		switch err.(type) {
		case *jwt.TokenExpiredError:
			return errors.JwtTokenExpired.With(err)
		default:
			return err
		}
	}

	tokenClaims, err := j.validateClaims(token, claims)
	if err != nil {
		return err
	}

	acMap, ok := ctx.Value(request.AccessControls).(map[string]interface{})
	if !ok {
		acMap = make(map[string]interface{})
	}
	acMap[j.name] = tokenClaims

	ctx = context.WithValue(ctx, request.AccessControls, acMap)
	*req = *req.WithContext(ctx)

	return nil
}

func (j *JWT) getValidationKey(_ *jwt.Token) (interface{}, error) {
	switch j.algorithm {
	case acjwt.AlgorithmRSA256, acjwt.AlgorithmRSA384, acjwt.AlgorithmRSA512:
		return j.pubKey, nil
	case acjwt.AlgorithmHMAC256, acjwt.AlgorithmHMAC384, acjwt.AlgorithmHMAC512:
		return j.hmacSecret, nil
	default: // this error case gets normally caught on configuration level
		return nil, errors.Configuration.Message("algorithm is not supported")
	}
}

func (j *JWT) validateClaims(token *jwt.Token, claims map[string]interface{}) (map[string]interface{}, error) {
	var tokenClaims jwt.MapClaims
	if tc, ok := token.Claims.(jwt.MapClaims); ok {
		tokenClaims = tc
	}

	if tokenClaims == nil {
		return nil, errors.JwtTokenInvalid.Message("token has no claims")
	}

	for _, key := range j.claimsRequired {
		if _, ok := tokenClaims[key]; !ok {
			return nil, errors.JwtTokenInvalid.Message("required claim is missing: " + key)
		}
	}

	for k, v := range claims {

		if k == "iss" || k == "aud" { // gets validated during parsing
			continue
		}

		val, exist := tokenClaims[k]
		if !exist {
			return nil, errors.JwtTokenInvalid.Message("required claim is missing: " + k)
		}

		if val != v {
			return nil, errors.JwtTokenInvalid.Messagef("unexpected value for claim %s: %q, expected %q", k, val, v)
		}
	}
	return tokenClaims, nil
}

func getBearer(val string) (string, error) {
	const bearer = "bearer "
	if strings.HasPrefix(strings.ToLower(val), bearer) {
		return strings.Trim(val[len(bearer):], " "), nil
	}
	return "", errors.JwtTokenExpired.Message("bearer required with authorization header")
}

func newParser(algo acjwt.Algorithm, claims map[string]interface{}) (*jwt.Parser, error) {
	options := []jwt.ParserOption{
		jwt.WithValidMethods([]string{algo.String()}),
		jwt.WithLeeway(time.Second),
	}

	if claims == nil {
		options = append(options, jwt.WithoutAudienceValidation())
		return jwt.NewParser(options...), nil
	}

	if iss, ok := claims["iss"]; ok {
		if err := isStringType(iss); err != nil {
			return nil, fmt.Errorf("iss: %w", err)
		}
		options = append(options, jwt.WithIssuer(iss.(string)))
	}

	if aud, ok := claims["aud"]; ok {
		if err := isStringType(aud); err != nil {
			return nil, fmt.Errorf("aud: %w", err)
		}
		options = append(options, jwt.WithAudience(aud.(string)))
	} else {
		options = append(options, jwt.WithoutAudienceValidation())
	}

	return jwt.NewParser(options...), nil
}

// parsePublicPEMKey tries to parse all supported publicKey variations which
// must be given in PEM encoded format.
func parsePublicPEMKey(key []byte) (pub *rsa.PublicKey, err error) {
	pemBlock, _ := pem.Decode(key)
	if pemBlock == nil {
		return nil, jwt.ErrKeyMustBePEMEncoded
	}
	pubKey, pubErr := x509.ParsePKCS1PublicKey(pemBlock.Bytes)
	if pubErr != nil {
		pkixKey, pkerr := x509.ParsePKIXPublicKey(pemBlock.Bytes)
		if pkerr != nil {
			cert, cerr := x509.ParseCertificate(pemBlock.Bytes)
			if cerr != nil {
				return nil, jwt.ErrNotRSAPublicKey
			}
			if k, ok := cert.PublicKey.(*rsa.PublicKey); ok {
				return k, nil
			}
			return nil, jwt.ErrNotRSAPublicKey
		}
		k, ok := pkixKey.(*rsa.PublicKey)
		if !ok {
			return nil, jwt.ErrNotRSAPublicKey
		}
		pubKey = k
	}
	return pubKey, nil
}

func isStringType(val interface{}) error {
	switch val.(type) {
	case string:
		return nil
	default:
		return fmt.Errorf("invalid value type")
	}
}
