package types

import (
	"net/url"

	"time"
)

type Site struct {
	Path         string
	Filename     string
	Fetched      bool
	Size         int64
	LastModified time.Time
	HasRandom    bool

	Notes string
}

type Data struct {
	RootUrl     *url.URL
	Sites       map[string]*Site
	SavePath    string
	RandomSites map[string]bool
}
