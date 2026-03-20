package hook

import (
	"fmt"
	"os"
	"strings"
)

// Resolver はフック SQL ファイルの存在を検証する。
type Resolver struct{}

// NewResolver は Resolver を生成する。
func NewResolver() *Resolver {
	return &Resolver{}
}

// Validate は指定されたすべてのファイルが存在することを確認する。
// 存在しないファイルが 1 つ以上ある場合は、すべてのパスを列挙したエラーを返す。
func (r *Resolver) Validate(files []string) error {
	var missing []string
	for _, f := range files {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("hook: missing SQL files: %s", strings.Join(missing, ", "))
	}
	return nil
}
