package firefly

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type ResolvedParams struct {
	ModelID              string
	Type                 ModelType
	UpstreamModelID      string
	UpstreamModelVersion string
	UpstreamModel        string
	Engine               string
	AspectRatio          string
	Width                int
	Height               int
	Quality              string
	Duration             int
	VideoResolution      string
	GenerateAudio        bool
	GenerateAudioSet     bool
	ReferenceMode        string
}

func ResolveImageModel(model, size, quality string) (*ResolvedParams, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		model = "nano-banana-pro"
	}
	def, ok := aliases[model]
	if !ok || def.kind != ModelTypeImage {
		return nil, fmt.Errorf("unknown Adobe image model")
	}
	tier, err := resolveImageQuality(quality)
	if err != nil {
		return nil, err
	}
	aspect, auto, err := resolveImageAspect(size)
	if err != nil {
		return nil, err
	}
	isGPTImage := def.upstreamModelID == "gpt-image"
	if isGPTImage && auto {
		return nil, fmt.Errorf("auto size is not supported for this Adobe image model")
	}
	if isGPTImage && !supportedGPTImageAspect(aspect) {
		return nil, fmt.Errorf("unsupported image aspect ratio")
	}
	w, h := 0, 0
	if !auto {
		if isGPTImage {
			w, h = gptImageSize(aspect, tier)
		} else {
			w, h = imageSize(aspect, tier)
		}
	}
	prefix := map[string]string{"nano-banana-pro": "firefly-nano-banana-pro", "nano-banana-v2": "firefly-nano-banana2", "nano-banana": "firefly-nano-banana", "gpt-image-2": "firefly-gpt-image-2"}[model]
	if prefix == "" {
		prefix = "firefly-nano-banana"
	}
	suffix := strings.ReplaceAll(aspect, ":", "x")
	if auto {
		suffix = "auto"
	}
	return &ResolvedParams{ModelID: prefix + "-" + tier + "-" + suffix, Type: ModelTypeImage, UpstreamModelID: def.upstreamModelID, UpstreamModelVersion: def.upstreamModelVersion, AspectRatio: aspect, Width: w, Height: h, Quality: tier}, nil
}

func ResolveVideoModel(model, resolution string, duration int, aspectRatio string, generateAudio *bool, referenceMode string) (*ResolvedParams, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		model = "veo3.1"
	}
	def, ok := aliases[model]
	if !ok || def.kind != ModelTypeVideo {
		return nil, fmt.Errorf("unknown Adobe video model")
	}
	aspect := normalizeAspect(aspectRatio)
	if aspect != "16:9" && aspect != "9:16" {
		return nil, fmt.Errorf("unsupported video aspect ratio")
	}
	res, err := resolveVideoResolution(resolution)
	if err != nil {
		return nil, err
	}
	if def.engine == "sora2" {
		if duration != 4 && duration != 8 && duration != 12 {
			return nil, fmt.Errorf("sora supports durations 4, 8 or 12 seconds")
		}
		res = "720p"
	} else if duration != 4 && duration != 6 && duration != 8 {
		return nil, fmt.Errorf("veo supports durations 4, 6 or 8 seconds")
	}
	refMode := strings.ToLower(strings.TrimSpace(referenceMode))
	if refMode == "" {
		refMode = def.defaultReferenceMode
	}
	if refMode != "" && refMode != "image" && refMode != "frame" && refMode != "general" {
		return nil, fmt.Errorf("unsupported reference mode")
	}
	if refMode == "image" && (def.engine != "veo31-standard" || duration != 8 || aspect != "16:9") {
		return nil, fmt.Errorf("image reference mode requires veo3.1, 8s and 16:9")
	}
	w, h := videoDimensions(aspect, res)
	p := &ResolvedParams{ModelID: model, Type: ModelTypeVideo, UpstreamModel: def.upstreamModel, Engine: def.engine, AspectRatio: aspect, Width: w, Height: h, Duration: duration, VideoResolution: res, ReferenceMode: refMode}
	if generateAudio != nil {
		p.GenerateAudio = *generateAudio
		p.GenerateAudioSet = true
	} else {
		p.GenerateAudio = true
	}
	return p, nil
}

