package qqbot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	imagedraw "image/draw"
	"image/png"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	channelStatusImageWidth     = 1200
	channelStatusImageMargin    = 52
	channelStatusImageGap       = 24
	channelStatusCardHeight     = 246
	channelStatusHeaderHeight   = 164
	channelStatusFooterHeight   = 72
	channelStatusMaxMonitors    = 12
	channelStatusTimelinePoints = 30
	channelStatusMaxPNGBytes    = 5 << 20
)

var errChannelStatusFontUnavailable = errors.New("qqbot channel status font unavailable")

type ChannelStatusRenderer struct {
	fontPath string

	fontOnce sync.Once
	fontErr  error
	regular  *opentype.Font
	bold     *opentype.Font
}

type channelStatusFaces struct {
	title   font.Face
	heading font.Face
	body    font.Face
	small   font.Face
	tiny    font.Face
	closers []font.Face
}

func NewChannelStatusRenderer() *ChannelStatusRenderer {
	return &ChannelStatusRenderer{fontPath: strings.TrimSpace(os.Getenv("QQBOT_CHANNEL_CHECK_FONT_PATH"))}
}

func newChannelStatusRendererWithFonts(regular, bold *opentype.Font) *ChannelStatusRenderer {
	if bold == nil {
		bold = regular
	}
	return &ChannelStatusRenderer{regular: regular, bold: bold}
}

func (r *ChannelStatusRenderer) Render(ctx context.Context, items []*service.UserMonitorView, generatedAt time.Time) ([]byte, error) {
	if r == nil {
		return nil, errChannelStatusFontUnavailable
	}
	if err := r.ensureFonts(); err != nil {
		return nil, err
	}
	faces, err := r.newFaces()
	if err != nil {
		return nil, err
	}
	defer faces.close()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	views := append([]*service.UserMonitorView(nil), items...)
	sort.SliceStable(views, func(i, j int) bool {
		return channelStatusSortKey(views[i]) < channelStatusSortKey(views[j])
	})
	visible := views
	if len(visible) > channelStatusMaxMonitors {
		visible = visible[:channelStatusMaxMonitors]
	}

	rows := (len(visible) + 1) / 2
	height := channelStatusHeaderHeight + channelStatusFooterHeight
	if rows == 0 {
		height = 430
	} else {
		height += rows*channelStatusCardHeight + maxInt(0, rows-1)*channelStatusImageGap
	}
	canvas := image.NewRGBA(image.Rect(0, 0, channelStatusImageWidth, height))
	imagedraw.Draw(canvas, canvas.Bounds(), image.NewUniform(hexColor("#F4F7F9")), image.Point{}, imagedraw.Src)

	r.drawHeader(canvas, faces, visible, generatedAt)
	if len(visible) == 0 {
		r.drawEmpty(canvas, faces)
	} else {
		r.drawCards(ctx, canvas, faces, visible)
	}
	r.drawFooter(canvas, faces, len(views)-len(visible), generatedAt)

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var output bytes.Buffer
	if err := png.Encode(&output, canvas); err != nil {
		return nil, fmt.Errorf("encode qqbot channel status png: %w", err)
	}
	if output.Len() > channelStatusMaxPNGBytes {
		return nil, fmt.Errorf("qqbot channel status png exceeds %d bytes", channelStatusMaxPNGBytes)
	}
	return output.Bytes(), nil
}

func (r *ChannelStatusRenderer) drawHeader(img *image.RGBA, faces channelStatusFaces, items []*service.UserMonitorView, generatedAt time.Time) {
	drawText(img, faces.title, hexColor("#102A43"), channelStatusImageMargin, 68, "PokeAPI Channel Status")
	drawText(img, faces.body, hexColor("#627D98"), channelStatusImageMargin, 102, "渠道监控实时快照 · 7 天可用率")

	status := "全部正常"
	statusColor := hexColor("#067647")
	statusFill := hexColor("#DFF7E8")
	if len(items) == 0 {
		status = "暂无数据"
		statusColor = hexColor("#526D82")
		statusFill = hexColor("#EAF0F5")
	}
	for _, item := range items {
		if item == nil || strings.ToLower(strings.TrimSpace(item.PrimaryStatus)) != "operational" {
			status = "部分异常"
			statusColor = hexColor("#9A3412")
			statusFill = hexColor("#FFEDD5")
			break
		}
	}
	labelWidth := textWidth(faces.body, status) + 50
	x := channelStatusImageWidth - channelStatusImageMargin - labelWidth
	fillRoundedRect(img, image.Rect(x, 40, x+labelWidth, 82), 21, statusFill)
	fillCircle(img, x+21, 61, 6, statusColor)
	drawText(img, faces.body, statusColor, x+36, 68, status)

	timestamp := generatedAt.Format("2006-01-02 15:04:05 MST")
	timestampWidth := textWidth(faces.small, timestamp)
	drawText(img, faces.small, hexColor("#829AB1"), channelStatusImageWidth-channelStatusImageMargin-timestampWidth, 108, timestamp)
}

