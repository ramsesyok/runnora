package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ramsesyok/runnora/internal/app"
	"github.com/ramsesyok/runnora/internal/evidence"
	"github.com/ramsesyok/runnora/internal/reporter"
)

// defaultReportDir は runnora.yaml の report.dir を省略したときの、実行ごとのフォルダを作る場所。
const defaultReportDir = "reports"

// runFolder は 1 回の run の出力先 (reports/<日時>-<スイート名>/)。
type runFolder struct {
	// Dir は実行ごとのフォルダ (絶対パス)。
	Dir string
	// EvidenceDir は証跡の保存先 (絶対パス)。証跡を保存しない場合は ""。
	EvidenceDir string
}

// evidenceOptions は証跡の保存先を決めるための指定。
type evidenceOptions struct {
	// flagDir は --evidence-dir (カレントディレクトリ基準)。
	flagDir string
	// projectDir は runnora.yaml の evidence.dir (プロジェクトルート基準)。
	projectDir string
	disabled   bool
}

// newRunFolder は base (report.dir) の下に <日時>-<名前> のフォルダを作る。
// 同じ名前があれば -2、-3 … を付ける (上書きしない)。
func newRunFolder(root, base, name string, now time.Time, ev evidenceOptions) (*runFolder, error) {
	if base == "" {
		base = defaultReportDir
	}
	if !filepath.IsAbs(base) {
		base = filepath.Join(root, filepath.FromSlash(base))
	}
	if name == "" {
		name = "run"
	}
	stem := now.Format("20060102-150405") + "-" + evidence.SafeName(name)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return nil, err
	}
	var dir string
	for i := 1; ; i++ {
		dir = filepath.Join(base, stem)
		if i > 1 {
			dir = fmt.Sprintf("%s-%d", dir, i)
		}
		err := os.Mkdir(dir, 0o755)
		if err == nil {
			break
		}
		if !os.IsExist(err) {
			return nil, err
		}
	}
	f := &runFolder{Dir: dir}
	switch {
	case ev.disabled:
	case ev.flagDir != "":
		abs, err := filepath.Abs(ev.flagDir)
		if err != nil {
			return nil, err
		}
		f.EvidenceDir = abs
	case ev.projectDir != "":
		f.EvidenceDir = ev.projectDir
		if !filepath.IsAbs(f.EvidenceDir) {
			f.EvidenceDir = filepath.Join(root, filepath.FromSlash(f.EvidenceDir))
		}
	default:
		f.EvidenceDir = filepath.Join(dir, "evidence")
	}
	return f, nil
}

// relEvidenceDir は report.json に書く証跡のフォルダ (実行ごとのフォルダからの相対パス、外なら絶対パス)。
func (f *runFolder) relEvidenceDir() string {
	if f.EvidenceDir == "" {
		return ""
	}
	rel, err := filepath.Rel(f.Dir, f.EvidenceDir)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || len(rel) > 2 && rel[:3] == ".."+string(filepath.Separator) {
		return filepath.ToSlash(f.EvidenceDir)
	}
	return filepath.ToSlash(rel)
}

// writeReports は実行ごとのフォルダに report.json (と、junit のときは report.xml) を書く。
func (f *runFolder) writeReports(rep *reporter.Report, format string) (string, error) {
	jsonPath := filepath.Join(f.Dir, "report.json")
	if err := writeReportFile("json", jsonPath, rep); err != nil {
		return "", err
	}
	if format == "junit" {
		if err := writeReportFile("junit", filepath.Join(f.Dir, "report.xml"), rep); err != nil {
			return "", err
		}
	}
	return jsonPath, nil
}

// removeIfEmpty は何も書かなかった実行ごとのフォルダを消す (実行前に失敗した場合)。
func (f *runFolder) removeIfEmpty() {
	if f.EvidenceDir != "" {
		_ = os.Remove(f.EvidenceDir)
	}
	_ = os.Remove(f.Dir)
}

func writeReportFile(format, path string, rep *reporter.Report) error {
	r, err := reporter.NewFileReporter(format, path)
	if err != nil {
		return &app.AppError{ExitCode: 5, Cause: fmt.Errorf("report: %w", err)}
	}
	writeErr := r.Write(rep)
	if closeErr := r.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		return &app.AppError{ExitCode: 5, Cause: fmt.Errorf("report write: %w", writeErr)}
	}
	return nil
}
