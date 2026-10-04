package httpx

import (
	"fmt"
	"html/template"
	"net/http"
	"path"
	"sort"
	"strings"

	"github.com/haukened/gone/internal/domain"
)

// IndexRenderer abstracts template execution for easier testing.
// Typically implemented by a thin wrapper around html/template.Template.
type IndexRenderer interface {
	Execute(w http.ResponseWriter, data any) error
}

// TemplateRenderer implements IndexRenderer using html/template.
type TemplateRenderer struct{ T *template.Template }

// Execute renders the wrapped template with data into w. It returns any
// error from template execution.
func (tr TemplateRenderer) Execute(w http.ResponseWriter, data any) error {
	return tr.T.Execute(w, data)
}

// IndexView supplies dynamic config values to the index template.
type IndexView struct {
	MaxBytes      int64
	MaxBytesHuman string
	MinTTLSeconds int
	MaxTTLSeconds int
	TTLOptions    []TTLOptionView
	MinTTLHuman   string
	MaxTTLHuman   string
}

// TTLOptionView is the subset of a domain TTLOption needed by the template.
// Label is the canonical value submitted to the API, Short is the compact
// visible text (for example "1d"), Display is the spelled-out text announced
// to assistive technology, DurationSeconds is provided for client-side
// scripting, and Default marks the preselected option.
type TTLOptionView struct {
	Label           string
	Short           string
	Display         string
	DurationSeconds int
	Default         bool
}

// friendlyTTL renders a duration in seconds as short, readable English using
// the largest whole unit among days, hours, minutes, and seconds.
//
// Parameters:
//   - sec: the duration in seconds; values <= 0 render as "0 sec".
//
// Returns text such as "1 day", "2 hours", "30 min", or "45 sec".
func friendlyTTL(sec int) string {
	switch {
	case sec <= 0:
		return "0 sec"
	case sec%86400 == 0:
		return plural(sec/86400, "day")
	case sec%3600 == 0:
		return plural(sec/3600, "hour")
	case sec%60 == 0:
		return fmt.Sprintf("%d min", sec/60)
	default:
		return fmt.Sprintf("%d sec", sec)
	}
}

// plural formats n with unit, adding an "s" when n is not 1.
//
// Parameters:
//   - n: the count.
//   - unit: the singular unit name.
//
// Returns:
//   - string: for example "1 day" or "3 days".
func plural(n int, unit string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", unit)
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

func humanBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	suffixes := []string{"KB", "MB", "GB", "TB"}
	f := float64(n)
	for _, s := range suffixes {
		f /= 1024
		if f < 1024 {
			return fmt.Sprintf("%.1f %s", f, s)
		}
	}
	return fmt.Sprintf("%.1f PB", f/1024)
}

// humanTTL renders a duration compactly in the largest whole unit among days,
// hours, minutes, and seconds.
//
// Parameters:
//   - sec: the duration in seconds; values <= 0 render as "0s".
//
// Returns text such as "1d", "2h", "3m", or "45s".
func humanTTL(sec int) string {
	if sec <= 0 {
		return "0s"
	}
	if sec%86400 == 0 { // whole days
		return fmt.Sprintf("%dd", sec/86400)
	}
	if sec%3600 == 0 { // whole hours
		return fmt.Sprintf("%dh", sec/3600)
	}
	if sec%60 == 0 { // whole minutes
		return fmt.Sprintf("%dm", sec/60)
	}
	return fmt.Sprintf("%ds", sec)
}

// handleIndex renders the root HTML page.
func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" { // only exact root handled here; let outer fallback produce 404
		return
	}
	if h.IndexTmpl == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("index unavailable"))
		return
	}
	// Standard HTML + no-store headers applied via shared helper.
	view := IndexView{
		MaxBytes:      h.MaxBody,
		MaxBytesHuman: humanBytes(h.MaxBody),
		MinTTLSeconds: int(h.MinTTL.Seconds()),
		MaxTTLSeconds: int(h.MaxTTL.Seconds()),
	}
	view.MinTTLHuman = humanTTL(view.MinTTLSeconds)
	view.MaxTTLHuman = humanTTL(view.MaxTTLSeconds)
	view.TTLOptions = ttlOptionViews(h.TTLOptions)
	renderTemplate(w, h.IndexTmpl, view)
}

// ttlOptionViews converts TTL options to template views sorted shortest
// first, marking the longest option as the default selection.
//
// Parameters:
//   - opts: the configured TTL options; the slice is not modified.
//
// Returns:
//   - []TTLOptionView: the sorted views, or nil when opts is empty.
func ttlOptionViews(opts []domain.TTLOption) []TTLOptionView {
	if len(opts) == 0 {
		return nil
	}
	tmp := make([]domain.TTLOption, len(opts))
	copy(tmp, opts)
	sort.SliceStable(tmp, func(i, j int) bool { return tmp[i].Duration < tmp[j].Duration })
	views := make([]TTLOptionView, 0, len(tmp))
	for _, opt := range tmp {
		sec := int(opt.Duration.Seconds())
		views = append(views, TTLOptionView{Label: opt.Label, Short: humanTTL(sec), Display: friendlyTTL(sec), DurationSeconds: sec})
	}
	views[len(views)-1].Default = true
	return views
}

// staticTypes pins Content-Type for asset extensions that the platform MIME
// table may not know (for example .woff2 on minimal container images), so
// nosniff never blocks them.
var staticTypes = map[string]string{
	".woff2": "font/woff2",
	".ico":   "image/x-icon",
}

// staticHandler serves embedded/static assets under /static/.
func (h *Handler) staticHandler() http.Handler {
	fs := h.Assets
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent directory listings; require a file with extension
		if strings.HasSuffix(r.URL.Path, "/") || path.Ext(r.URL.Path) == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Long-lived caching; caller can fingerprint filenames later.
		w.Header().Set("Cache-Control", "public, max-age=300")
		if ct, ok := staticTypes[path.Ext(r.URL.Path)]; ok {
			w.Header().Set("Content-Type", ct)
		}
		http.FileServer(fs).ServeHTTP(w, r)
	})
}
