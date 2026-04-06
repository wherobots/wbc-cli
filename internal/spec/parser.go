package spec

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/pb33f/libopenapi"
	highbase "github.com/pb33f/libopenapi/datamodel/high/base"
	highv3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
)

var pathParamPattern = regexp.MustCompile(`\{([^{}]+)\}`)

func Parse(rawSpec []byte, openAPIURL string) (*RuntimeSpec, error) {
	document, err := libopenapi.NewDocument(rawSpec)
	if err != nil {
		return nil, fmt.Errorf("create openapi document: %w", err)
	}

	model, err := document.BuildV3Model()
	if err != nil {
		return nil, fmt.Errorf("build openapi v3 model: %w", err)
	}

	specModel := model.Model
	parsed := &RuntimeSpec{
		Raw:        rawSpec,
		BaseURL:    deriveBaseURL(&specModel, openAPIURL),
		Operations: make([]*Operation, 0),
	}

	if specModel.Paths == nil || specModel.Paths.PathItems == nil {
		return parsed, nil
	}

	for pathPair := orderedmap.First(specModel.Paths.PathItems); pathPair != nil; pathPair = pathPair.Next() {
		pathTemplate := pathPair.Key()
		pathItem := pathPair.Value()
		if pathItem == nil {
			continue
		}
		ops := pathItem.GetOperations()
		if ops == nil {
			continue
		}

		for opPair := orderedmap.First(ops); opPair != nil; opPair = opPair.Next() {
			method := strings.ToUpper(opPair.Key())
			op := opPair.Value()
			if op == nil {
				continue
			}

			pathParamOrder := extractPathParamOrder(pathTemplate)
			pathParams, queryParams := collectParameters(pathItem.Parameters, op.Parameters, pathParamOrder)

			parsed.Operations = append(parsed.Operations, &Operation{
				Method:         method,
				Path:           pathTemplate,
				OperationID:    op.OperationId,
				Summary:        op.Summary,
				Description:    op.Description,
				PathParams:     pathParams,
				PathParamOrder: pathParamOrder,
				QueryParams:    queryParams,
				RequestBody:    extractRequestBodyInfo(op),
				Excluded:       isOperationExcluded(op),
			})
		}
	}

	sort.Slice(parsed.Operations, func(i, j int) bool {
		if parsed.Operations[i].Path == parsed.Operations[j].Path {
			return parsed.Operations[i].Method < parsed.Operations[j].Method
		}
		return parsed.Operations[i].Path < parsed.Operations[j].Path
	})

	return parsed, nil
}

func deriveBaseURL(doc *highv3.Document, openAPIURL string) string {
	if doc != nil && len(doc.Servers) > 0 && doc.Servers[0] != nil && strings.TrimSpace(doc.Servers[0].URL) != "" {
		serverURL := strings.TrimSpace(doc.Servers[0].URL)
		parsedServer, err := url.Parse(serverURL)
		if err == nil {
			if parsedServer.IsAbs() {
				return strings.TrimRight(parsedServer.String(), "/")
			}
			if openAPIURL != "" {
				parsedSpecURL, parseErr := url.Parse(openAPIURL)
				if parseErr == nil {
					return strings.TrimRight(parsedSpecURL.ResolveReference(parsedServer).String(), "/")
				}
			}
		}
		return strings.TrimRight(serverURL, "/")
	}

	if openAPIURL == "" {
		return ""
	}

	parsedSpecURL, err := url.Parse(openAPIURL)
	if err != nil {
		return ""
	}
	parsedSpecURL.Path = ""
	parsedSpecURL.RawQuery = ""
	parsedSpecURL.Fragment = ""
	return strings.TrimRight(parsedSpecURL.String(), "/")
}

func isOperationExcluded(op *highv3.Operation) bool {
	if op == nil || op.Extensions == nil {
		return false
	}
	for pair := orderedmap.First(op.Extensions); pair != nil; pair = pair.Next() {
		if pair.Key() == "x-exclude-from-cli" {
			node := pair.Value()
			if node != nil && strings.ToLower(strings.TrimSpace(node.Value)) == "true" {
				return true
			}
		}
	}
	return false
}

func extractPathParamOrder(pathTemplate string) []string {
	matches := pathParamPattern.FindAllStringSubmatch(pathTemplate, -1)
	seen := make(map[string]struct{}, len(matches))
	order := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		name := strings.TrimSpace(match[1])
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		order = append(order, name)
	}
	return order
}