func (r *ChannelStatusRenderer) drawEmpty(img *image.RGBA, faces channelStatusFaces) {
	rect := image.Rect(channelStatusImageMargin, channelStatusHeaderHeight, channelStatusImageWidth-channelStatusImageMargin, 350)
	fillRoundedRect(img, rect, 24, hexColor("#FFFFFF"))
	strokeRoundedRect(img, rect, 24, 1, hexColor("#D9E2EC"))
	centerX := channelStatusImageWidth / 2
	fillCircle(img, centerX, 225, 30, hexColor("#EAF0F5"))
	drawCenteredText(img, faces.heading, hexColor("#334E68"), centerX, 288, "暂无渠道监控数据")
	drawCenteredText(img, faces.small, hexColor("#829AB1"), centerX, 320, "管理员启用渠道监控后，状态会显示在这里。")
}

func (r *ChannelStatusRenderer) drawCards(ctx context.Context, img *image.RGBA, faces channelStatusFaces, items []*service.UserMonitorView) {
	cardWidth := (channelStatusImageWidth - 2*channelStatusImageMargin - channelStatusImageGap) / 2
	for index, item := range items {
		if ctx.Err() != nil {
			return
		}
		row := index / 2
		column := index % 2
		x := channelStatusImageMargin + column*(cardWidth+channelStatusImageGap)
		y := channelStatusHeaderHeight + row*(channelStatusCardHeight+channelStatusImageGap)
		r.drawCard(img, faces, item, image.Rect(x, y, x+cardWidth, y+channelStatusCardHeight))
	}
}

func (r *ChannelStatusRenderer) drawCard(img *image.RGBA, faces channelStatusFaces, item *service.UserMonitorView, rect image.Rectangle) {
	fillRoundedRect(img, rect, 20, hexColor("#FFFFFF"))
	strokeRoundedRect(img, rect, 20, 1, hexColor("#D9E2EC"))
	if item == nil {
		return
	}

	padding := 22
	left := rect.Min.X + padding
	top := rect.Min.Y + padding
	statusLabel, statusColor, statusFill := channelStatusStyle(item.PrimaryStatus)
	pillWidth := textWidth(faces.small, statusLabel) + 30
	pillRect := image.Rect(rect.Max.X-padding-pillWidth, top, rect.Max.X-padding, top+34)
	fillRoundedRect(img, pillRect, 17, statusFill)
	drawCenteredText(img, faces.small, statusColor, (pillRect.Min.X+pillRect.Max.X)/2, top+23, statusLabel)

	nameMaxWidth := pillRect.Min.X - left - 14
	name := truncateText(faces.heading, strings.TrimSpace(item.Name), nameMaxWidth)
	if name == "" {
		name = "未命名渠道"
	}
	drawText(img, faces.heading, hexColor("#102A43"), left, top+28, name)

	provider := strings.ToUpper(strings.TrimSpace(item.Provider))
	if provider == "" {
		provider = "UNKNOWN"
	}
	providerColor := providerStatusColor(item.Provider)
	providerWidth := textWidth(faces.tiny, provider) + 18
	providerRect := image.Rect(left, top+43, left+providerWidth, top+70)
	fillRoundedRect(img, providerRect, 8, withAlpha(providerColor, 28))
	drawCenteredText(img, faces.tiny, providerColor, (providerRect.Min.X+providerRect.Max.X)/2, top+61, provider)

	modelX := providerRect.Max.X + 10
	modelMaxWidth := rect.Max.X - padding - modelX
	model := truncateText(faces.small, strings.TrimSpace(item.PrimaryModel), modelMaxWidth)
	if model == "" {
		model = "--"
	}
	drawText(img, faces.small, hexColor("#627D98"), modelX, top+62, model)

	if group := strings.TrimSpace(item.GroupName); group != "" {
		group = truncateText(faces.tiny, group, rect.Dx()-2*padding)
		drawText(img, faces.tiny, hexColor("#829AB1"), left, top+87, "分组 · "+group)
	}

	metricTop := top + 102
	metricGap := 12
	metricWidth := (rect.Dx() - 2*padding - metricGap) / 2
	drawMetric(img, faces, image.Rect(left, metricTop, left+metricWidth, metricTop+58), "API 延迟", formatChannelLatency(item.PrimaryLatencyMs), "ms")
	drawMetric(img, faces, image.Rect(left+metricWidth+metricGap, metricTop, rect.Max.X-padding, metricTop+58), "PING", formatChannelLatency(item.PrimaryPingLatencyMs), "ms")

	availabilityLabel := fmt.Sprintf("%.2f%%", clampAvailability(item.Availability7d))
	drawText(img, faces.tiny, hexColor("#829AB1"), left, metricTop+86, "7 天可用率")
	availabilityWidth := textWidth(faces.body, availabilityLabel)
	drawText(img, faces.body, availabilityColor(item.Availability7d), rect.Max.X-padding-availabilityWidth, metricTop+88, availabilityLabel)

	drawTimeline(img, item.Timeline, image.Rect(left, rect.Max.Y-36, rect.Max.X-padding, rect.Max.Y-18))
}

