package indexer

// Deduper tracks releases already seen within one search round. The
// identity key comes from Release.Identity (infohash preferred, URL
// fallback, indexer+title last).
type Deduper struct {
	seen map[string]struct{}
}

// NewDeduper returns an empty Deduper.
func NewDeduper() *Deduper { return &Deduper{seen: map[string]struct{}{}} }

// Seen returns true if r's identity was already recorded. The first
// call for a new identity records it and returns false.
func (d *Deduper) Seen(r Release) bool {
	key := r.Identity()
	if _, ok := d.seen[key]; ok {
		return true
	}
	d.seen[key] = struct{}{}
	return false
}

// Filter returns the subset of releases not already seen, preserving
// input order.
func (d *Deduper) Filter(in []Release) []Release {
	out := make([]Release, 0, len(in))
	for _, r := range in {
		if !d.Seen(r) {
			out = append(out, r)
		}
	}
	return out
}
