package service

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type imageUploadTestPart struct {
	name        string
	filename    string
	contentType string
	value       []byte
}

type blockingImageUploadBody struct {
	closed chan struct{}
	once   sync.Once
}

func newBlockingImageUploadBody() *blockingImageUploadBody {
	return &blockingImageUploadBody{closed: make(chan struct{})}
}

func (b *blockingImageUploadBody) Read([]byte) (int, error) {
	<-b.closed
	return 0, io.ErrClosedPipe
}

func (b *blockingImageUploadBody) Close() error {
	b.once.Do(func() { close(b.closed) })
	return nil
}

type panickingImageUploadBody struct{}

func (panickingImageUploadBody) Read([]byte) (int, error) { panic("upload reader panic") }
func (panickingImageUploadBody) Close() error             { return nil }

func validPNGBytes(extra int) []byte {
	data := append([]byte(nil), []byte("\x89PNG\r\n\x1a\n")...)
	return append(data, bytes.Repeat([]byte{0x01}, extra)...)
}

func newImageUploadTestContext(t *testing.T, ctx context.Context, parts []imageUploadTestPart) *gin.Context {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, item := range parts {
		if item.filename == "" {
			require.NoError(t, writer.WriteField(item.name, string(item.value)))
			continue
		}
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", `form-data; name="`+item.name+`"; filename="`+item.filename+`"`)
		header.Set("Content-Type", item.contentType)
		part, err := writer.CreatePart(header)
		require.NoError(t, err)
		_, err = part.Write(item.value)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(body.Bytes())).WithContext(ctx)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	return c
}

func newRawImageUploadTestContext(ctx context.Context, body io.ReadCloser) *gin.Context {
	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil).WithContext(ctx)
	req.Body = body
	req.ContentLength = -1
	req.Header.Set("Content-Type", "multipart/form-data; boundary=test-boundary")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	return c
}

func newImageUploadTempTestService(t *testing.T, cfg *config.Config) *OpenAIImageUploadTempService {
	t.Helper()
	t.Setenv("DATA_DIR", t.TempDir())
	return NewOpenAIImageUploadTempService(cfg)
}

func TestOpenAIImageUploadTempServiceParsePreservesPartsAndCleans(t *testing.T) {
	svc := newImageUploadTempTestService(t, nil)
	c := newImageUploadTestContext(t, context.Background(), []imageUploadTestPart{
		{name: "prompt", value: []byte("replace background")},
		{name: "compat_field", value: []byte("keep-me")},
		{name: "image", filename: "source.png", contentType: "image/png", value: validPNGBytes(8)},
		{name: "model", value: []byte("gpt-image-2")},
	})

	parsed, err := svc.ParseRequest(c)
	require.NoError(t, err)
	require.Len(t, parsed.MultipartParts, 4)
	require.Equal(t, []string{"prompt", "compat_field", "image", "model"}, []string{
		parsed.MultipartParts[0].FormName,
		parsed.MultipartParts[1].FormName,
		parsed.MultipartParts[2].FormName,
		parsed.MultipartParts[3].FormName,
	})
	require.Equal(t, []byte("keep-me"), parsed.MultipartParts[1].Value)
	require.Len(t, parsed.Uploads, 1)
	require.Empty(t, parsed.Uploads[0].Data)
	require.FileExists(t, parsed.Uploads[0].FilePath)
	require.Equal(t, "image/png", parsed.Uploads[0].ContentType)

	requestDir := parsed.TempDir
	parsed.Cleanup()
	parsed.Cleanup()
	_, statErr := os.Stat(requestDir)
	require.True(t, os.IsNotExist(statErr))
}

