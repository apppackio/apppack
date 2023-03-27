package metrics

import (
	"fmt"
	"strings"
	"time"

	"github.com/apppackio/apppack/app"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/widgets/linechart"
)

type TimeFrame struct {
	Name        string
	ShortcutKey string
	Duration    time.Duration
}

func (t *TimeFrame) Clone() *TimeFrame {
	return &TimeFrame{
		Name:        t.Name,
		ShortcutKey: t.ShortcutKey,
		Duration:    t.Duration,
	}
}

// Period returns the resolution in seconds to graph for the time frame
func (t *TimeFrame) Period() int64 {
	switch {
	case t.Duration.Hours() <= 24:
		return 60
	case t.Duration.Hours() <= 24*3:
		return 4 * 60
	default:
		return 10 * 60
	}
}

type MetricOptions struct {
	Timeframe TimeFrame
	UTC       bool
}

func (o *MetricOptions) TimeframeStart() time.Time {
	return time.Now().Add(-1 * o.Timeframe.Duration)
}

type AppMetrics interface {
	GetApp() *app.App
	GetOptions() *MetricOptions
	MetricDataQueries() []*cloudwatch.MetricDataQuery
	MetricColor(name *string) cell.Color
	LineChartOptions() []linechart.Option
	Title() string
	ShortName() string
	GetService() string
}

type ServiceUtilizationMetrics struct {
	App     *app.App
	Options *MetricOptions
	Service string
}

func (m *ServiceUtilizationMetrics) GetApp() *app.App { return m.App }

func (m *ServiceUtilizationMetrics) GetOptions() *MetricOptions { return m.Options }

func (m *ServiceUtilizationMetrics) GetService() string { return m.Service }

func (m *ServiceUtilizationMetrics) Title() string {
	return fmt.Sprintf("%s utilization (%%)", m.Service)
}

func (m *ServiceUtilizationMetrics) ShortName() string {
	return fmt.Sprintf("%s util", m.Service)
}

func (*ServiceUtilizationMetrics) MetricColor(name *string) cell.Color {
	switch *name {
	case "cpu":
		return cell.ColorGreen
	case "memory":
		return cell.ColorBlue
	default:
		return cell.ColorGray
	}
}

func (*ServiceUtilizationMetrics) LineChartOptions() []linechart.Option {
	return []linechart.Option{
		linechart.YAxisCustomScale(0, 100),
		linechart.YAxisFormattedValues(linechart.ValueFormatterRound),
	}
}

func (m *ServiceUtilizationMetrics) MetricDataQueries() []*cloudwatch.MetricDataQuery {
	return []*cloudwatch.MetricDataQuery{
		{
			Id: aws.String("cpu"),
			MetricStat: &cloudwatch.MetricStat{
				Metric: &cloudwatch.Metric{
					Namespace:  aws.String("AWS/ECS"),
					MetricName: aws.String("CPUUtilization"),
					Dimensions: []*cloudwatch.Dimension{
						{
							Name:  aws.String("ClusterName"),
							Value: aws.String(m.App.Settings.Cluster.Name),
						},
						{
							Name:  aws.String("ServiceName"),
							Value: aws.String(m.GetApp().ServiceName(m.Service)),
						},
					},
				},
				Period: aws.Int64(m.Options.Timeframe.Period()),
				Stat:   aws.String("Maximum"),
			},
		},
		{
			Id: aws.String("memory"),
			MetricStat: &cloudwatch.MetricStat{
				Metric: &cloudwatch.Metric{
					Namespace:  aws.String("AWS/ECS"),
					MetricName: aws.String("MemoryUtilization"),
					Dimensions: []*cloudwatch.Dimension{
						{
							Name:  aws.String("ClusterName"),
							Value: aws.String(m.App.Settings.Cluster.Name),
						},
						{
							Name:  aws.String("ServiceName"),
							Value: aws.String(m.GetApp().ServiceName(m.Service)),
						},
					},
				},
				Period: aws.Int64(m.Options.Timeframe.Period()),
				Stat:   aws.String("Maximum"),
			},
		},
	}
}

type ResponseTimeMetrics struct {
	App     *app.App
	Options *MetricOptions
	Stat    string
}

func (m *ResponseTimeMetrics) GetApp() *app.App { return m.App }

func (m *ResponseTimeMetrics) GetOptions() *MetricOptions { return m.Options }

