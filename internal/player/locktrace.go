package player

import (
	"time"

	"git.hemmalab.se/scuttle/tuiplay/internal/logging"
	"github.com/gopxl/beep/v2/speaker"
)

// lockSpeaker takes the speaker lock. In trace mode it logs the wait and a
// slow acquisition, so a lock-order hang shows the last holder and waiter.
func lockSpeaker(where string) {
	if !logging.Enabled() {
		speaker.Lock()
		return
	}
	logging.Trace("speaker lock wait", "at", where)
	start := time.Now()
	speaker.Lock()
	if d := time.Since(start); d > 100*time.Millisecond {
		logging.Warn("speaker lock slow", "at", where, "waited", d)
	} else {
		logging.Trace("speaker lock held", "at", where)
	}
}

// unlockSpeaker releases the speaker lock.
func unlockSpeaker(where string) {
	speaker.Unlock()
	logging.Trace("speaker unlock", "at", where)
}
