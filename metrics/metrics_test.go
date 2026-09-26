package metrics_test

import (
	"testing"
	"time"

	"github.com/apppackio/apppack/app"
	"github.com/apppackio/apppack/metrics"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/mum4k/termdash/cell"
)

// testApp builds an App with just the settings the metric queries read, so
// none of this needs AWS.
func testApp() *app.App {
	a := &app.App{Name: "myapp", Settings: &app.Settings{}}
	a.Settings.Cluster.Name = "cluster-1"
	a.Settings.TargetGroup.Suffix = "targetgroup/tg/abc123"
	a.Settings.LoadBalancer.Suffix = "app/lb/def456"

	return a
}

func testOptions() *metrics.MetricOptions {
	return &metrics.MetricOptions{
		Timeframe: metrics.TimeFrame{Name: "1 day", ShortcutKey: "d", Duration: 24 * time.Hour},
	}
}

// TestServiceUtilizationTitle -- cmd/dash.go passes Title() straight to
// termdash's container.BorderTitle, which assigns the string verbatim. There
// is no format verb to escape.
func TestServiceUtilizationTitle(t *testing.T) {
	t.Parallel()

	m := &metrics.ServiceUtilizationMetrics{Service: "web"}

	if got, want := m.Title(), "web utilization (%)"; got != want {
		t.Errorf("Title() = %q, want %q", got, want)
	}
}

func TestTimeFramePeriod(t *testing.T) {
	t.Parallel()

	// Boundaries matter: CloudWatch bills per datapoint, and too fine a
	// period over a long window blows past MaxDatapoints.
	tests := []struct {
		name     string
		duration time.Duration
		want     int32
	}{
		{"one hour", time.Hour, 60},
		{"exactly one day", 24 * time.Hour, 60},
		{"just over one day", 24*time.Hour + time.Minute, 4 * 60},
		{"exactly three days", 72 * time.Hour, 4 * 60},
		{"just over three days", 72*time.Hour + time.Minute, 10 * 60},
		{"one week", 7 * 24 * time.Hour, 10 * 60},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tf := &metrics.TimeFrame{Duration: tt.duration}
			if got := tf.Period(); got != tt.want {
				t.Errorf("Period() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestTimeFrameCloneIsIndependent(t *testing.T) {
	t.Parallel()

	original := &metrics.TimeFrame{Name: "1 day", ShortcutKey: "d", Duration: 24 * time.Hour}
	clone := original.Clone()

	if *clone != *original {
		t.Fatalf("Clone() = %+v, want %+v", *clone, *original)
	}

	clone.Name = "changed"
	clone.Duration = time.Hour

	if original.Name != "1 day" || original.Duration != 24*time.Hour {
		t.Errorf("writing to the clone changed the original: %+v", *original)
	}
}

func TestTimeframeStart(t *testing.T) {
	t.Parallel()

	o := &metrics.MetricOptions{Timeframe: metrics.TimeFrame{Duration: 2 * time.Hour}}

	got := o.TimeframeStart()
	want := time.Now().Add(-2 * time.Hour)

	if diff := got.Sub(want); diff > time.Second || diff < -time.Second {
		t.Errorf("TimeframeStart() = %v, want within 1s of %v", got, want)
	}
}

func TestTitlesAndShortNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		metric        metrics.AppMetrics
		wantTitle     string
		wantShortName string
		wantService   string
	}{
		{
			name:          "service utilization",
			metric:        &metrics.ServiceUtilizationMetrics{Service: "worker"},
			wantTitle:     "worker utilization (%)",
			wantShortName: "worker util",
			wantService:   "worker",
		},
		{
			name:          "response time, average",
			metric:        &metrics.ResponseTimeMetrics{Stat: "Average"},
			wantTitle:     "response time (seconds)",
			wantShortName: "resp time (avg)",
			wantService:   "web",
		},
		{
			name:          "response time, percentile",
			metric:        &metrics.ResponseTimeMetrics{Stat: "p95"},
			wantTitle:     "response time (seconds)",
			wantShortName: "resp time (p95)",
			wantService:   "web",
		},
		{
			name:          "status code",
			metric:        &metrics.StatusCodeMetrics{Code: "5xx"},
			wantTitle:     "5xx responses (count)",
			wantShortName: "5xx responses",
			wantService:   "web",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.metric.Title(); got != tt.wantTitle {
				t.Errorf("Title() = %q, want %q", got, tt.wantTitle)
			}

			if got := tt.metric.ShortName(); got != tt.wantShortName {
				t.Errorf("ShortName() = %q, want %q", got, tt.wantShortName)
			}

			if got := tt.metric.GetService(); got != tt.wantService {
				t.Errorf("GetService() = %q, want %q", got, tt.wantService)
			}
		})
	}
}

