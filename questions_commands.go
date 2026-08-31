package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ry023/semdiff/internal/groups"
	"github.com/ry023/semdiff/internal/questions"
)

func runQuestions(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("questions requires wait or answer")
	}
	switch args[0] {
	case "wait":
		fs := flag.NewFlagSet("questions wait", flag.ContinueOnError)
		jsonOut := fs.Bool("json", false, "JSON output")
		draftPath := fs.String("draft", defaultGroupingDraftPath, "draft path used to locate the default groups file")
		positional, err := parseInterspersed(fs, args[1:])
		if err != nil {
			return err
		}
		if len(positional) > 1 {
			return errors.New("questions wait accepts at most one <groups-file>")
		}
		groupsPath := ""
		if len(positional) == 1 {
			groupsPath = positional[0]
		} else {
			groupsPath, err = defaultGroupsPath(*draftPath)
			if err != nil {
				return fmt.Errorf("locate default groups file from draft: %w", err)
			}
		}
		store, err := questionStore(groupsPath)
		if err != nil {
			return err
		}
		question, err := store.Wait(ctx)
		if err != nil {
			return err
		}
		if *jsonOut {
			return printJSON(question)
		}
		fmt.Printf("%s  %s\n", question.ID, question.Question)
		return nil
	case "answer":
		fs := flag.NewFlagSet("questions answer", flag.ContinueOnError)
		stdin := fs.Bool("stdin", false, "read answer from stdin")
		jsonOut := fs.Bool("json", false, "JSON output")
		draftPath := fs.String("draft", defaultGroupingDraftPath, "draft path used to locate the default groups file")
		positional, err := parseInterspersed(fs, args[1:])
		if err != nil {
			return err
		}
		if (len(positional) != 1 && len(positional) != 2) || !*stdin {
			return errors.New("questions answer requires [<groups-file>] <question-id> --stdin")
		}
		groupsPath, questionID := "", ""
		if len(positional) == 2 {
			groupsPath, questionID = positional[0], positional[1]
		} else {
			groupsPath, err = defaultGroupsPath(*draftPath)
			if err != nil {
				return fmt.Errorf("locate default groups file from draft: %w", err)
			}
			questionID = positional[0]
		}
		body, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(body)) == "" {
			return errors.New("answer from stdin is empty")
		}
		store, err := questionStore(groupsPath)
		if err != nil {
			return err
		}
		thread, err := store.Answer(questionID, string(body))
		if err != nil {
			return err
		}
		if *jsonOut {
			return printJSON(thread)
		}
		fmt.Printf("answered %s\n", questionID)
		return nil
	default:
		return fmt.Errorf("unknown questions command %q", args[0])
	}
}

func questionStore(groupsPath string) (questions.Store, error) {
	file, err := groups.Load(groupsPath)
	if err != nil {
		return questions.Store{}, err
	}
	return questions.Store{
		Path: questions.DefaultPath(groupsPath, file.BaseSHA, file.HeadSHA), BaseSHA: file.BaseSHA, HeadSHA: file.HeadSHA,
	}, nil
}
