package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
)

const (
	defaultOpenAIImageUploadMaxFiles        = 4
	defaultOpenAIImageUploadMaxFileBytes    = int64(20 << 20)
	defaultOpenAIImageUploadMaxTotalBytes   = int64(80 << 20)
	defaultOpenAIImageUploadMaxTextBytes    = int64(64 << 10)
	defaultOpenAIImageUploadTempTTL         = 6 * time.Hour
	defaultOpenAIImageUploadCleanupInterval = 30 * time.Minute
)

type OpenAIImageUploadLimitError struct {
	Message string
}

func (e *OpenAIImageUploadLimitError) Error() string {
	if e == nil {
		return "image upload limit exceeded"
	}
	return e.Message
}

func IsOpenAIImageUploadLimitError(err error) bool {
	var limitErr *OpenAIImageUploadLimitError
	return errors.As(err, &limitErr)
}

type OpenAIImagesMultipartPart struct {
	Header   textproto.MIMEHeader
	FormName string
	FileName string
	FilePath string
	Value    []byte
	IsFile   bool
	Size     int64
}

type OpenAIImageUploadTempService struct {
	root            string
	maxFiles        int
	maxFileBytes    int64
	maxTotalBytes   int64
	maxTextBytes    int64
	ttl             time.Duration
	cleanupInterval time.Duration

	mu      sync.Mutex
	started bool
	stopCh  chan struct{}
	doneCh  chan struct{}
}

func NewOpenAIImageUploadTempService(cfg *config.Config) *OpenAIImageUploadTempService {
	maxFiles := defaultOpenAIImageUploadMaxFiles
	maxFileBytes := defaultOpenAIImageUploadMaxFileBytes
	maxTotalBytes := defaultOpenAIImageUploadMaxTotalBytes
	maxTextBytes := defaultOpenAIImageUploadMaxTextBytes
	ttl := defaultOpenAIImageUploadTempTTL
	cleanupInterval := defaultOpenAIImageUploadCleanupInterval
	if cfg != nil {
		if cfg.Gateway.ImageUploadMaxFiles > 0 {
			maxFiles = cfg.Gateway.ImageUploadMaxFiles
		}
		if cfg.Gateway.ImageUploadMaxFileBytes > 0 {
			maxFileBytes = cfg.Gateway.ImageUploadMaxFileBytes
		}
		if cfg.Gateway.ImageUploadMaxTotalBytes > 0 {
			maxTotalBytes = cfg.Gateway.ImageUploadMaxTotalBytes
		}
		if cfg.Gateway.ImageUploadMaxTextFieldBytes > 0 {
			maxTextBytes = cfg.Gateway.ImageUploadMaxTextFieldBytes
		}
		if cfg.Gateway.ImageUploadTempTTLHours > 0 {
			ttl = time.Duration(cfg.Gateway.ImageUploadTempTTLHours) * time.Hour
		}
		if cfg.Gateway.ImageUploadCleanupIntervalMinutes > 0 {
			cleanupInterval = time.Duration(cfg.Gateway.ImageUploadCleanupIntervalMinutes) * time.Minute
		}
	}
	dataDir := strings.TrimSpace(os.Getenv("DATA_DIR"))
	if dataDir == "" {
		if info, err := os.Stat("/app/data"); err == nil && info.IsDir() {
			dataDir = "/app/data"
		} else {
			dataDir = "."
		}
	}
	return &OpenAIImageUploadTempService{
		root:            filepath.Join(dataDir, "tmp", "playground-images"),
		maxFiles:        maxFiles,
		maxFileBytes:    maxFileBytes,
		maxTotalBytes:   maxTotalBytes,
		maxTextBytes:    maxTextBytes,
		ttl:             ttl,
		cleanupInterval: cleanupInterval,
	}
}

func ProvideOpenAIImageUploadTempService(cfg *config.Config) *OpenAIImageUploadTempService {
	svc := NewOpenAIImageUploadTempService(cfg)
	svc.Start()
	return svc
}

