package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const maxDocumentBytes = 1 << 20

type valueKind uint8

const (
	objectValue valueKind = iota + 1
	arrayValue
	stringValue
	integerValue
)

type documentSchema struct {
	name     string
	fields   map[string]map[string]struct{}
	kinds    map[string]valueKind
	required []string
}

type parseFailure struct {
	field   string
	message string
	line    int
}

var bucketSpecSchema = documentSchema{
	name: "bucket spec",
	fields: map[string]map[string]struct{}{
		"":         fieldSet("apiVersion", "kind", "metadata", "spec"),
		"metadata": fieldSet("name"),
		"spec":     fieldSet("environment", "owner"),
	},
	kinds: map[string]valueKind{
		"": objectValue, "metadata": objectValue, "spec": objectValue,
		"apiVersion": stringValue, "kind": stringValue, "metadata.name": stringValue,
		"spec.environment": stringValue, "spec.owner": stringValue,
	},
	required: []string{"apiVersion", "kind", "metadata.name", "spec.environment", "spec.owner"},
}

var standardsSchema = documentSchema{
	name: "platform standards",
	fields: map[string]map[string]struct{}{
		"":            fieldSet("apiVersion", "kind", "spec"),
		"spec":        fieldSet("aws", "environments", "bucket"),
		"spec.aws":    fieldSet("accountId", "region"),
		"spec.bucket": fieldSet("module", "bucketNamePrefix", "noncurrentVersionExpirationDays"),
	},
	kinds: map[string]valueKind{
		"": objectValue, "spec": objectValue, "spec.aws": objectValue,
		"spec.bucket": objectValue, "apiVersion": stringValue, "kind": stringValue,
		"spec.aws.accountId": stringValue, "spec.aws.region": stringValue,
		"spec.environments": arrayValue, "spec.bucket.module": stringValue,
		"spec.bucket.bucketNamePrefix":                stringValue,
		"spec.bucket.noncurrentVersionExpirationDays": integerValue,
	},
	required: []string{
		"apiVersion", "kind", "spec.aws.accountId", "spec.aws.region",
		"spec.environments", "spec.bucket.module",
		"spec.bucket.noncurrentVersionExpirationDays",
	},
}

func fieldSet(names ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		set[name] = struct{}{}
	}
	return set
}

// ParseSpec strictly decodes data as a developer Spec in the given format.
// A non-empty []Diagnostic means the returned Spec is not usable.
func ParseSpec(data []byte, format Format) (Spec, []Diagnostic) {
	var spec Spec
	if diagnostics := decodeDocument(data, format, bucketSpecSchema, &spec); len(diagnostics) != 0 {
		return Spec{}, diagnostics
	}
	if spec.APIVersion != "stacksmith/v1alpha1" {
		return Spec{}, diagnostic(bucketSpecSchema.name, "apiVersion", fmt.Sprintf("unsupported apiVersion %q; use %q.", spec.APIVersion, "stacksmith/v1alpha1"))
	}
	if spec.Kind != "Bucket" {
		return Spec{}, diagnostic(bucketSpecSchema.name, "kind", fmt.Sprintf("unsupported kind %q; use %q.", spec.Kind, "Bucket"))
	}
	return spec, nil
}

// ParseStandards strictly decodes data as PlatformStandards in the given
// format. A non-empty []Diagnostic means the returned PlatformStandards is
// not usable.
func ParseStandards(data []byte, format Format) (PlatformStandards, []Diagnostic) {
	var standards PlatformStandards
	if diagnostics := decodeDocument(data, format, standardsSchema, &standards); len(diagnostics) != 0 {
		return PlatformStandards{}, diagnostics
	}
	if standards.APIVersion != "stacksmith/v1alpha1" {
		return PlatformStandards{}, diagnostic(standardsSchema.name, "apiVersion", fmt.Sprintf("unsupported apiVersion %q; use %q.", standards.APIVersion, "stacksmith/v1alpha1"))
	}
	if standards.Kind != "PlatformStandards" {
		return PlatformStandards{}, diagnostic(standardsSchema.name, "kind", fmt.Sprintf("unsupported kind %q; use %q.", standards.Kind, "PlatformStandards"))
	}
	if standards.Spec.Bucket.Module != "s3-private/v1" {
		return PlatformStandards{}, diagnostic(standardsSchema.name, "spec.bucket.module", fmt.Sprintf("unsupported module %q; use %q.", standards.Spec.Bucket.Module, "s3-private/v1"))
	}
	return standards, nil
}