func (r *ChannelStatusRenderer) drawFooter(img *image.RGBA, faces channelStatusFaces, remaining int, generatedAt time.Time) {
	footerY := img.Bounds().Max.Y - 34
	text := "数据来源：PokeAPI 渠道监控"
	if remaining > 0 {
		text = fmt.Sprintf("另有 %d 个监控未展示，请前往站内状态页查看完整信息", remaining)
	}
	drawText(img, faces.small, hexColor("#829AB1"), channelStatusImageMargin, footerY, text)
	right := "自动生成 · " + generatedAt.Format("15:04:05")
	drawText(img, faces.small, hexColor("#9FB3C8"), channelStatusImageWidth-channelStatusImageMargin-textWidth(faces.small, right), footerY, right)
}

func (r *ChannelStatusRenderer) ensureFonts() error {
	r.fontOnce.Do(func() {
		if r.regular != nil {
			if r.bold == nil {
				r.bold = r.regular
			}
			return
		}
		regularCandidates := []string{
			"/usr/share/fonts/noto/NotoSansCJK-Regular.ttc",
			"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
			"/usr/share/fonts/noto-cjk/NotoSansCJK-Regular.ttc",
			`C:\Windows\Fonts\msyh.ttc`,
			"/System/Library/Fonts/PingFang.ttc",
		}
		if r.fontPath != "" {
			regularCandidates = append([]string{r.fontPath}, regularCandidates...)
		}
		r.regular, r.fontErr = loadFirstOpenTypeFont(regularCandidates)
		if r.fontErr != nil {
			r.fontErr = fmt.Errorf("%w: %v", errChannelStatusFontUnavailable, r.fontErr)
			return
		}
		boldCandidates := []string{
			"/usr/share/fonts/noto/NotoSansCJK-Bold.ttc",
			"/usr/share/fonts/opentype/noto/NotoSansCJK-Bold.ttc",
			"/usr/share/fonts/noto-cjk/NotoSansCJK-Bold.ttc",
			`C:\Windows\Fonts\msyhbd.ttc`,
			"/System/Library/Fonts/PingFang.ttc",
		}
		r.bold, _ = loadFirstOpenTypeFont(boldCandidates)
		if r.bold == nil {
			r.bold = r.regular
		}
	})
	return r.fontErr
}

func loadFirstOpenTypeFont(paths []string) (*opentype.Font, error) {
	var lastErr error
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			lastErr = err
			continue
		}
		if collection, err := opentype.ParseCollection(raw); err == nil && collection.NumFonts() > 0 {
			parsed, fontErr := collection.Font(0)
			if fontErr == nil {
				return parsed, nil
			}
			lastErr = fontErr
			continue
		}
		parsed, err := opentype.Parse(raw)
		if err == nil {
			return parsed, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no usable font path")
	}
	return nil, lastErr
}

