package viewer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"strings"

	"github.com/ry023/semdiff/internal/questions"
)

type viewerCapabilities struct {
	Questions string `json:"questions"`
	APIBase   string `json:"api_base,omitempty"`
}

type viewerBootstrap struct {
	Page         Page               `json:"page"`
	Capabilities viewerCapabilities `json:"capabilities"`
	Threads      []questions.Thread `json:"threads,omitempty"`
}

func renderReactHTML(page Page, threads []questions.Thread, interactive bool, basePath string) ([]byte, error) {
	css, err := assets.ReadFile("dist/viewer.css")
	if err != nil {
		return nil, err
	}
	js, err := assets.ReadFile("dist/viewer.js")
	if err != nil {
		return nil, err
	}
	capabilities := viewerCapabilities{Questions: "disabled"}
	if interactive {
		capabilities = viewerCapabilities{Questions: "interactive", APIBase: strings.TrimSuffix(basePath, "/") + "/api/questions"}
	} else if len(threads) > 0 {
		capabilities.Questions = "readonly"
	}
	data, err := json.Marshal(viewerBootstrap{
		Page:         page,
		Capabilities: capabilities,
		Threads:      threads,
	})
	if err != nil {
		return nil, err
	}

	// Go's JSON encoder escapes HTML-sensitive runes. Keep the script and style
	// end tags inert as an additional guard when embedded assets change.
	script := strings.ReplaceAll(string(js), "</script", `<\/script`)
	style := strings.ReplaceAll(string(css), "</style", `<\/style`)
	var output bytes.Buffer
	fmt.Fprintf(&output, "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><title>Semantic Changes</title><style>%s</style></head><body><div id=\"root\"></div><script id=\"semdiff-data\" type=\"application/json\">%s</script><script>%s</script></body></html>", style, data, script)
	return output.Bytes(), nil
}

func renderReactError(message string) []byte {
	return []byte("<!doctype html><title>Semantic Changes</title><p>" + html.EscapeString(message) + "</p>")
}
