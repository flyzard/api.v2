package domain

import (
	"time"

	// tzdata is bundled so Europe/Lisbon resolves even on minimal container
	// images that strip /usr/share/zoneinfo. The deploy must fail loudly if
	// the location can't be loaded rather than silently formatting in UTC.
	_ "time/tzdata"
)

// lisbonLocation is the canonical clock for AT certification. Every signing
// and storage path normalizes through it so DST transitions, caller-side UTC
// conventions, and host-clock skew don't shift the calendar day used in the
// SAF-T export or the hash chain.
var lisbonLocation *time.Location

func init() {
	loc, err := time.LoadLocation("Europe/Lisbon")
	if err != nil {
		panic("domain: cannot load Europe/Lisbon timezone (tzdata missing?): " + err.Error())
	}
	lisbonLocation = loc
}