func (r *ChannelStatusRenderer) newFaces() (channelStatusFaces, error) {
	create := func(source *opentype.Font, size float64) (font.Face, error) {
		return opentype.NewFace(source, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	}
	var faces channelStatusFaces
	var err error
	if faces.title, err = create(r.bold, 34); err != nil {
		return faces, err
	}
	faces.closers = append(faces.closers, faces.title)
	if faces.heading, err = create(r.bold, 22); err != nil {
		faces.close()
		return faces, err
	}
	faces.closers = append(faces.closers, faces.heading)
	if faces.body, err = create(r.regular, 17); err != nil {
		faces.close()
		return faces, err
	}
	faces.closers = append(faces.closers, faces.body)
	if faces.small, err = create(r.regular, 14); err != nil {
		faces.close()
		return faces, err
	}
	faces.closers = append(faces.closers, faces.small)
	if faces.tiny, err = create(r.regular, 12); err != nil {
		faces.close()
		return faces, err
	}
	faces.closers = append(faces.closers, faces.tiny)
	return faces, nil
}

func (f channelStatusFaces) close() {
	for _, face := range f.closers {
		if closer, ok := face.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	}
}

func drawMetric(img *image.RGBA, faces channelStatusFaces, rect image.Rectangle, label, value, unit string) {
	fillRoundedRect(img, rect, 12, hexColor("#F4F7F9"))
	drawText(img, faces.tiny, hexColor("#829AB1"), rect.Min.X+14, rect.Min.Y+20, label)
	drawText(img, faces.heading, hexColor("#243B53"), rect.Min.X+14, rect.Min.Y+48, value)
	if value != "--" {
		drawText(img, faces.tiny, hexColor("#9FB3C8"), rect.Min.X+18+textWidth(faces.heading, value), rect.Min.Y+47, unit)
	}
}

func drawTimeline(img *image.RGBA, points []service.UserMonitorTimelinePoint, rect image.Rectangle) {
	gap := 3
	barWidth := (rect.Dx() - (channelStatusTimelinePoints-1)*gap) / channelStatusTimelinePoints
	if barWidth < 2 {
		barWidth = 2
	}
	real := append([]service.UserMonitorTimelinePoint(nil), points...)
	if len(real) > channelStatusTimelinePoints {
		real = real[:channelStatusTimelinePoints]
	}
	for left, right := 0, len(real)-1; left < right; left, right = left+1, right-1 {
		real[left], real[right] = real[right], real[left]
	}
	padding := channelStatusTimelinePoints - len(real)
	for index := 0; index < channelStatusTimelinePoints; index++ {
		status := "empty"
		if index >= padding {
			status = strings.ToLower(strings.TrimSpace(real[index-padding].Status))
		}
		barColor, ratio := timelineStyle(status)
		height := int(math.Round(float64(rect.Dy()) * ratio))
		if height < 3 {
			height = 3
		}
		x := rect.Min.X + index*(barWidth+gap)
		fillRoundedRect(img, image.Rect(x, rect.Max.Y-height, x+barWidth, rect.Max.Y), minInt(2, barWidth/2), barColor)
	}
}

func channelStatusSortKey(item *service.UserMonitorView) string {
	if item == nil {
		return "\xff"
	}
	return strings.ToLower(strings.TrimSpace(item.Provider + "\x00" + item.Name))
}

func channelStatusStyle(status string) (string, color.RGBA, color.RGBA) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "operational":
		return "正常", hexColor("#067647"), hexColor("#DFF7E8")
	case "degraded":
		return "降级", hexColor("#9A3412"), hexColor("#FFEDD5")
	case "failed":
		return "失败", hexColor("#B42318"), hexColor("#FEE4E2")
	case "error":
		return "错误", hexColor("#B42318"), hexColor("#FEE4E2")
	default:
		return "无历史", hexColor("#526D82"), hexColor("#EAF0F5")
	}
}

func timelineStyle(status string) (color.RGBA, float64) {
	switch status {
	case "operational":
		return hexColor("#12B76A"), 1
	case "degraded":
		return hexColor("#F79009"), .65
	case "failed", "error":
		return hexColor("#F04438"), .38
	default:
		return hexColor("#CBD5E1"), .2
	}
}

func providerStatusColor(provider string) color.RGBA {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai":
		return hexColor("#067647")
	case "anthropic":
		return hexColor("#B54708")
	case "gemini":
		return hexColor("#175CD3")
	case "grok":
		return hexColor("#344054")
	default:
		return hexColor("#526D82")
	}
}

