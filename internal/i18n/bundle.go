package i18n

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
)

// Bundle holds the loaded catalogs. It is read-only after Load and safe for
// concurrent use.
type Bundle struct {
	locales  []localeInfo                   // loaded locales in picker order
	catalogs map[string]map[string]*message // locale tag to key to message
	wire     map[string][]byte              // locale tag to its catalog as sent to the browser
	sums     map[string]string              // locale tag to a short hash of its wire catalog
}

// Options controls Load.
type Options struct {
	// Pseudo adds the PseudoLocale, generated from the base catalog. Use it
	// in development only.
	Pseudo bool
}

// Load reads every "<tag>.json" catalog in dir whose tag is a known locale
// and checks each against the base catalog: the same keys, and for every key
// the same argument and slot names. Other files (such as notes for
// translators) are ignored.
//
// Parameters:
//   - fsys: filesystem holding the catalogs.
//   - dir: directory inside fsys.
//   - opts: load options.
//
// Returns:
//   - *Bundle: the loaded catalogs.
//   - error: non-nil if the base catalog is missing or any catalog is invalid.
func Load(fsys fs.FS, dir string, opts Options) (*Bundle, error) {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		return nil, err
	}
	entries, err := fs.ReadDir(sub, ".")
	if err != nil {
		return nil, err
	}
	b := &Bundle{catalogs: map[string]map[string]*message{}, wire: map[string][]byte{}, sums: map[string]string{}}
	for _, e := range entries {
		tag, ok := catalogTag(e)
		if !ok {
			continue
		}
		if b.catalogs[tag], err = readCatalog(sub, e.Name()); err != nil {
			return nil, err
		}
	}
	if err := b.finish(opts); err != nil {
		return nil, err
	}
	return b, nil
}

// catalogTag returns the locale tag a directory entry is the catalog for, or
// false when it is not a "<tag>.json" file for a known, non-pseudo locale.
func catalogTag(e fs.DirEntry) (string, bool) {
	tag, isJSON := strings.CutSuffix(e.Name(), ".json")
	info, ok := lookupInfo(tag)
	return tag, isJSON && !e.IsDir() && ok && info.Tag == tag && tag != PseudoLocale
}

// readCatalog reads and parses one catalog file.
func readCatalog(fsys fs.FS, name string) (map[string]*message, error) {
	raw, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, err
	}
	cat, err := parseCatalog(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return cat, nil
}

// finish checks the catalogs against the base, adds the pseudo-locale when
// asked, and prepares the browser copies.
func (b *Bundle) finish(opts Options) error {
	base, ok := b.catalogs[BaseLocale]
	if !ok {
		return fmt.Errorf("missing %s.json", BaseLocale)
	}
	for tag, cat := range b.catalogs {
		if err := checkCatalog(base, cat); err != nil {
			return fmt.Errorf("%s.json: %w", tag, err)
		}
	}
	if opts.Pseudo {
		b.catalogs[PseudoLocale] = pseudoCatalog(base)
	}
	for _, k := range known {
		cat, ok := b.catalogs[k.Tag]
		if !ok {
			continue
		}
		b.locales = append(b.locales, k)
		w, err := json.Marshal(wireCatalog(cat))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(w)
		b.wire[k.Tag], b.sums[k.Tag] = w, hex.EncodeToString(sum[:6])
	}
	return nil
}

// parseCatalog decodes a catalog file, skipping the "$schema" entry.
func parseCatalog(raw []byte) (map[string]*message, error) {
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	cat := make(map[string]*message, len(entries))
	for key, val := range entries {
		if key == "$schema" {
			continue
		}
		m, err := parseMessage(val)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		cat[key] = m
	}
	return cat, nil
}

// checkCatalog reports the first difference between cat and base: a missing
// or extra key, or a message whose argument or slot names differ.
func checkCatalog(base, cat map[string]*message) error {
	for _, key := range sortedKeys(base) {
		m, ok := cat[key]
		if !ok {
			return fmt.Errorf("missing key %s", key)
		}
		if got, want := m.signature(), base[key].signature(); got != want {
			return fmt.Errorf("%s: placeholders [%s], want [%s]", key, got, want)
		}
	}
	for key := range cat {
		if _, ok := base[key]; !ok {
			return fmt.Errorf("unknown key %s", key)
		}
	}
	return nil
}
