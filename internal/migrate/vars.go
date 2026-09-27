package migrate

import (
	"fmt"
	"regexp"
	"strings"
)

// extractedVar は runbook の直書きの値から作った変数。
type extractedVar struct {
	name  string
	value string
}

// varNames は直書きの値と変数名の対応を管理する。同じ値には同じ変数名を使う。
type varNames struct {
	byValue   map[string]string
	used      map[string]bool
	extracted []extractedVar
}

func newVarNames(envs []*environment) *varNames {
	n := &varNames{byValue: map[string]string{}, used: map[string]bool{}}
	for _, e := range envs {
		for k, v := range e.vars {
			n.used[k] = true
			if k == "ORACLE_DSN" {
				if _, ok := n.byValue[key("db", v)]; !ok {
					n.byValue[key("db", v)] = k
				}
			}
		}
	}
	return n
}

func key(kind, value string) string { return kind + "\x00" + value }

var nonIdent = regexp.MustCompile(`[^A-Za-z0-9]+`)

// nameFor は値に対応する変数名を返す。初めての値なら名前を決めて記録する。
//
//	HTTP の endpoint: API_URL、2 つ目以降の値は <ランナー名>_URL
//	DB の DSN:       ORACLE_DSN (oracle:// 以外は DB_DSN)、2 つ目以降は <ランナー名>_DSN
//	gRPC の addr:    GRPC_ADDR、2 つ目以降は <ランナー名>_ADDR
func (n *varNames) nameFor(kind, runner, value string) string {
	if name, ok := n.byValue[key(kind, value)]; ok {
		return name
	}
	runnerName := strings.Trim(strings.ToUpper(nonIdent.ReplaceAllString(runner, "_")), "_")
	var candidates []string
	switch kind {
	case "http":
		candidates = []string{"API_URL", runnerName + "_URL"}
	case "db":
		if strings.HasPrefix(value, "oracle://") {
			candidates = []string{"ORACLE_DSN", runnerName + "_DSN"}
		} else {
			candidates = []string{"DB_DSN", runnerName + "_DSN"}
		}
	default:
		candidates = []string{"GRPC_ADDR", runnerName + "_ADDR"}
	}
	name := ""
	for _, c := range candidates {
		if !n.used[c] {
			name = c
			break
		}
	}
	if name == "" {
		base := candidates[len(candidates)-1]
		for i := 2; ; i++ {
			if c := fmt.Sprintf("%s_%d", base, i); !n.used[c] {
				name = c
				break
			}
		}
	}
	n.used[name] = true
	n.byValue[key(kind, value)] = name
	n.extracted = append(n.extracted, extractedVar{name: name, value: value})
	return name
}
