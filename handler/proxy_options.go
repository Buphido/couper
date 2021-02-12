package handler

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/docker/go-units"
	"github.com/hashicorp/hcl/v2"

	"github.com/avenga/couper/config"
	"github.com/avenga/couper/errors"
)

type ProxyOptions struct {
	BasicAuth                            string
	ConnectTimeout, Timeout, TTFBTimeout time.Duration
	Context                              hcl.Body
	BackendName                          string
	CORS                                 *CORSOptions
	NoProxyFromEnv                       bool
	DisableCertValidation                bool
	DisableConnectionReuse               bool
	HTTP2                                bool
	MaxConnections                       int
	OpenAPI                              *OpenAPIValidatorOptions
	ErrorTemplate                        *errors.Template
	Kind                                 string
	Proxy                                string
	RequestBodyLimit                     int64
}

func NewProxyOptions(
	conf *config.Backend, corsOpts *CORSOptions, noProxyFromEnv bool,
	errTpl *errors.Template, kind string,
) (*ProxyOptions, error) {
	var timeout, connTimeout, ttfbTimeout time.Duration
	if err := parseDuration(conf.Timeout, &timeout); err != nil {
		return nil, err
	}
	if err := parseDuration(conf.TTFBTimeout, &ttfbTimeout); err != nil {
		return nil, err
	}
	if err := parseDuration(conf.ConnectTimeout, &connTimeout); err != nil {
		return nil, err
	}

	bodyLimit, err := units.FromHumanSize(conf.RequestBodyLimit)
	if err != nil {
		return nil, fmt.Errorf("backend bodyLimit: %v", err)
	}

	cors := corsOpts
	if cors == nil { // Could be nil on non api context like 'free' endpoints or definitions.
		cors = &CORSOptions{}
	}

	openAPIValidatorOptions, err := NewOpenAPIValidatorOptions(conf.OpenAPI)
	if err != nil {
		return nil, err
	}

	return &ProxyOptions{
		BasicAuth:              conf.BasicAuth,
		BackendName:            conf.Name,
		CORS:                   cors,
		Context:                conf.Remain,
		ConnectTimeout:         connTimeout,
		DisableCertValidation:  conf.DisableCertValidation,
		DisableConnectionReuse: conf.DisableConnectionReuse,
		HTTP2:                  conf.HTTP2,
		MaxConnections:         conf.MaxConnections,
		NoProxyFromEnv:         noProxyFromEnv,
		OpenAPI:                openAPIValidatorOptions,
		ErrorTemplate:          errTpl,
		Kind:                   kind,
		Proxy:                  conf.Proxy,
		RequestBodyLimit:       bodyLimit,
		TTFBTimeout:            ttfbTimeout,
		Timeout:                timeout,
	}, nil
}

func (po *ProxyOptions) Hash() string {
	h := sha256.New()
	// exclude hcl list
	opts := *po
	opts.Context = nil
	h.Write([]byte(fmt.Sprintf("%v", opts)))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// parseDuration sets the target value if the given duration string is not empty.
func parseDuration(src string, target *time.Duration) error {
	d, err := time.ParseDuration(src)
	if src != "" && err != nil {
		return err
	}
	*target = d
	return nil
}