func collectParameters(pathItemParams []*highv3.Parameter, opParams []*highv3.Parameter, pathParamOrder []string) ([]Parameter, []Parameter) {
	type entry struct {
		param Parameter
	}
	merged := make(map[string]entry)
	order := make([]string, 0)

	upsert := func(params []*highv3.Parameter, override bool) {
		for _, p := range params {
			if p == nil || p.Name == "" || p.In == "" {
				continue
			}
			key := strings.ToLower(p.In) + ":" + p.Name
			converted := Parameter{
				Name:     p.Name,
				Location: strings.ToLower(p.In),
				Required: isParamRequired(p),
				Type:     resolveParamType(p),
			}
			if _, exists := merged[key]; !exists {
				order = append(order, key)
			}
			if !override {
				if _, exists := merged[key]; !exists {
					merged[key] = entry{param: converted}
				}
				continue
			}
			merged[key] = entry{param: converted}
		}
	}

	upsert(pathItemParams, false)
	upsert(opParams, true)

	pathParamsByName := make(map[string]Parameter)
	queryParams := make([]Parameter, 0)
	for _, key := range order {
		param := merged[key].param
		switch param.Location {
		case "path":
			pathParamsByName[param.Name] = param
		case "query":
			queryParams = append(queryParams, param)
		}
	}

	pathParams := make([]Parameter, 0, len(pathParamOrder))
	seenPath := make(map[string]struct{}, len(pathParamOrder))
	for _, name := range pathParamOrder {
		if p, exists := pathParamsByName[name]; exists {
			pathParams = append(pathParams, p)
		} else {
			pathParams = append(pathParams, Parameter{
				Name:     name,
				Location: "path",
				Required: true,
				Type:     "string",
			})
		}
		seenPath[name] = struct{}{}
	}
	for _, key := range order {
		param := merged[key].param
		if param.Location != "path" {
			continue
		}
		if _, exists := seenPath[param.Name]; exists {
			continue
		}
		pathParams = append(pathParams, param)
	}

	return pathParams, queryParams
}

func extractRequestBodyInfo(op *highv3.Operation) *RequestBodyInfo {
	if op == nil || op.RequestBody == nil || op.RequestBody.Content == nil {
		return nil
	}

	contentType, mediaType := pickPreferredMediaType(op.RequestBody.Content)
	if mediaType == nil {
		return &RequestBodyInfo{
			Required:    isRequestBodyRequired(op.RequestBody),
			ContentType: contentType,
			SchemaType:  "object",
		}
	}

	schemaType := "object"
	var fields []BodyField
	if mediaType.Schema != nil {
		schemaType = resolveSchemaType(mediaType.Schema)
		fields = extractBodyFields(mediaType.Schema.Schema())
	}

	return &RequestBodyInfo{
		Required:    isRequestBodyRequired(op.RequestBody),
		ContentType: contentType,
		SchemaType:  schemaType,
		Fields:      fields,
	}
}

func pickPreferredMediaType(content *orderedmap.Map[string, *highv3.MediaType]) (string, *highv3.MediaType) {
	if content == nil {
		return "", nil
	}
	var firstKey string
	var firstMedia *highv3.MediaType
	for pair := orderedmap.First(content); pair != nil; pair = pair.Next() {
		contentType := pair.Key()
		mediaType := pair.Value()
		if firstMedia == nil {
			firstKey, firstMedia = contentType, mediaType
		}
		if strings.Contains(strings.ToLower(contentType), "json") {
			return contentType, mediaType
		}
	}
	return firstKey, firstMedia
}

func extractBodyFields(schema *highbase.Schema) []BodyField {
	if schema == nil || schema.Properties == nil || orderedmap.Len(schema.Properties) == 0 {
		return nil
	}

	required := make(map[string]struct{}, len(schema.Required))
	for _, name := range schema.Required {
		required[name] = struct{}{}
	}

	fields := make([]BodyField, 0, orderedmap.Len(schema.Properties))
	for prop := orderedmap.First(schema.Properties); prop != nil; prop = prop.Next() {
		name := prop.Key()
		propSchema := prop.Value()
		fieldType := "string"
		if propSchema != nil {
			fieldType = resolveSchemaType(propSchema)
		}
		_, isRequired := required[name]
		fields = append(fields, BodyField{Name: name, Type: fieldType, Required: isRequired})
	}

	sort.Slice(fields, func(i, j int) bool {
		if fields[i].Required != fields[j].Required {
			return fields[i].Required
		}
		return fields[i].Name < fields[j].Name
	})

	return fields
}

func resolveParamType(param *highv3.Parameter) string {
	if param == nil || param.Schema == nil {
		return "string"
	}
	return resolveSchemaType(param.Schema)
}

func resolveSchemaType(proxy *highbase.SchemaProxy) string {
	if proxy == nil {
		return "string"
	}

	schema := proxy.Schema()
	if schema == nil {
		rebuilt, err := proxy.BuildSchema()
		if err != nil || rebuilt == nil {
			return "string"
		}
		schema = rebuilt
	}

	if len(schema.Type) > 0 && schema.Type[0] != "" {
		return schema.Type[0]
	}
	if schema.Properties != nil && orderedmap.Len(schema.Properties) > 0 {
		return "object"
	}
	if schema.Items != nil && schema.Items.IsA() && schema.Items.A != nil {
		return "array"
	}
	return "string"
}

func isParamRequired(param *highv3.Parameter) bool {
	if param == nil {
		return false
	}
	if strings.EqualFold(param.In, "path") {
		return true
	}
	return param.Required != nil && *param.Required
}

func isRequestBodyRequired(requestBody *highv3.RequestBody) bool {
	return requestBody != nil && requestBody.Required != nil && *requestBody.Required
}
