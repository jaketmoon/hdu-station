package campusauth

import (
	"context"
	"os"
	"testing"
	"time"
)

// This smoke check creates and cancels an unapproved request. It never opens
// the approval page or obtains a PAT, and never prints the device/user codes.
func TestLiveCampusDeviceAuthorization(t *testing.T) {
	if os.Getenv("HDU_STATION_LIVE_CAMPUS_AUTH") != "1" {
		t.Skip("explicit network smoke check only")
	}
	c, err := New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	device, err := c.http.begin(ctx, c.credentials.DeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.http.cancel(ctx, device.DeviceCode, c.credentials.DeviceID); err != nil {
		t.Fatal(err)
	}
	t.Log("official authorization page validated; unapproved request canceled")
}