func TestOpenAIImageUploadTempServiceAcceptsRepeatedImageFieldForms(t *testing.T) {
	svc := newImageUploadTempTestService(t, nil)
	c := newImageUploadTestContext(t, context.Background(), []imageUploadTestPart{
		{name: "image", filename: "one.png", contentType: "image/png", value: validPNGBytes(1)},
		{name: "image[]", filename: "two.png", contentType: "image/png", value: validPNGBytes(2)},
		{name: "image[]", filename: "three.png", contentType: "image/png", value: validPNGBytes(3)},
		{name: "image[0]", filename: "four.png", contentType: "image/png", value: validPNGBytes(4)},
		{name: "model", value: []byte("gpt-image-2")},
	})

	parsed, err := svc.ParseRequest(c)
	require.NoError(t, err)
	defer parsed.Cleanup()
	require.Len(t, parsed.Uploads, 4)
	require.Equal(t, []string{"image", "image[]", "image[]", "image[0]"}, []string{
		parsed.Uploads[0].FieldName,
		parsed.Uploads[1].FieldName,
		parsed.Uploads[2].FieldName,
		parsed.Uploads[3].FieldName,
	})
}

func TestOpenAIImageUploadTempServiceRejectsMalformedImageArrayField(t *testing.T) {
	svc := newImageUploadTempTestService(t, nil)
	c := newImageUploadTestContext(t, context.Background(), []imageUploadTestPart{
		{name: "image[broken", filename: "one.png", contentType: "image/png", value: validPNGBytes(1)},
		{name: "model", value: []byte("gpt-image-2")},
	})

	_, err := svc.ParseRequest(c)
	require.EqualError(t, err, "image file is required")
	entries, readErr := os.ReadDir(svc.Root())
	require.NoError(t, readErr)
	require.Empty(t, entries)
}

func TestOpenAIImageUploadTempServiceLimits(t *testing.T) {
	t.Run("file count", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Gateway.ImageUploadMaxFiles = 1
		svc := newImageUploadTempTestService(t, cfg)
		c := newImageUploadTestContext(t, context.Background(), []imageUploadTestPart{
			{name: "image", filename: "one.png", contentType: "image/png", value: validPNGBytes(0)},
			{name: "mask", filename: "two.png", contentType: "image/png", value: validPNGBytes(0)},
		})
		_, err := svc.ParseRequest(c)
		require.Error(t, err)
		require.True(t, IsOpenAIImageUploadLimitError(err))
	})

	t.Run("single file max plus one", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Gateway.ImageUploadMaxFileBytes = 8
		cfg.Gateway.ImageUploadMaxTotalBytes = 64
		svc := newImageUploadTempTestService(t, cfg)
		c := newImageUploadTestContext(t, context.Background(), []imageUploadTestPart{
			{name: "image", filename: "large.png", contentType: "image/png", value: validPNGBytes(1)},
		})
		_, err := svc.ParseRequest(c)
		require.Error(t, err)
		require.True(t, IsOpenAIImageUploadLimitError(err))
	})

	t.Run("text field", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Gateway.ImageUploadMaxTextFieldBytes = 4
		cfg.Gateway.ImageUploadMaxTotalBytes = 64
		svc := newImageUploadTempTestService(t, cfg)
		c := newImageUploadTestContext(t, context.Background(), []imageUploadTestPart{{name: "prompt", value: []byte("12345")}})
		_, err := svc.ParseRequest(c)
		require.Error(t, err)
		require.True(t, IsOpenAIImageUploadLimitError(err))
	})
}

func TestOpenAIImageUploadTempServiceValidatesMIMEAndMagic(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		data        []byte
	}{
		{name: "mismatched mime", contentType: "image/jpeg", data: validPNGBytes(0)},
		{name: "unsupported signature", contentType: "image/png", data: []byte("not-an-image")},
		{name: "non image mime", contentType: "text/plain", data: validPNGBytes(0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newImageUploadTempTestService(t, nil)
			c := newImageUploadTestContext(t, context.Background(), []imageUploadTestPart{{name: "image", filename: "input.bin", contentType: tt.contentType, value: tt.data}})
			_, err := svc.ParseRequest(c)
			require.Error(t, err)
			require.False(t, IsOpenAIImageUploadLimitError(err))
		})
	}
}

