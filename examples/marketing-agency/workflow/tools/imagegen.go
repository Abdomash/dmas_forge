package tools

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	openai "github.com/openai/openai-go"
	"github.com/vaastav/agentic_blueprint/ai_runtime/core"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	imageProviderReal = "real"
	imageProviderMock = "mock"
)

const imageTracerName = "github.com/vaastav/agentic_blueprint/examples/marketing-agency/workflow/tools/imagegen"

type ImageProvider interface {
	Mode() string
	GenerateJPEG(ctx context.Context, prompt string) ([]byte, error)
}

type OpenAIImageProvider struct {
	client *openai.Client
}

func (p OpenAIImageProvider) Mode() string {
	return imageProviderReal
}

func (p OpenAIImageProvider) GenerateJPEG(ctx context.Context, prompt string) ([]byte, error) {
	return generateJPEG(ctx, p.client, prompt)
}

type MockImageProvider struct{}

func (p MockImageProvider) Mode() string {
	return imageProviderMock
}

func (p MockImageProvider) GenerateJPEG(ctx context.Context, prompt string) ([]byte, error) {
	return generateMockJPEG(prompt)
}

type UnavailableImageProvider struct {
	mode string
}

func (p UnavailableImageProvider) Mode() string {
	return p.mode
}

func (p UnavailableImageProvider) GenerateJPEG(ctx context.Context, prompt string) ([]byte, error) {
	return nil, fmt.Errorf("image provider mode %q is selected but no image mock provider is configured", p.mode)
}

func ImageProviderFromEnv(client *openai.Client) ImageProvider {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("DMAS_IMAGE_API_MODE")))
	switch mode {
	case "", imageProviderReal:
		return OpenAIImageProvider{client: client}
	case imageProviderMock:
		return MockImageProvider{}
	default:
		return UnavailableImageProvider{mode: mode}
	}
}

// ImageGenTool returns the OpenAI function-calling tool definition for
// generate_image. The LLM supplies a prompt and receives metadata for the
// generated local JPEG file.
func ImageGenTool() openai.ChatCompletionToolParam {
	return openai.ChatCompletionToolParam{
		Function: openai.FunctionDefinitionParam{
			Name:        "generate_image",
			Description: openai.String("Generate a logo image from a prompt."),
			Parameters: openai.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"prompt": map[string]interface{}{
						"type":        "string",
						"description": "Image prompt",
					},
				},
				"required": []string{"prompt"},
			},
		},
	}
}

// ImageGenHandler returns a tool handler that calls the DALL-E API,
// converts the resulting PNG to JPEG, stores it locally, and returns file
// metadata. The image bytes never flow through the LLM context.
func ImageGenHandler(client *openai.Client) core.ToolHandlerFn {
	return ImageGenHandlerWithProvider(ImageProviderFromEnv(client))
}

func ImageGenHandlerWithProvider(provider ImageProvider) core.ToolHandlerFn {
	return func(ctx context.Context, tc openai.ChatCompletionMessageToolCall) (string, error) {
		tracer := trace.SpanFromContext(ctx).TracerProvider().Tracer(imageTracerName)
		ctx, span := tracer.Start(ctx, "tool.image.generate",
			trace.WithAttributes(
				attribute.String("tool.name", "generate_image"),
				attribute.String("provider_mode", provider.Mode()),
			),
		)
		defer span.End()

		if tc.Function.Name != "generate_image" {
			err := fmt.Errorf("unsupported tool: %s", tc.Function.Name)
			recordToolError(span, err)
			return "", err
		}

		var args struct {
			Prompt string `json:"prompt"`
		}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			recordToolError(span, err)
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		if strings.TrimSpace(args.Prompt) == "" {
			err := fmt.Errorf("empty prompt")
			recordToolError(span, err)
			return "", err
		}

		jpegBytes, err := provider.GenerateJPEG(ctx, args.Prompt)
		if err != nil {
			recordToolError(span, err)
			return "", err
		}

		path, err := saveJPEG(jpegBytes, imageOutputName(provider.Mode(), args.Prompt))
		if err != nil {
			recordToolError(span, err)
			return "", err
		}
		span.SetAttributes(
			attribute.Int("tool.output.size_bytes", len(jpegBytes)),
			attribute.String("tool.output.mime_type", "image/jpeg"),
		)
		span.SetStatus(codes.Ok, "")

		return marshalJSON(map[string]interface{}{
			"status":     "success",
			"path":       path,
			"filename":   filepath.Base(path),
			"mime_type":  "image/jpeg",
			"size_bytes": len(jpegBytes),
		})
	}
}

