package library

// NewEmbySource returns a LibrarySource backed by an Emby server. Emby
// and Jellyfin share the same REST API dialect; the difference is the
// auth-header name (Emby uses X-Emby-Token, Jellyfin prefers
// X-MediaBrowser-Token but accepts X-Emby-Token for compatibility).
// Rather than duplicate 100% of the adapter we delegate to
// NewJellyfinSource with EmbyMode=true.
func NewEmbySource(cfg JellyfinConfig) LibrarySource {
	cfg.EmbyMode = true
	return NewJellyfinSource(cfg)
}
