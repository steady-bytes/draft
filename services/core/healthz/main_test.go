package main

import "testing"

func TestHealth(t *testing.T) {
	status := "healthy"

	healthz := &Healthz{
		status: status,
	}

	res := healthz.Health()

	if res != status {
		t.Errorf("status is incorrect")
	}
}
