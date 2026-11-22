package api

import (
	"encoding"
	"encoding/json"
	"reflect"
	"strings"
	"time"

	"github.com/ritoon/zerechnung/invoice"
	"github.com/ritoon/zerechnung/validate"
)

type obj = map[string]any

// openAPI builds the OpenAPI 3.1 document of the API. The schemas of the
// invoice are derived from the Go types of package invoice, so that they
// follow the model.
func openAPI(version string) ([]byte, error) {
	g := &schemaGen{defs: obj{}}
	g.ref(reflect.TypeFor[invoice.Invoice]())
	g.ref(reflect.TypeFor[validate.Finding]())
	schemas := g.defs
	for name, t := range map[string]reflect.Type{
		"InvoiceRequest":  reflect.TypeFor[InvoiceRequest](),
		"InvoiceResponse": reflect.TypeFor[InvoiceResponse](),
		"XMLRequest":      reflect.TypeFor[XMLRequest](),
		"Report":          reflect.TypeFor[Report](),
		"Problem":         reflect.TypeFor[Problem](),
		"VerifyResponse":  reflect.TypeFor[VerifyResponse](),
		"Info":            reflect.TypeFor[Info](),
	} {
		schemas[name] = g.schema(t)
	}
	// The raw JSON fields hold an invoice.
	schemas["InvoiceRequest"].(obj)["properties"].(obj)["invoice"] = ref("Invoice")
	schemas["XMLRequest"].(obj)["properties"].(obj)["invoice"] = ref("Invoice")

	pdf := obj{"application/pdf": obj{"schema": obj{"type": "string", "format": "binary"}}}
	document := obj{
		"required": true,
		"content": obj{
			"application/pdf":          obj{"schema": obj{"type": "string", "format": "binary"}},
			"application/xml":          obj{"schema": obj{"type": "string", "format": "binary"}},
			"application/octet-stream": obj{"schema": obj{"type": "string", "format": "binary"}},
			"multipart/form-data": obj{"schema": obj{
				"type": "object", "required": []string{"file"},
				"properties": obj{"file": obj{"type": "string", "format": "binary"}},
			}},
		},
	}
	errs := func(codes ...string) obj {
		out := obj{}
		desc := map[string]string{
			"400": "Malformed request", "401": "Missing or invalid credentials", "403": "The token lacks a required scope",
			"413": "Request body too large", "422": "The document is refused; the problem tells why",
			"500": "Internal error", "501": "Signing is not configured", "503": "The signing certificate is not valid",
		}
		for _, c := range append([]string{"400", "401", "413"}, codes...) {
			out[c] = obj{"description": desc[c], "content": obj{"application/problem+json": obj{"schema": ref("Problem")}}}
		}
		return out
	}
	merge := func(a, b obj) obj {
		for k, v := range b {
			a[k] = v
		}
		return a
	}
	q := func(name, desc string, schema obj) obj {
		return obj{"name": name, "in": "query", "description": desc, "schema": schema}
	}
	str := obj{"type": "string"}
	boolean := obj{"type": "boolean"}
	validationParams := []obj{
		q("rules", "Rule sets to add, separated by commas: fr, de, choruspro, peppol, nl, it.", str),
		q("auto", "Add the national rules of the seller's country.", boolean),
		q("profile", "Validate against this profile instead of BT-24.", str),
		q("reject_basicwl", "Report BASIC WL as an error under the French rules.", boolean),
		q("service_code_required", "Chorus Pro: the public buyer requires a service code.", boolean),
		q("engagement_required", "Chorus Pro: the public buyer requires an engagement number.", boolean),
	}
	security := []obj{{"bearer": []string{}}, {"basic": []string{}}}

	doc := obj{
		"openapi": "3.1.0",
		"info": obj{
			"title":       "ZeRechnung server",
			"version":     version,
			"description": "Creates Factur-X, ZUGFeRD and XRechnung invoices end to end (PDF/A-3 rendering, CII XML, PAdES signature), validates, signs, verifies and converts e-invoices.",
			"license":     obj{"name": "MIT", "identifier": "MIT"},
		},
		"components": obj{
			"schemas": schemas,
			"securitySchemes": obj{
				"bearer": obj{"type": "http", "scheme": "bearer", "bearerFormat": "JWT"},
				"basic":  obj{"type": "http", "scheme": "basic"},
			},
		},
		"security": security,
		"paths": obj{
			"/v1/invoices": obj{"post": obj{
				"operationId": "createInvoice",
				"summary":     "Create a Factur-X invoice end to end",
				"description": "Renders the invoice as a PDF/A-3 document (or takes the PDF given), embeds its CII XML, checks the whole document and signs it in PAdES. Send JSON, or a multipart form with the parts request (the JSON options), invoice, xml, pdf and attachment. The PDF is returned, or a JSON object with Accept: application/json.",
				"requestBody": obj{"required": true, "content": obj{
					"application/json": obj{"schema": ref("InvoiceRequest")},
					"multipart/form-data": obj{"schema": obj{"type": "object", "properties": obj{
						"request":    ref("InvoiceRequest"),
						"invoice":    ref("Invoice"),
						"xml":        obj{"type": "string", "format": "binary"},
						"pdf":        obj{"type": "string", "format": "binary"},
						"attachment": obj{"type": "array", "items": obj{"type": "string", "format": "binary"}},
					}}},
				}},
				"responses": merge(obj{"201": obj{"description": "The invoice", "content": merge(obj{"application/json": obj{"schema": ref("InvoiceResponse")}}, pdf)}}, errs("422", "500", "501", "503")),
			}},
			"/v1/xml": obj{"post": obj{
				"operationId": "writeXML",
				"summary":     "Write an invoice as CII, UBL, FatturaPA or Facturae XML",
				"requestBody": obj{"required": true, "content": obj{"application/json": obj{"schema": ref("XMLRequest")}}},
				"responses":   merge(obj{"200": obj{"description": "The XML", "content": obj{"application/xml": obj{"schema": str}}}}, errs("422")),
			}},
			"/v1/validate": obj{"post": obj{
				"operationId": "validate",
				"summary":     "Validate a hybrid PDF or an XML invoice (CII, UBL, FatturaPA, Facturae)",
				"parameters":  validationParams,
				"requestBody": document,
				"responses":   merge(obj{"200": obj{"description": "The report, whether the invoice is valid or not", "content": obj{"application/json": obj{"schema": ref("Report")}}}}, errs("422")),
			}},
			"/v1/sign": obj{"post": obj{
				"operationId": "sign",
				"summary":     "Sign a PDF (PAdES B-B) or a FatturaPA document (CAdES or XAdES B-B)",
				"parameters": []obj{
					q("format", "For a FatturaPA document: cades (.xml.p7m, default) or xades.", obj{"type": "string", "enum": []string{"cades", "xades"}}),
					q("reason", "Reason of a PDF signature.", str),
					q("location", "Location of a PDF signature.", str),
				},
				"requestBody": document,
				"responses": merge(obj{"200": obj{"description": "The signed document", "content": merge(obj{
					"application/pkcs7-mime": obj{"schema": obj{"type": "string", "format": "binary"}},
					"application/xml":        obj{"schema": str},
				}, pdf)}}, errs("422", "500", "501", "503")),
			}},
			"/v1/verify": obj{"post": obj{
				"operationId": "verify",
				"summary":     "Check the integrity of the signatures of a PDF or a FatturaPA file (not the trust in the certificates)",
				"requestBody": document,
				"responses":   merge(obj{"200": obj{"description": "The signatures", "content": obj{"application/json": obj{"schema": ref("VerifyResponse")}}}}, errs()),
			}},
			"/v1/extract": obj{"post": obj{
				"operationId": "extract",
				"summary":     "Extract the XML of a hybrid PDF, or read an invoice as JSON",
				"parameters":  []obj{q("format", "xml (default, PDF only) or json.", obj{"type": "string", "enum": []string{"xml", "json"}})},
				"requestBody": document,
				"responses": merge(obj{"200": obj{"description": "The XML or the invoice", "content": obj{
					"application/xml":  obj{"schema": str},
					"application/json": obj{"schema": ref("Invoice")},
				}}}, errs("422")),
			}},
			"/v1/pdfa": obj{"post": obj{
				"operationId": "convertPDFA",
				"summary":     "Convert a PDF to PDF/A-3, or tell why it cannot be",
				"requestBody": document,
				"responses":   merge(obj{"200": obj{"description": "The PDF/A-3 document", "content": pdf}}, errs("422")),
			}},
			"/v1/info": obj{"get": obj{
				"operationId": "info",
				"summary":     "What the server can do, and with which signing key",
				"responses":   merge(obj{"200": obj{"description": "The capabilities", "content": obj{"application/json": obj{"schema": ref("Info")}}}}, errs()),
			}},
			"/healthz": obj{"get": obj{
				"operationId": "healthz", "summary": "Liveness probe", "security": []obj{},
				"responses": obj{"200": obj{"description": "The process is alive"}},
			}},
			"/readyz": obj{"get": obj{
				"operationId": "readyz", "summary": "Readiness probe", "security": []obj{},
				"responses": obj{"200": obj{"description": "Ready"}, "503": obj{"description": "Not ready: starting or stopping"}},
			}},
			"/metrics": obj{"get": obj{
				"operationId": "metrics", "summary": "Prometheus metrics", "security": []obj{},
				"responses": obj{"200": obj{"description": "The metrics", "content": obj{"text/plain": obj{"schema": str}}}},
			}},
		},
	}
	return json.MarshalIndent(doc, "", "  ")
}