func availabilityColor(value float64) color.RGBA {
	switch {
	case value >= 99:
		return hexColor("#067647")
	case value >= 95:
		return hexColor("#B54708")
	default:
		return hexColor("#B42318")
	}
}

func clampAvailability(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func formatChannelLatency(value *int) string {
	if value == nil || *value < 0 {
		return "--"
	}
	return fmt.Sprintf("%d", *value)
}

func drawText(img *image.RGBA, face font.Face, textColor color.Color, x, baseline int, value string) {
	drawer := font.Drawer{Dst: img, Src: image.NewUniform(textColor), Face: face, Dot: fixed.P(x, baseline)}
	drawer.DrawString(value)
}

func drawCenteredText(img *image.RGBA, face font.Face, textColor color.Color, centerX, baseline int, value string) {
	drawText(img, face, textColor, centerX-textWidth(face, value)/2, baseline, value)
}

func textWidth(face font.Face, value string) int {
	drawer := font.Drawer{Face: face}
	return drawer.MeasureString(value).Ceil()
}

func truncateText(face font.Face, value string, maxWidth int) string {
	value = strings.TrimSpace(value)
	if value == "" || maxWidth <= 0 {
		return ""
	}
	if textWidth(face, value) <= maxWidth {
		return value
	}
	ellipsis := "…"
	available := maxWidth - textWidth(face, ellipsis)
	if available <= 0 {
		return ellipsis
	}
	runes := []rune(value)
	low, high := 0, len(runes)
	for low < high {
		middle := (low + high + 1) / 2
		if textWidth(face, string(runes[:middle])) <= available {
			low = middle
		} else {
			high = middle - 1
		}
	}
	return string(runes[:low]) + ellipsis
}

func fillRoundedRect(img *image.RGBA, rect image.Rectangle, radius int, fill color.Color) {
	if rect.Empty() {
		return
	}
	radius = minInt(radius, minInt(rect.Dx(), rect.Dy())/2)
	if radius <= 0 {
		imagedraw.Draw(img, rect, image.NewUniform(fill), image.Point{}, imagedraw.Over)
		return
	}
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		offset := roundedRectOffset(y, rect, radius)
		span := image.Rect(rect.Min.X+offset, y, rect.Max.X-offset, y+1)
		imagedraw.Draw(img, span, image.NewUniform(fill), image.Point{}, imagedraw.Src)
	}
}

func strokeRoundedRect(img *image.RGBA, rect image.Rectangle, radius, width int, stroke color.Color) {
	if width <= 0 || rect.Empty() {
		return
	}
	fillRoundedRect(img, rect, radius, stroke)
	inner := rect.Inset(width)
	if !inner.Empty() {
		fillRoundedRect(img, inner, maxInt(0, radius-width), hexColor("#FFFFFF"))
	}
}

func roundedRectOffset(y int, rect image.Rectangle, radius int) int {
	centerTop := rect.Min.Y + radius
	centerBottom := rect.Max.Y - radius - 1
	var dy int
	switch {
	case y < centerTop:
		dy = centerTop - y
	case y > centerBottom:
		dy = y - centerBottom
	default:
		return 0
	}
	inside := radius*radius - dy*dy
	if inside <= 0 {
		return radius
	}
	return radius - int(math.Sqrt(float64(inside)))
}

func fillCircle(img *image.RGBA, centerX, centerY, radius int, fill color.Color) {
	for y := centerY - radius; y <= centerY+radius; y++ {
		dy := y - centerY
		inside := radius*radius - dy*dy
		if inside < 0 {
			continue
		}
		dx := int(math.Sqrt(float64(inside)))
		span := image.Rect(centerX-dx, y, centerX+dx+1, y+1)
		imagedraw.Draw(img, span, image.NewUniform(fill), image.Point{}, imagedraw.Src)
	}
}

func hexColor(value string) color.RGBA {
	value = strings.TrimPrefix(strings.TrimSpace(value), "#")
	var red, green, blue uint8
	if len(value) == 6 {
		var parsed uint64
		_, _ = fmt.Sscanf(value, "%06x", &parsed)
		red = uint8(parsed >> 16)
		green = uint8(parsed >> 8)
		blue = uint8(parsed)
	}
	return color.RGBA{R: red, G: green, B: blue, A: 255}
}

func withAlpha(value color.RGBA, alpha uint8) color.RGBA {
	value.A = alpha
	return value
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
