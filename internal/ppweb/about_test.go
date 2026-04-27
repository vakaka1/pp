package ppweb

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDescribeReleaseStatusPatchUpdate(t *testing.T) {
	info := describeReleaseStatus(
		"v1.0.22",
		&gitHubRelease{TagName: "v1.0.23"},
		time.Time{},
		"",
	)

	if !info.UpdateAvailable {
		t.Fatalf("expected update to be available")
	}
	if info.Severity != "patch" {
		t.Fatalf("expected patch severity, got %q", info.Severity)
	}
	if info.IndicatorTone != "warning" {
		t.Fatalf("expected warning tone, got %q", info.IndicatorTone)
	}
}

func TestDescribeReleaseStatusMajorUpdate(t *testing.T) {
	info := describeReleaseStatus(
		"v1.0.23",
		&gitHubRelease{TagName: "v1.1.0"},
		time.Time{},
		"",
	)

	if !info.UpdateAvailable {
		t.Fatalf("expected update to be available")
	}
	if info.Severity != "major" {
		t.Fatalf("expected major severity, got %q", info.Severity)
	}
	if info.IndicatorTone != "danger" {
		t.Fatalf("expected danger tone, got %q", info.IndicatorTone)
	}
}

func TestDescribeReleaseStatusDevBuildDoesNotOfferUpdate(t *testing.T) {
	info := describeReleaseStatus(
		"dev",
		&gitHubRelease{TagName: "v1.0.23"},
		time.Time{},
		"",
	)

	if info.UpdateAvailable {
		t.Fatalf("expected dev build to avoid automatic update")
	}
	if info.IndicatorTone != "neutral" {
		t.Fatalf("expected neutral tone, got %q", info.IndicatorTone)
	}
	if !strings.Contains(info.StatusLabel, "локальная сборка") {
		t.Fatalf("expected local build status label, got %q", info.StatusLabel)
	}
}

func TestDescribeReleaseStatusUnknownBuildDoesNotOfferUpdate(t *testing.T) {
	info := describeReleaseStatus(
		"none",
		&gitHubRelease{TagName: "v1.0.23"},
		time.Time{},
		"",
	)

	if info.UpdateAvailable {
		t.Fatalf("expected non-release build to avoid automatic update")
	}
	if info.Severity != "none" {
		t.Fatalf("expected none severity, got %q", info.Severity)
	}
}

func TestSanitizeArchivePath(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{name: "strip root directory", input: "frontend/index.html", expect: "index.html"},
		{name: "drop traversal", input: "frontend/../../etc/passwd", expect: "etc/passwd"},
		{name: "ignore empty", input: "frontend/", expect: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if actual := sanitizeArchivePath(test.input, 1); actual != test.expect {
				t.Fatalf("sanitizeArchivePath(%q) = %q, want %q", test.input, actual, test.expect)
			}
		})
	}
}

func TestUpdateRunStatusOmitsEmptyTimes(t *testing.T) {
	payload, err := json.Marshal(updateRunStatus{
		State:   "idle",
		Message: "Обновление не запускалось.",
	})
	if err != nil {
		t.Fatalf("marshal update status: %v", err)
	}

	body := string(payload)
	if strings.Contains(body, "startedAt") || strings.Contains(body, "finishedAt") || strings.Contains(body, "0001-01-01") {
		t.Fatalf("expected empty timestamps to be omitted, got %s", body)
	}
}
