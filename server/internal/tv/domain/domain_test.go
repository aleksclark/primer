package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/aleksclark/primer/server/internal/tv/domain"
)

func TestAvailabilityWindowActiveAt(t *testing.T) {
	t.Parallel()
	start := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	window := domain.AvailabilityWindow{StartsAt: start, EndsAt: end}

	assert.False(t, window.ActiveAt(start.Add(-time.Second)), "before the window")
	assert.True(t, window.ActiveAt(start), "start is inclusive")
	assert.True(t, window.ActiveAt(start.Add(time.Hour)), "inside the window")
	assert.False(t, window.ActiveAt(end), "end is exclusive")
	assert.False(t, window.ActiveAt(end.Add(time.Second)), "after the window")
}

func TestPlayGrantRedeemable(t *testing.T) {
	t.Parallel()
	now := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	consumed := now.Add(-time.Minute)

	assert.True(t, domain.PlayGrant{ExpiresAt: now.Add(time.Minute)}.Redeemable(now))
	assert.False(t, domain.PlayGrant{ExpiresAt: now}.Redeemable(now), "expiry is exclusive")
	assert.False(t, domain.PlayGrant{ExpiresAt: now.Add(-time.Minute)}.Redeemable(now), "expired")
	assert.False(t,
		domain.PlayGrant{ExpiresAt: now.Add(time.Minute), ConsumedAt: &consumed}.Redeemable(now),
		"already consumed")
}

func TestMediaItemConsumesPlay(t *testing.T) {
	t.Parallel()
	assert.True(t, domain.MediaItem{Class: domain.ClassEntertainment}.ConsumesPlay())
	assert.False(t, domain.MediaItem{Class: domain.ClassEducational}.ConsumesPlay())
	assert.False(t, domain.MediaItem{Class: domain.ClassMixed}.ConsumesPlay())
}

func TestDevicePaired(t *testing.T) {
	t.Parallel()
	revoked := time.Now().UTC()
	assert.True(t, domain.Device{TokenHash: "abc"}.Paired())
	assert.False(t, domain.Device{}.Paired(), "no token yet")
	assert.False(t, domain.Device{TokenHash: "abc", RevokedAt: &revoked}.Paired(), "revoked")
}

func TestSessionCompletesPlay(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		completed   bool
		maxPosition int
		runtime     int
		want        bool
	}{
		{"explicit completion wins", true, 0, 3600, true},
		{"exactly at the threshold", false, 2880, 3600, true},
		{"just past the threshold", false, 3000, 3600, true},
		{"just under the threshold", false, 2879, 3600, false},
		{"barely started", false, 10, 3600, false},
		{"unknown runtime needs explicit completion", false, 9999, 0, false},
		{"unknown runtime with explicit completion", true, 0, 0, true},
		{"negative runtime is treated as unknown", false, 100, -1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := domain.SessionCompletesPlay(tc.completed, tc.maxPosition, tc.runtime)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestResumePositionSeconds(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0, domain.ResumePositionSeconds(0))
	assert.Equal(t, 0, domain.ResumePositionSeconds(20))
	assert.Equal(t, 0, domain.ResumePositionSeconds(30))
	assert.Equal(t, 870, domain.ResumePositionSeconds(900))
}

func TestSeekFloorSeconds(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0, domain.SeekFloorSeconds(0))
	assert.Equal(t, 0, domain.SeekFloorSeconds(120))
	assert.Equal(t, 0, domain.SeekFloorSeconds(300))
	assert.Equal(t, 600, domain.SeekFloorSeconds(900))
}
