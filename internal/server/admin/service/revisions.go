package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-idp/inlets/internal/server/admin/model"
	"github.com/go-idp/inlets/internal/server/config"
	"github.com/go-zoox/gormx"
	"gopkg.in/yaml.v3"
)

// maxRevisions is the soft cap for kept revisions. Cleanup happens
// opportunistically after each save/restore.
const maxRevisions = 200

// RevisionService manages ConfigRevision rows. It deliberately does not
// transact with the file write itself: the on-disk save is atomic
// (SaveRawAtomic), but the SQLite write and the file rename are not.
// On failure between the two, the next save will overwrite the orphan
// revision. See plan PR-2 §2.3 for the rationale.
type RevisionService struct{}

func NewRevisionService() *RevisionService { return &RevisionService{} }

// Save persists a new revision row with the given YAML. Returns the row.
func (s *RevisionService) Save(yamlText, summary, actor, clientIP string) (*model.ConfigRevision, error) {
	row := &model.ConfigRevision{
		YAML:      yamlText,
		Summary:   summary,
		Actor:     actor,
		ClientIP:  clientIP,
		BytesSize: len(yamlText),
	}
	if _, err := gormx.Create(row); err != nil {
		return nil, err
	}
	go s.cleanup() // best-effort, async
	return row, nil
}

// List returns the most recent N revisions, newest first.
func (s *RevisionService) List(limit int) ([]*model.ConfigRevision, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	var rows []*model.ConfigRevision
	err := gormx.GetDB().Order("created_at DESC").Limit(limit).Find(&rows).Error
	return rows, err
}

// Get returns one revision by ID.
func (s *RevisionService) Get(id uint) (*model.ConfigRevision, error) {
	var row model.ConfigRevision
	if err := gormx.GetDB().First(&row, id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// cleanup trims the revision table to maxRevisions rows by deleting the
// oldest ones. Called asynchronously after each save.
func (s *RevisionService) cleanup() {
	var count int64
	if err := gormx.GetDB().Model(&model.ConfigRevision{}).Count(&count).Error; err != nil {
		return
	}
	if count <= maxRevisions {
		return
	}
	// Delete oldest (count - maxRevisions) rows.
	var ids []uint
	if err := gormx.GetDB().
		Model(&model.ConfigRevision{}).
		Order("created_at ASC").
		Limit(int(count - maxRevisions)).
		Pluck("id", &ids).Error; err != nil {
		return
	}
	if len(ids) > 0 {
		gormx.GetDB().Where("id IN ?", ids).Delete(&model.ConfigRevision{})
	}
}

// UnifiedDiff returns a small line-based unified diff between two byte
// slices. We avoid external deps by implementing a tiny LCS-based diff
// sufficient for human-readable config change review. The output is
// intentionally simple (no hunk headers, no line counts) — adequate for
// the admin UI which colors +/- lines.
func UnifiedDiff(oldText, newText string) string {
	oldLines := splitLines(oldText)
	newLines := splitLines(newText)
	edits := lcsDiff(oldLines, newLines)
	var sb strings.Builder
	for _, e := range edits {
		switch e.kind {
		case ' ':
			sb.WriteByte(' ')
			sb.WriteString(e.text)
		case '-':
			sb.WriteByte('-')
			sb.WriteString(e.text)
		case '+':
			sb.WriteByte('+')
			sb.WriteString(e.text)
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

type diffEdit struct {
	kind byte // ' ', '-', '+'
	text string
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "\n")
	// Drop the trailing empty string from a trailing newline.
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

// lcsDiff computes a line-level diff using the standard LCS DP algorithm.
// Output is a list of edits (kept, removed, added) representing the path.
func lcsDiff(a, b []string) []diffEdit {
	n, m := len(a), len(b)
	// dp[i][j] = LCS length of a[:i] and b[:j]
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}
	// Backtrack
	out := make([]diffEdit, 0, n+m)
	i, j := n, m
	for i > 0 || j > 0 {
		switch {
		case i > 0 && j > 0 && a[i-1] == b[j-1]:
			out = append(out, diffEdit{' ', a[i-1]})
			i--
			j--
		case j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]):
			out = append(out, diffEdit{'+', b[j-1]})
			j--
		default:
			out = append(out, diffEdit{'-', a[i-1]})
			i--
		}
	}
	// Reverse
	for l, r := 0, len(out)-1; l < r; l, r = l+1, r-1 {
		out[l], out[r] = out[r], out[l]
	}
	return out
}

// Errors surfaced to handlers.
var (
	ErrRevisionNotFound = errors.New("config revision not found")
)

// ErrParseYAML is returned when a save/restore cannot parse the incoming
// YAML into a FileConfig.
func ErrParseYAML(err error) error { return fmt.Errorf("parse config: %w", err) }

// ConfigRevisionView is the list-time projection of a ConfigRevision row.
type ConfigRevisionView struct {
	ID        uint   `json:"id"`
	CreatedAt string `json:"createdAt"`
	Actor     string `json:"actor"`
	ClientIP  string `json:"clientIp"`
	Summary   string `json:"summary"`
	BytesSize int    `json:"bytesSize"`
}

func toView(r *model.ConfigRevision) *ConfigRevisionView {
	return &ConfigRevisionView{
		ID:        r.ID,
		CreatedAt: r.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		Actor:     r.Actor,
		ClientIP:  r.ClientIP,
		Summary:   r.Summary,
		BytesSize: r.BytesSize,
	}
}

func ToViews(rows []*model.ConfigRevision) []*ConfigRevisionView {
	out := make([]*ConfigRevisionView, 0, len(rows))
	for _, r := range rows {
		out = append(out, toView(r))
	}
	return out
}

// ParseConfig is a helper for handler-side validation that mirrors
// config.parseDocument but is exported via the service layer.
func ParseConfig(raw []byte) (*config.FileConfig, error) {
	var cfg config.FileConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
