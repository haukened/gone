package httpx

import (
	"html/template"
	"net/http"
	"path"
	"sort"
	"strings"

	"github.com/haukened/gone/v3/internal/domain"
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

// IndexView supplies dynamic config values to the compose templates. Sizes
// and durations are raw numbers; the templates format them for the locale.
type IndexView struct {
	MaxBytes      int64
	MinTTLSeconds int
	MaxTTLSeconds int
	TTLOptions    []TTLOptionView
}

// TTLOptionView is the subset of a domain TTLOption needed by the template.
// Label is the canonical value submitted to the API, DurationSeconds is shown
// in the visitor's language and provided for client-side scripting, and
// Default marks the preselected option.
type TTLOptionView struct {
	Label           string
	DurationSeconds int
	Default         bool
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
	renderTemplate(w, r, h.IndexTmpl, h.indexView())
}

// indexView builds the size and TTL values shared by the pages with a
// compose form (send, request, and reply).
//
// Returns the view.
func (h *Handler) indexView() IndexView {
	view := IndexView{
		MaxBytes:      h.MaxBody,
		MinTTLSeconds: int(h.MinTTL.Seconds()),
		MaxTTLSeconds: int(h.MaxTTL.Seconds()),
	}
	view.TTLOptions = ttlOptionViews(h.TTLOptions)
	return view
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
		views = append(views, TTLOptionView{Label: opt.Label, DurationSeconds: sec})
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
		// Fingerprinted URLs (?v=<content hash>, from the page templates) never
		// change content, so caches may keep them for a year. Bare URLs get a
		// short lifetime so an upgrade is picked up quickly.
		if r.URL.Query().Has("v") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=300")
		}
		if ct, ok := staticTypes[path.Ext(r.URL.Path)]; ok {
			w.Header().Set("Content-Type", ct)
		}
		http.FileServer(fs).ServeHTTP(w, r)
	})
}
