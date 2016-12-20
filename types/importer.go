package types

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"time"
)

type Site struct {
	Key SiteKey
	//Extension       string
	MimeType        string
	HighestRevision int
	// HasRandom is true when there's a listing of random sites on the page.
	// Used to find unlinked pages.
	HasRandom bool
	Revisions map[int]*Revision

	Notes string
}
type Revision struct {
	Revision int
	Author   string
	Message  string
	Date     time.Time
	Size     int64
}

// SiteKey is a unique identifier for a site.
// If isAttachment is true, the file will be downloaded as an attachment.
type SiteKey struct {
	Path         string
	IsAttachment bool
}

// MarshalText converts this SiteKey to 'path\nIsAttachment', so that we can use
// it as a map key
func (s SiteKey) MarshalText() (text []byte, err error) {
	pathWithoutNewlines := strings.Replace(s.Path, "\n", "", -1) // shouldn't have them anyway
	return []byte(pathWithoutNewlines + "\n" + strconv.FormatBool(s.IsAttachment)), nil
}

// UnmarshalText parses 'path\nIsAttachment', so that we can use
// it as a map key
func (s *SiteKey) UnmarshalText(text []byte) error {
	split := strings.SplitN(string(text), "\n", 2)
	s.Path = split[0]
	if len(split) < 2 {
		return errors.New("SiteKey must be in format 'Path\\nboolean'. Got: " +
			string(text))
	}
	s.IsAttachment, _ = strconv.ParseBool(split[1])
	return nil
}

// MarshalJSON marshals this SiteKey as a JSON object, so that we
// get the expected value when using it as a value
func (s SiteKey) MarshalJSON() ([]byte, error) {
	s2 := struct {
		Path         string
		IsAttachment bool
	}{s.Path, s.IsAttachment}
	return json.Marshal(&s2)
}

// UnmarshalJSON unmarshals this SiteKey from a JSON object, so that we
// get the expected value when using it as a value
func (s *SiteKey) UnmarshalJSON(data []byte) error {
	if bytes.HasPrefix(data, []byte{'"'}) {
		// note even the map key sometimes uses this method... which makes NO sense...
		// so work around.
		var str string
		err := json.Unmarshal(data, &str)
		if err != nil {
			return err
		}
		return s.UnmarshalText([]byte(str))
	}
	s2 := struct {
		Path         string
		IsAttachment bool
	}{s.Path, s.IsAttachment}
	err := json.Unmarshal(data, &s2)
	s.Path = s2.Path
	s.IsAttachment = s2.IsAttachment
	return err
}

type Data struct {
	RootUrl  *url.URL
	Sites    map[SiteKey]*Site
	SavePath string

	// Set of all sites (using map key) having HasRandom=true.
	RandomSites map[SiteKey]bool
}

// SiteUrl returns the (idealized) canonical URL for a site or file
func (s SiteKey) SiteUrl(d Data) string {
	return d.RootUrl.String() + s.Path
}

// RequestUrl returns the URL used for downloading a site or file
func (s SiteKey) RequestUrl(d Data) string {
	if !s.IsAttachment {
		return s.SiteUrl(d) + "?action=raw"
	}

	parsedSiteUrl, _ := url.Parse(s.SiteUrl(d))
	query := url.Values{}
	query.Set("action", "AttachFile")
	query.Set("do", "get")
	parsedSiteUrl.RawQuery = query.Encode()

	safeUrl := parsedSiteUrl.Scheme + "://" + parsedSiteUrl.Host + "/" +
		parsedSiteUrl.Path + "?" +
		strings.Replace(parsedSiteUrl.RawQuery, " ", "%20", -1)

	return safeUrl
}

func (s Site) FileSavePath(revision int, d *Data) string {
	path := s.Key.Path
	if path == "/" || path == "" {
		path = "index"
	}
	return fmt.Sprintf("%s.d/%s/%d", d.SavePath, path, revision)
}
