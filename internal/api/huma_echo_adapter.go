package api

import (
	"context"
	"crypto/md5"
	"crypto/tls"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/labstack/echo/v5"
)

func NewEchoV5Adapter(e *echo.Echo, config huma.Config) huma.API {
	config.Components.Schemas = huma.NewMapRegistry(
		"#/components/schemas/", func(t reflect.Type, hint string) string {
			pkg := t.PkgPath()
			name := t.Name()
			if name == "" {
				h := md5.Sum([]byte(t.String()))
				return hint + fmt.Sprintf("_%x", h[:4])
			}
			if pkg != "" {
				parts := strings.Split(pkg, "/")
				if len(parts) >= 2 {
					return parts[len(parts)-2] + "_" + parts[len(parts)-1] + "_" + name
				}
				return parts[len(parts)-1] + "_" + name
			}
			return name
		},
	)
	a := &echoV5Adapter{e: e}
	humaAPI := huma.NewAPI(config, a)
	a.humaAPI = humaAPI
	return humaAPI
}

type echoV5Adapter struct {
	e       *echo.Echo
	humaAPI huma.API
}

func (a *echoV5Adapter) Handle(op *huma.Operation, handler func(huma.Context)) {
	path := humaPathToEcho(op.Path)
	a.e.Add(
		op.Method, path, func(c *echo.Context) error {
			handler(&echoV5Context{op: op, c: c, humaAPI: a.humaAPI})
			return nil
		},
	)
}

func (a *echoV5Adapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.e.ServeHTTP(w, r)
}

func humaPathToEcho(path string) string {
	var b strings.Builder
	for i := 0; i < len(path); i++ {
		if path[i] == '{' {
			b.WriteByte(':')
			i++
			for i < len(path) && path[i] != '}' {
				b.WriteByte(path[i])
				i++
			}
		} else {
			b.WriteByte(path[i])
		}
	}
	return b.String()
}

type echoV5Context struct {
	op      *huma.Operation
	c       *echo.Context
	humaAPI huma.API
	status  int
}

func (c *echoV5Context) Operation() *huma.Operation { return c.op }
func (c *echoV5Context) API() huma.API              { return c.humaAPI }
func (c *echoV5Context) Context() context.Context   { return c.c.Request().Context() }
func (c *echoV5Context) Method() string             { return c.c.Request().Method }
func (c *echoV5Context) Host() string               { return c.c.Request().Host }
func (c *echoV5Context) URL() url.URL               { return *c.c.Request().URL }
func (c *echoV5Context) Param(name string) string   { return c.c.Param(name) }
func (c *echoV5Context) Query(name string) string   { return c.c.QueryParam(name) }
func (c *echoV5Context) Header(name string) string  { return c.c.Request().Header.Get(name) }
func (c *echoV5Context) RemoteAddr() string         { return c.c.Request().RemoteAddr }
func (c *echoV5Context) TLS() *tls.ConnectionState  { return c.c.Request().TLS }

func (c *echoV5Context) EachHeader(cb func(name, value string)) {
	for name, values := range c.c.Request().Header {
		for _, v := range values {
			cb(name, v)
		}
	}
}

func (c *echoV5Context) BodyReader() io.Reader { return c.c.Request().Body }

func (c *echoV5Context) GetMultipartForm() (*multipart.Form, error) {
	if err := c.c.Request().ParseMultipartForm(8 * 1024 * 1024); err != nil {
		return nil, err
	}
	return c.c.Request().MultipartForm, nil
}

func (c *echoV5Context) SetReadDeadline(_ time.Time) error { return nil }

func (c *echoV5Context) SetStatus(code int) {
	c.status = code
	c.c.Response().WriteHeader(code)
}

func (c *echoV5Context) Status() int { return c.status }

func (c *echoV5Context) AppendHeader(name, value string) {
	c.c.Response().Header().Add(name, value)
}

func (c *echoV5Context) SetHeader(name, value string) {
	c.c.Response().Header().Set(name, value)
}

func (c *echoV5Context) BodyWriter() io.Writer { return c.c.Response() }

func (c *echoV5Context) Version() huma.ProtoVersion {
	r := c.c.Request()
	return huma.ProtoVersion{Proto: strconv.Itoa(r.ProtoMajor), ProtoMajor: r.ProtoMinor}
}