func (s *OpenAIImageUploadTempService) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}

func (s *OpenAIImageUploadTempService) Start() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.stopCh = make(chan struct{})
	s.doneCh = make(chan struct{})
	stopCh := s.stopCh
	doneCh := s.doneCh
	s.mu.Unlock()

	_ = s.CleanupStale(time.Now())
	go func() {
		defer close(doneCh)
		ticker := time.NewTicker(s.cleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = s.CleanupStale(time.Now())
			case <-stopCh:
				return
			}
		}
	}()
}

func (s *OpenAIImageUploadTempService) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return
	}
	s.started = false
	close(s.stopCh)
	doneCh := s.doneCh
	s.mu.Unlock()
	<-doneCh
}

func (s *OpenAIImageUploadTempService) ensureRoot() error {
	if s == nil || strings.TrimSpace(s.root) == "" {
		return fmt.Errorf("image upload temporary root is not configured")
	}
	if info, err := os.Lstat(s.root); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("image upload temporary root is unsafe")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return fmt.Errorf("create image upload temporary root: %w", err)
	}
	info, err := os.Lstat(s.root)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("image upload temporary root is unsafe")
	}
	return nil
}

func (s *OpenAIImageUploadTempService) safeRequestDir(path string) (string, error) {
	rootAbs, err := filepath.Abs(filepath.Clean(s.root))
	if err != nil {
		return "", err
	}
	pathAbs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || strings.Contains(rel, string(os.PathSeparator)) {
		return "", fmt.Errorf("unsafe image upload request directory")
	}
	return pathAbs, nil
}

func (s *OpenAIImageUploadTempService) CleanupRequest(path string) error {
	if s == nil || strings.TrimSpace(path) == "" {
		return nil
	}
	safePath, err := s.safeRequestDir(path)
	if err != nil {
		return err
	}
	info, err := os.Lstat(safePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("unsafe image upload request directory")
	}
	return os.RemoveAll(safePath)
}

func (s *OpenAIImageUploadTempService) CleanupStale(now time.Time) error {
	if err := s.ensureRoot(); err != nil {
		return err
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return err
	}
	cutoff := now.Add(-s.ttl)
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			continue
		}
		path, safeErr := s.safeRequestDir(filepath.Join(s.root, entry.Name()))
		if safeErr != nil {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.RemoveAll(path)
	}
	return nil
}

func (s *OpenAIImageUploadTempService) ParseRequest(c *gin.Context) (*OpenAIImagesRequest, error) {
	if c == nil || c.Request == nil {
		return nil, fmt.Errorf("missing request context")
	}
	endpoint := normalizeOpenAIImagesEndpointPath(c.Request.URL.Path)
	if endpoint == "" {
		return nil, fmt.Errorf("unsupported images endpoint")
	}
	contentType := strings.TrimSpace(c.GetHeader("Content-Type"))
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		return nil, fmt.Errorf("invalid multipart content-type")
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return nil, fmt.Errorf("multipart boundary is required")
	}
	if err := s.ensureRoot(); err != nil {
		return nil, err
	}
	requestDir, err := os.MkdirTemp(s.root, "request-")
	if err != nil {
		return nil, fmt.Errorf("create image upload request directory: %w", err)
	}
	ownershipTransferred := false
	defer func() {
		if !ownershipTransferred {
			_ = s.CleanupRequest(requestDir)
		}
	}()
	stopBodyClose := context.AfterFunc(c.Request.Context(), func() {
		_ = c.Request.Body.Close()
	})
	defer stopBodyClose()

	req := &OpenAIImagesRequest{
		Endpoint:          endpoint,
		ContentType:       contentType,
		Multipart:         true,
		MultipartBoundary: boundary,
		TempDir:           requestDir,
		N:                 1,
	}
	var cleanupOnce sync.Once
	req.cleanup = func() { cleanupOnce.Do(func() { _ = s.CleanupRequest(requestDir) }) }
	if err := s.parseMultipart(c.Request.Context(), c.Request.Body, boundary, req); err != nil {
		req.Cleanup()
		return nil, err
	}
	applyOpenAIImagesDefaults(req)
	if err := validateOpenAIImagesModel(req.Model); err != nil {
		req.Cleanup()
		return nil, err
	}
	if isOpenAIPlatformImageModel(req.Model) && !openAIImageModelSupportsEndpoint(req.Model, req.Endpoint) {
		req.Cleanup()
		return nil, fmt.Errorf("model %q does not support this images endpoint", req.Model)
	}
	req.SizeTier = normalizeOpenAIImageSizeTier(req.Size)
	req.RequiredCapability = classifyOpenAIImagesCapability(req)
	ownershipTransferred = true
	return req, nil
}

type openAIImageContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r openAIImageContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.reader.Read(p)
	if contextErr := r.ctx.Err(); contextErr != nil {
		return n, contextErr
	}
	return n, err
}

func (s *OpenAIImageUploadTempService) parseMultipart(ctx context.Context, body io.Reader, boundary string, req *OpenAIImagesRequest) error {
	reader := multipart.NewReader(openAIImageContextReader{ctx: ctx, reader: body}, boundary)
	fileCount := 0
	var totalBytes int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read multipart body: %w", err)
		}
		partHeader := cloneMultipartHeader(part.Header)
		name := strings.TrimSpace(part.FormName())
		fileName := strings.TrimSpace(part.FileName())
		if fileName == "" {
			data, size, readErr := readLimitedMultipartPart(part, s.maxTextBytes, s.maxTotalBytes-totalBytes)
			_ = part.Close()
			if readErr != nil {
				return readErr
			}
			totalBytes += size
			req.MultipartParts = append(req.MultipartParts, OpenAIImagesMultipartPart{Header: partHeader, FormName: name, Value: data, Size: size})
			if err := applyOpenAIImagesMultipartTextField(req, name, string(data)); err != nil {
				return err
			}
			continue
		}

		fileCount++
		if fileCount > s.maxFiles {
			_ = part.Close()
			return &OpenAIImageUploadLimitError{Message: fmt.Sprintf("multipart file count exceeds limit of %d", s.maxFiles)}
		}
		file, err := os.CreateTemp(req.TempDir, "part-")
		if err != nil {
			_ = part.Close()
			return fmt.Errorf("create image upload temporary file: %w", err)
		}
		filePath := file.Name()
		limit := s.maxFileBytes
		if remaining := s.maxTotalBytes - totalBytes; remaining < limit {
			limit = remaining
		}
		if limit < 0 {
			limit = 0
		}
		size, copyErr := io.Copy(file, io.LimitReader(part, limit+1))
		closeErr := file.Close()
		_ = part.Close()
		if copyErr != nil {
			return fmt.Errorf("read multipart file: %w", copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close image upload temporary file: %w", closeErr)
		}
		if size > s.maxFileBytes {
			return &OpenAIImageUploadLimitError{Message: fmt.Sprintf("multipart file exceeds limit of %d bytes", s.maxFileBytes)}
		}
		if totalBytes+size > s.maxTotalBytes {
			return &OpenAIImageUploadLimitError{Message: fmt.Sprintf("multipart body exceeds limit of %d bytes", s.maxTotalBytes)}
		}
		totalBytes += size
		detectedType, err := validateOpenAIImageUploadFile(filePath, partHeader.Get("Content-Type"))
		if err != nil {
			return err
		}
		width, height := parseOpenAIImageDimensions(partHeader)
		upload := OpenAIImagesUpload{FieldName: name, FileName: fileName, ContentType: detectedType, FilePath: filePath, Size: size, Width: width, Height: height}
		req.MultipartParts = append(req.MultipartParts, OpenAIImagesMultipartPart{Header: partHeader, FormName: name, FileName: fileName, FilePath: filePath, IsFile: true, Size: size})
		if name == "mask" {
			req.HasMask = true
			req.MaskUpload = &upload
		}
		if isOpenAIImageUploadFieldName(name) {
			req.Uploads = append(req.Uploads, upload)
		}
	}
	if len(req.Uploads) == 0 && req.IsEdits() {
		return fmt.Errorf("image file is required")
	}
	return nil
}

