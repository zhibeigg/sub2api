package qqbot

import (
	"bytes"
	"context"
	"image/png"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

func TestChannelStatusRendererProducesDecodableBoundedPNG(t *testing.T) {
	parsedFont, err := opentype.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	renderer := newChannelStatusRendererWithFonts(parsedFont, parsedFont)
	apiLatency := 368
	pingLatency := 92
	items := []*service.UserMonitorView{
		{
			ID:                   1,
			Name:                 "OpenAI 主渠道",
			Provider:             "openai",
			GroupName:            "默认分组",
			PrimaryModel:         "gpt-5.4",
			PrimaryStatus:        "operational",
			PrimaryLatencyMs:     &apiLatency,
			PrimaryPingLatencyMs: &pingLatency,
			Availability7d:       99.97,
			Timeline: []service.UserMonitorTimelinePoint{
				{Status: "operational"},
				{Status: "degraded"},
				{Status: "failed"},
			},
		},
	}
	output, err := renderer.Render(context.Background(), items, time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(output) == 0 || len(output) > channelStatusMaxPNGBytes {
		t.Fatalf("unexpected PNG size: %d", len(output))
	}
	decoded, err := png.DecodeConfig(bytes.NewReader(output))
	if err != nil {
		t.Fatalf("decode generated PNG: %v", err)
	}
	if decoded.Width != channelStatusImageWidth || decoded.Height != channelStatusHeaderHeight+channelStatusCardHeight+channelStatusFooterHeight {
		t.Fatalf("unexpected PNG dimensions: %dx%d", decoded.Width, decoded.Height)
	}
}

func TestChannelStatusRendererHandlesEmptyAndTruncatedViews(t *testing.T) {
	parsedFont, err := opentype.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	renderer := newChannelStatusRendererWithFonts(parsedFont, parsedFont)
	if output, err := renderer.Render(context.Background(), nil, time.Now()); err != nil || len(output) == 0 {
		t.Fatalf("render empty output=%d err=%v", len(output), err)
	}

	items := make([]*service.UserMonitorView, channelStatusMaxMonitors+4)
	for index := range items {
		items[index] = &service.UserMonitorView{Name: "Monitor", Provider: "anthropic", PrimaryStatus: "unknown", Availability7d: -5}
	}
	output, err := renderer.Render(context.Background(), items, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.DecodeConfig(bytes.NewReader(output))
	if err != nil {
		t.Fatal(err)
	}
	expectedRows := channelStatusMaxMonitors / 2
	expectedHeight := channelStatusHeaderHeight + channelStatusFooterHeight + expectedRows*channelStatusCardHeight + (expectedRows-1)*channelStatusImageGap
	if decoded.Height != expectedHeight {
		t.Fatalf("truncated PNG height=%d want=%d", decoded.Height, expectedHeight)
	}
}
