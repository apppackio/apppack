/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/apppackio/apppack/app"
	"github.com/apppackio/apppack/metrics"
	"github.com/apppackio/apppack/ui"
	"github.com/mum4k/termdash"
	"github.com/mum4k/termdash/align"
	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/container"
	"github.com/mum4k/termdash/container/grid"
	"github.com/mum4k/termdash/keyboard"
	"github.com/mum4k/termdash/linestyle"
	"github.com/mum4k/termdash/terminal/tcell"
	"github.com/mum4k/termdash/terminal/terminalapi"
	"github.com/mum4k/termdash/widgetapi"
	"github.com/mum4k/termdash/widgets/button"
	"github.com/mum4k/termdash/widgets/linechart"
	"github.com/mum4k/termdash/widgets/text"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type LegendItem struct {
	Name  string
	Color cell.Color
}

const (
	redrawInterval     = 250 * time.Millisecond
	buttonWidth        = 20
	contentContainerID = "content"
)

func BuildLineChart(ctx context.Context, appMetrics metrics.AppMetrics) (*linechart.LineChart, *text.Text, error) {
	lc, err := linechart.New(
		append(appMetrics.LineChartOptions(),
			linechart.AxesCellOpts(cell.FgColor(cell.ColorGray)),
			linechart.YLabelCellOpts(cell.FgColor(cell.ColorGray)),
			linechart.XLabelCellOpts(cell.FgColor(cell.ColorGray)),
		)...,
	)
	if err != nil {
		return nil, nil, err
	}
	legendText, err := text.New()
	if err != nil {
		return nil, nil, err
	}

	go periodic(ctx, 30*time.Second, func() error {
		legend, err := populateLineChart(appMetrics, lc)
		if err != nil {
			return err
		}
		legendText.Reset()
		if err = legendText.Write("  "); err != nil {
			return err
		}
		for _, l := range legend {
			if err = legendText.Write(
				fmt.Sprintf("… %s  ", l.Name),
				text.WriteCellOpts(cell.FgColor(appMetrics.MetricColor(&l.Name)))); err != nil {
				return err
			}
		}
		return nil
	})
	return lc, legendText, nil
}

// periodic executes the provided closure periodically every interval.
// Exits when the context expires.
func periodic(ctx context.Context, interval time.Duration, fn func() error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	if err := fn(); err != nil {
		logrus.WithFields(logrus.Fields{"error": err}).Error("error executing periodic function")
	}

	for {
		select {
		case <-ticker.C:
			if err := fn(); err != nil {
				logrus.WithFields(logrus.Fields{"error": err}).Error("error executing periodic function")
			}
		case <-ctx.Done():
			return
		}
	}
}

func serviceEventsText(ctx context.Context, a *app.App, options *metrics.MetricOptions, service string) (*text.Text, error) {
	textWidget, err := text.New(
		text.WrapAtWords(),
		text.RollContent(),
	)
	if err != nil {
		return nil, err
	}
	_ = textWidget.Write("loading", text.WriteCellOpts(cell.FgColor(cell.ColorGray)))

	go periodic(ctx, 10*time.Second, func() error {
		events, err := a.GetECSEvents(service)
		if err != nil {
			return err
		}
		textWidget.Reset()
		for _, e := range events {
			if options.TimeframeStart().After(*e.CreatedAt) {
				continue
			}
			var created time.Time
			if options.UTC {
				created = e.CreatedAt.UTC()
			} else {
				created = e.CreatedAt.Local()
			}
			if err := textWidget.Write(
				created.Format("Jan 02, 2006 15:04:05 MST"), text.WriteCellOpts(cell.FgColor(cell.ColorGray))); err != nil {
				return err
			}
			if err := textWidget.Write(fmt.Sprintf(" %s\n", *e.Message)); err != nil {
				return err
			}
		}
		return nil
	})
	return textWidget, nil
}

func labelsFromTimestamps(timestamps []*time.Time, utc bool) map[int]string {
	labels := map[int]string{}
	format := "15:04"
	if len(timestamps) == 0 {
		return labels
	}
	firstDay := timestamps[0].Format("02")
	lastDay := timestamps[len(timestamps)-1].Format("02")
	if firstDay != lastDay {
		format = "2T15:04"
	}
	for i, t := range timestamps {
		if utc {
			labels[i] = t.UTC().Format(format)
		} else {
			labels[i] = t.Local().Format(format)
		}
	}
	return labels
}

