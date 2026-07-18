package firefly

import "testing"

func TestCatalogAndAliases(t *testing.T) {
	want := map[string]bool{"nano-banana-pro": true, "nano-banana-v2": true, "nano-banana": true, "veo3": true, "veo3.1": true, "sora": true, "sora-2-pro": true}
	for _, id := range PublicModelIDs() {
		delete(want, id)
	}
	if len(want) != 0 {
		t.Fatalf("missing public models: %v", want)
	}
	if !IsImageAlias("nano-banana-v2") || !IsImageAlias("gpt-image-2") || !IsVideoAlias("sora2-pro") || !IsKnownAlias("veo3.1-flash") {
		t.Fatal("alias classification failed")
	}
	for _, id := range PublicModelIDs() {
		if id == "gpt-image-2" {
			t.Fatal("hidden gpt-image-2 alias must not be publicly listed")
		}
	}
}
func TestResolveModels(t *testing.T) {
	p, err := ResolveImageModel("nano-banana-v2", "1024x768", "4k")
	if err != nil {
		t.Fatal(err)
	}
	if p.AspectRatio != "4:3" || p.Quality != "4k" || p.UpstreamModelVersion != "nano-banana-3" {
		t.Fatalf("bad image params: %+v", p)
	}
	hd, err := ResolveImageModel("nano-banana-pro", "16:9", "hd")
	if err != nil || hd.Quality != "4k" {
		t.Fatalf("hd quality mapping failed: %+v err=%v", hd, err)
	}
	if _, err := ResolveImageModel("nano-banana-pro", "16:9", "mystery"); err == nil {
		t.Fatal("unknown image quality must be rejected")
	}
	gpt, err := ResolveImageModel("gpt-image-2", "16:9", "4k")
	if err != nil || gpt.Width != 3328 || gpt.Height != 1872 || gpt.UpstreamModelID != "gpt-image" {
		t.Fatalf("bad hidden gpt-image params: %+v err=%v", gpt, err)
	}
	if _, err := ResolveImageModel("gpt-image-2", "auto", "2k"); err == nil {
		t.Fatal("gpt-image-2 must reject auto size")
	}
	v, err := ResolveVideoModel("sora-2-pro", "1080p", 8, "9:16", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if v.VideoResolution != "720p" || v.Width != 720 || v.Height != 1280 || !v.GenerateAudio {
		t.Fatalf("bad video params: %+v", v)
	}
	if _, err := ResolveVideoModel("veo3", "480p", 8, "16:9", nil, ""); err == nil {
		t.Fatal("480p must be rejected for Adobe video")
	}
}
func TestPayloadCandidates(t *testing.T) {
	p, _ := ResolveImageModel("nano-banana-pro", "16:9", "2k")
	items := buildImagePayloadCandidates(p, "prompt", []string{"a", "b"})
	if len(items) != 5 {
		t.Fatalf("candidates=%d", len(items))
	}
	m, ok := items[0]["modelSpecificPayload"].(map[string]any)
	if !ok {
		t.Fatalf("modelSpecificPayload has type %T", items[0]["modelSpecificPayload"])
	}
	if m["aspectRatio"] != "16:9" {
		t.Fatalf("aspect=%v", m["aspectRatio"])
	}
	gpt, _ := ResolveImageModel("gpt-image-2", "16:9", "4k")
	gptItems := buildImagePayloadCandidates(gpt, "prompt", []string{"subject"})
	if len(gptItems) != 2 || gptItems[0]["modelId"] != "gpt-image" {
		t.Fatalf("bad gpt-image candidates: %#v", gptItems)
	}
	gptSpecific, ok := gptItems[1]["modelSpecificPayload"].(map[string]any)
	if !ok {
		t.Fatalf("modelSpecificPayload has type %T", gptItems[1]["modelSpecificPayload"])
	}
	if gptSpecific["size"] != "3328x1872" {
		t.Fatalf("bad gpt-image size: %v", gptSpecific["size"])
	}
	v, _ := ResolveVideoModel("veo3.1", "1080p", 8, "16:9", nil, "image")
	payload, err := buildVideoPayload(v, "p", []string{"a", "b", "c"})
	if err != nil {
		t.Fatal(err)
	}
	if payload["modelVersion"] != "3.1-generate" {
		t.Fatal("wrong veo payload")
	}
	if err := ValidateVideoReferenceCount(v, 3); err != nil {
		t.Fatalf("veo image references rejected: %v", err)
	}
	fast, _ := ResolveVideoModel("veo3", "720p", 8, "16:9", nil, "frame")
	if err := ValidateVideoReferenceCount(fast, 3); err == nil {
		t.Fatal("veo frame mode accepted too many references")
	}
	sora, _ := ResolveVideoModel("sora", "720p", 8, "16:9", nil, "")
	if err := ValidateVideoReferenceCount(sora, 2); err == nil {
		t.Fatal("sora accepted too many references")
	}
}
