package firefly

import (
	"fmt"
	"strings"
	"time"
)

type Payload map[string]any

type sizeSpec struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}
type referenceBlob struct {
	ID              string `json:"id"`
	Usage           string `json:"usage"`
	PromptReference int    `json:"promptReference,omitempty"`
}

const maxPromptRunes = 4000

func buildImagePayloadCandidates(params *ResolvedParams, prompt string, referenceIDs []string) []Payload {
	if params != nil && params.UpstreamModelID == "gpt-image" {
		return buildGPTImagePayloadCandidates(params, prompt, referenceIDs)
	}
	prompt = firstRunes(prompt, maxPromptRunes)
	seed := int(time.Now().UnixMilli() % 999999)
	placeholder := sizeSpec{Width: 2048, Height: 2048}
	switch params.Quality {
	case "1k":
		placeholder = sizeSpec{1024, 1024}
	case "4k":
		placeholder = sizeSpec{4096, 4096}
	}
	// nano-banana-v2 的 HAR 固定使用 1024 方形占位，真正比例仍由 modelSpecificPayload 决定。
	if params.UpstreamModelVersion == "nano-banana-3" {
		placeholder = sizeSpec{1024, 1024}
	}
	modelSpecificPayload := map[string]any{"parameters": map[string]any{"addWatermark": false}}
	base := Payload{"modelId": params.UpstreamModelID, "modelVersion": params.UpstreamModelVersion, "n": 1, "prompt": prompt, "size": placeholder, "seeds": []int{seed}, "groundSearch": false, "output": map[string]any{"storeInputs": true}, "generationMetadata": map[string]string{"module": "text2image", "submodule": "ff-image-generate"}, "modelSpecificPayload": modelSpecificPayload}
	if params.AspectRatio != "auto" {
		modelSpecificPayload["aspectRatio"] = params.AspectRatio
	}
	blobs := make([]referenceBlob, 0, len(referenceIDs))
	for _, id := range referenceIDs {
		if strings.TrimSpace(id) != "" {
			blobs = append(blobs, referenceBlob{ID: id, Usage: "general"})
		}
	}
	base["referenceBlobs"] = blobs
	if len(blobs) == 0 || params.AspectRatio == "auto" {
		return []Payload{base}
	}
	// 浏览器形态优先；随后依次回退到不带 aspect、legacy image2image、多图退化首图。
	c1 := clonePayload(base)
	c2 := clonePayload(base)
	c2["modelSpecificPayload"] = map[string]any{"parameters": map[string]any{"addWatermark": false}}
	c3 := clonePayload(base)
	c3["generationMetadata"] = map[string]string{"module": "image2image", "submodule": "ff-image-generate"}
	out := []Payload{c1, c2, c3}
	if len(blobs) > 1 {
		c4 := clonePayload(c2)
		c4["referenceBlobs"] = []referenceBlob{blobs[0]}
		c5 := clonePayload(c3)
		c5["referenceBlobs"] = []referenceBlob{blobs[0]}
		out = append(out, c4, c5)
	}
	return out
}