func readLimitedMultipartPart(part io.Reader, partLimit, totalRemaining int64) ([]byte, int64, error) {
	limit := partLimit
	if totalRemaining < limit {
		limit = totalRemaining
	}
	if limit < 0 {
		limit = 0
	}
	data, err := io.ReadAll(io.LimitReader(part, limit+1))
	if err != nil {
		return nil, 0, fmt.Errorf("read multipart field: %w", err)
	}
	size := int64(len(data))
	if size > partLimit {
		return nil, size, &OpenAIImageUploadLimitError{Message: fmt.Sprintf("multipart text field exceeds limit of %d bytes", partLimit)}
	}
	if size > totalRemaining {
		return nil, size, &OpenAIImageUploadLimitError{Message: "multipart body exceeds total size limit"}
	}
	return data, size, nil
}

func validateOpenAIImageUploadFile(path, declaredContentType string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	var header [16]byte
	n, readErr := io.ReadFull(file, header[:])
	_ = file.Close()
	if readErr != nil && readErr != io.ErrUnexpectedEOF {
		return "", fmt.Errorf("read image upload signature: %w", readErr)
	}
	detected := detectOpenAIImageMIME(header[:n])
	if detected == "" {
		return "", fmt.Errorf("unsupported image file signature; only PNG, JPEG, and WebP are allowed")
	}
	declared, _, err := mime.ParseMediaType(strings.TrimSpace(declaredContentType))
	if err != nil && strings.TrimSpace(declaredContentType) != "" {
		return "", fmt.Errorf("invalid image content-type")
	}
	declared = strings.ToLower(strings.TrimSpace(declared))
	if declared != "" && declared != "application/octet-stream" && declared != detected {
		return "", fmt.Errorf("image content-type does not match file signature")
	}
	return detected, nil
}

func detectOpenAIImageMIME(header []byte) string {
	if len(header) >= 8 && string(header[:8]) == "\x89PNG\r\n\x1a\n" {
		return "image/png"
	}
	if len(header) >= 3 && header[0] == 0xff && header[1] == 0xd8 && header[2] == 0xff {
		return "image/jpeg"
	}
	if len(header) >= 12 && string(header[:4]) == "RIFF" && string(header[8:12]) == "WEBP" {
		return "image/webp"
	}
	return ""
}

func applyOpenAIImagesMultipartTextField(req *OpenAIImagesRequest, name, rawValue string) error {
	value := strings.TrimSpace(rawValue)
	switch name {
	case "model":
		req.Model = value
		req.ExplicitModel = value != ""
	case "prompt":
		req.Prompt = value
	case "size":
		req.Size = value
		req.ExplicitSize = value != ""
	case "response_format":
		req.ResponseFormat = strings.ToLower(value)
	case "stream":
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid stream field value")
		}
		req.Stream = parsed
	case "n":
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 {
			return fmt.Errorf("n must be a positive integer")
		}
		req.N = n
	case "quality":
		req.Quality = value
		req.HasNativeOptions = true
	case "background":
		req.Background = value
		req.HasNativeOptions = true
	case "output_format":
		req.OutputFormat = value
		req.HasNativeOptions = true
	case "moderation":
		req.Moderation = value
		req.HasNativeOptions = true
	case "input_fidelity":
		req.InputFidelity = value
		req.HasNativeOptions = true
	case "style":
		req.Style = value
		req.HasNativeOptions = true
	case "output_compression":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid output_compression field value")
		}
		req.OutputCompression = &n
		req.HasNativeOptions = true
	case "partial_images":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid partial_images field value")
		}
		req.PartialImages = &n
		req.HasNativeOptions = true
	default:
		if isOpenAINativeImageOption(name) && value != "" {
			req.HasNativeOptions = true
		}
	}
	return nil
}
