package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
)

var errCommandHelp = errors.New("command help displayed")

// parseCommand uses Kong for leaf command flags and positional arguments.
// Command execution and domain validation remain in their existing packages.
func parseCommand[T any](name string, args []string) (T, error) {
	var options T
	parserOptions := []kong.Option{kong.Name("semdiff " + name), kong.NoDefaultHelp(), kong.Writers(os.Stdout, os.Stderr)}
	if japaneseLocale() {
		parserOptions = append(parserOptions, kong.ValueFormatter(func(value *kong.Value) string {
			if translated, ok := commandHelpJA[value.Help]; ok {
				return translated
			}
			return kong.DefaultHelpValueFormatter(value)
		}))
	}
	parser, err := kong.New(&options, parserOptions...)
	if err != nil {
		return options, fmt.Errorf("build %s parser: %w", name, err)
	}
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			var help bytes.Buffer
			parser.Stdout = &help
			ctx, err := parser.Parse(nil)
			if err != nil {
				return options, err
			}
			if err := ctx.PrintUsage(false); err != nil {
				return options, err
			}
			output := help.String()
			if japaneseLocale() {
				output = strings.NewReplacer("Usage:", "使い方:", "Arguments:", "引数:", "Flags:", "フラグ:", "[flags]", "[フラグ]", "=STRING", "=<文字列>").Replace(output)
			}
			fmt.Fprint(os.Stdout, output)
			return options, errCommandHelp
		}
	}
	_, err = parser.Parse(args)
	return options, err
}

type rangeJSONArgs struct {
	Range string `arg:"" optional:"" name:"base..head" help:"Git range to inspect."`
	JSON  bool   `help:"Print JSON output."`
}

type versionArgs struct {
	JSON bool `help:"Print JSON output."`
}

type showArgs struct {
	Args  []string `arg:"" optional:"" name:"groups-file fragment-id"`
	JSON  bool     `help:"Print JSON output."`
	Draft string   `help:"Read a grouping draft instead of a finalized review."`
}

type validateArgs struct {
	GroupsFile string `arg:"" optional:"" name:"groups-file"`
	JSON       bool   `help:"Print JSON output."`
	Draft      string `default:".semdiff/grouping-draft.json" help:"Draft used to locate the default groups file."`
}

type viewArgs struct {
	GroupsFile     string  `arg:"" optional:"" name:"groups-file"`
	Addr           *string `help:"Viewer listen address."`
	HTML           string  `help:"Write a self-contained HTML file instead of serving the viewer."`
	IncludeAnswers bool    `name:"include-answers" help:"Include answered questions in an HTML export."`
	Draft          *string `help:"Use a draft to locate the groups file."`
	Exact          bool    `help:"Require a finalized review for the current range."`
}

type groupingInitArgs struct {
	Range string `arg:"" optional:"" name:"base..head"`
	Draft string `default:".semdiff/grouping-draft.json" help:"Grouping draft path."`
	From  string `help:"Finalized groups file used to seed this draft."`
	JSON  bool   `help:"Print JSON output."`
	Force bool   `help:"Replace an existing draft."`
}

type groupingApplyArgs struct {
	Operations string `arg:"" optional:"" name:"operations-file"`
	Draft      string `default:".semdiff/grouping-draft.json" help:"Grouping draft path."`
	JSON       bool   `help:"Print JSON output."`
}

type groupingStatusArgs struct {
	Draft string `default:".semdiff/grouping-draft.json" help:"Grouping draft path."`
	JSON  bool   `help:"Print JSON output."`
}

type groupingInspectArgs struct {
	Draft       string `default:".semdiff/grouping-draft.json" help:"Grouping draft path."`
	JSON        bool   `help:"Print JSON output."`
	Unassigned  bool   `help:"Show unassigned fragments."`
	Suggestions bool   `help:"Show Git-derived fragment suggestions."`
	Group       string `help:"Show one group."`
	Fragment    string `help:"Show one fragment."`
}

type groupingFinalizeArgs struct {
	GroupsFile string `arg:"" optional:"" name:"groups-file"`
	Draft      string `default:".semdiff/grouping-draft.json" help:"Grouping draft path."`
	JSON       bool   `help:"Print JSON output."`
}

type questionSessionStartArgs struct {
	GroupsFile string `arg:"" optional:"" name:"groups-file"`
	JSON       bool   `help:"Print JSON output."`
	Draft      string `default:".semdiff/grouping-draft.json" help:"Draft used to locate the default groups file."`
}

type questionWaitArgs struct {
	GroupsFile string `arg:"" optional:"" name:"groups-file"`
	JSON       bool   `help:"Print JSON output."`
	Session    string `help:"Answer session ID."`
	Draft      string `default:".semdiff/grouping-draft.json" help:"Draft used to locate the default groups file."`
}

type questionAnswerArgs struct {
	Args  []string `arg:"" optional:"" name:"groups-file question-id"`
	Stdin bool     `help:"Read answer from stdin."`
	JSON  bool     `help:"Print JSON output."`
	Draft string   `default:".semdiff/grouping-draft.json" help:"Draft used to locate the default groups file."`
}

type remoteViewIndexArgs struct {
	Addr       string `default:"127.0.0.1:7363" help:"Listen address."`
	Remote     string `help:"Git remote name."`
	Repository string `help:"Artifact repository URL or path."`
	Branch     string `help:"Artifact branch."`
}

type remoteViewArgs struct {
	Range      string `arg:"" optional:"" name:"base..head"`
	Addr       string `default:"127.0.0.1:7363" help:"Listen address."`
	Remote     string `help:"Git remote name."`
	Repository string `help:"Artifact repository URL or path."`
	Branch     string `help:"Artifact branch."`
}

type remotePullArgs struct {
	Range      string `arg:"" optional:"" name:"base..head"`
	Remote     string `help:"Git remote name."`
	Repository string `help:"Artifact repository URL or path."`
	Branch     string `help:"Artifact branch."`
	Force      bool   `help:"Overwrite an existing groups file without asking."`
	NoClobber  bool   `name:"no-clobber" help:"Fail if the groups file exists."`
}

type remotePushArgs struct {
	Range      string `arg:"" optional:"" name:"base..head"`
	Remote     string `help:"Git remote name."`
	Repository string `help:"Artifact repository URL or path."`
	Branch     string `help:"Artifact branch."`
	Draft      string `default:".semdiff/grouping-draft.json" help:"Draft used to locate the default groups file."`
	GroupsFile string `name:"groups-file" help:"Groups file to publish."`
}

type publishArgs struct {
	GroupsFile string `arg:"" optional:"" name:"groups-file"`
	Remote     string `help:"Git remote name."`
	Repository string `help:"Artifact repository URL or path."`
	Branch     string `help:"Artifact branch."`
	Draft      string `default:".semdiff/grouping-draft.json" help:"Draft used to locate the default groups file."`
}

type resolveArgs struct {
	Range string `arg:"" optional:"" name:"base..head"`
	JSON  bool   `help:"Print JSON output."`
	Exact bool   `help:"Require a review for the exact current range."`
}
