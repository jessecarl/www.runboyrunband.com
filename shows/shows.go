// Copyright 2013 Jesse Allen. All rights reserved
// Released under the MIT license found in the LICENSE file.

package shows

import (
	"errors"
	"strconv"
	"time"
)

const (
	apiBaseURL = "http://api.songkick.com/api/3.0/artists/"
)

// Represents the SongKick Artist Calendar.
//
// Uses both the calendar and gigography endpoints.
type Calendar struct {
	artistID int
	apiKey   string
}

// Create a new Calendar given the SongKick ArtistID and an API Key
func New(artistID int, apiKey string) *Calendar {
	c := new(Calendar)
	c.Init(artistID, apiKey)
	return c
}

// Sets up a Calendar for a given artist
func (c *Calendar) Init(artistID int, apiKey string) {
	c.artistID = artistID
	c.apiKey = apiKey
}

type getResponse struct {
	Events []Event
	Err    error
}

// Returns both Upcoming and Past Event slices with the given limit
func (c *Calendar) All(limit int) ([]Event, []Event, error) {
	var pevent, fevent []Event
	past, future := make(chan getResponse), make(chan getResponse)
	getem := func(fn func(int) ([]Event, error), ch chan getResponse) {
		gr := getResponse{}
		gr.Events, gr.Err = fn(limit)
		ch <- gr
	}

	go getem(c.Past, past)
	go getem(c.Upcoming, future)

	for {
		if pevent != nil && fevent != nil {
			return fevent, pevent, nil
		}
		select {
		case res := <-past:
			if res.Err != nil {
				return nil, nil, res.Err
			}
			pevent = res.Events
		case res := <-future:
			if res.Err != nil {
				return nil, nil, res.Err
			}
			fevent = res.Events
		case <-time.After(30 * time.Second):
			return nil, nil, errors.New("API Request Timeout")
		}
	}

	return fevent, pevent, nil
}

// Returns a slice of Events from the SongKick calendar endpoint
func (c *Calendar) Upcoming(limit int) ([]Event, error) {
	return c.get(apiBaseURL+strconv.FormatInt(int64(c.artistID), 10)+"/calendar.json?order=asc&apikey="+c.apiKey, limit)
}

// Returns a slice of Events from the SongKick gigography endpoint
func (c *Calendar) Past(limit int) ([]Event, error) {
	return c.get(apiBaseURL+strconv.FormatInt(int64(c.artistID), 10)+"/gigography.json?order=desc&apikey="+c.apiKey, limit)
}

func (c *Calendar) get(url string, limit int) ([]Event, error) {
	const (
		maxPageSize = 50
		chunkSize   = 5 // number of events to process at once
	)

	var (
		err  error
		next chan skArtistCalendar
	)
	// we want the arrays under these slices to be allocated once and only once if we can,
	// so we set the capacity to the limit
	events := make([]Event, 0, limit)
	queue := make([]skEvent, 0, limit)

	stop := make(chan chan bool) // channel we can use to tell our worker to stop
	stopped := make(chan bool)
	receive := make(chan skResponse)
	fetch := make(chan skArtistCalendar)

	ac := new(skArtistCalendar)
	ac.Endpoint = url

	// Worker
	go func(in chan skArtistCalendar, out chan skResponse, stop chan chan bool) {
		var err error
		for {
			tmp := new(skArtistCalendar)
			select {
			case *tmp = <-in:
				tmp, err = tmp.next()
				out <- skResponse{tmp, err}
			case stopped := <-stop:
				stopped <- true
				return
			}
		}
	}(fetch, receive, stop)

	// initial fetch
	fetch <- *ac

	// local loop
	for {
		var (
			failure    chan chan bool
			success    chan chan bool
			queueReady chan bool
		)

		if err != nil {
			// exit with error
			failure = stop
		} else if ac.Page > 0 && limit == len(events) {
			// exit with success
			success = stop
		} else if len(queue) > 0 {
			queueReady = make(chan bool)
			go func() { queueReady <- true }()
		}

		select {
		case failure <- stopped: // failure
			<-stopped
			return nil, err
		case success <- stopped: // success
			<-stopped
			return events, nil
		case next <- *ac: // start fetch
			next = nil // block additional fetches
		case res := <-receive: // end fetch
			err = res.Err
			if err != nil {
				break
			}
			ac = res.Cal
			if len(ac.Events) > 0 {
				queue = append(queue, ac.Events...)
			}
			if limit == 0 || limit > ac.TotalEntries {
				limit = ac.TotalEntries
			}
			if len(queue)+len(events) < limit {
				next = fetch
			}
		case <-queueReady:
			// try to drain the queue one chunk at a time
			s := chunkSize
			if s > limit-len(events) {
				s = limit - len(events)
			}
			if s > len(queue) {
				s = len(queue)
			}
			mini := queue[:s]
			queue = queue[s:]
			for _, e := range mini {
				events = append(events, e.Event())
			}
		case <-time.After(30 * time.Second):
			err = errors.New("API Timeout")
		}

	}
	panic("get() did not exit correctly")
}
