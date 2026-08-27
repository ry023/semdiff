package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ry023/semdiff/internal/categories"
	"github.com/ry023/semdiff/internal/gitdiff"
	"github.com/ry023/semdiff/internal/groupingdraft"
	"github.com/ry023/semdiff/internal/groups"
	"github.com/ry023/semdiff/internal/model"
)

const defaultGroupingDraftPath = ".semdiff/grouping-draft.json"

func formatFragmentRanges(fragment model.Fragment) string {
	if fragment.FileMetadata && len(fragment.Ranges) == 0 {
		return "metadata"
	}
	parts := make([]string, 0, len(fragment.Ranges))
	for _, span := range fragment.Ranges {
		oldSide, newSide := "∅", "∅"
		if span.Old != nil {
			oldSide = fmt.Sprintf("%d,%d", span.Old.Start, span.Old.Lines)
		}
		if span.New != nil {
			newSide = fmt.Sprintf("%d,%d", span.New.Start, span.New.Lines)
		}
		parts = append(parts, fmt.Sprintf("-%s +%s", oldSide, newSide))
	}
	return strings.Join(parts, "; ")
}

func runGrouping(ctx context.Context, runner gitdiff.Runner, args []string) error {
	if len(args) == 0 {
		return errors.New("grouping requires init, apply, status, inspect, or finalize")
	}
	switch args[0] {
	case "init":
		return runGroupingInit(ctx, runner, args[1:])
	case "apply":
		return runGroupingApply(args[1:])
	case "status":
		return runGroupingStatus(args[1:])
	case "inspect":
		return runGroupingInspect(args[1:])
	case "finalize":
		return runGroupingFinalize(ctx, runner, args[1:])
	default:
		return fmt.Errorf("unknown grouping command %q", args[0])
	}
}

func runGroupingInit(ctx context.Context, runner gitdiff.Runner, args []string) error {
	fs := flag.NewFlagSet("grouping init", flag.ContinueOnError)
	draftPath := fs.String("draft", defaultGroupingDraftPath, "draft path")
	jsonOut := fs.Bool("json", false, "JSON output")
	force := fs.Bool("force", false, "replace an existing draft")
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(positional) != 1 {
		return errors.New("grouping init requires <base>..<head>")
	}
	if _, err := os.Stat(*draftPath); err == nil && !*force {
		return fmt.Errorf("grouping draft already exists at %s (use --force to replace it)", *draftPath)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("check grouping draft: %w", err)
	}
	inv, err := runner.Changes(ctx, positional[0])
	if err != nil {
		return err
	}
	paths := make([]string, 0, len(inv.Changes))
	for _, fragment := range inv.Changes {
		paths = append(paths, fragment.Path)
	}
	draft := groupingdraft.New(inv, categories.ClassifyPaths(paths))
	if err := groupingdraft.SaveAtomic(*draftPath, draft); err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(struct {
			DraftPath string               `json:"draft_path"`
			Status    groupingdraft.Status `json:"status"`
		}{*draftPath, draft.Status()})
	}
	fmt.Printf("initialized grouping draft: %s (%d suggestions)\n", *draftPath, len(draft.Suggestions))
	return nil
}

func runGroupingApply(args []string) error {
	fs := flag.NewFlagSet("grouping apply", flag.ContinueOnError)
	draftPath := fs.String("draft", defaultGroupingDraftPath, "draft path")
	jsonOut := fs.Bool("json", false, "JSON output")
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(positional) != 1 {
		return errors.New("grouping apply requires <operations-file|->")
	}
	draft, err := groupingdraft.Load(*draftPath)
	if err != nil {
		return err
	}
	request, err := readApplyRequest(positional[0])
	if err != nil {
		return err
	}
	updated, err := groupingdraft.Apply(draft, request)
	if err != nil {
		return err
	}
	if err := groupingdraft.SaveAtomic(*draftPath, updated); err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(updated.Status())
	}
	fmt.Printf("applied %d operation(s) to %s; revision %d\n", len(request.Operations), *draftPath, updated.Revision)
	return nil
}

func runGroupingStatus(args []string) error {
	fs := flag.NewFlagSet("grouping status", flag.ContinueOnError)
	draftPath := fs.String("draft", defaultGroupingDraftPath, "draft path")
	jsonOut := fs.Bool("json", false, "JSON output")
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(positional) != 0 {
		return errors.New("grouping status does not take positional arguments")
	}
	draft, err := groupingdraft.Load(*draftPath)
	if err != nil {
		return err
	}
	status := draft.Status()
	if *jsonOut {
		return printJSON(status)
	}
	fmt.Printf("revision %d: %d suggestions; %d/%d authored fragments assigned, %d described\n", status.Revision, status.SuggestionCount, status.AssignedFragmentCount, status.FragmentCount, status.DescribedFragmentCount)
	if status.ReadyToFinalize {
		fmt.Println("ready to finalize")
	} else {
		fmt.Printf("not ready to finalize: %d unassigned, %d undescribed\n", len(status.UnassignedFragmentIDs), len(status.UndescribedFragmentIDs))
	}
	return nil
}

