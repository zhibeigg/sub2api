package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseArkVideoRequest(t *testing.T) {
	body := []byte(`{"model":"doubao-seedance-2-0","prompt":"a cat","seconds":5,"resolution":"720p","ratio":"16:9"}`)
	info := parseArkVideoRequest(body)
	if info.Model != "doubao-seedance-2-0" || info.Prompt != "a cat" || info.Seconds != 5 || info.Resolution != "720p" || info.Ratio != "16:9" {
		t.Fatalf("unexpected parse: %+v", info)
	}

	// alias fields: input / duration / size / aspect_ratio
	body2 := []byte(`{"model":"m","input":"dog","duration":10,"size":"1080p","aspect_ratio":"9:16"}`)
	info2 := parseArkVideoRequest(body2)
	if info2.Prompt != "dog" || info2.Seconds != 10 || info2.Resolution != "1080p" || info2.Ratio != "9:16" {
		t.Fatalf("unexpected alias parse: %+v", info2)
	}
}

func TestBuildArkVideoTaskBody(t *testing.T) {
	info := arkVideoSubmitInfo{Prompt: "a red balloon", Seconds: 5, Resolution: "720p", Ratio: "16:9"}
	raw, err := buildArkVideoTaskBody(info, "doubao-seedance-1-0-pro-250528")
	if err != nil {
		t.Fatalf("build err: %v", err)
	}
	var m struct {
		Model   string `json:"model"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m.Model != "doubao-seedance-1-0-pro-250528" {
		t.Errorf("model = %q", m.Model)
	}
	if len(m.Content) != 1 || m.Content[0].Type != "text" {
		t.Fatalf("content shape wrong: %+v", m.Content)
	}
	text := m.Content[0].Text
	for _, want := range []string{"a red balloon", "--dur 5", "--rs 720p", "--rt 16:9"} {
		if !strings.Contains(text, want) {
			t.Errorf("text missing %q: %q", want, text)
		}
	}
}

func TestBuildArkVideoTaskBodyAdaptiveRatioSkipsFlag(t *testing.T) {
	info := arkVideoSubmitInfo{Prompt: "x", Seconds: 5, Resolution: "480p", Ratio: "adaptive"}
	raw, _ := buildArkVideoTaskBody(info, "m")
	if strings.Contains(string(raw), "--rt") {
		t.Errorf("adaptive ratio should not emit --rt: %s", raw)
	}
	if !strings.Contains(string(raw), "--rs 480p") {
		t.Errorf("expected --rs 480p: %s", raw)
	}
}

func TestNormalizeArkResolution(t *testing.T) {
	cases := map[string]string{"720P": "720p", "1080p": "1080p", "480p": "480p", "4k": "", "": ""}
	for in, want := range cases {
		if got := normalizeArkResolution(in); got != want {
			t.Errorf("normalizeArkResolution(%q)=%q want %q", in, got, want)
		}
	}
}

func TestIsArkVideoModel(t *testing.T) {
	if !IsArkVideoModel("doubao-seedance-2-0-260128") || !IsArkVideoModel("doubao-seedance-1-0-pro") {
		t.Error("seedance models should be detected")
	}
	if IsArkVideoModel("deepseek-v4-flash") || IsArkVideoModel("glm-5.2") {
		t.Error("non-video models should not be detected")
	}
}

func TestArkVideoRequestSessionHash(t *testing.T) {
	if ArkVideoRequestSessionHash("") != "" {
		t.Error("empty id -> empty hash")
	}
	h := ArkVideoRequestSessionHash("cgt-123")
	if !strings.HasPrefix(h, "ark-video:") {
		t.Errorf("expected ark-video prefix, got %q", h)
	}
	if h != ArkVideoRequestSessionHash("cgt-123") {
		t.Error("hash should be deterministic")
	}
}

func TestArkVideoBaseURL(t *testing.T) {
	if got := arkVideoBaseURL("https://ark.cn-beijing.volces.com/api/v3/"); got != "https://ark.cn-beijing.volces.com/api/v3" {
		t.Errorf("trailing slash not trimmed: %q", got)
	}
	if got := arkVideoBaseURL(""); got != "https://ark.cn-beijing.volces.com/api/v3" {
		t.Errorf("empty base default wrong: %q", got)
	}
}

func TestSeedanceFallbackPricing(t *testing.T) {
	svc := &BillingService{}
	svc.fallbackPrices = make(map[string]*ModelPricing)
	svc.initFallbackPricing()
	p := svc.getFallbackPricing("doubao-seedance-1-0-pro-250528")
	if p == nil || p.OutputPricePerToken <= 0 {
		t.Fatalf("seedance fallback pricing missing: %+v", p)
	}
}