// generateJPEG calls the DALL-E 3 images API with b64_json response
// format, decodes the PNG payload, and re-encodes it as JPEG (quality 85).
func generateJPEG(ctx context.Context, client *openai.Client, prompt string) ([]byte, error) {
	resp, err := client.Images.Generate(ctx, openai.ImageGenerateParams{
		Model:          openai.ImageModelDallE3,
		Prompt:         prompt,
		N:              openai.Int(1),
		Quality:        openai.ImageGenerateParamsQualityStandard,
		Size:           openai.ImageGenerateParamsSize1024x1024,
		ResponseFormat: openai.ImageGenerateParamsResponseFormatB64JSON,
	})
	if err != nil {
		return nil, fmt.Errorf("generate image: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("empty image response")
	}

	raw, err := base64.StdEncoding.DecodeString(resp.Data[0].B64JSON)
	if err != nil {
		return nil, fmt.Errorf("decode base64 payload: %w", err)
	}

	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decode png: %w", err)
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		return nil, fmt.Errorf("encode jpeg: %w", err)
	}

	return buf.Bytes(), nil
}

func generateMockJPEG(prompt string) ([]byte, error) {
	trimmed := strings.TrimSpace(prompt)
	if trimmed == "" {
		return nil, fmt.Errorf("empty prompt")
	}

	sum := sha256.Sum256([]byte(trimmed))
	const width = 512
	const height = 512
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	base := color.RGBA{R: sum[0], G: sum[1], B: sum[2], A: 0xff}
	accent := color.RGBA{R: sum[3], G: sum[4], B: sum[5], A: 0xff}
	detail := color.RGBA{R: sum[6], G: sum[7], B: sum[8], A: 0xff}

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			switch {
			case y < height/3:
				img.Set(x, y, color.RGBA{
					R: blendColor(base.R, accent.R, x, width),
					G: blendColor(base.G, accent.G, x, width),
					B: blendColor(base.B, accent.B, x, width),
					A: 0xff,
				})
			case y < 2*height/3:
				if ((x/32)+(y/32)+int(sum[9]))%2 == 0 {
					img.Set(x, y, accent)
				} else {
					img.Set(x, y, base)
				}
			default:
				img.Set(x, y, color.RGBA{
					R: blendColor(accent.R, detail.R, y-(2*height/3), height/3),
					G: blendColor(accent.G, detail.G, y-(2*height/3), height/3),
					B: blendColor(accent.B, detail.B, y-(2*height/3), height/3),
					A: 0xff,
				})
			}
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		return nil, fmt.Errorf("encode mock jpeg: %w", err)
	}
	return buf.Bytes(), nil
}

func saveJPEG(data []byte, nameHint string) (string, error) {
	dir := filepath.Join(os.TempDir(), "dmas_forge", "marketing-agency", "logos")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create logo output directory: %w", err)
	}
	if nameHint != "" {
		path := filepath.Join(dir, nameHint)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return "", fmt.Errorf("write logo image: %w", err)
		}
		return path, nil
	}
	file, err := os.CreateTemp(dir, "logo_*.jpg")
	if err != nil {
		return "", fmt.Errorf("create logo image: %w", err)
	}
	path := file.Name()
	if _, err := file.Write(data); err != nil {
		file.Close()
		return "", fmt.Errorf("write logo image: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close logo image: %w", err)
	}
	return path, nil
}

func imageOutputName(mode string, prompt string) string {
	if mode != imageProviderMock {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(prompt)))
	return fmt.Sprintf("logo_%x.jpg", sum[:6])
}

func blendColor(start uint8, end uint8, pos int, span int) uint8 {
	if span <= 1 {
		return start
	}
	return uint8((int(start)*(span-pos-1) + int(end)*pos) / (span - 1))
}

func marshalJSON(v interface{}) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func recordToolError(span trace.Span, err error) {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