func decodeDocument(data []byte, format Format, schema documentSchema, target any) []Diagnostic {
	if len(data) > maxDocumentBytes {
		return diagnostic(schema.name, "", "document exceeds the 1 MiB limit; reduce its size.")
	}
	if !utf8.Valid(data) {
		return diagnostic(schema.name, "", "document is not valid UTF-8; save it as UTF-8 and retry.")
	}

	var raw any
	var failure *parseFailure
	switch format {
	case FormatYAML:
		raw, failure = readYAMLDocument(data, schema)
	case FormatJSON:
		raw, failure = readJSONDocument(data, schema)
	default:
		return diagnostic(schema.name, "", "unsupported input format; use FormatYAML or FormatJSON.")
	}
	if failure != nil {
		return failureDiagnostic(schema.name, failure)
	}
	for _, path := range schema.required {
		if _, ok := lookup(raw, path); !ok {
			return diagnostic(schema.name, path, "required field is missing; provide a value.")
		}
	}

	if format == FormatYAML {
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		decoder.KnownFields(true)
		if err := decoder.Decode(target); err != nil {
			return diagnostic(schema.name, "", fmt.Sprintf("invalid YAML document; correct its syntax and field types (%v).", err))
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			if err == nil {
				return diagnostic(schema.name, "", "multiple YAML documents are not supported; provide exactly one.")
			}
			return diagnostic(schema.name, "", fmt.Sprintf("invalid YAML after the document; provide exactly one valid document (%v).", err))
		}
		return nil
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return diagnostic(schema.name, "", fmt.Sprintf("invalid JSON document; correct its syntax and field types (%v).", err))
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return diagnostic(schema.name, "", "trailing JSON data is not supported; provide exactly one object.")
		}
		return diagnostic(schema.name, "", fmt.Sprintf("invalid JSON after the object; provide exactly one valid object (%v).", err))
	}
	return nil
}

func readYAMLDocument(data []byte, schema documentSchema) (any, *parseFailure) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, &parseFailure{message: fmt.Sprintf("invalid YAML; provide one valid document (%v).", err)}
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, &parseFailure{message: "multiple YAML documents are not supported; provide exactly one."}
		}
		return nil, &parseFailure{message: fmt.Sprintf("invalid YAML after the document (%v).", err)}
	}
	return inspectYAML(&document, "", schema)
}

