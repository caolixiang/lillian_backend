package task

const (
	StatusQueued  = "queued"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusError   = "error"
)

type RequestPayload struct {
	Prompt             string         `json:"prompt"`
	Params             map[string]any `json:"params"`
	InputImageDataURLs []string       `json:"inputImageDataUrls"`
	MaskDataURL        string         `json:"maskDataUrl,omitempty"`
}

type Output struct {
	Key         string `json:"key"`
	ContentType string `json:"contentType"`
	URL         string `json:"url,omitempty"`
}
