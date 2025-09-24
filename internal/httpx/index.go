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
// DurationSeconds is provided for potential client-side scripting.
type TTLOptionView struct {
	Label           string
	DurationSeconds int
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

// humanTTL renders a duration in the largest whole unit among hours, minutes, seconds.
// Examples: 7200s -> "2h"; 180s -> "3m"; 45s -> "45s".
func humanTTL(sec int) string {
	if sec <= 0 {
		return "0s"
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
	if r.URL.Path != "/" { // only exact root handled here
		h.writeError(w, http.StatusNotFound, "not found")
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
	if len(h.TTLOptions) > 0 {
		// copy then sort descending by duration so longest appears first (default selected)
		tmp := make([]domain.TTLOption, len(h.TTLOptions))
		copy(tmp, h.TTLOptions)
		sort.Slice(tmp, func(i, j int) bool { return tmp[i].Duration > tmp[j].Duration })
		view.TTLOptions = make([]TTLOptionView, 0, len(tmp))
		for _, opt := range tmp {
			view.TTLOptions = append(view.TTLOptions, TTLOptionView{Label: opt.Label, DurationSeconds: int(opt.Duration.Seconds())})
		}
	}
	renderTemplate(w, h.IndexTmpl, view)
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
		http.FileServer(fs).ServeHTTP(w, r)
	})
}
