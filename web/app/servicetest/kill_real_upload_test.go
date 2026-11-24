//go:build crashtest

package servicetest

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"
	"time"

	corepub "github.com/AlexGalhardo/stuarty/core/publish"
)

// uploadSize is the size of the file the upload cases send: big enough that a fraction of it is many reads of
// the network, small enough to stay out of the way.
const uploadSize = 1 << 20

// A real process is killed when the network holds a fraction of the bytes of a resumable upload session. The
// next process finds the session address in publish/uploads/<key>.json, asks the network how much it holds and
// sends the rest from there: one session, one finished video, the whole file.
func TestKillRealUploadResumesAtTheReceivedOffset(t *testing.T) {
	for _, c := range []struct {
		name     string
		num, den int64
	}{{"a quarter", 1, 4}, {"a half", 1, 2}, {"nine tenths", 9, 10}} {
		t.Run(c.name, func(t *testing.T) {
			data := make([]byte, uploadSize)
			if _, err := rand.Read(data); err != nil {
				t.Fatal(err)
			}

			sum := sha256.Sum256(data)
			want := hex.EncodeToString(sum[:])

			w := newKillWorldFor(t, "youtube", data)
			cut := int64(uploadSize) * c.num / c.den
			reached := w.yt.StallAt(cut)

			offset := w.dueOffset()
			child := w.child(t, offset)

			waitForClosed(t, reached, "the network to hold the bytes of the cut", child.output)

			sessions := func() []string {
				files, _ := filepath.Glob(filepath.Join(w.dir, "publish", "uploads", "*.json"))

				return files
			}

			if got := sessions(); len(got) != 1 {
				t.Fatalf("session files at the kill = %v, want exactly one", got)
			}

			child.kill(t)

			if got, n := w.yt.Received(0), w.yt.Sessions(); got != cut || n != 1 || w.yt.Finished() != 0 {
				t.Fatalf("at the kill: %d bytes held, %d sessions, %d finished; want %d bytes, 1 session, 0 finished",
					got, n, w.yt.Finished(), cut)
			}

			// The lease is alive: nothing starts, nothing is asked of the network.
			same := w.open(t, offset)

			if n, err := same.Sched.Tick(context.Background()); err != nil || n != 0 {
				t.Fatalf("a pass inside the lease started %d sends, %v", n, err)
			}

			same.Sched.Wait()

			if w.yt.Received(0) != cut || w.yt.Sessions() != 1 {
				t.Fatalf("a pass inside the lease touched the upload: %d bytes, %d sessions", w.yt.Received(0), w.yt.Sessions())
			}

			// The lease ends: the same upload key is sent again and the session continues.
			after := w.open(t, offset+corepubLease+2*time.Second)

			if n, err := after.Sched.Tick(context.Background()); err != nil || n != 1 {
				t.Fatalf("a pass after the lease started %d sends, %v", n, err)
			}

			after.Sched.Wait()

			got := w.target(t, after)
			if got.State != corepub.TargetPublished || got.RemoteID == "" || got.Attempts != 2 {
				t.Fatalf("the target after the resume = %s %+v", got.State, got)
			}

			if w.yt.Sessions() != 1 || w.yt.Finished() != 1 {
				t.Errorf("sessions = %d, finished = %d, want 1 and 1: the resume started another upload",
					w.yt.Sessions(), w.yt.Finished())
			}

			if w.yt.Received(0) != uploadSize || w.yt.Digest(0) != want {
				t.Errorf("the network holds %d bytes with digest %s, want %d with %s",
					w.yt.Received(0), w.yt.Digest(0), uploadSize, want)
			}

			puts := w.yt.Puts()
			if len(puts) != 2 || puts[0].Start != 0 || puts[0].Bytes != cut ||
				puts[1].Start != cut || puts[1].Bytes != uploadSize-cut {
				t.Errorf("the PUTs of file bytes = %+v, want one that began at 0 and was cut at %d, one that resumed at %d",
					puts, cut, cut)
			}

			if left := sessions(); len(left) != 0 {
				t.Errorf("the session file stayed after the video was finished: %v", left)
			}
		})
	}
}
