package testpublish

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service/social/youtube"
)

func TestScheduledYouTubeFixtureOpensAgainAndSendsTheExport(t *testing.T) {
	network := NewFakeYouTube(t)
	opts := Options{Dir: t.TempDir(), Connector: youtube.New(network.Config()), VaultKey: make([]byte, 32)}
	env, err := Open(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = env.Close() })
	project, err := env.NewProject("Scheduled video")
	if err != nil {
		t.Fatal(err)
	}
	connection, err := env.SignIn("youtube")
	if err != nil {
		t.Fatal(err)
	}
	media := []byte("fixture video")
	exportID := SeedExport(t, env.Store, opts.Dir, UI, project, "job_0000009001", ".mp4", media, time.Now())
	scheduled, err := env.ScheduleAndApprove(project, exportID, connection, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	before, err := env.Target(scheduled)
	if err != nil || before.State != publish.TargetScheduled || network.Sessions() != 0 {
		t.Fatalf("before the scheduled time: %+v, %v", before, err)
	}

	opts.Offset = time.Until(scheduled.At) + time.Second
	reopened, err := Open(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if n, err := reopened.Sched.Tick(t.Context()); err != nil || n != 1 {
		t.Fatalf("Tick = %d, %v", n, err)
	}
	reopened.Sched.Wait()
	after, err := reopened.Target(scheduled)
	if err != nil || after.State != publish.TargetPublished {
		t.Fatalf("after the scheduled time: %+v, %v", after, err)
	}
	sum := sha256.Sum256(media)
	if network.Sessions() != 1 || network.Finished() != 1 || network.Received(0) != int64(len(media)) ||
		network.Digest(0) != hex.EncodeToString(sum[:]) || len(network.Puts()) != 1 {
		t.Fatal("the fixture did not record one complete upload with the original bytes")
	}
}
