package spec

import "strings"

type RuntimeSpec struct {
	Raw        []byte
	BaseURL    string
	Operations []*Operation
}

type Operation struct {
	Method         string
	Path           string
	OperationID    string
	Summary        string
	CommandPath    []string
	Verb           string
	PathParams     []Parameter
	PathParamOrder []string
	QueryParams    []Parameter
	RequestBody    *RequestBodyInfo
	Excluded       bool
}

type Parameter struct {
	Name     string
	Location string
	Required bool
	Type     string
}

type RequestBodyInfo struct {
	Required    bool
	ContentType string
	SchemaType  string
	Fields      []BodyField
}

type BodyField struct {
	Name     string
	Type     string
	Required bool
}

func (o *Operation) Key() string {
	return strings.ToUpper(o.Method) + " " + o.Path
}

func (o *Operation) RequiredPathParamNames() []string {
	return append([]string(nil), o.PathParamOrder...)
}

func (o *Operation) RequiredQueryParamNames() []string {
	var required []string
	for _, param := range o.QueryParams {
		if param.Required {
			required = append(required, param.Name)
		}
	}
	return required
}

func (o *Operation) RequiredBodyParamNames() []string {
	if o.RequestBody == nil {
		return nil
	}
	required := make([]string, 0, len(o.RequestBody.Fields))
	for _, field := range o.RequestBody.Fields {
		if field.Required {
			required = append(required, field.Name)
		}
	}
	return required
}