func buildGPTImagePayloadCandidates(params *ResolvedParams, prompt string, referenceIDs []string) []Payload {
	prompt = firstRunes(prompt, maxPromptRunes)
	seed := int(time.Now().UnixMilli() % 999999)
	detailLevel := map[string]int{"1k": 1, "2k": 3, "4k": 5}[params.Quality]
	if detailLevel == 0 {
		detailLevel = 3
	}
	size := fmt.Sprintf("%dx%d", params.Width, params.Height)
	base := Payload{
		"modelId":      "gpt-image",
		"modelVersion": "2",
		"n":            1,
		"prompt":       prompt,
		"seeds":        []int{seed},
		"output":       map[string]any{"storeInputs": true},
		"generationMetadata": map[string]string{
			"module": "text2image", "submodule": "ff-image-generate",
		},
		"modelSpecificPayload": map[string]any{"size": size},
		"generationSettings":   map[string]any{"detailLevel": detailLevel},
	}
	blobs := make([]referenceBlob, 0, len(referenceIDs))
	for _, id := range referenceIDs {
		if strings.TrimSpace(id) != "" {
			blobs = append(blobs, referenceBlob{ID: id, Usage: "subject"})
		}
	}
	if len(blobs) == 0 {
		c1 := clonePayload(base)
		c1["referenceBlobs"] = []referenceBlob{}
		c2 := clonePayload(base)
		delete(c2, "modelSpecificPayload")
		c2["size"] = sizeSpec{Width: params.Width, Height: params.Height}
		c2["referenceBlobs"] = []referenceBlob{}
		c3 := clonePayload(c2)
		delete(c3, "referenceBlobs")
		return []Payload{c1, c2, c3}
	}
	c1 := clonePayload(base)
	c1["modelSpecificPayload"] = map[string]any{"size": "auto"}
	c1["referenceBlobs"] = blobs
	c2 := clonePayload(base)
	c2["referenceBlobs"] = blobs
	return []Payload{c1, c2}
}

func ValidateVideoReferenceCount(params *ResolvedParams, count int) error {
	if params == nil || params.Type != ModelTypeVideo || count < 0 {
		return fmt.Errorf("invalid video reference configuration")
	}
	maxReferences := 2
	if params.Engine == "sora2" {
		maxReferences = 1
	} else if params.ReferenceMode == "image" {
		maxReferences = 3
	}
	if count > maxReferences {
		return fmt.Errorf("too many reference images")
	}
	return nil
}

func buildVideoPayload(params *ResolvedParams, prompt string, referenceIDs []string) (Payload, error) {
	if err := ValidateVideoReferenceCount(params, len(referenceIDs)); err != nil {
		return nil, err
	}
	prompt = firstRunes(prompt, maxPromptRunes)
	seed := int(time.Now().UnixMilli() % 999999)
	size := sizeSpec{params.Width, params.Height}
	audio := true
	if params.GenerateAudioSet {
		audio = params.GenerateAudio
	}
	base := Payload{"n": 1, "seeds": []int{seed}, "output": map[string]any{"storeInputs": true}, "prompt": prompt, "size": size, "duration": params.Duration, "generateAudio": audio, "referenceBlobs": []any{}, "generationMetadata": map[string]string{"module": "text2video", "submodule": "ff-video-generate"}}
	if params.Engine == "sora2" {
		base["modelId"] = "sora"
		if strings.Contains(params.UpstreamModel, "pro") {
			base["modelVersion"] = "sora-2-pro"
		} else {
			base["modelVersion"] = "sora-2"
		}
		base["negativePrompt"] = "cartoon, vector art, & bad aesthetics & poor aesthetic"
		if len(referenceIDs) == 1 {
			base["referenceBlobs"] = []referenceBlob{{ID: referenceIDs[0], Usage: "general", PromptReference: 1}}
		}
		return base, nil
	}
	version := map[string]string{"veo31-standard": "3.1-generate", "veo31-fast": "3.1-fast-generate", "veo31-lite": "3.1-lite-generate"}[params.Engine]
	if version == "" {
		return nil, fmt.Errorf("unsupported video engine")
	}
	base["modelId"] = "veo"
	base["modelVersion"] = version
	base["negativePrompt"] = ""
	usage := "general"
	if params.ReferenceMode == "image" {
		usage = "asset"
	} else if params.ReferenceMode == "frame" || params.Engine == "veo31-fast" || params.Engine == "veo31-lite" {
		usage = "frame"
	}
	blobs := make([]referenceBlob, 0, len(referenceIDs))
	for i, id := range referenceIDs {
		b := referenceBlob{ID: id, Usage: usage}
		if usage != "asset" {
			b.PromptReference = i + 1
		}
		blobs = append(blobs, b)
	}
	base["referenceBlobs"] = blobs
	return base, nil
}

func clonePayload(src Payload) Payload {
	dst := make(Payload, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
func firstRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