func populateLineChart(appMetrics metrics.AppMetrics, lc *linechart.LineChart) ([]*LegendItem, error) {
	metrics, err := metrics.FetchMetrics(appMetrics)
	if err != nil {
		return nil, err
	}
	var legend []*LegendItem
	for _, metric := range metrics.MetricDataResults {
		// "mm" is a special prefix for metrics that start with numbers to make them valid for Cloudwatch (e.g. "mm2xx")
		name := strings.TrimPrefix(*metric.Id, "mm")
		color := appMetrics.MetricColor(&name)
		var values []float64
		for _, v := range metric.Values {
			values = append(values, *v)
		}
		labels := labelsFromTimestamps(metric.Timestamps, appMetrics.GetOptions().UTC)
		err = lc.Series(name, values,
			linechart.SeriesCellOpts(cell.FgColor(color)),
			linechart.SeriesXLabels(labels),
		)
		if err != nil {
			return nil, err
		}
		legend = append(legend, &LegendItem{Name: name, Color: color})
	}
	return legend, nil
}

func UpdateDashContent(ctx context.Context, c *container.Container, metric metrics.AppMetrics) {
	lines, legend, err := BuildLineChart(ctx, metric)
	checkErr(err)
	events, err := serviceEventsText(ctx, metric.GetApp(), metric.GetOptions(), metric.GetService())
	checkErr(err)
	checkErr(c.Update(
		contentContainerID,
		container.SplitHorizontal(
			container.Top(
				container.ID("top"),
				container.Border(linestyle.Light),
				container.BorderTitle(metric.Title()),
				container.SplitHorizontal(
					container.Top(container.ID("graph"), container.PlaceWidget(lines)),
					container.Bottom(container.ID("legend"), container.PlaceWidget(legend)),
					container.SplitPercent(99),
				),
			),
			container.Bottom(
				container.ID("bottom"),
				container.Border(linestyle.Light),
				container.BorderTitle(fmt.Sprintf("Events (%s)", metric.GetService())),
				container.PlaceWidget(events),
			),
			container.SplitPercent(70),
		),
	))
}