func inspectYAML(node *yaml.Node, path string, schema documentSchema) (any, *parseFailure) {
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) != 1 {
			return nil, &parseFailure{field: path, message: "document must contain one value; provide one top-level object.", line: node.Line}
		}
		return inspectYAML(node.Content[0], path, schema)
	}
	if node.Kind == yaml.AliasNode {
		return nil, &parseFailure{field: path, message: "YAML aliases are not supported; write the value explicitly.", line: node.Line}
	}
	if node.Anchor != "" {
		return nil, &parseFailure{field: path, message: "YAML anchors are not supported; write the value explicitly.", line: node.Line}
	}
	if node.Kind == yaml.ScalarNode && node.Tag == "!!null" {
		return nil, &parseFailure{field: path, message: "null is not supported; provide a non-null value.", line: node.Line}
	}

	if expected, ok := expectedKind(path, schema); ok && !yamlKindMatches(node, expected) {
		return nil, &parseFailure{field: path, message: expectedTypeMessage(expected), line: node.Line}
	}

	switch node.Kind {
	case yaml.ScalarNode:
		if !allowedYAMLScalarTag(node.Tag) {
			return nil, &parseFailure{field: path, message: "custom or non-JSON YAML tags are not supported; use a plain JSON-compatible scalar.", line: node.Line}
		}
		if expected, ok := expectedKind(path, schema); ok && expected == stringValue && node.Tag != "!!str" {
			return nil, &parseFailure{field: path, message: "expected a string; quote or provide a string value.", line: node.Line}
		}
		if expected, ok := expectedKind(path, schema); ok && expected == integerValue {
			var value int
			if node.Tag != "!!int" || node.Decode(&value) != nil {
				return nil, &parseFailure{field: path, message: "expected a whole number representable as a Go int; provide an integer value.", line: node.Line}
			}
		}
		return node.Value, nil
	case yaml.MappingNode:
		if node.Tag != "!!map" {
			return nil, &parseFailure{field: path, message: "custom YAML tags are not supported; use an ordinary mapping.", line: node.Line}
		}
		values := make(map[string]any, len(node.Content)/2)
		seen := make(map[string]struct{}, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if key.Anchor != "" {
				return nil, &parseFailure{field: path, message: "YAML anchors are not supported; write the value explicitly.", line: key.Line}
			}
			if key.Tag == "!!merge" {
				return nil, &parseFailure{field: joinField(path, key.Value), message: "YAML merge keys are not supported; write the fields explicitly.", line: key.Line}
			}
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
				return nil, &parseFailure{field: path, message: "mapping keys must be strings; use quoted or plain string keys.", line: key.Line}
			}
			if _, ok := seen[key.Value]; ok {
				return nil, &parseFailure{field: joinField(path, key.Value), message: "duplicate key; remove one occurrence.", line: key.Line}
			}
			seen[key.Value] = struct{}{}
			if allowed, ok := schema.fields[path]; ok {
				if _, known := allowed[key.Value]; !known {
					return nil, &parseFailure{field: joinField(path, key.Value), message: "unknown field; use only fields defined by the document schema.", line: key.Line}
				}
			}
			field := joinField(path, key.Value)
			decoded, failure := inspectYAML(value, field, schema)
			if failure != nil {
				return nil, failure
			}
			values[key.Value] = decoded
		}
		return values, nil
	case yaml.SequenceNode:
		if node.Tag != "!!seq" {
			return nil, &parseFailure{field: path, message: "custom YAML tags are not supported; use an ordinary sequence.", line: node.Line}
		}
		values := make([]any, len(node.Content))
		for i, child := range node.Content {
			field := fmt.Sprintf("%s[%d]", path, i)
			decoded, failure := inspectYAML(child, field, schema)
			if failure != nil {
				return nil, failure
			}
			values[i] = decoded
		}
		return values, nil
	default:
		return nil, &parseFailure{field: path, message: "unsupported YAML value; use a mapping, sequence, string, boolean, or number.", line: node.Line}
	}
}

func allowedYAMLScalarTag(tag string) bool {
	switch tag {
	case "!!str", "!!int", "!!float", "!!bool", "!!null":
		return true
	default:
		return false
	}
}

func yamlKindMatches(node *yaml.Node, expected valueKind) bool {
	switch expected {
	case objectValue:
		return node.Kind == yaml.MappingNode
	case arrayValue:
		return node.Kind == yaml.SequenceNode
	case stringValue, integerValue:
		return node.Kind == yaml.ScalarNode
	default:
		return true
	}
}

func readJSONDocument(data []byte, schema documentSchema) (any, *parseFailure) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	value, failure := readJSONValue(decoder, "", schema)
	if failure != nil {
		return nil, failure
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return nil, &parseFailure{message: "trailing JSON data is not supported; provide exactly one object."}
		}
		return nil, &parseFailure{message: fmt.Sprintf("invalid JSON after the object; provide exactly one valid object (%v).", err)}
	}
	return value, nil
}

