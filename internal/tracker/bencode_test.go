package tracker

import "testing"

func TestParseTrackerResponse(t *testing.T) {
	resp, err := ParseResponse([]byte("d8:intervali1800e8:completei3e10:incompletei2ee"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Interval != 1800 || resp.Seeders != 2 || resp.Leechers != 2 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestParseTrackerFailure(t *testing.T) {
	resp, err := ParseResponse([]byte("d14:failure reason14:not registerede"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Failure != "not registered" {
		t.Fatalf("failure = %q", resp.Failure)
	}
}
