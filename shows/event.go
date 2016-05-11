// Copyright 2013 Jesse Allen. All rights reserved
// Released under the MIT license found in the LICENSE file.

package shows

import (
	"time"
)

type EventType int

// Used to distinguish festivals and other concerts
const (
	Festival EventType = iota
	Concert
	ListeningRoom
	Club
)

// Events roughly match the schema.org event type
// TODO: Update URL to a map instead of a slice
type Event struct {
	Description string
	Image       string
	Name        string
	SameAs      string
	URL         []string
	Duration    time.Duration
	EndDate     time.Time
	StartDate   time.Time
	Location    Place
	Performer   []MusicGroup
	Type        EventType
}

func (e Event) Event() Event {
	return e
}

// Utility mainly useful for templates. Returns true if Event is a Festival
func (e Event) Festival() bool {
	return e.Type == Festival
}

// Utility mainly useful for templates. Returns true if Event is a Concert
func (e Event) Concert() bool {
	return e.Type == Concert
}

// Indication that the artist is the only performer
func (e Event) Solo() bool {
	return len(e.Performer) == 1
}

// Locations roughly match http://schema.org/Place
type Place struct {
	Description string
	Image       string
	Name        string
	SameAs      string
	URL         []string
	Address     Address
	Geo         GeoCoordinates
	Telephone   string
}

// Roughly matches http://schema.org/PostalAddress
type Address struct {
	AddressCountry      string
	AddressLocality     string
	AddressRegion       string
	PostalCode          string
	PostOfficeBoxNumber string
	StreetAddress       string
}

// Roughly matches http://schema.org/GeoCoordinates
type GeoCoordinates struct {
	Lat, Lng  float32
	Elevation int
}

// roughly matches http://schema.org/MusicGroup
type MusicGroup struct {
	Description string
	Image       string
	Name        string
	SameAs      string
	URL         []string
}

type Eventer interface {
	Event() Event
}
