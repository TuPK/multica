package main

import "testing"

func TestHubMaxUploadBytesFromEnv(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  int64
	}{
		{name: "unset keeps the default", value: "", want: 0},
		{name: "blank keeps the default", value: "   ", want: 0},
		{name: "positive byte count", value: "33554432", want: 33554432},
		{name: "zero is not a cap", value: "0", want: 0},
		{name: "negative is not a cap", value: "-5", want: 0},
		{name: "not a number keeps the default", value: "16MiB", want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HUB_MAX_UPLOAD_BYTES", tc.value)
			if got := hubMaxUploadBytesFromEnv(); got != tc.want {
				t.Fatalf("hubMaxUploadBytesFromEnv() = %d, want %d", got, tc.want)
			}
		})
	}
}
