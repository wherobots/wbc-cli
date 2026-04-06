package commands

import (
	"fmt"
	"strings"
)

type GlobalFlags struct {
	JSONBody string
	Query    []string
	DryRun   bool
	Tree     bool
	Yes      bool
}

type QueryPair struct {
	Key   string
	Value string
}

func ParseQueryPairs(raw []string) ([]QueryPair, error) {
	pairs := make([]QueryPair, 0, len(raw))
	for _, item := range raw {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid query pair %q (expected key=value)", item)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			return nil, fmt.Errorf("invalid query pair %q (missing key)", item)
		}
		pairs = append(pairs, QueryPair{Key: key, Value: value})
	}
	return pairs, nil
}