// dashCmd represents the dash command
var dashCmd = &cobra.Command{
	Use:                   "dash",
	Short:                 "app metrics dashboard",
	Long:                  "[EXPERIMENTAL] interactive terminal-based dashboard for app metrics",
	DisableFlagsInUseLine: true,
	Args:                  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		ui.StartSpinner()
		a, err := app.Init(AppName, UseAWSCredentials, MaxSessionDurationSeconds)
		checkErr(err)
		checkErr(a.LoadSettings())
		t, err := tcell.New(tcell.ColorMode(terminalapi.ColorMode256))
		checkErr(err)
		defer t.Close()
		ui.Spinner.Stop()
		widgetCtx, widgetCancel := context.WithCancel(context.Background())
		rootContainer, err := container.New(t,
			container.ID("root"),
			container.SplitVertical(
				container.Left(
					container.ID("navigation"),
				),
				container.Right(
					container.ID(contentContainerID),
				),
				container.SplitFixed(buttonWidth+5),
			),
		)
		checkErr(err)
		timeframes := []metrics.TimeFrame{
			{Name: "hour", ShortcutKey: "h", Duration: time.Hour},
			{Name: "three hours", ShortcutKey: "r", Duration: 3 * time.Hour},
			{Name: "six hours", ShortcutKey: "s", Duration: 6 * time.Hour},
			{Name: "twelve hours", ShortcutKey: "e", Duration: 12 * time.Hour},
			{Name: "day", ShortcutKey: "d", Duration: 24 * time.Hour},
			{Name: "three days", ShortcutKey: "t", Duration: 3 * 24 * time.Hour},
			{Name: "week", ShortcutKey: "w", Duration: 7 * 24 * time.Hour},
		}
		options := metrics.MetricOptions{
			Timeframe: timeframes[2],
			UTC:       false,
		}
		buttonOpts := []button.Option{
			button.DisableShadow(),
			button.Height(1),
			button.PressedFillColor(cell.ColorWhite),
		}
		services, err := a.GetServices()
		checkErr(err)

		var appMetrics []metrics.AppMetrics
		for _, svc := range services {
			appMetrics = append(appMetrics, &metrics.ServiceUtilizationMetrics{App: a, Options: &options, Service: svc})
		}
		appMetrics = append(appMetrics,
			&metrics.StatusCodeMetrics{App: a, Options: &options, Code: "2xx"},
			&metrics.StatusCodeMetrics{App: a, Options: &options, Code: "3xx"},
			&metrics.StatusCodeMetrics{App: a, Options: &options, Code: "4xx"},
			&metrics.StatusCodeMetrics{App: a, Options: &options, Code: "5xx"},
			&metrics.ResponseTimeMetrics{App: a, Options: &options, Stat: "Average"},
			&metrics.ResponseTimeMetrics{App: a, Options: &options, Stat: "p99"},
		)
		currentMetric := appMetrics[0]
		var graphButtons []widgetapi.Widget
		for i := range appMetrics {
			shortcut := i + 1
			shortcutRune, _ := utf8.DecodeRuneInString(fmt.Sprintf("%d", shortcut))
			idx := i // create new int in scope for closure below
			opts := append(
				buttonOpts,
				button.GlobalKey(keyboard.Key(shortcutRune)),
				button.FillColor(cell.ColorGreen),
			)
			text := fmt.Sprintf("[%d] %s", shortcut, appMetrics[idx].ShortName())
			padding := buttonWidth - len(text)
			button, err := button.New(
				fmt.Sprintf("%s%*s", text, padding, " "),
				func() error {
					widgetCancel()
					currentMetric = appMetrics[idx]
					UpdateDashContent(widgetCtx, rootContainer, appMetrics[idx])
					return nil
				},
				opts...,
			)
			checkErr(err)
			graphButtons = append(graphButtons, button)
		}

		builder := grid.New()
		for _, w := range graphButtons {
			builder.Add(
				grid.RowHeightPerc(100/len(graphButtons),
					grid.Widget(
						w,
						container.AlignVertical(align.VerticalTop),
						container.AlignHorizontal(align.HorizontalLeft),
					),
				),
			)
		}
		gridOpts, err := builder.Build()
		checkErr(err)
		gridOpts = append(
			gridOpts,
			container.BorderTitle("Graph"),
			container.PaddingTop(1),
			container.AlignVertical(align.VerticalTop),
			container.Border(linestyle.Light),
			container.BorderColor(cell.ColorGray),
		)
		err = rootContainer.Update("navigation",
			container.SplitHorizontal(
				container.Top(gridOpts...),
				container.Bottom(container.ID("timeframeNav")),
				container.SplitFixed(len(graphButtons)+4),
			),
		)
		checkErr(err)
		var timeframeButtons []widgetapi.Widget

		for _, tf := range timeframes {
			thisTimeframe := tf.Clone()
			shortcutRune, _ := utf8.DecodeRuneInString(tf.ShortcutKey)
			opts := append(
				buttonOpts,
				button.GlobalKey(keyboard.Key(shortcutRune)),
				button.FillColor(cell.ColorBlue),
			)
			text := strings.Replace(tf.Name, tf.ShortcutKey, fmt.Sprintf("[%s]", tf.ShortcutKey), 1)
			padding := buttonWidth - len(text)
			button, err := button.New(
				fmt.Sprintf("%s%*s", text, padding, " "),
				func() error {
					widgetCancel()
					options.Timeframe = *thisTimeframe
					UpdateDashContent(widgetCtx, rootContainer, currentMetric)
					return nil
				},
				opts...,
			)
			checkErr(err)
			timeframeButtons = append(timeframeButtons, button)
		}
		builder = grid.New()
		for _, w := range timeframeButtons {
			builder.Add(
				grid.RowHeightPerc(100/len(timeframeButtons),
					grid.Widget(
						w,
						container.AlignVertical(align.VerticalTop),
						container.AlignHorizontal(align.HorizontalCenter),
					),
				),
			)
		}
		gridOpts, err = builder.Build()
		checkErr(err)
		gridOpts = append(
			gridOpts,
			container.BorderTitle("Timeframe"),
			container.PaddingTop(1),
			container.Border(linestyle.Light),
			container.BorderColor(cell.ColorGray),
		)
		err = rootContainer.Update("timeframeNav", gridOpts...)
		checkErr(err)
		ctx, cancel := context.WithCancel(context.Background())
		quitter := func(k *terminalapi.Keyboard) {
			if k.Key == keyboard.KeyEsc || k.Key == keyboard.KeyCtrlC || k.Key == 'q' {
				widgetCancel()
				cancel()
			}
		}
		UpdateDashContent(widgetCtx, rootContainer, currentMetric)
		checkErr(termdash.Run(
			ctx,
			t,
			rootContainer,
			termdash.KeyboardSubscriber(quitter),
			termdash.RedrawInterval(redrawInterval),
		))
	},
}

func init() {
	rootCmd.AddCommand(dashCmd)
	dashCmd.PersistentFlags().StringVarP(&AppName, "app-name", "a", "", "app name (required)")
	dashCmd.MarkPersistentFlagRequired("app-name")
	dashCmd.PersistentFlags().BoolVar(&UseAWSCredentials, "aws-credentials", false, "use AWS credentials instead of AppPack.io federation")
}