func readJSONValue(decoder *json.Decoder, path string, schema documentSchema) (any, *parseFailure) {
	token, err := decoder.Token()
	if err != nil {
		return nil, &parseFailure{field: path, message: fmt.Sprintf("invalid JSON; provide one valid object (%v).", err)}
	}
	if token == nil {
		return nil, &parseFailure{field: path, message: "null is not supported; provide a non-null value."}
	}
	if failure := checkJSONKind(token, path, schema); failure != nil {
		return nil, failure
	}

	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return token, nil
	}
	switch delimiter {
	case '{':
		values := make(map[string]any)
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, &parseFailure{field: path, message: fmt.Sprintf("invalid JSON object key (%v).", err)}
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, &parseFailure{field: path, message: "JSON object keys must be strings."}
			}
			field := joinField(path, key)
			if _, ok := seen[key]; ok {
				return nil, &parseFailure{field: field, message: "duplicate key; remove one occurrence."}
			}
			seen[key] = struct{}{}
			if allowed, ok := schema.fields[path]; ok {
				if _, known := allowed[key]; !known {
					return nil, &parseFailure{field: field, message: "unknown field; use only fields defined by the document schema."}
				}
			}
			value, failure := readJSONValue(decoder, field, schema)
			if failure != nil {
				return nil, failure
			}
			values[key] = value
		}
		if _, err := decoder.Token(); err != nil {
			return nil, &parseFailure{field: path, message: fmt.Sprintf("invalid JSON object ending (%v).", err)}
		}
		return values, nil
	case '[':
		values := make([]any, 0)
		for i := 0; decoder.More(); i++ {
			value, failure := readJSONValue(decoder, fmt.Sprintf("%s[%d]", path, i), schema)
			if failure != nil {
				return nil, failure
			}
			values = append(values, value)
		}
		if _, err := decoder.Token(); err != nil {
			return nil, &parseFailure{field: path, message: fmt.Sprintf("invalid JSON array ending (%v).", err)}
		}
		return values, nil
	default:
		return nil, &parseFailure{field: path, message: "unexpected JSON delimiter; provide a valid object or array."}
	}
}

func checkJSONKind(token any, path string, schema documentSchema) *parseFailure {
	expected, ok := expectedKind(path, schema)
	if !ok {
		return nil
	}
	matches := false
	switch expected {
	case objectValue:
		if delimiter, ok := token.(json.Delim); ok {
			matches = delimiter == '{'
		}
	case arrayValue:
		if delimiter, ok := token.(json.Delim); ok {
			matches = delimiter == '['
		}
	case stringValue:
		_, matches = token.(string)
	case integerValue:
		if number, ok := token.(json.Number); ok {
			_, err := strconv.ParseInt(number.String(), 10, strconv.IntSize)
			matches = err == nil
		}
	}
	if !matches {
		return &parseFailure{field: path, message: expectedTypeMessage(expected)}
	}
	return nil
}

func expectedKind(path string, schema documentSchema) (valueKind, bool) {
	if strings.HasPrefix(path, "spec.environments[") && schema.name == standardsSchema.name {
		return stringValue, true
	}
	kind, ok := schema.kinds[path]
	return kind, ok
}

func expectedTypeMessage(expected valueKind) string {
	switch expected {
	case objectValue:
		return "expected an object; provide a mapping/object value."
	case arrayValue:
		return "expected an array; provide a sequence/array value."
	case stringValue:
		return "expected a string; provide a string value."
	case integerValue:
		return "expected a whole number representable as a Go int; provide an integer value."
	default:
		return "value has the wrong type; use the type required by the field."
	}
}

func lookup(value any, path string) (any, bool) {
	current := value
	for _, part := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func joinField(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}

func failureDiagnostic(document string, failure *parseFailure) []Diagnostic {
	message := failure.message
	if failure.line > 0 {
		message = fmt.Sprintf("line %d: %s", failure.line, message)
	}
	return diagnostic(document, failure.field, message)
}

func diagnostic(document, field, message string) []Diagnostic {
	return []Diagnostic{{Document: document, Field: field, Message: message}}
}