func ref(name string) obj { return obj{"$ref": "#/components/schemas/" + name} }

// schemaGen derives JSON schemas from Go types, as encoding/json encodes
// them. Named structs become components.
type schemaGen struct{ defs obj }

var (
	textMarshaler = reflect.TypeFor[encoding.TextMarshaler]()
	timeType      = reflect.TypeFor[time.Time]()
	rawJSON       = reflect.TypeFor[json.RawMessage]()
)

// ref returns a reference to the component of a named struct, building it
// on first use.
func (g *schemaGen) ref(t reflect.Type) obj {
	name := t.Name()
	if _, ok := g.defs[name]; !ok {
		g.defs[name] = obj{} // breaks the recursion
		g.defs[name] = g.object(t)
	}
	return ref(name)
}

func (g *schemaGen) schema(t reflect.Type) obj {
	switch t {
	case reflect.TypeFor[invoice.Decimal]():
		return obj{"type": "string", "pattern": `^[+-]?[0-9]+(\.[0-9]+)?$`, "description": "Exact decimal number, as a string", "examples": []string{"450.00"}}
	case reflect.TypeFor[invoice.Date]():
		return obj{"type": "string", "format": "date"}
	case reflect.TypeFor[invoice.Profile]():
		return obj{"type": "string", "description": "Profile: MINIMUM, BASIC WL, BASIC, EN 16931, EXTENDED, XRECHNUNG, EXTENDED-CTC-FR, PEPPOL, NLCIUS, CIUS-IT, CIUS-PT (UBL only), or a specification identifier (BT-24)", "examples": []string{"EN 16931"}}
	case timeType:
		return obj{"type": "string", "format": "date-time"}
	case rawJSON:
		return obj{}
	}
	if t.Implements(textMarshaler) || reflect.PointerTo(t).Implements(textMarshaler) {
		return obj{"type": "string"}
	}
	switch t.Kind() {
	case reflect.Pointer:
		return g.schema(t.Elem())
	case reflect.Struct:
		if t.Name() != "" && t.PkgPath() != "" && !strings.HasSuffix(t.PkgPath(), "/api") {
			return g.ref(t)
		}
		return g.object(t)
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return obj{"type": "string", "contentEncoding": "base64"}
		}
		return obj{"type": "array", "items": g.schema(t.Elem())}
	case reflect.Map:
		return obj{"type": "object", "additionalProperties": g.schema(t.Elem())}
	case reflect.String:
		return obj{"type": "string"}
	case reflect.Bool:
		return obj{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return obj{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return obj{"type": "number"}
	}
	return obj{}
}

func (g *schemaGen) object(t reflect.Type) obj {
	props := obj{}
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name := f.Name
		if tag, ok := f.Tag.Lookup("json"); ok {
			n, _, _ := strings.Cut(tag, ",")
			if n == "-" {
				continue
			}
			if n != "" {
				name = n
			}
		}
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			for k, v := range g.object(f.Type)["properties"].(obj) {
				props[k] = v
			}
			continue
		}
		props[name] = g.schema(f.Type)
	}
	return obj{"type": "object", "properties": props}
}
