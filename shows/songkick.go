// Copyright 2013 Jesse Allen. All rights reserved
// Released under the MIT license found in the LICENSE file.

package shows

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

type skEvent struct {
	ID          int             `json:"id"`
	Type        string          `json:"type"` // Concert|Festival
	URI         string          `json:"uri"`
	DisplayName string          `json:"displayName"`
	Start       skDate          `json:"start"`
	End         skDate          `json:"end"`
	Performance []skPerformance `json:"performance"`
	Location    skLocation      `json:"location"`
	Venue       skVenue         `json:"venue"`
	Popularity  float32         `json:"popularity"`
	Series      struct {
		DisplayName string `json:"displayName"`
	} `json:"series"`
}

func (ske skEvent) Event() Event {
	e := Event{
		Name:      ske.DisplayName,
		SameAs:    ske.URI,
		URL:       []string{ske.URI},
		StartDate: time.Time(ske.Start),
		EndDate:   time.Time(ske.End),
		Duration: func(start, end time.Time) time.Duration {
			if !start.IsZero() && !end.IsZero() {
				return end.Sub(start)
			}
			return 0
		}(time.Time(ske.Start), time.Time(ske.End)),
		Location: Place{
			Name:   ske.Venue.DisplayName,
			SameAs: ske.Venue.URI,
			URL:    []string{ske.Venue.URI},
			Geo: GeoCoordinates{
				Lat: ske.Venue.Lat,
				Lng: ske.Venue.Lng,
			},
			Address: Address{
				AddressCountry:  ske.Venue.MetroArea.Country.DisplayName,
				AddressRegion:   ske.Venue.MetroArea.State.DisplayName,
				AddressLocality: ske.Venue.MetroArea.DisplayName,
			},
		},
		Performer: func(skp []skPerformance) []MusicGroup {
			p := make([]MusicGroup, 0)
			for _, i := range skp {
				p = append(p, MusicGroup{
					Name:   i.Artist.DisplayName,
					SameAs: i.Artist.URI,
					URL:    []string{i.Artist.URI},
				})
			}
			return p
		}(ske.Performance),
		Type: func(t string) EventType {
			switch t {
			case "Concert":
				return Concert
			case "Festival":
				return Festival
			default:
				return Concert
			}
		}(ske.Type),
	}
	return e
}

type skPerformance struct {
	ID           int      `json:"id"`
	Artist       skArtist `json:"artist"`
	DisplayName  string   `json:"displayName"`
	BillingIndex int      `json:"billingIndex"`
	Billing      string   `json:"billing"` // headline|support
}

type skArtist struct {
	ID          int            `json:"id"`
	URI         string         `json:"uri"`
	DisplayName string         `json:"displayName"`
	Identifier  []skIdentifier `json:"identifier"`
}

type skIdentifier struct {
	HREF string `json:"href"`
	MBID string `json:"mbid"`
}

type skLocation struct {
	City string  `json:"city"`
	Lat  float32 `json:"lat"`
	Lng  float32 `json:"lng"`
}

type skVenue struct {
	ID          int         `json:"id"`
	DisplayName string      `json:"displayName"`
	URI         string      `json:"uri"`
	MetroArea   skMetroArea `json:"metroArea"`
	Lat         float32     `json:"lat"`
	Lng         float32     `json:"lng"`
}

type skMetroArea struct {
	ID          int    `json:"id"`
	URI         string `json:"uri"`
	DisplayName string `json:"displayName"`
	Country     struct {
		DisplayName string `json:"displayName"`
	} `json:"country"`
	State struct {
		DisplayName string `json:"displayName"`
	} `json:"state"`
}

type skDate time.Time

// UnmarshalJSON implements the json.Unmarshaler interface.
// Time is expected in RFC3339 format.
func (d *skDate) UnmarshalJSON(data []byte) error {
	var raw struct {
		Date     string `json:"date"`
		DateTime string `json:"datetime"`
	}
	json.Unmarshal(data, &raw)
	if len(raw.DateTime) > 0 { // use this value by default
		if t, err := time.Parse("2006-01-02T15:04:05-0700", raw.DateTime); err != nil {
			return err
		} else {
			*d = skDate(t)
			return nil
		}
	} else if len(raw.Date) > 0 {
		if t, err := time.Parse("2006-01-02", raw.Date); err != nil {
			return err
		} else {
			*d = skDate(t)
			return nil
		}
	}
	return errors.New("No Date or Datetime")
}

type skArtistCalendar struct {
	Events       []skEvent
	TotalEntries int
	PerPage      int
	Page         int
	Endpoint     string // URL to Get
}

// The JSON is really verbose, we take it down to a more simple structure
func (ac *skArtistCalendar) UnmarshalJSON(data []byte) error {
	tmp := struct {
		ResultsPage struct {
			Results struct {
				Event []skEvent `json:"event"`
			} `json:"results"`
			TotalEntries int `json:"totalEntries"`
			PerPage      int `json:"perPage"`
			Page         int `json:"page"`
		} `json:"resultsPage"`
	}{}
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	ac.Events = tmp.ResultsPage.Results.Event
	ac.TotalEntries = tmp.ResultsPage.TotalEntries
	ac.PerPage = tmp.ResultsPage.PerPage
	ac.Page = tmp.ResultsPage.Page
	return nil
}

// returns the count of events polled so far
func (ac skArtistCalendar) count() int {
	count := ac.PerPage * ac.Page
	if count > ac.TotalEntries {
		count = ac.TotalEntries
	}
	return count
}

func getSkArtistCalendar(url string) (*skArtistCalendar, error) {
	// this is a zero calendar, no url and no pages
	return (&skArtistCalendar{Endpoint: url}).next()
}

const (
	ErrorNoEndpoint   = "skArtistCalendar must have a URL Endpoint"
	ErrorNoMoreEvents = "skArtistCalendar has retrieved all events"
)

func (ac skArtistCalendar) next() (*skArtistCalendar, error) {
	if len(ac.Endpoint) < 1 {
		return nil, errors.New(ErrorNoEndpoint)
	}
	if ac.count() == ac.TotalEntries && ac.Page > 0 { // not really an error...
		return nil, errors.New(ErrorNoMoreEvents)
	}
	ac.Page++ // next page

	url := ac.Endpoint
	if ac.PerPage > 0 {
		url += fmt.Sprintf("&page=%d&per_page=%d", ac.Page, ac.PerPage)
	}
	// just so we can have a nice timer
	resp, err := func() (*http.Response, error) {
		t := time.Now()
		defer func() {
			log.Printf("\x1b[1;35mGet:\x1b[0m \x1b[34m%12d\x1b[0mµs \x1b[33m%s\x1b[0m", time.Since(t)/1000, url)
		}()
		r, e := http.Get(url)
		if e != nil {
			return nil, e
		} else if r.StatusCode != http.StatusOK {
			return nil, errors.New("API Error:" + r.Status)
		}
		return r, nil
	}()
	if err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, err
	}

	if err := json.Unmarshal(buf.Bytes(), &ac); err != nil {
		return nil, err
	}
	return &ac, nil
}

// utility type for async access
type skResponse struct {
	Cal *skArtistCalendar
	Err error
}
