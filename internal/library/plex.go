package library

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jeffWelling/commentarr/internal/title"
)

// PlexConfig configures a Plex backend.
type PlexConfig struct {
	BaseURL string // e.g. https://plex.home.example.com:32400
	Token   string // X-Plex-Token
	Name    string // metric/log label
	Timeout time.Duration
}

// plexSource implements LibrarySource against Plex's REST API.
type plexSource struct {
	cfg PlexConfig
	hc  *http.Client
}

// NewPlexSource returns a LibrarySource backed by the given Plex server.
func NewPlexSource(cfg PlexConfig) LibrarySource {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &plexSource{cfg: cfg, hc: &http.Client{Timeout: timeout}}
}

func (p *plexSource) Name() string { return p.cfg.Name }

// Plex's /library/sections returns MediaContainer → Directory[] (XML).
type plexSections struct {
	XMLName     xml.Name        `xml:"MediaContainer"`
	Directories []plexDirectory `xml:"Directory"`
}

type plexDirectory struct {
	Key  string `xml:"key,attr"`
	Type string `xml:"type,attr"` // "movie" | "show"
}

// Plex's /library/sections/{id}/all returns MediaContainer → Video[] (XML).
type plexVideos struct {
	XMLName xml.Name    `xml:"MediaContainer"`
	Videos  []plexVideo `xml:"Video"`
}

type plexVideo struct {
	RatingKey string     `xml:"ratingKey,attr"`
	Title     string     `xml:"title,attr"`
	Year      int        `xml:"year,attr"`
	Guid      string     `xml:"guid,attr"`
	Type      string     `xml:"type,attr"`     // "movie" | "episode"
	ParentTitle string   `xml:"parentTitle,attr"`
	GrandparentTitle string `xml:"grandparentTitle,attr"`
	Index     int        `xml:"index,attr"`    // episode number
	ParentIndex int      `xml:"parentIndex,attr"` // season number
	Media     []plexMedia `xml:"Media"`
}

type plexMedia struct {
	Parts []plexPart `xml:"Part"`
}

type plexPart struct {
	File string `xml:"file,attr"`
}

// List enumerates every movie + episode across every section.
func (p *plexSource) List(ctx context.Context) ([]title.Title, error) {
	sections, err := p.fetchSections(ctx)
	if err != nil {
		return nil, err
	}
	var out []title.Title
	for _, sec := range sections {
		if sec.Type != "movie" && sec.Type != "show" {
			continue
		}
		vids, err := p.fetchSectionAll(ctx, sec.Key)
		if err != nil {
			return nil, fmt.Errorf("section %s: %w", sec.Key, err)
		}
		for _, v := range vids {
			out = append(out, plexVideoToTitle(v))
		}
	}
	return out, nil
}

// Refresh asks Plex to rescan the section containing path. For v1 we
// do the simplest thing: tell Plex to refresh every movie+show section.
// A future adapter can map paths to sections for targeted rescans.
func (p *plexSource) Refresh(ctx context.Context, path string) error {
	sections, err := p.fetchSections(ctx)
	if err != nil {
		return err
	}
	for _, sec := range sections {
		if sec.Type != "movie" && sec.Type != "show" {
			continue
		}
		u := fmt.Sprintf("%s/library/sections/%s/refresh", strings.TrimRight(p.cfg.BaseURL, "/"), sec.Key)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		p.setAuth(req)
		resp, err := p.hc.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
	}
	return nil
}

func (p *plexSource) setAuth(req *http.Request) {
	req.Header.Set("X-Plex-Token", p.cfg.Token)
	req.Header.Set("Accept", "application/xml")
}

func (p *plexSource) fetchSections(ctx context.Context) ([]plexDirectory, error) {
	u := strings.TrimRight(p.cfg.BaseURL, "/") + "/library/sections"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	p.setAuth(req)
	resp, err := p.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("plex sections: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("plex sections %d: %s", resp.StatusCode, body)
	}
	var secs plexSections
	if err := xml.NewDecoder(resp.Body).Decode(&secs); err != nil {
		return nil, fmt.Errorf("plex decode sections: %w", err)
	}
	return secs.Directories, nil
}

func (p *plexSource) fetchSectionAll(ctx context.Context, sectionKey string) ([]plexVideo, error) {
	u, _ := url.Parse(strings.TrimRight(p.cfg.BaseURL, "/") + "/library/sections/" + url.PathEscape(sectionKey) + "/all")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	p.setAuth(req)
	resp, err := p.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("plex section all: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("plex section all %d: %s", resp.StatusCode, body)
	}
	var vs plexVideos
	if err := xml.NewDecoder(resp.Body).Decode(&vs); err != nil {
		return nil, fmt.Errorf("plex decode videos: %w", err)
	}
	return vs.Videos, nil
}

func plexVideoToTitle(v plexVideo) title.Title {
	var filePath string
	if len(v.Media) > 0 && len(v.Media[0].Parts) > 0 {
		filePath = v.Media[0].Parts[0].File
	}
	kind := title.KindMovie
	if v.Type == "episode" {
		kind = title.KindEpisode
	}
	displayName := v.Title
	if v.Type == "episode" {
		displayName = fmt.Sprintf("%s - S%02dE%02d", v.GrandparentTitle, v.ParentIndex, v.Index)
	}
	return title.Title{
		ID:          "plex:" + v.RatingKey,
		Kind:        kind,
		DisplayName: displayName,
		Year:        v.Year,
		SeriesID: func() string {
			if v.Type == "episode" {
				return "plex-series:" + v.GrandparentTitle
			}
			return ""
		}(),
		Season:   v.ParentIndex,
		Episode:  v.Index,
		FilePath: filePath,
	}
}
