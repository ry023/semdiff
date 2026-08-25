package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ry023/semdiff/internal/gitdiff"
	"github.com/ry023/semdiff/internal/groups"
	"github.com/ry023/semdiff/internal/model"
	"github.com/ry023/semdiff/internal/viewer"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
	  semdiff commits <base>..<head> [--json]
	  semdiff fragments <base>..<head> [--json]
	  semdiff show <fragment-id> [--json]
	  semdiff validate <groups-file> [--json]
	  semdiff view <groups-file> [--addr 127.0.0.1:8080]`)
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		usage()
		return errors.New("command is required")
	}
	r := gitdiff.Runner{Dir: "."}
	switch args[0] {
	case "commits":
		fs := flag.NewFlagSet("commits", flag.ContinueOnError)
		jsonOut := fs.Bool("json", false, "JSON output")
		positional, err := parseInterspersed(fs, args[1:])
		if err != nil {
			return err
		}
		if len(positional) != 1 {
			return errors.New("commits requires <base>..<head>")
		}
		cs, err := r.Commits(ctx, positional[0])
		if err != nil {
			return err
		}
		if *jsonOut {
			return printJSON(cs)
		}
		for _, c := range cs {
			fmt.Printf("%.12s  %s  %s  (%d files)\n", c.SHA, c.Subject, c.Author, c.FilesChanged)
		}
		return nil
	case "fragments":
		fs := flag.NewFlagSet("fragments", flag.ContinueOnError)
		jsonOut := fs.Bool("json", false, "JSON output")
		positional, err := parseInterspersed(fs, args[1:])
		if err != nil {
			return err
		}
		if len(positional) != 1 {
			return errors.New("fragments requires <base>..<head>")
		}
		inv, err := r.Fragments(ctx, positional[0])
		if err != nil {
			return err
		}
		if err = saveCache(inv); err != nil {
			return fmt.Errorf("save inventory cache: %w", err)
		}
		if *jsonOut {
			light := inv.Fragments
			for i := range light {
				light[i].Patch = ""
			}
			return printJSON(light)
		}
		for _, f := range inv.Fragments {
			fmt.Printf("%s  %s  -%d,%d +%d,%d\n", f.ID, f.Path, f.OldStart, f.OldLines, f.NewStart, f.NewLines)
		}
		return nil
	case "show":
		fs := flag.NewFlagSet("show", flag.ContinueOnError)
		jsonOut := fs.Bool("json", false, "JSON output")
		positional, err := parseInterspersed(fs, args[1:])
		if err != nil {
			return err
		}
		if len(positional) != 1 {
			return errors.New("show requires <fragment-id>")
		}
		inv, err := loadCache()
		if err != nil {
			return fmt.Errorf("load inventory (run fragments first): %w", err)
		}
		for _, f := range inv.Fragments {
			if f.ID == positional[0] {
				if *jsonOut {
					return printJSON(f)
				}
				fmt.Print(f.Patch)
				return nil
			}
		}
		return fmt.Errorf("fragment %s not found in latest inventory", positional[0])
	case "validate":
		fs := flag.NewFlagSet("validate", flag.ContinueOnError)
		jsonOut := fs.Bool("json", false, "JSON output")
		positional, err := parseInterspersed(fs, args[1:])
		if err != nil {
			return err
		}
		if len(positional) != 1 {
			return errors.New("validate requires <groups-file>")
		}
		g, inv, problems, err := loadAndValidate(ctx, r, positional[0])
		_ = g
		_ = inv
		if err != nil {
			return err
		}
		if *jsonOut {
			result := struct {
				Valid         bool     `json:"valid"`
				FragmentCount int      `json:"fragment_count"`
				GroupCount    int      `json:"group_count"`
				Errors        []string `json:"errors"`
			}{len(problems) == 0, len(inv.Fragments), len(g.Groups), problems}
			_ = printJSON(result)
		} else if len(problems) == 0 {
			fmt.Printf("valid: %d fragments assigned exactly once across %d groups\n", len(inv.Fragments), len(g.Groups))
		} else {
			for _, p := range problems {
				fmt.Fprintln(os.Stderr, "-", p)
			}
		}
		if len(problems) > 0 {
			return fmt.Errorf("validation failed with %d error(s)", len(problems))
		}
		return nil
	case "view":
		fs := flag.NewFlagSet("view", flag.ContinueOnError)
		addr := fs.String("addr", "127.0.0.1:8080", "listen address")
		positional, err := parseInterspersed(fs, args[1:])
		if err != nil {
			return err
		}
		if len(positional) != 1 {
			return errors.New("view requires <groups-file>")
		}
		g, inv, problems, err := loadAndValidate(ctx, r, positional[0])
		if err != nil {
			return err
		}
		if len(problems) > 0 {
			return fmt.Errorf("groups file is invalid: %s", strings.Join(problems, "; "))
		}
		paths := make([]string, 0, len(inv.Fragments))
		for _, fragment := range inv.Fragments {
			paths = append(paths, fragment.Path)
		}
		fileContents := r.FileContents(ctx, inv.BaseSHA, inv.HeadSHA, paths)
		h, err := viewer.Handler(viewer.Build(g, inv, fileContents))
		if err != nil {
			return err
		}
		srv := &http.Server{Addr: *addr, Handler: h, ReadHeaderTimeout: 5 * time.Second}
		stopCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
		defer stop()
		go func() {
			<-stopCtx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutdownCtx)
		}()
		log.Printf("Semantic Diff Viewer: http://%s", *addr)
		err = srv.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			name := strings.TrimLeft(strings.SplitN(a, "=", 2)[0], "-")
			f := fs.Lookup(name)
			if f != nil && !strings.Contains(a, "=") && i+1 < len(args) {
				boolFlag := false
				if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok {
					boolFlag = bf.IsBoolFlag()
				}
				if !boolFlag {
					i++
					flags = append(flags, args[i])
				}
			}
		} else {
			pos = append(pos, a)
		}
	}
	return pos, fs.Parse(flags)
}
func printJSON(v any) error {
	e := json.NewEncoder(os.Stdout)
	e.SetIndent("", "  ")
	return e.Encode(v)
}
func cachePath() (string, error) {
	root, err := filepath.Abs(".semdiff")
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "inventory.json"), nil
}
func saveCache(inv model.Inventory) error {
	p, err := cachePath()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	b, err := json.Marshal(inv)
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0644)
}
func loadCache() (model.Inventory, error) {
	p, err := cachePath()
	if err != nil {
		return model.Inventory{}, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return model.Inventory{}, err
	}
	var inv model.Inventory
	err = json.Unmarshal(b, &inv)
	return inv, err
}
func loadAndValidate(ctx context.Context, r gitdiff.Runner, path string) (model.GroupsFile, model.Inventory, []string, error) {
	g, err := groups.Load(path)
	if err != nil {
		return g, model.Inventory{}, nil, err
	}
	inv, err := r.Fragments(ctx, g.BaseSHA+".."+g.HeadSHA)
	if err != nil {
		return g, inv, nil, err
	}
	return g, inv, groups.Validate(g, inv), nil
}
