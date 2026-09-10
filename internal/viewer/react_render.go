package viewer

import (
	"encoding/json"
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
	Threads      []viewerThread     `json:"threads,omitempty"`
}

type viewerThread struct {
	questions.Thread
	Turns []viewerTurn `json:"turns"`
}

type viewerTurn struct {
	questions.Turn
}

func buildViewerThread(thread questions.Thread) viewerThread {
	view := viewerThread{Thread: thread}
	for _, turn := range thread.Turns {
		view.Turns = append(view.Turns, viewerTurn{Turn: turn})
	}
	return view
}

func buildViewerThreads(threads []questions.Thread) []viewerThread {
	views := make([]viewerThread, 0, len(threads))
	for _, thread := range threads {
		views = append(views, buildViewerThread(thread))
	}
	return views
}

func renderReactHTML(page Page, threads []questions.Thread, interactive bool, basePath string) ([]byte, error) {
	shell, err := assets.ReadFile("frontend/shell.html")
	if err != nil {
		return nil, err
	}
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
		Threads:      buildViewerThreads(threads),
	})
	if err != nil {
		return nil, err
	}

	// Go's JSON encoder escapes HTML-sensitive runes. Keep the script and style
	// end tags inert as an additional guard when embedded assets change.
	script := strings.ReplaceAll(string(js), "</script", `<\/script`)
	style := strings.ReplaceAll(string(css), "</style", `<\/style`)
	document := strings.NewReplacer(
		"__SEMDIFF_STYLE__", style,
		"__SEMDIFF_DATA__", string(data),
		"__SEMDIFF_SCRIPT__", script,
	).Replace(string(shell))
	return []byte(document), nil
}
