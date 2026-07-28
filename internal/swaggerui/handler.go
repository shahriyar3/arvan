package swaggerui

import (
	htmlTemplate "html/template"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	textTemplate "text/template"

	"github.com/gin-gonic/gin"
	"github.com/swaggo/swag"
	"golang.org/x/net/webdav"

	swaggerFiles "github.com/swaggo/files"

	"github.com/shahriyar/arvan/internal/seed"
)

const securitySchemeName = "AccountToken"

// Config stores Swagger UI configuration.
type Config struct {
	URL                      string
	DocExpansion             string
	InstanceName             string
	Title                    string
	DefaultModelsExpandDepth int
	DeepLinking              bool
	PersistAuthorization     bool
	DefaultAccountToken      string
}

type swaggerConfig struct {
	URL                      string
	DocExpansion             string
	Title                    string
	Oauth2RedirectURL        htmlTemplate.JS
	DefaultModelsExpandDepth int
	DeepLinking              bool
	PersistAuthorization     bool
	DefaultAccountToken      string
}

func (config Config) toSwaggerConfig() swaggerConfig {
	return swaggerConfig{
		URL:          config.URL,
		DocExpansion: config.DocExpansion,
		Title:        config.Title,
		Oauth2RedirectURL: "`${window.location.protocol}//${window.location.host}$" +
			"{window.location.pathname.split('/').slice(0, window.location.pathname.split('/').length - 1).join('/')}" +
			"/oauth2-redirect.html`",
		DefaultModelsExpandDepth: config.DefaultModelsExpandDepth,
		DeepLinking:              config.DeepLinking,
		PersistAuthorization:     config.PersistAuthorization,
		DefaultAccountToken:      config.DefaultAccountToken,
	}
}

// Register mounts Swagger UI with a pre-authorized demo account token so protected
// endpoints work from the UI with a single Execute click (after `make seed`).
func Register(router gin.IRoutes) {
	cfg := &Config{
		URL:                      "doc.json",
		DocExpansion:             "list",
		InstanceName:             swag.Name,
		Title:                    "SMS Gateway API",
		DefaultModelsExpandDepth: 1,
		DeepLinking:              true,
		PersistAuthorization:     true,
		DefaultAccountToken:      seed.AccountAToken,
	}
	router.GET("/swagger/*any", wrapHandler(cfg, swaggerFiles.Handler))
}

func wrapHandler(config *Config, handler *webdav.Handler) gin.HandlerFunc {
	var once sync.Once

	if config.InstanceName == "" {
		config.InstanceName = swag.Name
	}
	if config.Title == "" {
		config.Title = "Swagger UI"
	}

	index, _ := htmlTemplate.New("swagger_index.html").Parse(swaggerIndexTpl)
	js, _ := textTemplate.New("swagger_index.js").Parse(initializerJSTpl)
	css, _ := textTemplate.New("swagger_index.css").Parse(swaggerStyleTpl)

	matcher := regexp.MustCompile(`(.*)(index\.html|index\.css|swagger-initializer\.js|doc\.json|favicon-16x16\.png|favicon-32x32\.png|/oauth2-redirect\.html|swagger-ui\.css|swagger-ui\.css\.map|swagger-ui\.js|swagger-ui\.js\.map|swagger-ui-bundle\.js|swagger-ui-bundle\.js\.map|swagger-ui-standalone-preset\.js|swagger-ui-standalone-preset\.js\.map)[?|.]*`)

	return func(ctx *gin.Context) {
		if ctx.Request.Method != http.MethodGet {
			ctx.AbortWithStatus(http.StatusMethodNotAllowed)
			return
		}

		matches := matcher.FindStringSubmatch(ctx.Request.RequestURI)
		if len(matches) != 3 {
			ctx.String(http.StatusNotFound, http.StatusText(http.StatusNotFound))
			return
		}

		path := matches[2]
		once.Do(func() {
			handler.Prefix = matches[1]
		})

		switch filepath.Ext(path) {
		case ".html":
			ctx.Header("Content-Type", "text/html; charset=utf-8")
		case ".css":
			ctx.Header("Content-Type", "text/css; charset=utf-8")
		case ".js":
			ctx.Header("Content-Type", "application/javascript")
		case ".png":
			ctx.Header("Content-Type", "image/png")
		case ".json":
			ctx.Header("Content-Type", "application/json; charset=utf-8")
		}

		swaggerCfg := config.toSwaggerConfig()

		switch path {
		case "index.html":
			_ = index.Execute(ctx.Writer, swaggerCfg)
		case "index.css":
			_ = css.Execute(ctx.Writer, swaggerCfg)
		case "swagger-initializer.js":
			_ = js.Execute(ctx.Writer, swaggerCfg)
		case "doc.json":
			doc, err := swag.ReadDoc(config.InstanceName)
			if err != nil {
				ctx.AbortWithStatus(http.StatusInternalServerError)
				return
			}
			ctx.String(http.StatusOK, doc)
		default:
			handler.ServeHTTP(ctx.Writer, ctx.Request)
		}
	}
}

// DisablingRegister turns Swagger UI off when envName is set (non-empty).
func DisablingRegister(router gin.IRoutes, envName string) {
	if os.Getenv(envName) != "" {
		router.GET("/swagger/*any", func(c *gin.Context) {
			c.String(http.StatusNotFound, "")
		})
		return
	}
	Register(router)
}