func TestOpenAIImageUploadTempServiceCancellationAndStaleCleanup(t *testing.T) {
	t.Run("cancelled parse removes request directory", func(t *testing.T) {
		svc := newImageUploadTempTestService(t, nil)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		c := newImageUploadTestContext(t, ctx, []imageUploadTestPart{{name: "image", filename: "input.png", contentType: "image/png", value: validPNGBytes(0)}})
		_, err := svc.ParseRequest(c)
		require.ErrorIs(t, err, context.Canceled)
		entries, readErr := os.ReadDir(svc.Root())
		require.NoError(t, readErr)
		require.Empty(t, entries)
	})

	t.Run("startup removes stale directory only inside root", func(t *testing.T) {
		svc := newImageUploadTempTestService(t, nil)
		require.NoError(t, svc.ensureRoot())
		staleDir := filepath.Join(svc.Root(), "request-stale")
		freshDir := filepath.Join(svc.Root(), "request-fresh")
		require.NoError(t, os.Mkdir(staleDir, 0o700))
		require.NoError(t, os.Mkdir(freshDir, 0o700))
		old := time.Now().Add(-7 * time.Hour)
		require.NoError(t, os.Chtimes(staleDir, old, old))

		svc.Start()
		t.Cleanup(svc.Stop)
		_, staleErr := os.Stat(staleDir)
		require.True(t, os.IsNotExist(staleErr))
		require.DirExists(t, freshDir)

		outside := t.TempDir()
		require.Error(t, svc.CleanupRequest(outside))
		require.DirExists(t, outside)
	})
}

func TestOpenAIImageUploadTempServiceTimeoutInterruptsBlockedReadAndCleans(t *testing.T) {
	svc := newImageUploadTempTestService(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	body := newBlockingImageUploadBody()
	c := newRawImageUploadTestContext(ctx, body)

	_, err := svc.ParseRequest(c)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	entries, readErr := os.ReadDir(svc.Root())
	require.NoError(t, readErr)
	require.Empty(t, entries)
}

func TestOpenAIImageUploadTempServiceParserPanicCleansRequestDirectory(t *testing.T) {
	svc := newImageUploadTempTestService(t, nil)
	c := newRawImageUploadTestContext(context.Background(), panickingImageUploadBody{})

	require.PanicsWithValue(t, "upload reader panic", func() {
		_, _ = svc.ParseRequest(c)
	})
	entries, readErr := os.ReadDir(svc.Root())
	require.NoError(t, readErr)
	require.Empty(t, entries)
}

func TestOpenAIImagesMultipartStreamRebuildsFromFiles(t *testing.T) {
	svc := newImageUploadTempTestService(t, nil)
	c := newImageUploadTestContext(t, context.Background(), []imageUploadTestPart{
		{name: "unknown", value: []byte("value")},
		{name: "model", value: []byte("gpt-image-1")},
		{name: "image", filename: "source.png", contentType: "image/png", value: validPNGBytes(3)},
	})
	parsed, err := svc.ParseRequest(c)
	require.NoError(t, err)
	defer parsed.Cleanup()

	body, contentType, err := newOpenAIImagesMultipartStream(parsed, "gpt-image-2")
	require.NoError(t, err)
	defer body.Close()
	_, params, err := mime.ParseMediaType(contentType)
	require.NoError(t, err)
	reader := multipart.NewReader(body, params["boundary"])

	var names []string
	values := map[string][]byte{}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		data, err := io.ReadAll(part)
		require.NoError(t, err)
		names = append(names, part.FormName())
		values[part.FormName()] = data
	}
	require.Equal(t, []string{"unknown", "model", "image"}, names)
	require.Equal(t, []byte("value"), values["unknown"])
	require.Equal(t, []byte("gpt-image-2"), values["model"])
	require.Equal(t, validPNGBytes(3), values["image"])
}
