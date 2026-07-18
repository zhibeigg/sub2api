package firefly

type ModelType string

const (
	ModelTypeImage ModelType = "image"
	ModelTypeVideo ModelType = "video"
)

type ModelInfo struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"display_name"`
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Sizes       []string `json:"sizes,omitempty"`
	Qualities   []string `json:"qualities,omitempty"`
	Durations   []int    `json:"durations,omitempty"`
	Resolutions []string `json:"resolutions,omitempty"`
}

type modelDef struct {
	kind                 ModelType
	upstreamModelID      string
	upstreamModelVersion string
	upstreamModel        string
	engine               string
	defaultReferenceMode string
}

var publicModels = []ModelInfo{
	{ID: "nano-banana-pro", DisplayName: "Nano Banana Pro", Type: string(ModelTypeImage), Sizes: []string{"auto", "1024x1024", "1024x576", "576x1024"}, Qualities: []string{"1k", "2k", "4k"}},
	{ID: "nano-banana-v2", DisplayName: "Nano Banana V2", Type: string(ModelTypeImage), Sizes: []string{"auto", "1024x1024", "1024x576", "576x1024"}, Qualities: []string{"1k", "2k", "4k"}},
	{ID: "nano-banana", DisplayName: "Nano Banana", Type: string(ModelTypeImage), Sizes: []string{"auto", "1024x1024", "1024x576", "576x1024"}, Qualities: []string{"1k", "2k", "4k"}},
	{ID: "veo3", DisplayName: "Veo 3", Type: string(ModelTypeVideo), Durations: []int{4, 6, 8}, Resolutions: []string{"720p", "1080p"}},
	{ID: "veo3.1", DisplayName: "Veo 3.1", Type: string(ModelTypeVideo), Durations: []int{4, 6, 8}, Resolutions: []string{"720p", "1080p"}},
	{ID: "sora", DisplayName: "Sora 2", Type: string(ModelTypeVideo), Durations: []int{4, 8, 12}, Resolutions: []string{"720p"}},
	{ID: "sora-2-pro", DisplayName: "Sora 2 Pro", Type: string(ModelTypeVideo), Durations: []int{4, 8, 12}, Resolutions: []string{"720p"}},
}

var aliases = map[string]modelDef{
	"nano-banana-pro": {kind: ModelTypeImage, upstreamModelID: "gemini-flash", upstreamModelVersion: "nano-banana-2"},
	"nano-banana-v2":  {kind: ModelTypeImage, upstreamModelID: "gemini-flash", upstreamModelVersion: "nano-banana-3"},
	"nano-banana":     {kind: ModelTypeImage, upstreamModelID: "gemini-flash", upstreamModelVersion: "nano-banana-2"},
	// 隐藏图片别名：仅供 Adobe 账号显式模型映射使用，不出现在 PublicModels。
	"gpt-image-2": {kind: ModelTypeImage, upstreamModelID: "gpt-image", upstreamModelVersion: "2"},
	"veo3":        {kind: ModelTypeVideo, engine: "veo31-fast"},
	"veo3.1":      {kind: ModelTypeVideo, engine: "veo31-standard"},
	"sora":        {kind: ModelTypeVideo, engine: "sora2"},
	"sora-2-pro":  {kind: ModelTypeVideo, engine: "sora2", upstreamModel: "openai:firefly:colligo:sora2-pro"},
	// 隐藏兼容别名，只参与解析，不出现在 PublicModels。
	"sora2":         {kind: ModelTypeVideo, engine: "sora2"},
	"sora2-pro":     {kind: ModelTypeVideo, engine: "sora2", upstreamModel: "openai:firefly:colligo:sora2-pro"},
	"veo3.1-fast":   {kind: ModelTypeVideo, engine: "veo31-fast"},
	"veo3.1-flash":  {kind: ModelTypeVideo, engine: "veo31-fast"},
	"veo3.1-lite":   {kind: ModelTypeVideo, engine: "veo31-lite"},
	"veo3.1-ref":    {kind: ModelTypeVideo, engine: "veo31-standard", defaultReferenceMode: "image"},
	"firefly-sora2": {kind: ModelTypeVideo, engine: "sora2"},
	"firefly-veo31": {kind: ModelTypeVideo, engine: "veo31-standard"},
	"firefly-veo3":  {kind: ModelTypeVideo, engine: "veo31-fast"},
}

func PublicModels() []ModelInfo {
	out := make([]ModelInfo, len(publicModels))
	copy(out, publicModels)
	return out
}

func PublicModelIDs() []string {
	out := make([]string, 0, len(publicModels))
	for _, model := range publicModels {
		out = append(out, model.ID)
	}
	return out
}

func IsKnownAlias(model string) bool { _, ok := aliases[model]; return ok }
func IsImageAlias(model string) bool {
	def, ok := aliases[model]
	return ok && def.kind == ModelTypeImage
}
func IsVideoAlias(model string) bool {
	def, ok := aliases[model]
	return ok && def.kind == ModelTypeVideo
}