func (*ResponseTimeMetrics) GetService() string { return "web" }

func (*ResponseTimeMetrics) Title() string { return "response time (seconds)" }

func (m *ResponseTimeMetrics) ShortName() string {
	name := "resp time"
	if m.Stat == "Average" {
		return fmt.Sprintf("%s (avg)", name)
	}
	return fmt.Sprintf("%s (%s)", name, m.Stat)
}

func (*ResponseTimeMetrics) MetricColor(*string) cell.Color {
	return cell.ColorBlue
}

func (*ResponseTimeMetrics) LineChartOptions() []linechart.Option {
	return []linechart.Option{}
}

func (m *ResponseTimeMetrics) MetricDataQueries() []*cloudwatch.MetricDataQuery {
	return []*cloudwatch.MetricDataQuery{
		{
			Id: aws.String(strings.ToLower(m.Stat)),
			MetricStat: &cloudwatch.MetricStat{
				Metric: &cloudwatch.Metric{
					Namespace:  aws.String("AWS/ApplicationELB"),
					MetricName: aws.String("TargetResponseTime"),
					Dimensions: []*cloudwatch.Dimension{
						{
							Name:  aws.String("TargetGroup"),
							Value: aws.String(m.App.Settings.TargetGroup.Suffix),
						},
						{
							Name:  aws.String("LoadBalancer"),
							Value: aws.String(m.App.Settings.LoadBalancer.Suffix),
						},
					},
				},
				Period: aws.Int64(m.Options.Timeframe.Period()),
				Stat:   &m.Stat,
			},
		},
	}
}

// status codes
type StatusCodeMetrics struct {
	App     *app.App
	Options *MetricOptions
	Code    string
}

func (m *StatusCodeMetrics) GetApp() *app.App { return m.App }

func (m *StatusCodeMetrics) GetOptions() *MetricOptions { return m.Options }

func (*StatusCodeMetrics) GetService() string { return "web" }

func (m *StatusCodeMetrics) Title() string { return fmt.Sprintf("%s responses (count)", m.Code) }

func (m *StatusCodeMetrics) ShortName() string { return fmt.Sprintf("%s responses", m.Code) }

func (*StatusCodeMetrics) MetricColor(name *string) cell.Color {
	switch *name {
	case "2xx":
		return cell.ColorGreen
	case "3xx":
		return cell.ColorBlue
	case "4xx":
		return cell.ColorYellow
	case "5xx":
		return cell.ColorRed
	default:
		return cell.ColorGray
	}
}

func (*StatusCodeMetrics) LineChartOptions() []linechart.Option {
	return []linechart.Option{}
}

func (m *StatusCodeMetrics) MetricDataQueries() []*cloudwatch.MetricDataQuery {
	metricDataQueries := []*cloudwatch.MetricDataQuery{
		{
			Id: aws.String(fmt.Sprintf("mm%s", m.Code)),
			MetricStat: &cloudwatch.MetricStat{
				Metric: &cloudwatch.Metric{
					Namespace:  aws.String("AWS/ApplicationELB"),
					MetricName: aws.String(fmt.Sprintf("HTTPCode_Target_%s_Count", strings.ToUpper(m.Code))),
					Dimensions: []*cloudwatch.Dimension{
						{
							Name:  aws.String("TargetGroup"),
							Value: aws.String(m.App.Settings.TargetGroup.Suffix),
						},
						{
							Name:  aws.String("LoadBalancer"),
							Value: aws.String(m.App.Settings.LoadBalancer.Suffix),
						},
					},
				},
				Period: aws.Int64(m.Options.Timeframe.Period()),
				Stat:   aws.String("Sum"),
			},
		},
	}
	return metricDataQueries
}

func FetchMetrics(metrics AppMetrics) (*cloudwatch.GetMetricDataOutput, error) {
	app := metrics.GetApp()
	options := metrics.GetOptions()
	cloudwatchSvc := cloudwatch.New(app.Session)
	return cloudwatchSvc.GetMetricData(&cloudwatch.GetMetricDataInput{
		StartTime:         aws.Time(options.TimeframeStart()),
		EndTime:           aws.Time(time.Now()),
		ScanBy:            aws.String("TimestampAscending"),
		MaxDatapoints:     aws.Int64(10000),
		MetricDataQueries: metrics.MetricDataQueries(),
	})
}