func TestGetAppAndGetOptions(t *testing.T) {
	t.Parallel()

	a, o := testApp(), testOptions()

	for name, m := range map[string]metrics.AppMetrics{
		"utilization":   &metrics.ServiceUtilizationMetrics{App: a, Options: o, Service: "web"},
		"response time": &metrics.ResponseTimeMetrics{App: a, Options: o, Stat: "Average"},
		"status code":   &metrics.StatusCodeMetrics{App: a, Options: o, Code: "2xx"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if m.GetApp() != a {
				t.Error("GetApp() did not return the app it was built with")
			}

			if m.GetOptions() != o {
				t.Error("GetOptions() did not return the options it was built with")
			}
		})
	}
}

func TestMetricColor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		metric metrics.AppMetrics
		id     string
		want   cell.Color
	}{
		{"utilization cpu", &metrics.ServiceUtilizationMetrics{}, "cpu", cell.ColorGreen},
		{"utilization memory", &metrics.ServiceUtilizationMetrics{}, "memory", cell.ColorBlue},
		{"utilization unknown", &metrics.ServiceUtilizationMetrics{}, "disk", cell.ColorGray},
		{"response time", &metrics.ResponseTimeMetrics{}, "anything", cell.ColorBlue},
		{"status 2xx", &metrics.StatusCodeMetrics{}, "2xx", cell.ColorGreen},
		{"status 3xx", &metrics.StatusCodeMetrics{}, "3xx", cell.ColorBlue},
		{"status 4xx", &metrics.StatusCodeMetrics{}, "4xx", cell.ColorYellow},
		{"status 5xx", &metrics.StatusCodeMetrics{}, "5xx", cell.ColorRed},
		{"status unknown", &metrics.StatusCodeMetrics{}, "1xx", cell.ColorGray},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.metric.MetricColor(&tt.id); got != tt.want {
				t.Errorf("MetricColor(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestLineChartOptions(t *testing.T) {
	t.Parallel()

	// Utilization is a percentage, so its axis is pinned to 0-100. The others
	// are unbounded and take the defaults.
	if got := len((&metrics.ServiceUtilizationMetrics{}).LineChartOptions()); got != 2 {
		t.Errorf("utilization LineChartOptions() has %d options, want 2", got)
	}

	if got := len((&metrics.ResponseTimeMetrics{}).LineChartOptions()); got != 0 {
		t.Errorf("response time LineChartOptions() has %d options, want 0", got)
	}

	if got := len((&metrics.StatusCodeMetrics{}).LineChartOptions()); got != 0 {
		t.Errorf("status code LineChartOptions() has %d options, want 0", got)
	}
}

func TestServiceUtilizationMetricDataQueries(t *testing.T) {
	t.Parallel()

	m := &metrics.ServiceUtilizationMetrics{App: testApp(), Options: testOptions(), Service: "worker"}

	queries := m.MetricDataQueries()
	if len(queries) != 2 {
		t.Fatalf("got %d queries, want 2 (cpu and memory)", len(queries))
	}

	for i, want := range []struct{ id, metricName string }{
		{"cpu", "CPUUtilization"},
		{"memory", "MemoryUtilization"},
	} {
		q := queries[i]

		if *q.Id != want.id {
			t.Errorf("query %d Id = %q, want %q", i, *q.Id, want.id)
		}

		if *q.MetricStat.Metric.MetricName != want.metricName {
			t.Errorf("query %d MetricName = %q, want %q", i, *q.MetricStat.Metric.MetricName, want.metricName)
		}

		if *q.MetricStat.Metric.Namespace != "AWS/ECS" {
			t.Errorf("query %d Namespace = %q, want AWS/ECS", i, *q.MetricStat.Metric.Namespace)
		}

		if *q.MetricStat.Stat != "Maximum" {
			t.Errorf("query %d Stat = %q, want Maximum", i, *q.MetricStat.Stat)
		}

		if *q.MetricStat.Period != 60 {
			t.Errorf("query %d Period = %d, want 60", i, *q.MetricStat.Period)
		}

		dims := dimensionMap(q.MetricStat.Metric.Dimensions)
		if dims["ClusterName"] != "cluster-1" {
			t.Errorf("query %d ClusterName = %q, want cluster-1", i, dims["ClusterName"])
		}

		// The dimension is the ECS service name, not the process name.
		if dims["ServiceName"] != "myapp-worker" {
			t.Errorf("query %d ServiceName = %q, want myapp-worker", i, dims["ServiceName"])
		}
	}
}

func TestResponseTimeMetricDataQueries(t *testing.T) {
	t.Parallel()

	m := &metrics.ResponseTimeMetrics{App: testApp(), Options: testOptions(), Stat: "p95"}

	queries := m.MetricDataQueries()
	if len(queries) != 1 {
		t.Fatalf("got %d queries, want 1", len(queries))
	}

	q := queries[0]

	// CloudWatch query ids must start with a lowercase letter.
	if *q.Id != "p95" {
		t.Errorf("Id = %q, want p95", *q.Id)
	}

	if *q.MetricStat.Metric.MetricName != "TargetResponseTime" {
		t.Errorf("MetricName = %q, want TargetResponseTime", *q.MetricStat.Metric.MetricName)
	}

	if *q.MetricStat.Metric.Namespace != "AWS/ApplicationELB" {
		t.Errorf("Namespace = %q, want AWS/ApplicationELB", *q.MetricStat.Metric.Namespace)
	}

	if *q.MetricStat.Stat != "p95" {
		t.Errorf("Stat = %q, want p95", *q.MetricStat.Stat)
	}

	dims := dimensionMap(q.MetricStat.Metric.Dimensions)
	if dims["TargetGroup"] != "targetgroup/tg/abc123" {
		t.Errorf("TargetGroup = %q", dims["TargetGroup"])
	}

	if dims["LoadBalancer"] != "app/lb/def456" {
		t.Errorf("LoadBalancer = %q", dims["LoadBalancer"])
	}
}

func TestStatusCodeMetricDataQueries(t *testing.T) {
	t.Parallel()

	m := &metrics.StatusCodeMetrics{App: testApp(), Options: testOptions(), Code: "5xx"}

	queries := m.MetricDataQueries()
	if len(queries) != 1 {
		t.Fatalf("got %d queries, want 1", len(queries))
	}

	q := queries[0]

	if *q.Id != "mm5xx" {
		t.Errorf("Id = %q, want mm5xx", *q.Id)
	}

	if *q.MetricStat.Metric.MetricName != "HTTPCode_Target_5XX_Count" {
		t.Errorf("MetricName = %q, want HTTPCode_Target_5XX_Count", *q.MetricStat.Metric.MetricName)
	}

	if *q.MetricStat.Stat != "Sum" {
		t.Errorf("Stat = %q, want Sum", *q.MetricStat.Stat)
	}
}

func dimensionMap(dims []types.Dimension) map[string]string {
	m := make(map[string]string, len(dims))
	for _, d := range dims {
		m[*d.Name] = *d.Value
	}

	return m
}