func resolveImageQuality(v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1k", "standard", "low":
		return "1k", nil
	case "", "auto", "2k", "medium":
		return "2k", nil
	case "4k", "ultra", "high", "hd":
		return "4k", nil
	default:
		return "", fmt.Errorf("unsupported image quality")
	}
}
func resolveVideoResolution(v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "720", "720p":
		return "720p", nil
	case "1080", "1080p", "fhd", "hd":
		return "1080p", nil
	default:
		return "", fmt.Errorf("unsupported video resolution")
	}
}
func resolveImageAspect(size string) (string, bool, error) {
	s := strings.ToLower(strings.TrimSpace(size))
	if s == "" {
		return "16:9", false, nil
	}
	if s == "auto" || s == "reference" || s == "ref" || s == "follow" {
		return "auto", true, nil
	}
	if strings.Contains(s, ":") {
		a := normalizeAspect(s)
		if !supportedImageAspect(a) {
			return "", false, fmt.Errorf("unsupported image aspect ratio")
		}
		return a, false, nil
	}
	parts := strings.SplitN(s, "x", 2)
	if len(parts) != 2 {
		return "", false, fmt.Errorf("invalid image size")
	}
	w, e1 := strconv.Atoi(parts[0])
	h, e2 := strconv.Atoi(parts[1])
	if e1 != nil || e2 != nil || w <= 0 || h <= 0 {
		return "", false, fmt.Errorf("invalid image size")
	}
	return nearestAspect(w, h), false, nil
}
func normalizeAspect(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.ReplaceAll(v, "x", ":")
	if v == "" {
		return "16:9"
	}
	return v
}
func supportedImageAspect(a string) bool {
	for _, v := range []string{"1:1", "2:3", "3:2", "3:4", "4:3", "4:5", "5:4", "9:16", "16:9", "21:9", "1:4", "4:1", "1:8", "8:1"} {
		if a == v {
			return true
		}
	}
	return false
}

func supportedGPTImageAspect(a string) bool {
	for _, v := range []string{"1:1", "2:3", "3:2", "3:4", "4:3", "4:5", "5:4", "9:16", "16:9", "21:9"} {
		if a == v {
			return true
		}
	}
	return false
}

func gptImageSize(aspect, tier string) (int, int) {
	table := map[string]map[string][2]int{
		"1k": {
			"1:1": {1024, 1024}, "3:2": {1536, 1024}, "2:3": {1024, 1536},
			"4:3": {1152, 864}, "3:4": {864, 1152}, "5:4": {1120, 896},
			"4:5": {896, 1120}, "16:9": {1280, 720}, "9:16": {720, 1280}, "21:9": {1456, 624},
		},
		"2k": {
			"1:1": {2048, 2048}, "3:2": {2496, 1664}, "2:3": {1664, 2496},
			"4:3": {2304, 1728}, "3:4": {1728, 2304}, "5:4": {2240, 1792},
			"4:5": {1792, 2240}, "16:9": {2560, 1440}, "9:16": {1440, 2560}, "21:9": {3024, 1296},
		},
		"4k": {
			"1:1": {2480, 2480}, "3:2": {3056, 2032}, "2:3": {2032, 3056},
			"4:3": {2880, 2160}, "3:4": {2160, 2880}, "5:4": {2784, 2224},
			"4:5": {2224, 2784}, "16:9": {3328, 1872}, "9:16": {1872, 3328}, "21:9": {3808, 1632},
		},
	}
	if tiers, ok := table[tier]; ok {
		if dims, ok := tiers[aspect]; ok {
			return dims[0], dims[1]
		}
	}
	return 1024, 1024
}

func nearestAspect(w, h int) string {
	vals := []struct {
		n string
		v float64
	}{{"8:1", 8}, {"4:1", 4}, {"21:9", 21.0 / 9}, {"16:9", 16.0 / 9}, {"3:2", 1.5}, {"4:3", 4.0 / 3}, {"5:4", 1.25}, {"1:1", 1}, {"4:5", .8}, {"3:4", .75}, {"2:3", 2.0 / 3}, {"9:16", 9.0 / 16}, {"1:4", .25}, {"1:8", .125}}
	r := float64(w) / float64(h)
	best := vals[0]
	d := math.Abs(math.Log(r / best.v))
	for _, x := range vals[1:] {
		nd := math.Abs(math.Log(r / x.v))
		if nd < d {
			best = x
			d = nd
		}
	}
	return best.n
}
func imageSize(a, q string) (int, int) {
	base := map[string]int{"1k": 1024, "2k": 2048, "4k": 4096}[q]
	p := strings.Split(a, ":")
	x, _ := strconv.Atoi(p[0])
	y, _ := strconv.Atoi(p[1])
	if x >= y {
		return round16(base), round16(base * y / x)
	}
	return round16(base * x / y), round16(base)
}
func videoDimensions(a, r string) (int, int) {
	if r == "1080p" {
		if a == "16:9" {
			return 1920, 1080
		}
		return 1080, 1920
	}
	if a == "16:9" {
		return 1280, 720
	}
	return 720, 1280
}
func round16(v int) int { return ((v + 8) / 16) * 16 }