func runGroupingInspect(args []string) error {
	fs := flag.NewFlagSet("grouping inspect", flag.ContinueOnError)
	draftPath := fs.String("draft", defaultGroupingDraftPath, "draft path")
	jsonOut := fs.Bool("json", false, "JSON output")
	unassigned := fs.Bool("unassigned", false, "show unassigned fragments")
	suggestions := fs.Bool("suggestions", false, "show Git-derived fragment suggestions")
	groupID := fs.String("group", "", "show one group")
	fragmentID := fs.String("fragment", "", "show one fragment")
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(positional) != 0 {
		return errors.New("grouping inspect does not take positional arguments")
	}
	selected := 0
	if *unassigned {
		selected++
	}
	if *suggestions {
		selected++
	}
	if *groupID != "" {
		selected++
	}
	if *fragmentID != "" {
		selected++
	}
	if selected != 1 {
		return errors.New("grouping inspect requires exactly one of --suggestions, --unassigned, --group, or --fragment")
	}
	draft, err := groupingdraft.Load(*draftPath)
	if err != nil {
		return err
	}
	if *unassigned {
		fragments := draft.UnassignedFragments()
		if *jsonOut {
			return printJSON(fragments)
		}
		for _, fragment := range fragments {
			fmt.Printf("%s  %s  %s\n", fragment.ID, fragment.Path, formatFragmentRanges(fragment))
		}
		return nil
	}
	if *suggestions {
		if *jsonOut {
			return printJSON(draft.Suggestions)
		}
		for _, suggestion := range draft.Suggestions {
			fmt.Printf("%s  %s  %s\n", suggestion.ID, suggestion.Path, formatFragmentRanges(suggestion))
		}
		return nil
	}
	if *fragmentID != "" {
		inspection, err := draft.FragmentInspection(*fragmentID)
		if err != nil {
			return err
		}
		if *jsonOut {
			return printJSON(inspection)
		}
		fmt.Printf("%s  %s  assignment=%s  category=%s\n", inspection.Fragment.ID, inspection.Fragment.Path, strings.Join(inspection.Assignments, ","), inspection.CategorySuggestion)
		if inspection.Fragment.Description != "" {
			fmt.Printf("description: %s\n", inspection.Fragment.Description)
		}
		return nil
	}
	group, err := draft.Group(*groupID)
	if err != nil {
		return err
	}
	status := draft.Status()
	var groupStatus groupingdraft.GroupStatus
	for _, candidate := range status.Groups {
		if candidate.ID == *groupID {
			groupStatus = candidate
			break
		}
	}
	result := struct {
		Group  groupingdraft.DraftGroup  `json:"group"`
		Status groupingdraft.GroupStatus `json:"status"`
	}{Group: group, Status: groupStatus}
	if *jsonOut {
		return printJSON(result)
	}
	fmt.Printf("%s: %s (%d fragments)\n", group.ID, group.Title, len(group.Members))
	for _, id := range group.Members {
		inspection, _ := draft.FragmentInspection(id)
		fmt.Printf("  %s: %s\n", id, inspection.Fragment.Description)
	}
	return nil
}

func runGroupingFinalize(ctx context.Context, runner gitdiff.Runner, args []string) error {
	fs := flag.NewFlagSet("grouping finalize", flag.ContinueOnError)
	draftPath := fs.String("draft", defaultGroupingDraftPath, "draft path")
	jsonOut := fs.Bool("json", false, "JSON output")
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(positional) != 1 {
		return errors.New("grouping finalize requires <groups-file>")
	}
	draft, err := groupingdraft.Load(*draftPath)
	if err != nil {
		return err
	}
	if errs := draft.FinalErrors(); len(errs) > 0 {
		if *jsonOut {
			_ = printJSON(struct {
				Valid  bool     `json:"valid"`
				Errors []string `json:"errors"`
			}{false, errs})
		}
		return fmt.Errorf("grouping draft is not ready to finalize: %s", strings.Join(errs, "; "))
	}
	inv, err := runner.Changes(ctx, draft.BaseSHA+".."+draft.HeadSHA)
	if err != nil {
		return fmt.Errorf("refresh change map: %w", err)
	}
	groupsFile := draft.ToGroupsFile()
	report := groups.ValidateReport(groupsFile, inv)
	if len(report.Errors) > 0 || len(report.Warnings) > 0 {
		issues := append(append([]string{}, report.Errors...), report.Warnings...)
		if *jsonOut {
			_ = printJSON(struct {
				Valid    bool     `json:"valid"`
				Errors   []string `json:"errors"`
				Warnings []string `json:"warnings"`
			}{false, report.Errors, report.Warnings})
		}
		return fmt.Errorf("current change map does not match grouping draft: %s", strings.Join(issues, "; "))
	}
	if err := saveJSONAtomic(positional[0], groupsFile); err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(struct {
			Valid    bool   `json:"valid"`
			Output   string `json:"output"`
			Revision int    `json:"revision"`
		}{true, positional[0], draft.Revision})
	}
	fmt.Printf("finalized groups file: %s\n", positional[0])
	return nil
}

func readApplyRequest(path string) (groupingdraft.ApplyRequest, error) {
	var reader io.Reader
	var file *os.File
	if path == "-" {
		reader = os.Stdin
	} else {
		opened, err := os.Open(path)
		if err != nil {
			return groupingdraft.ApplyRequest{}, err
		}
		file = opened
		defer file.Close()
		reader = file
	}
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var request groupingdraft.ApplyRequest
	if err := decoder.Decode(&request); err != nil {
		return request, fmt.Errorf("decode grouping operations: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return request, errors.New("grouping operations contain multiple JSON values")
		}
		return request, fmt.Errorf("decode grouping operations: %w", err)
	}
	return request, nil
}

func saveJSONAtomic(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".semdiff-output-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0644); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