const swaggerStyleTpl = `
html
{
    box-sizing: border-box;
    overflow: -moz-scrollbars-vertical;
    overflow-y: scroll;
}
*,
*:before,
*:after
{
    box-sizing: inherit;
}

body {
  margin:0;
  background: #fafafa;
}
`

const initializerJSTpl = `
window.onload = function() {
  const ui = SwaggerUIBundle({
    url: "{{.URL}}",
    dom_id: '#swagger-ui',
    validatorUrl: null,
    oauth2RedirectUrl: {{.Oauth2RedirectURL}},
    persistAuthorization: {{.PersistAuthorization}},
    presets: [
      SwaggerUIBundle.presets.apis,
      SwaggerUIStandalonePreset
    ],
    plugins: [
      SwaggerUIBundle.plugins.DownloadUrl
    ],
    layout: "StandaloneLayout",
    docExpansion: "{{.DocExpansion}}",
    deepLinking: {{.DeepLinking}},
    defaultModelsExpandDepth: {{.DefaultModelsExpandDepth}}
  })

  ui.preauthorizeApiKey("` + securitySchemeName + `", "{{.DefaultAccountToken}}")

  window.ui = ui
}
`

const swaggerIndexTpl = `<!-- HTML for static distribution bundle build -->
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>{{.Title}}</title>
  <link rel="stylesheet" type="text/css" href="./swagger-ui.css" >
  <link rel="icon" type="image/png" href="./favicon-32x32.png" sizes="32x32" />
  <link rel="icon" type="image/png" href="./favicon-16x16.png" sizes="16x16" />
  <link rel="stylesheet" type="text/css" href="index.css" />
</head>

<body>

<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" style="position:absolute;width:0;height:0">
  <defs>
    <symbol viewBox="0 0 20 20" id="unlocked">
          <path d="M15.8 8H14V5.6C14 2.703 12.665 1 10 1 7.334 1 6 2.703 6 5.6V6h2v-.801C8 3.754 8.797 3 10 3c1.203 0 2 .754 2 2.199V8H4c-.553 0-1 .646-1 1.199V17c0 .549.428 1.139.951 1.307l1.197.387C5.672 18.861 6.55 19 7.1 19h5.8c.549 0 1.428-.139 1.951-.307l1.196-.387c.524-.167.953-.757.953-1.306V9.199C17 8.646 16.352 8 15.8 8z"></path>
    </symbol>

    <symbol viewBox="0 0 20 20" id="locked">
      <path d="M15.8 8H14V5.6C14 2.703 12.665 1 10 1 7.334 1 6 2.703 6 5.6V8H4c-.553 0-1 .646-1 1.199V17c0 .549.428 1.139.951 1.307l1.197.387C5.672 18.861 6.55 19 7.1 19h5.8c.549 0 1.428-.139 1.951-.307l1.196-.387c.524-.167.953-.757.953-1.306V9.199C17 8.646 16.352 8 15.8 8zM12 8H8V5.199C8 3.754 8.797 3 10 3c1.203 0 2 .754 2 2.199V8z"/>
    </symbol>

    <symbol viewBox="0 0 20 20" id="close">
      <path d="M14.348 14.849c-.469.469-1.229.469-1.697 0L10 11.819l-2.651 3.029c-.469.469-1.229.469-1.697 0-.469-.469-.469-1.229 0-1.697l2.758-3.15-2.759-3.152c-.469-.469-.469-1.228 0-1.697.469-.469 1.228-.469 1.697 0L10 8.183l2.651-3.031c.469-.469 1.228-.469 1.697 0 .469.469.469 1.229 0 1.697l-2.758 3.152 2.758 3.15c.469.469.469 1.229 0 1.698z"/>
    </symbol>

    <symbol viewBox="0 0 20 20" id="large-arrow">
      <path d="M13.25 10L6.109 2.58c-.268-.27-.268-.707 0-.979.268-.27.701-.27.969 0l7.83 7.908c.268.271.268.709 0 .979l-7.83 7.908c-.268.271-.701.27-.969 0-.268-.269-.268-.707 0-.979L13.25 10z"/>
    </symbol>

    <symbol viewBox="0 0 20 20" id="large-arrow-down">
      <path d="M17.418 6.109c.272-.268.709-.268.979 0s.271.701 0 .969l-7.908 7.83c-.27.268-.707.268-.979 0l-7.908-7.83c-.27-.268-.27-.701 0-.969.271-.268.709-.268.979 0L10 13.25l7.418-7.141z"/>
    </symbol>


    <symbol viewBox="0 0 24 24" id="jump-to">
      <path d="M19 7v4H5.83l3.58-3.59L8 6l-6 6 6 6 1.41-1.41L5.83 13H21V7z"/>
    </symbol>

    <symbol viewBox="0 0 24 24" id="expand">
      <path d="M10 18h4v-2h-4v2zM3 6v2h18V6H3zm3 7h12v-2H6v2z"/>
    </symbol>

  </defs>
</svg>

<div id="swagger-ui"></div>

<script src="./swagger-ui-bundle.js"> </script>
<script src="./swagger-ui-standalone-preset.js"> </script>
<script src="./swagger-initializer.js"> </script>
</body>

</html>
`
