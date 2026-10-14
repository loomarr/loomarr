package episodeevidence

import (
	"bytes"
	"encoding/json"
	"errors"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// DecodeObject removes provider-controlled editorial members from one episode
// object, decodes them with occurrence-aware unavailable-evidence semantics, and
// returns the untouched structural members for the caller's domain projection.
func DecodeObject(raw []byte) ([]byte, Evidence, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	opening, err := dec.Token()
	if err != nil {
		return nil, Evidence{}, err
	}
	if delim, ok := opening.(json.Delim); !ok || delim != '{' {
		return nil, Evidence{}, errors.New("episode must be a JSON object")
	}

	type editorialField struct {
		value json.RawMessage
		count int
	}
	editorial := map[string]editorialField{}
	var structural bytes.Buffer
	structural.WriteByte('{')
	first := true
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return nil, Evidence{}, err
		}
		name, ok := keyToken.(string)
		if !ok {
			return nil, Evidence{}, errors.New("episode object member name is not a string")
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, Evidence{}, err
		}
		if canonical, ok := editorialName(name); ok {
			field := editorial[canonical]
			field.count++
			if field.count == 1 {
				field.value = value
			}
			editorial[canonical] = field
			continue
		}
		encodedName, err := json.Marshal(name)
		if err != nil {
			return nil, Evidence{}, err
		}
		if !first {
			structural.WriteByte(',')
		}
		first = false
		structural.Write(encodedName)
		structural.WriteByte(':')
		structural.Write(value)
	}
	closing, err := dec.Token()
	if err != nil {
		return nil, Evidence{}, err
	}
	if delim, ok := closing.(json.Delim); !ok || delim != '}' {
		return nil, Evidence{}, errors.New("episode JSON object is not closed")
	}
	structural.WriteByte('}')

	var rating float64
	if field := editorial["communityrating"]; field.count == 1 {
		_ = json.Unmarshal(field.value, &rating)
	}
	var overview string
	if field := editorial["overview"]; field.count == 1 {
		_ = json.Unmarshal(field.value, &overview)
	}
	var tags []string
	if field := editorial["tags"]; field.count == 1 {
		if json.Unmarshal(field.value, &tags) != nil {
			tags = nil
		}
	}
	return structural.Bytes(), Sanitize(rating, overview, tags), nil
}

func editorialName(name string) (string, bool) {
	canonical := norm.NFKC.String(cases.Fold().String(norm.NFKC.String(name)))
	switch canonical {
	case "communityrating", "overview", "tags":
		return canonical, true
	default:
		return "", false
	}
}
